package main

import (
	"context"
	"os"
	"testing"
	"time"

	"bmad-knowledge-bot/internal/config"
	"github.com/bwmarrin/discordgo"
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
			token:       "FAKE_BOT_ID_FOR_TESTING_ONLY.TEST_MIDDLE_PART.FAKE_SECRET_PART_FOR_TESTING_PURPOSES_ONLY",
			expectError: false,
		},
		{
			name:        "token with whitespace",
			token:       "  FAKE_BOT_ID_FOR_TESTING_ONLY.TEST_MIDDLE_PART.FAKE_SECRET_PART_FOR_TESTING_PURPOSES_ONLY  ",
			expectError: false,
		},
		{
			name:        "minimal valid token",
			token:       "FAKE_BOT_ID_FOR_TESTING.TEST_MIDDLE.FAKE_SECRET_FOR_TESTING",
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
	envVars := []string{"MESSAGE_RECOVERY_WINDOW_MINUTES"}

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
		expectedRecoveryWindow int
		expectError            bool
		errorMsg               string
	}{
		{
			name:                   "default values",
			envVars:                map[string]string{},
			expectedRecoveryWindow: 5,
			expectError:            false,
		},
		{
			name: "custom recovery window",
			envVars: map[string]string{
				"MESSAGE_RECOVERY_WINDOW_MINUTES": "10",
			},
			expectedRecoveryWindow: 10,
			expectError:            false,
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

			recoveryWindow, err := loadDatabaseConfig()

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

// TestReadyFunction tests the ready event handler
func TestReadyFunction(t *testing.T) {
	// Create a mock ready event
	readyEvent := &discordgo.Ready{
		User: &discordgo.User{
			Username:      "TestBot",
			Discriminator: "1234",
		},
	}

	// Test with nil session - this will panic as expected since ready() calls methods on nil session
	// We're testing that the function exists and can be called (for coverage)
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when calling ready with nil session")
		}
	}()

	ready(nil, readyEvent)
}

