package bot

import (
	"context"
	"log/slog"
	"time"

	"github.com/bwmarrin/discordgo"
)

// StatusRotator manages automatic rotation of Discord bot statuses
type StatusRotator struct {
	session              *discordgo.Session
	logger               *slog.Logger
	statusManager        *StatusManager
	interval             time.Duration
	batchRefreshInterval time.Duration
	enabled              bool
	stopChan             chan struct{}
	rotationCount        int // Track rotations to determine when to refresh batch
}

// NewStatusRotator creates a new status rotator
func NewStatusRotator(session *discordgo.Session, logger *slog.Logger) *StatusRotator {
	return &StatusRotator{
		session:              session,
		logger:               logger,
		statusManager:        globalStatusManager, // Use global status manager
		interval:             5 * time.Minute,     // Default 5 minutes
		batchRefreshInterval: 25 * time.Minute,    // Refresh batch every 25 minutes (5 rotations)
		enabled:              false,
		stopChan:             make(chan struct{}),
	}
}

// SetStatusManager sets the status manager for the rotator
func (sr *StatusRotator) SetStatusManager(statusManager *StatusManager) {
	sr.statusManager = statusManager
	sr.logger.Info("Status manager set for rotator")
}

// SetInterval sets the rotation interval
func (sr *StatusRotator) SetInterval(interval time.Duration) {
	sr.interval = interval
	sr.logger.Info("Status rotation interval set", "interval", interval)
}

// Start begins the status rotation loop
func (sr *StatusRotator) Start(ctx context.Context) {
	if sr.enabled {
		sr.logger.Warn("Status rotator already running")
		return
	}

	if sr.statusManager == nil {
		sr.logger.Error("Status manager not initialized - cannot start status rotation")
		return
	}

	sr.enabled = true
	sr.rotationCount = 0

	// Load initial batch
	if err := sr.statusManager.LoadNextBatch(ctx); err != nil {
		sr.logger.Error("Failed to load initial status batch", "error", err)
		// Continue anyway with fallback statuses
	}

	sr.logger.Info("Starting BMAD status rotation",
		"interval", sr.interval,
		"batch_refresh_interval", sr.batchRefreshInterval,
		"current_batch_size", sr.statusManager.GetStatusCount())

	// Set initial random status
	sr.rotateStatus(ctx)

	// Start rotation loop
	go sr.rotationLoop(ctx)
}

// Stop stops the status rotation
func (sr *StatusRotator) Stop() {
	if !sr.enabled {
		return
	}

	sr.enabled = false
	close(sr.stopChan)
	sr.logger.Info("Status rotation stopped")
}

// rotationLoop runs the status rotation in a goroutine
func (sr *StatusRotator) rotationLoop(ctx context.Context) {
	ticker := time.NewTicker(sr.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			sr.logger.Info("Status rotation stopped due to context cancellation")
			return
		case <-sr.stopChan:
			sr.logger.Info("Status rotation stopped")
			return
		case <-ticker.C:
			sr.rotateStatus(ctx)
		}
	}
}

// rotateStatus updates the bot's Discord status to a random BMAD status
func (sr *StatusRotator) rotateStatus(ctx context.Context) {
	if sr.session == nil {
		sr.logger.Error("Discord session not available for status update")
		return
	}

	// Check if we need to refresh the batch (every 5 rotations)
	if sr.rotationCount > 0 && sr.rotationCount%5 == 0 {
		sr.logger.Info("Refreshing status batch",
			"rotation_count", sr.rotationCount,
			"current_batch_size", sr.statusManager.GetStatusCount())

		if err := sr.statusManager.RefreshBatch(ctx); err != nil {
			sr.logger.Error("Failed to refresh status batch", "error", err)
			// Continue with existing batch
		} else {
			sr.logger.Info("Status batch refreshed successfully",
				"new_batch_size", sr.statusManager.GetStatusCount())
		}
	}

	var status BMADStatus
	var err error

	if sr.statusManager != nil {
		status, err = sr.statusManager.GetRandomStatus(ctx)
		if err != nil {
			sr.logger.Warn("Failed to get status from manager, using fallback", "error", err)
			status = BMADStatus{
				ActivityType: discordgo.ActivityTypeGame,
				Text:         "BMAD methodology",
			}
		}
	} else {
		// Fallback to legacy function
		status = GetRandomBMADStatus()
	}

	err = sr.session.UpdateStatusComplex(discordgo.UpdateStatusData{
		Activities: []*discordgo.Activity{{
			Name: status.Text,
			Type: status.ActivityType,
		}},
		Status: "online",
	})

	if err != nil {
		sr.logger.Error("Failed to update Discord status",
			"error", err,
			"activity_type", status.ActivityType,
			"text", status.Text)
		return
	}

	sr.rotationCount++

	// Log status update with activity type name for clarity
	activityTypeName := getActivityTypeName(status.ActivityType)
	sr.logger.Info("Discord status updated",
		"activity_type", activityTypeName,
		"text", status.Text,
		"rotation_count", sr.rotationCount,
		"batch_size", sr.statusManager.GetStatusCount(),
		"full_status", activityTypeName+" "+status.Text)
}

// getActivityTypeName converts ActivityType to readable string
func getActivityTypeName(activityType discordgo.ActivityType) string {
	switch activityType {
	case discordgo.ActivityTypeGame:
		return "Playing"
	case discordgo.ActivityTypeListening:
		return "Listening to"
	case discordgo.ActivityTypeWatching:
		return "Watching"
	case discordgo.ActivityTypeCompeting:
		return "Competing in"
	default:
		return "Unknown"
	}
}

// IsRunning returns whether the status rotator is currently running
func (sr *StatusRotator) IsRunning() bool {
	return sr.enabled
}

// GetInterval returns the current rotation interval
func (sr *StatusRotator) GetInterval() time.Duration {
	return sr.interval
}
