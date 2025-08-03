package bot

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Simple test implementation to improve coverage
func TestNewAdminCommands(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create a simple test - just verify constructor works
	adminCommands := NewAdminCommands(nil, nil, nil, logger)

	assert.NotNil(t, adminCommands)
	assert.Equal(t, logger, adminCommands.logger)
}

func TestIsValidTimeWindow(t *testing.T) {
	tests := []struct {
		name     string
		window   string
		expected bool
	}{
		{"valid minute", "minute", true},
		{"valid hour", "hour", true},
		{"valid day", "day", true},
		{"invalid window", "week", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidTimeWindow(tt.window)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHandleAdminHelp(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	adminCommands := NewAdminCommands(nil, nil, nil, logger)

	response := adminCommands.handleAdminHelp()

	assert.Contains(t, response, "🛡️ **Admin Commands Help:**")
	assert.Contains(t, response, "Rate Limiting:")
	assert.Contains(t, response, "Channel Restrictions:")
	assert.Contains(t, response, "ratelimit-status")
	assert.Contains(t, response, "ratelimit-reset")
	assert.Contains(t, response, "channel-restrictions")
	assert.Contains(t, response, "ADMIN_ROLE_NAMES")
}
