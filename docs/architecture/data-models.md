# Data Models

The following structs are used to manage application state both in memory and persistent storage.

## In-Memory State Models

### RateLimitState

**Purpose**: To hold the current state of the API usage counter.

```go
// RateLimitState tracks API call counts over a specific window.
type RateLimitState struct {
    Calls       []time.Time // Stores timestamps of recent calls
    Mutex       sync.Mutex
    Window      time.Duration // e.g., 1 minute
    Limit       int           // e.g., 60 calls per minute
}
```

## Database Models

### MessageState

**Purpose**: Persistent storage of last seen message information per Discord channel/thread for recovery after bot restarts.

**Database Table**: `message_states`

```go
// MessageState represents the last processed message state for a Discord channel/thread
type MessageState struct {
    ID               int64     `db:"id"`                // Primary key, auto-increment
    ChannelID        string    `db:"channel_id"`        // Discord channel ID (required)
    ThreadID         *string   `db:"thread_id"`         // Discord thread ID (nullable for regular channels)
    LastMessageID    string    `db:"last_message_id"`   // ID of the last processed message
    LastSeenTimestamp int64    `db:"last_seen_timestamp"` // Unix timestamp of last processed message
    CreatedAt        int64     `db:"created_at"`        // Record creation timestamp
    UpdatedAt        int64     `db:"updated_at"`        // Record last update timestamp
}
```

**Key Features**:
- Uses database struct tags for SQLite mapping
- ThreadID is nullable pointer for regular channels vs threads
- Unix timestamps for efficient time-based queries
- Supports unique constraint on (channel_id, thread_id) combination