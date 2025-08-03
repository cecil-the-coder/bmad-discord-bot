package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"bmad-knowledge-bot/internal/storage"
)

// ChannelRestrictor handles channel-based restrictions for bot operations
type ChannelRestrictor struct {
	storage storage.StorageService
	logger  *slog.Logger
}

// ChannelRestrictions represents the channel restriction configuration
type ChannelRestrictions struct {
	AllowedChannelIDs  []string
	RestrictDMs        bool
	AdminBypassEnabled bool
	Enabled            bool
}

// NewChannelRestrictor creates a new channel restrictor
func NewChannelRestrictor(storage storage.StorageService, logger *slog.Logger) *ChannelRestrictor {
	return &ChannelRestrictor{
		storage: storage,
		logger:  logger,
	}
}

// IsChannelAllowed checks if a channel is allowed for bot operations
func (cr *ChannelRestrictor) IsChannelAllowed(ctx context.Context, channelID string, isDM bool) (bool, error) {
	// Get channel restriction configuration
	restrictions, err := cr.getChannelRestrictions(ctx)
	if err != nil {
		cr.logger.Error("Failed to get channel restrictions", "error", err)
		// Default to allowing if config fails to load
		return true, nil
	}

	// If channel restrictions are disabled, allow all channels
	if !restrictions.Enabled {
		return true, nil
	}

	// Handle DM channels
	if isDM {
		// DMs are allowed by default unless explicitly restricted
		return !restrictions.RestrictDMs, nil
	}

	// If no allowed channels configured, allow all (empty list means no restrictions)
	if len(restrictions.AllowedChannelIDs) == 0 {
		return true, nil
	}

	// Check if channel is in allowed list
	for _, allowedID := range restrictions.AllowedChannelIDs {
		if channelID == allowedID {
			return true, nil
		}
	}

	// Channel not in allowed list
	cr.logger.Debug("Channel not in allowed list",
		"channel_id", channelID,
		"allowed_channels", len(restrictions.AllowedChannelIDs))
	return false, nil
}

// IsChannelAllowedForAdmin checks if a channel is allowed for admin users (with bypass)
func (cr *ChannelRestrictor) IsChannelAllowedForAdmin(ctx context.Context, channelID string, isDM bool, isAdmin bool) (bool, error) {
	// Get channel restriction configuration
	restrictions, err := cr.getChannelRestrictions(ctx)
	if err != nil {
		cr.logger.Error("Failed to get channel restrictions", "error", err)
		// Default to allowing if config fails to load
		return true, nil
	}

	// If admin bypass is enabled and user is admin, allow all channels
	if restrictions.AdminBypassEnabled && isAdmin {
		cr.logger.Debug("Admin bypass enabled for channel restriction",
			"channel_id", channelID,
			"is_dm", isDM)
		return true, nil
	}

	// Otherwise, use normal channel restriction logic
	return cr.IsChannelAllowed(ctx, channelID, isDM)
}

// GetChannelRestrictions returns the current channel restriction configuration
func (cr *ChannelRestrictor) GetChannelRestrictions(ctx context.Context) (*ChannelRestrictions, error) {
	return cr.getChannelRestrictions(ctx)
}

// getChannelRestrictions retrieves channel restrictions from database configuration
func (cr *ChannelRestrictor) getChannelRestrictions(ctx context.Context) (*ChannelRestrictions, error) {
	restrictions := &ChannelRestrictions{
		AllowedChannelIDs:  []string{},
		RestrictDMs:        false,
		AdminBypassEnabled: false,
		Enabled:            false,
	}

	// Get enabled status
	enabledConfig, err := cr.storage.GetConfiguration(ctx, "CHANNEL_RESTRICTIONS_ENABLED")
	if err == nil && enabledConfig.Value == "true" {
		restrictions.Enabled = true
	}

	// If not enabled, return early with defaults
	if !restrictions.Enabled {
		return restrictions, nil
	}

	// Get allowed channel IDs
	channelsConfig, err := cr.storage.GetConfiguration(ctx, "ALLOWED_CHANNEL_IDS")
	if err == nil && channelsConfig.Value != "" {
		for _, channelID := range strings.Split(channelsConfig.Value, ",") {
			trimmed := strings.TrimSpace(channelID)
			if trimmed != "" {
				restrictions.AllowedChannelIDs = append(restrictions.AllowedChannelIDs, trimmed)
			}
		}
	}

	// Get DM restriction setting
	dmConfig, err := cr.storage.GetConfiguration(ctx, "RESTRICT_DMS")
	if err == nil && dmConfig.Value == "true" {
		restrictions.RestrictDMs = true
	}

	// Get admin bypass setting
	bypassConfig, err := cr.storage.GetConfiguration(ctx, "ADMIN_CHANNEL_BYPASS_ENABLED")
	if err == nil && bypassConfig.Value == "true" {
		restrictions.AdminBypassEnabled = true
	}

	return restrictions, nil
}

