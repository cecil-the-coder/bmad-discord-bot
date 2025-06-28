# Data Models

No persistent database is required for the MVP. The following structs will be used to manage application state in memory.

## RateLimitState

**Purpose**: To hold the current state of the API usage counter.

```go
// RateLimitState tracks API call counts over a specific window.
type RateLimitState struct {
    Calls       []time.Time // Stores timestamps of recent calls
    Mutex       sync.Mutex
    Window      time.Duration // e.g., 1 minute
    Limit       int           // e.g., 60 calls per minute
}