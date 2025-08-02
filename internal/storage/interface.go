package storage

import (
	"context"
	"time"
)

// MessageState represents the last processed message state for a Discord channel/thread
type MessageState struct {
	ID                int64   `db:"id"`                  // Primary key, auto-increment
	ChannelID         string  `db:"channel_id"`          // Discord channel ID (required)
	ThreadID          *string `db:"thread_id"`           // Discord thread ID (nullable for regular channels)
	LastMessageID     string  `db:"last_message_id"`     // ID of the last processed message
	LastSeenTimestamp int64   `db:"last_seen_timestamp"` // Unix timestamp of last processed message
	CreatedAt         int64   `db:"created_at"`          // Record creation timestamp
	UpdatedAt         int64   `db:"updated_at"`          // Record last update timestamp
}

// ThreadOwnership represents bot-created thread ownership tracking
type ThreadOwnership struct {
	ID             int64  `db:"id"`               // Primary key, auto-increment
	ThreadID       string `db:"thread_id"`        // Discord thread ID (unique)
	OriginalUserID string `db:"original_user_id"` // ID of user who started the conversation
	CreatedBy      string `db:"created_by"`       // Bot ID that created the thread
	CreationTime   int64  `db:"creation_time"`    // Unix timestamp when thread was created
	CreatedAt      int64  `db:"created_at"`       // Record creation timestamp
	UpdatedAt      int64  `db:"updated_at"`       // Record last update timestamp
}

// Configuration represents a configuration key-value pair stored in the database
type Configuration struct {
	ID          int64  `db:"id"`           // Primary key, auto-increment
	Key         string `db:"config_key"`   // Configuration key (unique)
	Value       string `db:"config_value"` // Configuration value
	Type        string `db:"value_type"`   // Value type: string, int, bool, duration
	Category    string `db:"category"`     // Configuration category for organization
	Description string `db:"description"`  // Human-readable description
	CreatedAt   int64  `db:"created_at"`   // Record creation timestamp
	UpdatedAt   int64  `db:"updated_at"`   // Record last update timestamp
}

// StatusMessage represents a Discord bot status message stored in the database
type StatusMessage struct {
	ID           int64  `db:"id"`            // Primary key, auto-increment
	ActivityType string `db:"activity_type"` // Discord activity type (Playing, Listening, Watching, Competing)
	StatusText   string `db:"status_text"`   // Status message text
	Enabled      bool   `db:"enabled"`       // Whether this status is enabled for rotation
	CreatedAt    int64  `db:"created_at"`    // Record creation timestamp
	UpdatedAt    int64  `db:"updated_at"`    // Record last update timestamp
}

// StorageService defines the interface for message state persistence operations
type StorageService interface {
	// Initialize sets up the database connection and creates necessary tables
	Initialize(ctx context.Context) error

	// Close closes the database connection
	Close() error

	// GetMessageState retrieves the last seen message state for a channel/thread
	GetMessageState(ctx context.Context, channelID string, threadID *string) (*MessageState, error)

	// UpsertMessageState creates or updates the message state for a channel/thread
	UpsertMessageState(ctx context.Context, state *MessageState) error

	// GetAllMessageStates retrieves all message states for recovery purposes
	GetAllMessageStates(ctx context.Context) ([]*MessageState, error)

	// GetMessageStatesWithinWindow retrieves message states within a specific time window
	GetMessageStatesWithinWindow(ctx context.Context, windowDuration time.Duration) ([]*MessageState, error)

	// HealthCheck verifies that the database connection is working
	HealthCheck(ctx context.Context) error

	// GetThreadOwnership retrieves thread ownership information for a thread
	GetThreadOwnership(ctx context.Context, threadID string) (*ThreadOwnership, error)

	// UpsertThreadOwnership creates or updates thread ownership information
	UpsertThreadOwnership(ctx context.Context, ownership *ThreadOwnership) error

	// GetAllThreadOwnerships retrieves all thread ownership records
	GetAllThreadOwnerships(ctx context.Context) ([]*ThreadOwnership, error)

	// CleanupOldThreadOwnerships removes old thread ownership records
	CleanupOldThreadOwnerships(ctx context.Context, maxAge int64) error

	// GetConfiguration retrieves a configuration value by key
	GetConfiguration(ctx context.Context, key string) (*Configuration, error)

	// UpsertConfiguration creates or updates a configuration entry
	UpsertConfiguration(ctx context.Context, config *Configuration) error

	// GetConfigurationsByCategory retrieves all configurations in a category
	GetConfigurationsByCategory(ctx context.Context, category string) ([]*Configuration, error)

	// GetAllConfigurations retrieves all configurations
	GetAllConfigurations(ctx context.Context) ([]*Configuration, error)

	// DeleteConfiguration removes a configuration entry by key
	DeleteConfiguration(ctx context.Context, key string) error

	// GetStatusMessagesBatch retrieves a random batch of enabled status messages
	GetStatusMessagesBatch(ctx context.Context, limit int) ([]*StatusMessage, error)

	// AddStatusMessage creates a new status message
	AddStatusMessage(ctx context.Context, activityType, statusText string, enabled bool) error

	// UpdateStatusMessage updates the enabled status of a status message
	UpdateStatusMessage(ctx context.Context, id int64, enabled bool) error

	// GetAllStatusMessages retrieves all status messages
	GetAllStatusMessages(ctx context.Context) ([]*StatusMessage, error)

	// GetEnabledStatusMessagesCount returns the count of enabled status messages
	GetEnabledStatusMessagesCount(ctx context.Context) (int, error)
}
