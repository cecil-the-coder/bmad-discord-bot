package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"bmad-knowledge-bot/internal/monitor"
	"bmad-knowledge-bot/internal/storage"
)

// AdminCommands handles administrative commands for rate limiting and channel restrictions
type AdminCommands struct {
	storage           storage.StorageService
	userRateLimiter   *monitor.UserRateLimiter
	channelRestrictor *ChannelRestrictor
	logger            *slog.Logger
}

// NewAdminCommands creates a new admin commands handler
func NewAdminCommands(
	storage storage.StorageService,
	userRateLimiter *monitor.UserRateLimiter,
	channelRestrictor *ChannelRestrictor,
	logger *slog.Logger,
) *AdminCommands {
	return &AdminCommands{
		storage:           storage,
		userRateLimiter:   userRateLimiter,
		channelRestrictor: channelRestrictor,
		logger:            logger,
	}
}

// HandleAdminCommand processes admin commands for rate limiting and channel management
func (ac *AdminCommands) HandleAdminCommand(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate, command string, args []string) (string, error) {
	// Verify user is admin
	isAdmin, err := ac.isUserAdmin(ctx, s, m.Author.ID, m.GuildID)
	if err != nil {
		ac.logger.Error("Failed to check admin status", "error", err, "user_id", m.Author.ID)
		return "❌ Failed to verify admin permissions.", nil
	}
	if !isAdmin {
		return "🔒 This command requires admin permissions.", nil
	}

	switch command {
	case "ratelimit-status":
		return ac.handleRateLimitStatus(ctx, args)
	case "ratelimit-reset":
		return ac.handleRateLimitReset(ctx, args)
	case "ratelimit-config":
		return ac.handleRateLimitConfig(ctx, args)
	case "channel-restrictions":
		return ac.handleChannelRestrictions(ctx, args)
	case "admin-help":
		return ac.handleAdminHelp(), nil
	default:
		return "❓ Unknown admin command. Use `!admin-help` for available commands.", nil
	}
}

// handleRateLimitStatus shows rate limit status for a user or all users
func (ac *AdminCommands) handleRateLimitStatus(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "❓ Usage: `!ratelimit-status <user_id>` or `!ratelimit-status all`", nil
	}

	if args[0] == "all" {
		return ac.getAllUsersRateLimitStatus(ctx)
	}

	userID := args[0]
	status, err := ac.userRateLimiter.GetUserRateLimitStatus(ctx, userID)
	if err != nil {
		ac.logger.Error("Failed to get user rate limit status", "error", err, "user_id", userID)
		return "❌ Failed to retrieve rate limit status.", nil
	}

	return fmt.Sprintf("**Rate Limit Status for <@%s>:**\n%s", userID, ac.userRateLimiter.FormatRateLimitStatusMessage(status)), nil
}

// handleRateLimitReset resets rate limits for a user
func (ac *AdminCommands) handleRateLimitReset(ctx context.Context, args []string) (string, error) {
	if len(args) < 1 {
		return "❓ Usage: `!ratelimit-reset <user_id> [time_window]`\nTime windows: minute, hour, day (default: all)", nil
	}

	userID := args[0]
	timeWindow := "all"
	if len(args) > 1 {
		timeWindow = args[1]
	}

	if timeWindow == "all" {
		// Reset all time windows
		windows := []string{"minute", "hour", "day"}
		for _, window := range windows {
			err := ac.userRateLimiter.ResetUserRateLimit(ctx, userID, window)
			if err != nil {
				ac.logger.Error("Failed to reset rate limit", "error", err, "user_id", userID, "window", window)
				return fmt.Sprintf("❌ Failed to reset %s rate limit for <@%s>.", window, userID), nil
			}
		}
		return fmt.Sprintf("✅ Reset all rate limits for <@%s>.", userID), nil
	} else {
		// Reset specific time window
		if !isValidTimeWindow(timeWindow) {
			return "❓ Invalid time window. Valid options: minute, hour, day", nil
		}

		err := ac.userRateLimiter.ResetUserRateLimit(ctx, userID, timeWindow)
		if err != nil {
			ac.logger.Error("Failed to reset rate limit", "error", err, "user_id", userID, "window", timeWindow)
			return fmt.Sprintf("❌ Failed to reset %s rate limit for <@%s>.", timeWindow, userID), nil
		}
		return fmt.Sprintf("✅ Reset %s rate limit for <@%s>.", timeWindow, userID), nil
	}
}

// handleRateLimitConfig shows or updates rate limit configuration
func (ac *AdminCommands) handleRateLimitConfig(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return ac.showRateLimitConfig(ctx)
	}

	if len(args) < 2 {
		return "❓ Usage: `!ratelimit-config` (show) or `!ratelimit-config <setting> <value>`\nSettings: minute_limit, hour_limit, day_limit, enabled", nil
	}

	setting := args[0]
	value := args[1]

	return ac.updateRateLimitConfig(ctx, setting, value)
}

