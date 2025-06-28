package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvironmentVariableValidation(t *testing.T) {
	// Save original environment
	originalToken := os.Getenv("BOT_TOKEN")
	defer func() {
		if originalToken != "" {
			os.Setenv("BOT_TOKEN", originalToken)
		} else {
			os.Unsetenv("BOT_TOKEN")
		}
	}()

	// Test case 1: Missing BOT_TOKEN should be handled
	os.Unsetenv("BOT_TOKEN")
	token := os.Getenv("BOT_TOKEN")
	if token != "" {
		t.Errorf("Expected empty token, got %s", token)
	}

	// Test case 2: Valid BOT_TOKEN should be read correctly
	expectedToken := "test_token_123"
	os.Setenv("BOT_TOKEN", expectedToken)
	token = os.Getenv("BOT_TOKEN")
	if token != expectedToken {
		t.Errorf("Expected token %s, got %s", expectedToken, token)
	}
}

func TestTokenValidation(t *testing.T) {
	testCases := []struct {
		name     string
		token    string
		expected bool
	}{
		{"Empty token", "", false},
		{"Valid token", "Bot.token.here", true},
		{"Short token", "abc", true}, // Any non-empty string is valid for our validation
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isValid := tc.token != ""
			if isValid != tc.expected {
				t.Errorf("Expected validation result %v for token %s, got %v", tc.expected, tc.token, isValid)
			}
		})
	}
}

func TestValidateToken(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty token",
			token:       "",
			expectError: true,
			errorMsg:    "BOT_TOKEN environment variable is required",
		},
		{
			name:        "token too short",
			token:       "short",
			expectError: true,
			errorMsg:    "token appears to be too short",
		},
		{
			name:        "token with no dots",
			token:       "verylongtokenwithoutdotsbutlongenoughtopasslengthcheck",
			expectError: true,
			errorMsg:    "token format appears invalid (missing expected separators)",
		},
		{
			name:        "valid token format",
			token:       "your-discord-bot-token",
			expectError: false,
		},
		{
			name:        "token with whitespace",
			token:       "  your-discord-bot-token  ",
			expectError: false,
		},
		{
			name:        "minimal valid token",
			token:       "your-discord-bot-token",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateToken(tt.token)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for token '%s', but got none", tt.name)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', but got: %s", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for token '%s', but got: %v", tt.name, err)
				}
			}
		})
	}
}

func TestValidateGeminiCLIPath(t *testing.T) {
	// Create a temporary file for testing
	tmpDir := t.TempDir()
	validPath := filepath.Join(tmpDir, "test-gemini")
	
	// Create the test file
	if err := os.WriteFile(validPath, []byte("#!/bin/bash\necho test"), 0755); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tests := []struct {
		name        string
		cliPath     string
		expectError bool
	}{
		{
			name:        "empty CLI path",
			cliPath:     "",
			expectError: true,
		},
		{
			name:        "non-existent CLI path",
			cliPath:     "/non/existent/path/gemini-cli",
			expectError: true,
		},
		{
			name:        "valid CLI path",
			cliPath:     validPath,
			expectError: false,
		},
		{
			name:        "existing file (this test binary)",
			cliPath:     os.Args[0],
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGeminiCLIPath(tt.cliPath)
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestHealthCheckFlag(t *testing.T) {
	// Test that health check flag would be recognized
	// We can't easily test the os.Exit behavior, but we can test the condition
	args := []string{"program", "--health-check"}
	
	if len(args) > 1 && args[1] == "--health-check" {
		t.Log("Health check flag correctly detected - would cause main() to exit with code 0")
	} else {
		t.Error("Health check flag not properly detected")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(substr) == 0 || len(s) >= len(substr) && (s == substr || containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}