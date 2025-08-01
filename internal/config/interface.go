package config

import (
	"context"
	"time"
)

// ConfigService defines the interface for configuration management
type ConfigService interface {
	// Initialize sets up the configuration service and loads initial configuration
	Initialize(ctx context.Context) error

	// Close shuts down the configuration service and releases resources
	Close() error

	// GetConfig retrieves a configuration value by key, returns error if not found
	GetConfig(ctx context.Context, key string) (string, error)

	// GetConfigWithDefault retrieves a configuration value by key with fallback to default
	GetConfigWithDefault(ctx context.Context, key, defaultValue string) string

	// GetConfigInt retrieves a configuration value as integer
	GetConfigInt(ctx context.Context, key string) (int, error)

	// GetConfigIntWithDefault retrieves a configuration value as integer with default
	GetConfigIntWithDefault(ctx context.Context, key string, defaultValue int) int

	// GetConfigBool retrieves a configuration value as boolean
	GetConfigBool(ctx context.Context, key string) (bool, error)

	// GetConfigBoolWithDefault retrieves a configuration value as boolean with default
	GetConfigBoolWithDefault(ctx context.Context, key string, defaultValue bool) bool

	// GetConfigDuration retrieves a configuration value as time.Duration
	GetConfigDuration(ctx context.Context, key string) (time.Duration, error)

	// GetConfigDurationWithDefault retrieves a configuration value as duration with default
	GetConfigDurationWithDefault(ctx context.Context, key string, defaultValue time.Duration) time.Duration

	// SetConfig creates or updates a configuration value
	SetConfig(ctx context.Context, key, value, category, description string) error

	// SetConfigTyped creates or updates a configuration value with explicit type
	SetConfigTyped(ctx context.Context, key, value, valueType, category, description string) error

	// GetConfigsByCategory retrieves all configurations in a specific category
	GetConfigsByCategory(ctx context.Context, category string) (map[string]string, error)

	// GetAllConfigs retrieves all configurations as a key-value map
	GetAllConfigs(ctx context.Context) (map[string]string, error)

	// ReloadConfigs refreshes configuration from the database
	ReloadConfigs(ctx context.Context) error

	// ValidateConfig validates a configuration key-value pair before storing
	ValidateConfig(key, value string) error

	// DeleteConfig removes a configuration entry
	DeleteConfig(ctx context.Context, key string) error

	// HealthCheck verifies that the configuration service is working properly
	HealthCheck(ctx context.Context) error

	// StartAutoReload begins automatic configuration reloading at specified interval
	StartAutoReload(interval time.Duration) error

	// StopAutoReload stops automatic configuration reloading
	StopAutoReload()
}

// ValueType defines the supported configuration value types
type ValueType string

const (
	ValueTypeString   ValueType = "string"
	ValueTypeInt      ValueType = "int"
	ValueTypeBool     ValueType = "bool"
	ValueTypeDuration ValueType = "duration"
)

// ConfigChangeListener defines the interface for configuration change notifications
type ConfigChangeListener interface {
	OnConfigChanged(key, oldValue, newValue string)
}

// ConfigError represents configuration-related errors
type ConfigError struct {
	Key     string
	Message string
	Cause   error
}

func (e *ConfigError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *ConfigError) Unwrap() error {
	return e.Cause
}

// NewConfigError creates a new configuration error
func NewConfigError(key, message string, cause error) *ConfigError {
	return &ConfigError{
		Key:     key,
		Message: message,
		Cause:   cause,
	}
}