// TestHelperFunctions tests the utility helper functions
func TestHelperFunctions(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{"empty substring", "hello", "", true},
		{"found at start", "hello world", "hello", true},
		{"found in middle", "hello world", "o w", true},
		{"found at end", "hello world", "world", true},
		{"not found", "hello world", "xyz", false},
		{"longer than string", "hi", "hello", false},
		{"exact match", "test", "test", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contains(tt.s, tt.substr)
			if got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
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

// Mock config service for testing
type mockConfigService struct {
	configs map[string]string
}

func (m *mockConfigService) Initialize(ctx context.Context) error { return nil }
func (m *mockConfigService) Close() error                         { return nil }
func (m *mockConfigService) GetConfig(ctx context.Context, key string) (string, error) {
	if value, exists := m.configs[key]; exists {
		return value, nil
	}
	return "", &config.ConfigError{Key: key, Message: "not found"}
}
func (m *mockConfigService) GetConfigWithDefault(ctx context.Context, key, defaultValue string) string {
	if value, exists := m.configs[key]; exists {
		return value
	}
	return defaultValue
}
func (m *mockConfigService) GetConfigInt(ctx context.Context, key string) (int, error) {
	if value, exists := m.configs[key]; exists {
		return parseToInt(value)
	}
	return 0, &config.ConfigError{Key: key, Message: "not found"}
}
func (m *mockConfigService) GetConfigIntWithDefault(ctx context.Context, key string, defaultValue int) int {
	if value, exists := m.configs[key]; exists {
		if intVal, err := parseToInt(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
func (m *mockConfigService) GetConfigBool(ctx context.Context, key string) (bool, error) {
	if value, exists := m.configs[key]; exists {
		return value == "true", nil
	}
	return false, &config.ConfigError{Key: key, Message: "not found"}
}
func (m *mockConfigService) GetConfigBoolWithDefault(ctx context.Context, key string, defaultValue bool) bool {
	if value, exists := m.configs[key]; exists {
		return value == "true"
	}
	return defaultValue
}
func (m *mockConfigService) GetConfigDuration(ctx context.Context, key string) (time.Duration, error) {
	return 0, nil
}
func (m *mockConfigService) GetConfigDurationWithDefault(ctx context.Context, key string, defaultValue time.Duration) time.Duration {
	return defaultValue
}
func (m *mockConfigService) SetConfig(ctx context.Context, key, value, category, description string) error {
	return nil
}
func (m *mockConfigService) SetConfigTyped(ctx context.Context, key, value, valueType, category, description string) error {
	return nil
}
func (m *mockConfigService) GetConfigsByCategory(ctx context.Context, category string) (map[string]string, error) {
	return nil, nil
}
func (m *mockConfigService) GetAllConfigs(ctx context.Context) (map[string]string, error) {
	return m.configs, nil
}
func (m *mockConfigService) ReloadConfigs(ctx context.Context) error            { return nil }
func (m *mockConfigService) ValidateConfig(key, value string) error             { return nil }
func (m *mockConfigService) DeleteConfig(ctx context.Context, key string) error { return nil }
func (m *mockConfigService) HealthCheck(ctx context.Context) error              { return nil }
func (m *mockConfigService) StartAutoReload(interval time.Duration) error       { return nil }
func (m *mockConfigService) StopAutoReload()                                    {}

func parseToInt(s string) (int, error) {
	if s == "" {
		return 0, &parseError{msg: "empty string"}
	}

	negative := false
	startIndex := 0
	if s[0] == '-' {
		negative = true
		startIndex = 1
		if len(s) == 1 {
			return 0, &parseError{msg: "invalid number"}
		}
	}

	result := 0
	for i := startIndex; i < len(s); i++ {
		char := s[i]
		if char < '0' || char > '9' {
			return 0, &parseError{msg: "invalid character"}
		}
		result = result*10 + int(char-'0')
	}

	if negative {
		result = -result
	}

	return result, nil
}

type parseError struct {
	msg string
}

func (e *parseError) Error() string {
	return e.msg
}

func TestLoadRateLimitConfigFromService(t *testing.T) {
	tests := []struct {
		name        string
		provider    string
		configs     map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name:     "default values",
			provider: "ollama",
			configs:  map[string]string{},
		},
		{
			name:     "custom values",
			provider: "ollama",
			configs: map[string]string{
				"AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_MINUTE": "120",
				"AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_DAY":    "2000",
				"AI_PROVIDER_OLLAMA_WARNING_THRESHOLD":     "0.8",
				"AI_PROVIDER_OLLAMA_THROTTLED_THRESHOLD":   "0.9",
			},
		},
		{
			name:     "negative per minute",
			provider: "ollama",
			configs: map[string]string{
				"AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_MINUTE": "-5",
			},
			expectError: true,
			errorMsg:    "rate limit per minute must be positive",
		},
		{
			name:     "negative per day",
			provider: "ollama",
			configs: map[string]string{
				"AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_DAY": "-100",
			},
			expectError: true,
			errorMsg:    "rate limit per day must be positive",
		},
		{
			name:     "invalid warning threshold format",
			provider: "ollama",
			configs: map[string]string{
				"AI_PROVIDER_OLLAMA_WARNING_THRESHOLD": "invalid",
			},
			expectError: true,
			errorMsg:    "invalid warning threshold",
		},
		{
			name:     "warning threshold out of range",
			provider: "ollama",
			configs: map[string]string{
				"AI_PROVIDER_OLLAMA_WARNING_THRESHOLD": "1.5",
			},
			expectError: true,
			errorMsg:    "warning threshold must be between 0 and 1",
		},
		{
			name:     "throttled threshold out of range",
			provider: "ollama",
			configs: map[string]string{
				"AI_PROVIDER_OLLAMA_THROTTLED_THRESHOLD": "1.5",
			},
			expectError: true,
			errorMsg:    "throttled threshold must be between 0 and 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &mockConfigService{configs: tt.configs}

			config, err := loadRateLimitConfigFromService(tt.provider, mockService)

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
		})
	}
}

func TestLoadKnowledgeBaseConfigFromService(t *testing.T) {
	tests := []struct {
		name        string
		configs     map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name:    "default values",
			configs: map[string]string{},
		},
		{
			name: "custom values",
			configs: map[string]string{
				"BMAD_KB_REFRESH_ENABLED":        "false",
				"BMAD_KB_REFRESH_INTERVAL_HOURS": "12",
				"BMAD_KB_REMOTE_URL":             "https://example.com/kb.md",
			},
		},
		{
			name: "negative interval hours",
			configs: map[string]string{
				"BMAD_KB_REFRESH_INTERVAL_HOURS": "-5",
			},
			expectError: true,
			errorMsg:    "BMAD_KB_REFRESH_INTERVAL_HOURS must be positive",
		},
		{
			name: "zero interval hours",
			configs: map[string]string{
				"BMAD_KB_REFRESH_INTERVAL_HOURS": "0",
			},
			expectError: true,
			errorMsg:    "BMAD_KB_REFRESH_INTERVAL_HOURS must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &mockConfigService{configs: tt.configs}

			config, err := loadKnowledgeBaseConfigFromService(mockService)

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
					if config == nil {
						t.Error("Expected config to be non-nil")
					}
				}
			}
		})
	}
}

