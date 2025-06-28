package monitor

import (
	"log/slog"
)

// RateLimiter provides rate limiting functionality for API calls
type RateLimiter struct {
	logger *slog.Logger
}

// NewRateLimiter creates a new rate limiter instance
func NewRateLimiter(logger *slog.Logger) *RateLimiter {
	return &RateLimiter{
		logger: logger,
	}
}

// TODO: Implement rate limiting logic with go-cache in future stories