package bot

import (
	"fmt"
	"log/slog"
	"strings"
)

// Session manages Discord bot session lifecycle
type Session struct {
	logger *slog.Logger
	token  string
}

// NewSession creates a new Discord bot session
func NewSession(token string, logger *slog.Logger) *Session {
	return &Session{
		logger: logger,
		token:  token,
	}
}

// IsTokenValid validates the Discord bot token format and content
func (s *Session) IsTokenValid() error {
	if s.token == "" {
		return fmt.Errorf("bot token is empty")
	}

	// Basic token format validation
	token := strings.TrimSpace(s.token)
	if len(token) < 50 {
		return fmt.Errorf("token appears to be too short (expected at least 50 characters)")
	}

	// Discord bot tokens typically have specific patterns
	if !strings.Contains(token, ".") {
		return fmt.Errorf("token format appears invalid (missing expected separators)")
	}

	// Additional validation: Discord tokens typically have 3 parts separated by dots
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("token format appears invalid (expected 3 dot-separated parts)")
	}

	// Validate each part has reasonable length
	if len(parts[0]) < 15 || len(parts[1]) < 5 || len(parts[2]) < 20 {
		return fmt.Errorf("token format appears invalid (parts too short)")
	}

	s.logger.Debug("Token validation passed", "token_length", len(token))
	return nil
}

// GetToken returns the bot token (for internal use)
func (s *Session) GetToken() string {
	return s.token
}

// TODO: Implement Discord session management in current story