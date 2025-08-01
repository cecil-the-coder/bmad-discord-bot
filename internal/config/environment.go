package config

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"bmad-knowledge-bot/internal/storage"
)

// SecureConfigKeys defines configuration keys that must remain in environment variables for security reasons
var SecureConfigKeys = map[string]bool{
	"BOT_TOKEN":       true,
	"MYSQL_USERNAME":  true,
	"MYSQL_PASSWORD":  true,
	"MYSQL_HOST":      true,
	"MYSQL_PORT":      true,
	"MYSQL_DATABASE":  true,
	"MYSQL_TIMEOUT":   true,
	"GEMINI_CLI_PATH": true,
	"DATABASE_TYPE":   true, // Controls whether to use SQLite or MySQL
}

// HybridConfigService implements ConfigService with database-first loading and environment variable fallback
type HybridConfigService struct {
	databaseService *DatabaseConfigService
	databaseWorking bool
	envFallback     bool
	initialized     bool
}

// NewHybridConfigService creates a new hybrid configuration service with database-first loading and environment fallback
func NewHybridConfigService(storageService storage.StorageService) *HybridConfigService {
	return &HybridConfigService{
		databaseService: NewDatabaseConfigService(storageService),
		envFallback:     true,
	}
}

// Initialize sets up the configuration service and loads initial configuration
func (s *HybridConfigService) Initialize(ctx context.Context) error {
	// Try to initialize database service
	err := s.databaseService.Initialize(ctx)
	if err != nil {
		// Database initialization failed, fall back to environment variables only
		s.databaseWorking = false
		s.envFallback = true
		s.initialized = true
		return nil // Don't fail startup due to database issues
	}

	s.databaseWorking = true
	s.initialized = true
	return nil
}

// Close shuts down the configuration service and releases resources
func (s *HybridConfigService) Close() error {
	if s.databaseService != nil {
		return s.databaseService.Close()
	}
	return nil
}

// GetConfig retrieves a configuration value by key, returns error if not found
func (s *HybridConfigService) GetConfig(ctx context.Context, key string) (string, error) {
	// For secure keys, always use environment variables
	if SecureConfigKeys[key] {
		value := os.Getenv(key)
		if value == "" {
			return "", NewConfigError(key, "secure configuration not found in environment variables", nil)
		}
		return value, nil
	}

	// Try database first if available
	if s.initialized && s.databaseWorking && s.databaseService != nil {
		value, err := s.databaseService.GetConfig(ctx, key)
		if err == nil {
			return value, nil
		}
	}

	// Fall back to environment variables
	if s.envFallback {
		value := os.Getenv(key)
		if value != "" {
			return value, nil
		}
	}

	return "", NewConfigError(key, "configuration not found", nil)
}