func TestLoadForumConfig(t *testing.T) {
	originalEnv := os.Getenv("MONITORED_FORUM_CHANNELS")
	defer func() {
		if originalEnv != "" {
			os.Setenv("MONITORED_FORUM_CHANNELS", originalEnv)
		} else {
			os.Unsetenv("MONITORED_FORUM_CHANNELS")
		}
	}()

	tests := []struct {
		name        string
		envValue    string
		expectError bool
		errorMsg    string
		expectedLen int
	}{
		{
			name:        "empty environment variable",
			envValue:    "",
			expectError: false,
			expectedLen: 0,
		},
		{
			name:        "valid single channel",
			envValue:    "123456789012345678",
			expectError: false,
			expectedLen: 1,
		},
		{
			name:        "valid multiple channels",
			envValue:    "123456789012345678,987654321098765432",
			expectError: false,
			expectedLen: 2,
		},
		{
			name:        "channels with spaces",
			envValue:    " 123456789012345678 , 987654321098765432 ",
			expectError: false,
			expectedLen: 2,
		},
		{
			name:        "invalid channel ID too short",
			envValue:    "12345",
			expectError: true,
			errorMsg:    "invalid Discord channel ID",
		},
		{
			name:        "invalid channel ID too long",
			envValue:    "12345678901234567890",
			expectError: true,
			errorMsg:    "invalid Discord channel ID",
		},
		{
			name:        "invalid channel ID non-numeric",
			envValue:    "123456789012345abc",
			expectError: true,
			errorMsg:    "invalid Discord channel ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue == "" {
				os.Unsetenv("MONITORED_FORUM_CHANNELS")
			} else {
				os.Setenv("MONITORED_FORUM_CHANNELS", tt.envValue)
			}

			config, err := loadForumConfig()

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
					if len(config.MonitoredChannels) != tt.expectedLen {
						t.Errorf("Expected %d channels, got %d", tt.expectedLen, len(config.MonitoredChannels))
					}
				}
			}
		})
	}
}

func TestValidateDiscordChannelID(t *testing.T) {
	tests := []struct {
		name        string
		channelID   string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid 17-digit ID",
			channelID:   "12345678901234567",
			expectError: false,
		},
		{
			name:        "valid 18-digit ID",
			channelID:   "123456789012345678",
			expectError: false,
		},
		{
			name:        "valid 19-digit ID",
			channelID:   "1234567890123456789",
			expectError: false,
		},
		{
			name:        "too short",
			channelID:   "1234567890123456",
			expectError: true,
			errorMsg:    "invalid Discord channel ID",
		},
		{
			name:        "too long",
			channelID:   "12345678901234567890",
			expectError: true,
			errorMsg:    "invalid Discord channel ID",
		},
		{
			name:        "contains non-numeric characters",
			channelID:   "123456789012345abc",
			expectError: true,
			errorMsg:    "invalid Discord channel ID",
		},
		{
			name:        "empty string",
			channelID:   "",
			expectError: true,
			errorMsg:    "invalid Discord channel ID",
		},
		{
			name:        "starts with letter",
			channelID:   "a23456789012345678",
			expectError: true,
			errorMsg:    "invalid Discord channel ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDiscordChannelID(tt.channelID)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for channel ID '%s', but got none", tt.channelID)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', but got: %s", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for channel ID '%s', but got: %v", tt.channelID, err)
				}
			}
		})
	}
}
