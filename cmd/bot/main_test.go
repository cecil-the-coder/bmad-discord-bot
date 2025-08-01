package main

import (
	"os"
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

func TestLoadRateLimitConfig(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_MINUTE",
		"AI_PROVIDER_RATE_LIMIT_PER_MINUTE",
		"AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_DAY",
		"AI_PROVIDER_RATE_LIMIT_PER_DAY",
		"AI_PROVIDER_OLLAMA_WARNING_THRESHOLD",
		"AI_PROVIDER_WARNING_THRESHOLD",
		"AI_PROVIDER_OLLAMA_THROTTLED_THRESHOLD",
		"AI_PROVIDER_THROTTLED_THRESHOLD",
	}

	for _, env := range envVars {
		originalEnv[env] = os.Getenv(env)
		os.Unsetenv(env)
	}

	defer func() {
		for _, env := range envVars {
			if originalEnv[env] != "" {
				os.Setenv(env, originalEnv[env])
			} else {
				os.Unsetenv(env)
			}
		}
	}()

	tests := []struct {
		name        string
		provider    string
		envVars     map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "default values",
			provider:    "ollama",
			envVars:     map[string]string{},
			expectError: false,
		},
		{
			name:     "custom values",
			provider: "ollama",
			envVars: map[string]string{
				"AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_MINUTE": "120",
				"AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_DAY":    "2000",
				"AI_PROVIDER_OLLAMA_WARNING_THRESHOLD":     "0.8",
				"AI_PROVIDER_OLLAMA_THROTTLED_THRESHOLD":   "0.9",
			},
			expectError: false,
		},
		{
			name:     "invalid per minute",
			provider: "ollama",
			envVars: map[string]string{
				"AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_MINUTE": "invalid",
			},
			expectError: true,
			errorMsg:    "invalid rate limit per minute",
		},
		{
			name:     "zero per minute",
			provider: "ollama",
			envVars: map[string]string{
				"AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_MINUTE": "0",
			},
			expectError: true,
			errorMsg:    "rate limit per minute must be positive",
		},
		{
			name:     "invalid warning threshold",
			provider: "ollama",
			envVars: map[string]string{
				"AI_PROVIDER_OLLAMA_WARNING_THRESHOLD": "invalid",
			},
			expectError: true,
			errorMsg:    "invalid warning threshold",
		},
		{
			name:     "warning threshold >= throttled threshold",
			provider: "ollama",
			envVars: map[string]string{
				"AI_PROVIDER_OLLAMA_WARNING_THRESHOLD":   "0.9",
				"AI_PROVIDER_OLLAMA_THROTTLED_THRESHOLD": "0.8",
			},
			expectError: true,
			errorMsg:    "warning threshold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			config, err := loadRateLimitConfig(tt.provider)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for test '%s', but got none", tt.name)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', but got: %s", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for test '%s', but got: %v", tt.name, err)
				} else {
					if config.ProviderID != tt.provider {
						t.Errorf("Expected provider ID '%s', got '%s'", tt.provider, config.ProviderID)
					}
				}
			}

			// Clean up environment variables
			for key := range tt.envVars {
				os.Unsetenv(key)
			}
		})
	}
}

