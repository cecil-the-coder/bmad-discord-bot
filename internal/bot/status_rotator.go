package bot

import (
	"context"
	"log/slog"
	"time"

	"github.com/bwmarrin/discordgo"
)

// StatusRotator manages automatic rotation of Discord bot statuses
type StatusRotator struct {
	session  *discordgo.Session
	logger   *slog.Logger
	interval time.Duration
	enabled  bool
	stopChan chan struct{}
}

// NewStatusRotator creates a new status rotator
func NewStatusRotator(session *discordgo.Session, logger *slog.Logger) *StatusRotator {
	return &StatusRotator{
		session:  session,
		logger:   logger,
		interval: 5 * time.Minute, // Default 5 minutes
		enabled:  false,
		stopChan: make(chan struct{}),
	}
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

	sr.enabled = true
	InitRandomSeed() // Initialize random seed for status selection

	sr.logger.Info("Starting BMAD status rotation",
		"interval", sr.interval,
		"total_statuses", len(bmadStatuses))

	// Set initial random status
	sr.rotateStatus()

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
			sr.rotateStatus()
		}
	}
}

// rotateStatus updates the bot's Discord status to a random BMAD status
func (sr *StatusRotator) rotateStatus() {
	if sr.session == nil {
		sr.logger.Error("Discord session not available for status update")
		return
	}

	status := GetRandomBMADStatus()

	err := sr.session.UpdateStatusComplex(discordgo.UpdateStatusData{
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

	// Log status update with activity type name for clarity
	activityTypeName := getActivityTypeName(status.ActivityType)
	sr.logger.Info("Discord status updated",
		"activity_type", activityTypeName,
		"text", status.Text,
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