// GetConfigWithDefault retrieves a configuration value by key with fallback to default
func (s *HybridConfigService) GetConfigWithDefault(ctx context.Context, key, defaultValue string) string {
	value, err := s.GetConfig(ctx, key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetConfigInt retrieves a configuration value as integer
func (s *HybridConfigService) GetConfigInt(ctx context.Context, key string) (int, error) {
	value, err := s.GetConfig(ctx, key)
	if err != nil {
		return 0, err
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		return 0, NewConfigError(key, "invalid integer value", err)
	}

	return intValue, nil
}

// GetConfigIntWithDefault retrieves a configuration value as integer with default
func (s *HybridConfigService) GetConfigIntWithDefault(ctx context.Context, key string, defaultValue int) int {
	value, err := s.GetConfigInt(ctx, key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetConfigBool retrieves a configuration value as boolean
func (s *HybridConfigService) GetConfigBool(ctx context.Context, key string) (bool, error) {
	value, err := s.GetConfig(ctx, key)
	if err != nil {
		return false, err
	}

	// Parse boolean values flexibly
	switch strings.ToLower(value) {
	case "true", "1", "yes", "on", "enabled":
		return true, nil
	case "false", "0", "no", "off", "disabled":
		return false, nil
	default:
		return false, NewConfigError(key, "invalid boolean value", nil)
	}
}

// GetConfigBoolWithDefault retrieves a configuration value as boolean with default
func (s *HybridConfigService) GetConfigBoolWithDefault(ctx context.Context, key string, defaultValue bool) bool {
	value, err := s.GetConfigBool(ctx, key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetConfigDuration retrieves a configuration value as time.Duration
func (s *HybridConfigService) GetConfigDuration(ctx context.Context, key string) (time.Duration, error) {
	value, err := s.GetConfig(ctx, key)
	if err != nil {
		return 0, err
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, NewConfigError(key, "invalid duration value", err)
	}

	return duration, nil
}

// GetConfigDurationWithDefault retrieves a configuration value as duration with default
func (s *HybridConfigService) GetConfigDurationWithDefault(ctx context.Context, key string, defaultValue time.Duration) time.Duration {
	value, err := s.GetConfigDuration(ctx, key)
	if err != nil {
		return defaultValue
	}
	return value
}

// SetConfig creates or updates a configuration value (only for non-secure keys)
func (s *HybridConfigService) SetConfig(ctx context.Context, key, value, category, description string) error {
	// Prevent setting secure configuration keys
	if SecureConfigKeys[key] {
		return NewConfigError(key, "secure configuration keys cannot be modified through configuration service", nil)
	}

	// Only allow setting if database service is available
	if !s.initialized || !s.databaseWorking || s.databaseService == nil {
		return NewConfigError(key, "database configuration service not available", nil)
	}

	return s.databaseService.SetConfig(ctx, key, value, category, description)
}

// SetConfigTyped creates or updates a configuration value with explicit type (only for non-secure keys)
func (s *HybridConfigService) SetConfigTyped(ctx context.Context, key, value, valueType, category, description string) error {
	// Prevent setting secure configuration keys
	if SecureConfigKeys[key] {
		return NewConfigError(key, "secure configuration keys cannot be modified through configuration service", nil)
	}

	// Only allow setting if database service is available
	if !s.initialized || !s.databaseWorking || s.databaseService == nil {
		return NewConfigError(key, "database configuration service not available", nil)
	}

	return s.databaseService.SetConfigTyped(ctx, key, value, valueType, category, description)
}

// GetConfigsByCategory retrieves all configurations in a specific category
func (s *HybridConfigService) GetConfigsByCategory(ctx context.Context, category string) (map[string]string, error) {
	result := make(map[string]string)

	// Get database configurations if available
	if s.initialized && s.databaseWorking && s.databaseService != nil {
		dbConfigs, err := s.databaseService.GetConfigsByCategory(ctx, category)
		if err == nil {
			for k, v := range dbConfigs {
				result[k] = v
			}
		}
	}

	// Add environment variable configurations for the same category
	// This is a best-effort approach since we can't easily categorize env vars
	if s.envFallback {
		for _, env := range os.Environ() {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				key := parts[0]
				value := parts[1]

				// Include if it matches common category patterns
				if s.matchesCategory(key, category) {
					// Don't override database values
					if _, exists := result[key]; !exists {
						result[key] = value
					}
				}
			}
		}
	}

	return result, nil
}

// matchesCategory checks if an environment variable key matches a category pattern
func (s *HybridConfigService) matchesCategory(key, category string) bool {
	switch category {
	case "rate_limiting":
		return strings.Contains(key, "RATE_LIMIT")
	case "features":
		return strings.HasSuffix(key, "_ENABLED")
	case "ai_services":
		return strings.HasPrefix(key, "OLLAMA_") || strings.HasPrefix(key, "GEMINI_") || strings.Contains(key, "AI_PROVIDER")
	case "system":
		return strings.Contains(key, "TIMEOUT") || strings.Contains(key, "INTERVAL")
	case "monitoring":
		return strings.Contains(key, "STATUS") || strings.Contains(key, "MONITOR")
	default:
		return false
	}
}

// GetAllConfigs retrieves all configurations as a key-value map
func (s *HybridConfigService) GetAllConfigs(ctx context.Context) (map[string]string, error) {
	result := make(map[string]string)

	// Get database configurations if available
	if s.initialized && s.databaseWorking && s.databaseService != nil {
		dbConfigs, err := s.databaseService.GetAllConfigs(ctx)
		if err == nil {
			for k, v := range dbConfigs {
				result[k] = v
			}
		}
	}

	// Add environment variables (excluding secure ones for security)
	if s.envFallback {
		for _, env := range os.Environ() {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				key := parts[0]
				value := parts[1]

				// Skip secure configuration keys when returning all configs
				if !SecureConfigKeys[key] {
					// Don't override database values
					if _, exists := result[key]; !exists {
						result[key] = value
					}
				}
			}
		}
	}

	return result, nil
}

// ReloadConfigs refreshes configuration from the database
func (s *HybridConfigService) ReloadConfigs(ctx context.Context) error {
	if s.initialized && s.databaseWorking && s.databaseService != nil {
		return s.databaseService.ReloadConfigs(ctx)
	}
	return nil // No-op if database service not available
}

// ValidateConfig validates a configuration key-value pair before storing
func (s *HybridConfigService) ValidateConfig(key, value string) error {
	// Prevent validation of secure configuration keys
	if SecureConfigKeys[key] {
		return NewConfigError(key, "secure configuration keys cannot be validated through configuration service", nil)
	}

	if s.initialized && s.databaseWorking && s.databaseService != nil {
		return s.databaseService.ValidateConfig(key, value)
	}

	// Basic validation if database service not available
	if key == "" {
		return NewConfigError(key, "configuration key cannot be empty", nil)
	}

	return nil
}

// DeleteConfig removes a configuration entry (only for non-secure keys)
func (s *HybridConfigService) DeleteConfig(ctx context.Context, key string) error {
	// Prevent deletion of secure configuration keys
	if SecureConfigKeys[key] {
		return NewConfigError(key, "secure configuration keys cannot be deleted through configuration service", nil)
	}

	// Only allow deletion if database service is available
	if !s.initialized || !s.databaseWorking || s.databaseService == nil {
		return NewConfigError(key, "database configuration service not available", nil)
	}

	return s.databaseService.DeleteConfig(ctx, key)
}

// HealthCheck verifies that the configuration service is working properly
func (s *HybridConfigService) HealthCheck(ctx context.Context) error {
	// Check if we can access environment variables
	testEnvVar := os.Getenv("PATH")
	if testEnvVar == "" {
		return NewConfigError("", "environment variable access failed", nil)
	}

	// Check database service if available
	if s.initialized && s.databaseWorking && s.databaseService != nil {
		if err := s.databaseService.HealthCheck(ctx); err != nil {
			// Database health check failed, but we can still fall back to env vars
			// This is not a fatal error for the hybrid service
			return nil
		}
	}

	return nil
}

// StartAutoReload begins automatic configuration reloading at specified interval
func (s *HybridConfigService) StartAutoReload(interval time.Duration) error {
	if s.initialized && s.databaseWorking && s.databaseService != nil {
		return s.databaseService.StartAutoReload(interval)
	}
	return nil // No-op if database service not available
}

// StopAutoReload stops automatic configuration reloading
func (s *HybridConfigService) StopAutoReload() {
	if s.initialized && s.databaseWorking && s.databaseService != nil {
		s.databaseService.StopAutoReload()
	}
}

// AddConfigChangeListener adds a listener for configuration changes
func (s *HybridConfigService) AddConfigChangeListener(listener ConfigChangeListener) {
	if s.initialized && s.databaseWorking && s.databaseService != nil {
		s.databaseService.AddConfigChangeListener(listener)
	}
}

// RemoveConfigChangeListener removes a configuration change listener
func (s *HybridConfigService) RemoveConfigChangeListener(listener ConfigChangeListener) {
	if s.initialized && s.databaseWorking && s.databaseService != nil {
		s.databaseService.RemoveConfigChangeListener(listener)
	}
}

// IsDatabaseAvailable returns true if the database configuration service is available and working
func (s *HybridConfigService) IsDatabaseAvailable() bool {
	return s.initialized && s.databaseWorking && s.databaseService != nil
}

// SetEnvironmentFallback enables or disables environment variable fallback
func (s *HybridConfigService) SetEnvironmentFallback(enabled bool) {
	s.envFallback = enabled
}