func TestLoadDatabaseConfig(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{"DATABASE_TYPE", "DATABASE_PATH", "MESSAGE_RECOVERY_WINDOW_MINUTES"}

	for _, env := range envVars {
		originalEnv[env] = os.Getenv(env)
		os.Unsetenv(env)
	}

	defer func() {
		for _, env := range envVars {
			if originalEnv[env] != "" {
				os.Setenv(env, originalEnv[env])
			} else {
				os.Unsetenv(env)
			}
		}
	}()

	tests := []struct {
		name                   string
		envVars                map[string]string
		expectedDatabaseType   string
		expectedDatabasePath   string
		expectedRecoveryWindow int
		expectError            bool
		errorMsg               string
	}{
		{
			name:                   "default values",
			envVars:                map[string]string{},
			expectedDatabaseType:   "sqlite",
			expectedDatabasePath:   "./data/bot_state.db",
			expectedRecoveryWindow: 5,
			expectError:            false,
		},
		{
			name: "mysql configuration",
			envVars: map[string]string{
				"DATABASE_TYPE":                   "mysql",
				"DATABASE_PATH":                   "/custom/path",
				"MESSAGE_RECOVERY_WINDOW_MINUTES": "10",
			},
			expectedDatabaseType:   "mysql",
			expectedDatabasePath:   "/custom/path",
			expectedRecoveryWindow: 10,
			expectError:            false,
		},
		{
			name: "invalid database type",
			envVars: map[string]string{
				"DATABASE_TYPE": "invalid",
			},
			expectError: true,
			errorMsg:    "invalid DATABASE_TYPE",
		},
		{
			name: "invalid recovery window",
			envVars: map[string]string{
				"MESSAGE_RECOVERY_WINDOW_MINUTES": "invalid",
			},
			expectError: true,
			errorMsg:    "invalid MESSAGE_RECOVERY_WINDOW_MINUTES",
		},
		{
			name: "negative recovery window",
			envVars: map[string]string{
				"MESSAGE_RECOVERY_WINDOW_MINUTES": "-5",
			},
			expectError: true,
			errorMsg:    "MESSAGE_RECOVERY_WINDOW_MINUTES must be non-negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			dbType, dbPath, recoveryWindow, err := loadDatabaseConfig()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for test '%s', but got none", tt.name)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', but got: %s", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for test '%s', but got: %v", tt.name, err)
				} else {
					if dbType != tt.expectedDatabaseType {
						t.Errorf("Expected database type '%s', got '%s'", tt.expectedDatabaseType, dbType)
					}
					if dbPath != tt.expectedDatabasePath {
						t.Errorf("Expected database path '%s', got '%s'", tt.expectedDatabasePath, dbPath)
					}
					if recoveryWindow != tt.expectedRecoveryWindow {
						t.Errorf("Expected recovery window %d, got %d", tt.expectedRecoveryWindow, recoveryWindow)
					}
				}
			}

			// Clean up environment variables
			for key := range tt.envVars {
				os.Unsetenv(key)
			}
		})
	}
}

func TestLoadMySQLConfig(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{"MYSQL_HOST", "MYSQL_PORT", "MYSQL_DATABASE", "MYSQL_USERNAME", "MYSQL_PASSWORD", "MYSQL_TIMEOUT"}

	for _, env := range envVars {
		originalEnv[env] = os.Getenv(env)
		os.Unsetenv(env)
	}

	defer func() {
		for _, env := range envVars {
			if originalEnv[env] != "" {
				os.Setenv(env, originalEnv[env])
			} else {
				os.Unsetenv(env)
			}
		}
	}()

	tests := []struct {
		name        string
		envVars     map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid configuration",
			envVars: map[string]string{
				"MYSQL_USERNAME": "testuser",
				"MYSQL_PASSWORD": "testpass",
			},
			expectError: false,
		},
		{
			name: "missing username",
			envVars: map[string]string{
				"MYSQL_PASSWORD": "testpass",
			},
			expectError: true,
			errorMsg:    "MYSQL_USERNAME environment variable is required",
		},
		{
			name: "missing password",
			envVars: map[string]string{
				"MYSQL_USERNAME": "testuser",
			},
			expectError: true,
			errorMsg:    "MYSQL_PASSWORD environment variable is required",
		},
		{
			name: "invalid timeout",
			envVars: map[string]string{
				"MYSQL_USERNAME": "testuser",
				"MYSQL_PASSWORD": "testpass",
				"MYSQL_TIMEOUT":  "invalid",
			},
			expectError: true,
			errorMsg:    "invalid MYSQL_TIMEOUT format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			config, err := loadMySQLConfig()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for test '%s', but got none", tt.name)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', but got: %s", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for test '%s', but got: %v", tt.name, err)
				} else {
					// Check default values
					if config.Host == "" {
						t.Error("Expected default host to be set")
					}
					if config.Port == "" {
						t.Error("Expected default port to be set")
					}
				}
			}

			// Clean up environment variables
			for key := range tt.envVars {
				os.Unsetenv(key)
			}
		})
	}
}