// UpdateChannelRestrictions updates the channel restriction configuration
func (cr *ChannelRestrictor) UpdateChannelRestrictions(ctx context.Context, restrictions *ChannelRestrictions) error {
	now := time.Now().Unix()

	// Update enabled status
	enabledConfig := &storage.Configuration{
		Key:         "CHANNEL_RESTRICTIONS_ENABLED",
		Value:       fmt.Sprintf("%t", restrictions.Enabled),
		Type:        "bool",
		Category:    "channel_restrictions",
		Description: "Enable or disable channel restrictions for bot operations",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := cr.storage.UpsertConfiguration(ctx, enabledConfig); err != nil {
		return fmt.Errorf("failed to update channel restrictions enabled: %w", err)
	}

	// Update allowed channel IDs
	channelIDsStr := strings.Join(restrictions.AllowedChannelIDs, ",")
	channelsConfig := &storage.Configuration{
		Key:         "ALLOWED_CHANNEL_IDS",
		Value:       channelIDsStr,
		Type:        "string",
		Category:    "channel_restrictions",
		Description: "Comma-separated list of allowed channel IDs",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := cr.storage.UpsertConfiguration(ctx, channelsConfig); err != nil {
		return fmt.Errorf("failed to update allowed channel IDs: %w", err)
	}

	// Update DM restriction
	dmConfig := &storage.Configuration{
		Key:         "RESTRICT_DMS",
		Value:       fmt.Sprintf("%t", restrictions.RestrictDMs),
		Type:        "bool",
		Category:    "channel_restrictions",
		Description: "Restrict bot operations in DM channels",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := cr.storage.UpsertConfiguration(ctx, dmConfig); err != nil {
		return fmt.Errorf("failed to update DM restrictions: %w", err)
	}

	// Update admin bypass
	bypassConfig := &storage.Configuration{
		Key:         "ADMIN_CHANNEL_BYPASS_ENABLED",
		Value:       fmt.Sprintf("%t", restrictions.AdminBypassEnabled),
		Type:        "bool",
		Category:    "channel_restrictions",
		Description: "Allow admin users to bypass channel restrictions",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := cr.storage.UpsertConfiguration(ctx, bypassConfig); err != nil {
		return fmt.Errorf("failed to update admin bypass: %w", err)
	}

	cr.logger.Info("Updated channel restrictions",
		"enabled", restrictions.Enabled,
		"allowed_channels", len(restrictions.AllowedChannelIDs),
		"restrict_dms", restrictions.RestrictDMs,
		"admin_bypass", restrictions.AdminBypassEnabled)

	return nil
}

// FormatChannelRestrictionsStatus creates a user-friendly status message
func (cr *ChannelRestrictor) FormatChannelRestrictionsStatus(restrictions *ChannelRestrictions) string {
	if !restrictions.Enabled {
		return "🟢 **Channel Restrictions:** Disabled - Bot responds in all channels"
	}

	msg := "🔒 **Channel Restrictions:** Enabled\n"

	if len(restrictions.AllowedChannelIDs) == 0 {
		msg += "• **Allowed Channels:** All channels (no restrictions)\n"
	} else {
		msg += fmt.Sprintf("• **Allowed Channels:** %d channel(s) configured\n", len(restrictions.AllowedChannelIDs))
		for i, channelID := range restrictions.AllowedChannelIDs {
			if i < 5 { // Show first 5 channels
				msg += fmt.Sprintf("  - <#%s>\n", channelID)
			} else if i == 5 {
				msg += fmt.Sprintf("  - ... and %d more\n", len(restrictions.AllowedChannelIDs)-5)
				break
			}
		}
	}

	if restrictions.RestrictDMs {
		msg += "• **DM Messages:** Restricted\n"
	} else {
		msg += "• **DM Messages:** Allowed\n"
	}

	if restrictions.AdminBypassEnabled {
		msg += "• **Admin Bypass:** Enabled - Admins can use bot in any channel\n"
	} else {
		msg += "• **Admin Bypass:** Disabled - Restrictions apply to all users\n"
	}

	return msg
}