// handleChannelRestrictions shows or updates channel restrictions
func (ac *AdminCommands) handleChannelRestrictions(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		restrictions, err := ac.channelRestrictor.GetChannelRestrictions(ctx)
		if err != nil {
			ac.logger.Error("Failed to get channel restrictions", "error", err)
			return "❌ Failed to retrieve channel restrictions.", nil
		}
		return ac.channelRestrictor.FormatChannelRestrictionsStatus(restrictions), nil
	}

	if len(args) < 2 {
		return "❓ Usage: `!channel-restrictions` (show) or `!channel-restrictions <setting> <value>`\nSettings: enabled, add_channel, remove_channel, restrict_dms, admin_bypass", nil
	}

	setting := args[0]
	value := args[1]

	return ac.updateChannelRestrictions(ctx, setting, value)
}

// handleAdminHelp shows available admin commands
func (ac *AdminCommands) handleAdminHelp() string {
	return `🛡️ **Admin Commands Help:**

**Rate Limiting:**
• ` + "`!ratelimit-status <user_id>`" + ` - Show rate limit status for a user
• ` + "`!ratelimit-status all`" + ` - Show rate limit summary for all users
• ` + "`!ratelimit-reset <user_id> [window]`" + ` - Reset rate limits for a user
• ` + "`!ratelimit-config`" + ` - Show current rate limit configuration
• ` + "`!ratelimit-config <setting> <value>`" + ` - Update rate limit settings

**Channel Restrictions:**
• ` + "`!channel-restrictions`" + ` - Show current channel restrictions
• ` + "`!channel-restrictions enabled true/false`" + ` - Enable/disable restrictions
• ` + "`!channel-restrictions add_channel <channel_id>`" + ` - Add allowed channel
• ` + "`!channel-restrictions remove_channel <channel_id>`" + ` - Remove allowed channel
• ` + "`!channel-restrictions restrict_dms true/false`" + ` - Restrict DM messages
• ` + "`!channel-restrictions admin_bypass true/false`" + ` - Enable admin bypass

**General:**
• ` + "`!admin-help`" + ` - Show this help message

**Note:** All commands require admin permissions configured in ` + "`ADMIN_ROLE_NAMES`" + `.`
}

// Helper functions

// isUserAdmin checks if a user has admin privileges
func (ac *AdminCommands) isUserAdmin(ctx context.Context, s *discordgo.Session, userID string, guildID string) (bool, error) {
	if guildID == "" {
		// DMs - check if user is in admin user list (if configured)
		return false, nil
	}

	// Get user roles
	member, err := s.GuildMember(guildID, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get guild member: %w", err)
	}

	// Get guild roles for name mapping
	roles, err := s.GuildRoles(guildID)
	if err != nil {
		return false, fmt.Errorf("failed to get guild roles: %w", err)
	}

	// Create role ID to name mapping
	roleIDToName := make(map[string]string)
	for _, role := range roles {
		roleIDToName[role.ID] = role.Name
	}

	// Check admin status using rate limiter (which has the admin role logic)
	return ac.userRateLimiter.CheckUserAdminByRoles(ctx, member.Roles, roleIDToName)
}

// getAllUsersRateLimitStatus gets rate limit summary for all users
func (ac *AdminCommands) getAllUsersRateLimitStatus(ctx context.Context) (string, error) {
	// This is a simplified implementation - in practice, we'd need to query
	// the database for all users with rate limit records
	return "📊 **All Users Rate Limit Summary:**\n\nThis feature requires database query implementation.\nUse `!ratelimit-status <user_id>` for specific users.", nil
}

// showRateLimitConfig shows current rate limit configuration
func (ac *AdminCommands) showRateLimitConfig(ctx context.Context) (string, error) {
	configs := []string{
		"USER_RATE_LIMIT_PER_MINUTE",
		"USER_RATE_LIMIT_PER_HOUR",
		"USER_RATE_LIMIT_PER_DAY",
		"RATE_LIMITING_ENABLED",
		"ADMIN_ROLE_NAMES",
	}

	msg := "⚙️ **Rate Limit Configuration:**\n"
	for _, key := range configs {
		config, err := ac.storage.GetConfiguration(ctx, key)
		if err != nil {
			msg += fmt.Sprintf("• **%s:** Not configured\n", key)
		} else {
			msg += fmt.Sprintf("• **%s:** %s\n", key, config.Value)
		}
	}

	return msg, nil
}

