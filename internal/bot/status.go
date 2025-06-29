package bot

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// DiscordSession defines the interface for Discord session operations needed for status management
type DiscordSession interface {
	UpdateStatusComplex(data discordgo.UpdateStatusData) error
}

// StatusManager defines the interface for Discord presence management
type StatusManager interface {
	// UpdateStatusFromRateLimit updates Discord status based on rate limit state
	UpdateStatusFromRateLimit(providerID string, status string) error

	// SetOnline sets the bot status to Online (Green) with custom activity
	SetOnline(activity string) error

	// SetIdle sets the bot status to Idle (Yellow) with custom activity
	SetIdle(activity string) error

	// SetDoNotDisturb sets the bot status to Do Not Disturb (Red) with custom activity
	SetDoNotDisturb(activity string) error

	// GetCurrentStatus returns the current Discord status and activity
	GetCurrentStatus() (discordgo.Status, *discordgo.Activity)
}

// DiscordStatusManager implements the StatusManager interface
type DiscordStatusManager struct {
	session          BotSession
	logger           *slog.Logger
	mutex            sync.RWMutex
	currentStatus    discordgo.Status
	currentActivity  *discordgo.Activity
	lastUpdate       time.Time
	debounceInterval time.Duration
}

// NewDiscordStatusManager creates a new Discord status manager
func NewDiscordStatusManager(session BotSession, logger *slog.Logger) *DiscordStatusManager {
	return &DiscordStatusManager{
		session:          session,
		logger:           logger,
		currentStatus:    discordgo.StatusOnline,
		currentActivity:  nil,
		debounceInterval: 30 * time.Second, // Default debounce interval
	}
}

// SetDebounceInterval configures the minimum time between status updates
func (dsm *DiscordStatusManager) SetDebounceInterval(interval time.Duration) {
	dsm.mutex.Lock()
	defer dsm.mutex.Unlock()
	dsm.debounceInterval = interval
}

// UpdateStatusFromRateLimit updates Discord status based on rate limit state
func (dsm *DiscordStatusManager) UpdateStatusFromRateLimit(providerID string, status string) error {
	dsm.mutex.Lock()
	defer dsm.mutex.Unlock()

	// Check debouncing to prevent rapid status changes
	if time.Since(dsm.lastUpdate) < dsm.debounceInterval {
		dsm.logger.Debug("Status update debounced",
			"provider", providerID,
			"status", status,
			"time_since_last", time.Since(dsm.lastUpdate))
		return nil
	}

	var err error
	switch status {
	case "Normal":
		err = dsm.setOnlineLocked("API: Ready")
	case "Warning":
		err = dsm.setIdleLocked("API: Busy")
	case "Throttled":
		err = dsm.setDoNotDisturbLocked("API: Throttled")
	default:
		dsm.logger.Warn("Unknown rate limit status", "provider", providerID, "status", status)
		return fmt.Errorf("unknown status: %s", status)
	}

	if err != nil {
		dsm.logger.Error("Failed to update Discord status",
			"provider", providerID,
			"status", status,
			"error", err)
		return err
	}

	dsm.lastUpdate = time.Now()
	dsm.logger.Info("Discord status updated",
		"provider", providerID,
		"rate_limit_status", status,
		"discord_status", dsm.currentStatus,
		"activity", dsm.currentActivity.Name)

	return nil
}

// SetOnline sets the bot status to Online (Green) with custom activity
func (dsm *DiscordStatusManager) SetOnline(activity string) error {
	dsm.mutex.Lock()
	defer dsm.mutex.Unlock()
	return dsm.setOnlineLocked(activity)
}

// setOnlineLocked sets online status without acquiring lock (internal method)
func (dsm *DiscordStatusManager) setOnlineLocked(activity string) error {
	activityObj := &discordgo.Activity{
		Name: activity,
		Type: discordgo.ActivityTypeGame,
	}

	err := dsm.session.UpdatePresence(discordgo.StatusOnline, activityObj)

	if err != nil {
		return fmt.Errorf("failed to set online status: %w", err)
	}

	dsm.currentStatus = discordgo.StatusOnline
	dsm.currentActivity = activityObj
	return nil
}

// SetIdle sets the bot status to Idle (Yellow) with custom activity
func (dsm *DiscordStatusManager) SetIdle(activity string) error {
	dsm.mutex.Lock()
	defer dsm.mutex.Unlock()
	return dsm.setIdleLocked(activity)
}

// setIdleLocked sets idle status without acquiring lock (internal method)
func (dsm *DiscordStatusManager) setIdleLocked(activity string) error {
	activityObj := &discordgo.Activity{
		Name: activity,
		Type: discordgo.ActivityTypeGame,
	}

	err := dsm.session.UpdatePresence(discordgo.StatusIdle, activityObj)

	if err != nil {
		return fmt.Errorf("failed to set idle status: %w", err)
	}

	dsm.currentStatus = discordgo.StatusIdle
	dsm.currentActivity = activityObj
	return nil
}

// SetDoNotDisturb sets the bot status to Do Not Disturb (Red) with custom activity
func (dsm *DiscordStatusManager) SetDoNotDisturb(activity string) error {
	dsm.mutex.Lock()
	defer dsm.mutex.Unlock()
	return dsm.setDoNotDisturbLocked(activity)
}

// setDoNotDisturbLocked sets DND status without acquiring lock (internal method)
func (dsm *DiscordStatusManager) setDoNotDisturbLocked(activity string) error {
	activityObj := &discordgo.Activity{
		Name: activity,
		Type: discordgo.ActivityTypeGame,
	}

	err := dsm.session.UpdatePresence(discordgo.StatusDoNotDisturb, activityObj)

	if err != nil {
		return fmt.Errorf("failed to set do not disturb status: %w", err)
	}

	dsm.currentStatus = discordgo.StatusDoNotDisturb
	dsm.currentActivity = activityObj
	return nil
}

// GetCurrentStatus returns the current Discord status and activity
func (dsm *DiscordStatusManager) GetCurrentStatus() (discordgo.Status, *discordgo.Activity) {
	dsm.mutex.RLock()
	defer dsm.mutex.RUnlock()

	// Return copies to prevent external modification
	var activity *discordgo.Activity
	if dsm.currentActivity != nil {
		activity = &discordgo.Activity{
			Name: dsm.currentActivity.Name,
			Type: dsm.currentActivity.Type,
		}
	}

	return dsm.currentStatus, activity
}