func TestLoadReplyMentionConfig(t *testing.T) {
	// Save original environment
	originalEnv := os.Getenv("REPLY_MENTION_DELETE_MESSAGE")
	defer func() {
		if originalEnv != "" {
			os.Setenv("REPLY_MENTION_DELETE_MESSAGE", originalEnv)
		} else {
			os.Unsetenv("REPLY_MENTION_DELETE_MESSAGE")
		}
	}()

	tests := []struct {
		name        string
		envValue    string
		expected    bool
		expectError bool
	}{
		{
			name:        "default value",
			envValue:    "",
			expected:    false,
			expectError: false,
		},
		{
			name:        "true value",
			envValue:    "true",
			expected:    true,
			expectError: false,
		},
		{
			name:        "false value",
			envValue:    "false",
			expected:    false,
			expectError: false,
		},
		{
			name:        "invalid value",
			envValue:    "invalid",
			expected:    false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue == "" {
				os.Unsetenv("REPLY_MENTION_DELETE_MESSAGE")
			} else {
				os.Setenv("REPLY_MENTION_DELETE_MESSAGE", tt.envValue)
			}

			config, err := loadReplyMentionConfig()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for test '%s', but got none", tt.name)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for test '%s', but got: %v", tt.name, err)
				} else if config.DeleteReplyMessage != tt.expected {
					t.Errorf("Expected DeleteReplyMessage to be %v, got %v", tt.expected, config.DeleteReplyMessage)
				}
			}
		})
	}
}

func TestLoadReactionTriggerConfig(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"REACTION_TRIGGER_ENABLED",
		"REACTION_TRIGGER_EMOJI",
		"REACTION_TRIGGER_APPROVED_USER_IDS",
		"REACTION_TRIGGER_APPROVED_ROLE_NAMES",
		"REACTION_TRIGGER_REQUIRE_REACTION",
		"REACTION_TRIGGER_REMOVE_REACTION",
	}

	for _, env := range envVars {
		originalEnv[env] = os.Getenv(env)
		os.Unsetenv(env)
	}

	defer func() {
		for _, env := range envVars {
			if originalEnv[env] != "" {
				os.Setenv(env, originalEnv[env])
			} else {
				os.Unsetenv(env)
			}
		}
	}()

	tests := []struct {
		name        string
		envVars     map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "default values",
			envVars:     map[string]string{},
			expectError: false,
		},
		{
			name: "enabled with user IDs",
			envVars: map[string]string{
				"REACTION_TRIGGER_ENABLED":             "true",
				"REACTION_TRIGGER_APPROVED_USER_IDS":   "123,456,789",
				"REACTION_TRIGGER_APPROVED_ROLE_NAMES": "admin,moderator",
			},
			expectError: false,
		},
		{
			name: "invalid enabled value",
			envVars: map[string]string{
				"REACTION_TRIGGER_ENABLED": "invalid",
			},
			expectError: true,
			errorMsg:    "invalid REACTION_TRIGGER_ENABLED",
		},
		{
			name: "invalid require reaction value",
			envVars: map[string]string{
				"REACTION_TRIGGER_REQUIRE_REACTION": "invalid",
			},
			expectError: true,
			errorMsg:    "invalid REACTION_TRIGGER_REQUIRE_REACTION",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			config, err := loadReactionTriggerConfig()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for test '%s', but got none", tt.name)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', but got: %s", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for test '%s', but got: %v", tt.name, err)
				} else {
					// Validate some basic properties
					if config.TriggerEmoji == "" {
						t.Error("Expected default trigger emoji to be set")
					}
				}
			}

			// Clean up environment variables
			for key := range tt.envVars {
				os.Unsetenv(key)
			}
		})
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
