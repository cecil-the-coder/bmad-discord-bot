package bot

import (
	"log/slog"
	"os"
	"testing"
)

func TestNewSession(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	token := "test_token_123"

	session := NewSession(token, logger)

	if session == nil {
		t.Error("Expected non-nil session")
		return
	}

	if session.logger != logger {
		t.Error("Expected logger to be set correctly")
	}

	if session.token != token {
		t.Errorf("Expected token %s, got %s", token, session.token)
	}
}

func TestSessionTokenValidation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	testCases := []struct {
		name  string
		token string
	}{
		{"Valid token", "Bot.token.here"},
		{"Empty token", ""},
		{"Short token", "abc"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			session := NewSession(tc.token, logger)
			if session.token != tc.token {
				t.Errorf("Expected token %s, got %s", tc.token, session.token)
			}
		})
	}
}
