package storage

import (
	"context"
	"time"
)

// MessageState represents the last processed message state for a Discord channel/thread
type MessageState struct {
	ID                int64   `db:"id"`                 // Primary key, auto-increment
	ChannelID         string  `db:"channel_id"`         // Discord channel ID (required)
	ThreadID          *string `db:"thread_id"`          // Discord thread ID (nullable for regular channels)
	LastMessageID     string  `db:"last_message_id"`    // ID of the last processed message
	LastSeenTimestamp int64   `db:"last_seen_timestamp"` // Unix timestamp of last processed message
	CreatedAt         int64   `db:"created_at"`         // Record creation timestamp
	UpdatedAt         int64   `db:"updated_at"`         // Record last update timestamp
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
}