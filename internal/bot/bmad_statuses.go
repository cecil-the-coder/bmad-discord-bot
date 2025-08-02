package bot

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"

	"bmad-knowledge-bot/internal/storage"
	"github.com/bwmarrin/discordgo"
)

// BMADStatus represents a Discord status with activity type and text
type BMADStatus struct {
	ActivityType discordgo.ActivityType
	Text         string
}

// StatusManager handles dynamic batch loading of Discord statuses from MySQL
type StatusManager struct {
	storage      storage.StorageService
	logger       *slog.Logger
	currentBatch []BMADStatus
	batchIndex   int
	batchSize    int
	mu           sync.RWMutex
}

// NewStatusManager creates a new status manager with database backend
func NewStatusManager(storageService storage.StorageService, logger *slog.Logger, batchSize int) *StatusManager {
	return &StatusManager{
		storage:   storageService,
		logger:    logger,
		batchSize: batchSize,
	}
}

// LoadNextBatch fetches a new random batch of enabled status messages from the database
func (sm *StatusManager) LoadNextBatch(ctx context.Context) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	statusMessages, err := sm.storage.GetStatusMessagesBatch(ctx, sm.batchSize)
	if err != nil {
		return fmt.Errorf("failed to load status messages batch: %w", err)
	}

	if len(statusMessages) == 0 {
		return fmt.Errorf("no enabled status messages found in database")
	}

	// Convert storage.StatusMessage to bot.BMADStatus
	sm.currentBatch = make([]BMADStatus, 0, len(statusMessages))
	for _, msg := range statusMessages {
		activityType := parseActivityType(msg.ActivityType)
		if activityType == -1 {
			sm.logger.Warn("Unknown activity type in database", "type", msg.ActivityType, "id", msg.ID)
			continue
		}

		bmadStatus := BMADStatus{
			ActivityType: activityType,
			Text:         msg.StatusText,
		}
		sm.currentBatch = append(sm.currentBatch, bmadStatus)
	}

	sm.batchIndex = 0
	sm.logger.Info("New status batch loaded from database",
		"batch_size", len(sm.currentBatch),
		"requested_size", sm.batchSize)

	return nil
}

// GetRandomStatus returns a random status from the current batch, loading a new batch if needed
func (sm *StatusManager) GetRandomStatus(ctx context.Context) (BMADStatus, error) {
	sm.mu.RLock()
	batchEmpty := len(sm.currentBatch) == 0
	sm.mu.RUnlock()

	// Load initial or new batch if current batch is empty
	if batchEmpty {
		if err := sm.LoadNextBatch(ctx); err != nil {
			// Return fallback status if batch loading fails
			return BMADStatus{
				ActivityType: discordgo.ActivityTypeGame,
				Text:         "BMAD methodology",
			}, err
		}
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(sm.currentBatch) == 0 {
		// Return fallback status if still no statuses available
		return BMADStatus{
			ActivityType: discordgo.ActivityTypeGame,
			Text:         "BMAD methodology",
		}, fmt.Errorf("no status messages available")
	}

	// Return random status from current batch
	return sm.currentBatch[rand.Intn(len(sm.currentBatch))], nil
}

// GetStatusCount returns the count of statuses in the current batch
func (sm *StatusManager) GetStatusCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.currentBatch)
}

// RefreshBatch forces a refresh of the current batch
func (sm *StatusManager) RefreshBatch(ctx context.Context) error {
	return sm.LoadNextBatch(ctx)
}

// Legacy global variables for compatibility (will be removed)
var globalStatusManager *StatusManager

// InitializeStatusManager initializes the global status manager
func InitializeStatusManager(storageService storage.StorageService, logger *slog.Logger, batchSize int) {
	globalStatusManager = NewStatusManager(storageService, logger, batchSize)
}

// LoadBMADStatuses is a legacy function for compatibility - now initializes the status manager
func LoadBMADStatuses(filePath string, logger *slog.Logger) error {
	logger.Warn("LoadBMADStatuses is deprecated - status messages are now loaded from database",
		"deprecated_file", filePath)
	return fmt.Errorf("file-based status loading is deprecated - use InitializeStatusManager instead")
}

// parseActivityType converts string to Discord ActivityType
func parseActivityType(activityStr string) discordgo.ActivityType {
	switch strings.ToLower(activityStr) {
	case "playing":
		return discordgo.ActivityTypeGame
	case "listening":
		return discordgo.ActivityTypeListening
	case "watching":
		return discordgo.ActivityTypeWatching
	case "competing":
		return discordgo.ActivityTypeCompeting
	default:
		return -1 // Invalid type
	}
}

// GetRandomBMADStatus returns a random BMAD-themed Discord status (legacy compatibility)
func GetRandomBMADStatus() BMADStatus {
	if globalStatusManager == nil {
		// Fallback status if manager not initialized
		return BMADStatus{
			ActivityType: discordgo.ActivityTypeGame,
			Text:         "BMAD methodology",
		}
	}

	status, err := globalStatusManager.GetRandomStatus(context.Background())
	if err != nil {
		// Return fallback status on error
		return BMADStatus{
			ActivityType: discordgo.ActivityTypeGame,
			Text:         "BMAD methodology",
		}
	}
	return status
}

// GetStatusCount returns the number of statuses in the current batch (legacy compatibility)
func GetStatusCount() int {
	if globalStatusManager == nil {
		return 0
	}
	return globalStatusManager.GetStatusCount()
}

// InitRandomSeed initializes the random seed (deprecated: rand.Seed is no longer needed in Go 1.20+)
func InitRandomSeed() {
	// rand.Seed is deprecated as of Go 1.20 - the random number generator is automatically seeded
}