// updateRateLimitConfig updates rate limit configuration
func (ac *AdminCommands) updateRateLimitConfig(ctx context.Context, setting string, value string) (string, error) {
	var key string
	var valueType string

	switch setting {
	case "minute_limit":
		key = "USER_RATE_LIMIT_PER_MINUTE"
		valueType = "int"
		if _, err := strconv.Atoi(value); err != nil {
			return "❌ Invalid value for minute_limit. Must be a number.", nil
		}
	case "hour_limit":
		key = "USER_RATE_LIMIT_PER_HOUR"
		valueType = "int"
		if _, err := strconv.Atoi(value); err != nil {
			return "❌ Invalid value for hour_limit. Must be a number.", nil
		}
	case "day_limit":
		key = "USER_RATE_LIMIT_PER_DAY"
		valueType = "int"
		if _, err := strconv.Atoi(value); err != nil {
			return "❌ Invalid value for day_limit. Must be a number.", nil
		}
	case "enabled":
		key = "RATE_LIMITING_ENABLED"
		valueType = "bool"
		if value != "true" && value != "false" {
			return "❌ Invalid value for enabled. Must be true or false.", nil
		}
	default:
		return "❌ Unknown setting. Valid options: minute_limit, hour_limit, day_limit, enabled", nil
	}

	// Update configuration
	now := time.Now().Unix()
	config := &storage.Configuration{
		Key:         key,
		Value:       value,
		Type:        valueType,
		Category:    "rate_limiting",
		Description: fmt.Sprintf("Updated by admin command at %d", now),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err := ac.storage.UpsertConfiguration(ctx, config)
	if err != nil {
		ac.logger.Error("Failed to update configuration", "error", err, "key", key)
		return "❌ Failed to update configuration.", nil
	}

	return fmt.Sprintf("✅ Updated %s to %s", setting, value), nil
}

// updateChannelRestrictions updates channel restriction settings
func (ac *AdminCommands) updateChannelRestrictions(ctx context.Context, setting string, value string) (string, error) {
	restrictions, err := ac.channelRestrictor.GetChannelRestrictions(ctx)
	if err != nil {
		return "❌ Failed to get current channel restrictions.", nil
	}

	switch setting {
	case "enabled":
		if value == "true" {
			restrictions.Enabled = true
		} else if value == "false" {
			restrictions.Enabled = false
		} else {
			return "❌ Invalid value for enabled. Must be true or false.", nil
		}

	case "add_channel":
		// Add channel to allowed list
		channelID := strings.TrimSpace(value)
		if channelID == "" {
			return "❌ Channel ID cannot be empty.", nil
		}

		// Check if already exists
		for _, existing := range restrictions.AllowedChannelIDs {
			if existing == channelID {
				return fmt.Sprintf("ℹ️ Channel <#%s> is already in the allowed list.", channelID), nil
			}
		}

		restrictions.AllowedChannelIDs = append(restrictions.AllowedChannelIDs, channelID)

	case "remove_channel":
		// Remove channel from allowed list
		channelID := strings.TrimSpace(value)
		if channelID == "" {
			return "❌ Channel ID cannot be empty.", nil
		}

		newList := []string{}
		found := false
		for _, existing := range restrictions.AllowedChannelIDs {
			if existing != channelID {
				newList = append(newList, existing)
			} else {
				found = true
			}
		}

		if !found {
			return fmt.Sprintf("ℹ️ Channel <#%s> was not in the allowed list.", channelID), nil
		}

		restrictions.AllowedChannelIDs = newList

	case "restrict_dms":
		if value == "true" {
			restrictions.RestrictDMs = true
		} else if value == "false" {
			restrictions.RestrictDMs = false
		} else {
			return "❌ Invalid value for restrict_dms. Must be true or false.", nil
		}

	case "admin_bypass":
		if value == "true" {
			restrictions.AdminBypassEnabled = true
		} else if value == "false" {
			restrictions.AdminBypassEnabled = false
		} else {
			return "❌ Invalid value for admin_bypass. Must be true or false.", nil
		}

	default:
		return "❌ Unknown setting. Valid options: enabled, add_channel, remove_channel, restrict_dms, admin_bypass", nil
	}

	// Update restrictions
	err = ac.channelRestrictor.UpdateChannelRestrictions(ctx, restrictions)
	if err != nil {
		ac.logger.Error("Failed to update channel restrictions", "error", err)
		return "❌ Failed to update channel restrictions.", nil
	}

	return fmt.Sprintf("✅ Updated channel restrictions: %s", setting), nil
}

// isValidTimeWindow checks if a time window is valid
func isValidTimeWindow(window string) bool {
	validWindows := []string{"minute", "hour", "day"}
	for _, valid := range validWindows {
		if window == valid {
			return true
		}
	}
	return false
}
