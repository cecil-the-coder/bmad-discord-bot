package config

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"bmad-knowledge-bot/internal/storage"
)

// DatabaseConfigService implements ConfigService using a database backend
type DatabaseConfigService struct {
	storageService storage.StorageService
	cache          map[string]*storage.Configuration
	cacheMutex     sync.RWMutex
	listeners      []ConfigChangeListener
	listenerMutex  sync.RWMutex
	autoReloadStop chan struct{}
	autoReloadDone chan struct{}
}

// NewDatabaseConfigService creates a new database-backed configuration service
func NewDatabaseConfigService(storageService storage.StorageService) *DatabaseConfigService {
	return &DatabaseConfigService{
		storageService: storageService,
		cache:          make(map[string]*storage.Configuration),
		listeners:      make([]ConfigChangeListener, 0),
	}
}

// Initialize sets up the configuration service and loads initial configuration
func (s *DatabaseConfigService) Initialize(ctx context.Context) error {
	// Load all configurations into cache
	return s.ReloadConfigs(ctx)
}

// Close shuts down the configuration service and releases resources
func (s *DatabaseConfigService) Close() error {
	// Stop auto-reload if running
	s.StopAutoReload()

	// Clear cache
	s.cacheMutex.Lock()
	s.cache = make(map[string]*storage.Configuration)
	s.cacheMutex.Unlock()

	return nil
}

// ReloadConfigs refreshes configuration from the database
func (s *DatabaseConfigService) ReloadConfigs(ctx context.Context) error {
	configs, err := s.storageService.GetAllConfigurations(ctx)
	if err != nil {
		return NewConfigError("", "failed to load configurations from database", err)
	}

	s.cacheMutex.Lock()
	oldCache := s.cache
	s.cache = make(map[string]*storage.Configuration)

	// Update cache with new configurations
	for _, config := range configs {
		s.cache[config.Key] = config
	}
	s.cacheMutex.Unlock()

	// Notify listeners of changes
	s.notifyConfigChanges(oldCache, s.cache)

	return nil
}

// notifyConfigChanges compares old and new cache and notifies listeners of changes
func (s *DatabaseConfigService) notifyConfigChanges(oldCache, newCache map[string]*storage.Configuration) {
	s.listenerMutex.RLock()
	defer s.listenerMutex.RUnlock()

	if len(s.listeners) == 0 {
		return
	}

	// Check for changed or new values
	for key, newConfig := range newCache {
		oldConfig, existed := oldCache[key]
		oldValue := ""
		if existed {
			oldValue = oldConfig.Value
		}

		if !existed || oldConfig.Value != newConfig.Value {
			for _, listener := range s.listeners {
				listener.OnConfigChanged(key, oldValue, newConfig.Value)
			}
		}
	}

	// Check for deleted values
	for key, oldConfig := range oldCache {
		if _, exists := newCache[key]; !exists {
			for _, listener := range s.listeners {
				listener.OnConfigChanged(key, oldConfig.Value, "")
			}
		}
	}
}

// GetConfig retrieves a configuration value by key, returns error if not found
func (s *DatabaseConfigService) GetConfig(ctx context.Context, key string) (string, error) {
	s.cacheMutex.RLock()
	config, exists := s.cache[key]
	s.cacheMutex.RUnlock()

	if !exists {
		return "", NewConfigError(key, "configuration not found", nil)
	}

	return config.Value, nil
}

// GetConfigWithDefault retrieves a configuration value by key with fallback to default
func (s *DatabaseConfigService) GetConfigWithDefault(ctx context.Context, key, defaultValue string) string {
	value, err := s.GetConfig(ctx, key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetConfigInt retrieves a configuration value as integer
func (s *DatabaseConfigService) GetConfigInt(ctx context.Context, key string) (int, error) {
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
func (s *DatabaseConfigService) GetConfigIntWithDefault(ctx context.Context, key string, defaultValue int) int {
	value, err := s.GetConfigInt(ctx, key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetConfigBool retrieves a configuration value as boolean
func (s *DatabaseConfigService) GetConfigBool(ctx context.Context, key string) (bool, error) {
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
func (s *DatabaseConfigService) GetConfigBoolWithDefault(ctx context.Context, key string, defaultValue bool) bool {
	value, err := s.GetConfigBool(ctx, key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetConfigDuration retrieves a configuration value as time.Duration
func (s *DatabaseConfigService) GetConfigDuration(ctx context.Context, key string) (time.Duration, error) {
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
func (s *DatabaseConfigService) GetConfigDurationWithDefault(ctx context.Context, key string, defaultValue time.Duration) time.Duration {
	value, err := s.GetConfigDuration(ctx, key)
	if err != nil {
		return defaultValue
	}
	return value
}

// SetConfig creates or updates a configuration value
func (s *DatabaseConfigService) SetConfig(ctx context.Context, key, value, category, description string) error {
	return s.SetConfigTyped(ctx, key, value, string(ValueTypeString), category, description)
}

// SetConfigTyped creates or updates a configuration value with explicit type
func (s *DatabaseConfigService) SetConfigTyped(ctx context.Context, key, value, valueType, category, description string) error {
	// Validate the configuration
	if err := s.ValidateConfig(key, value); err != nil {
		return err
	}

	// Validate the value type
	if err := s.validateValueType(valueType, value); err != nil {
		return NewConfigError(key, "invalid value for type", err)
	}

	// Create configuration object
	config := &storage.Configuration{
		Key:         key,
		Value:       value,
		Type:        valueType,
		Category:    category,
		Description: description,
	}

	// Store in database
	if err := s.storageService.UpsertConfiguration(ctx, config); err != nil {
		return NewConfigError(key, "failed to store configuration", err)
	}

	// Update cache
	s.cacheMutex.Lock()
	oldValue := ""
	if oldConfig, exists := s.cache[key]; exists {
		oldValue = oldConfig.Value
	}
	s.cache[key] = config
	s.cacheMutex.Unlock()

	// Notify listeners if value changed
	if oldValue != value {
		s.listenerMutex.RLock()
		for _, listener := range s.listeners {
			listener.OnConfigChanged(key, oldValue, value)
		}
		s.listenerMutex.RUnlock()
	}

	return nil
}

// validateValueType validates that a value matches the specified type
func (s *DatabaseConfigService) validateValueType(valueType, value string) error {
	switch ValueType(valueType) {
	case ValueTypeString:
		return nil // Strings are always valid
	case ValueTypeInt:
		_, err := strconv.Atoi(value)
		return err
	case ValueTypeBool:
		switch strings.ToLower(value) {
		case "true", "false", "1", "0", "yes", "no", "on", "off", "enabled", "disabled":
			return nil
		default:
			return fmt.Errorf("invalid boolean value: %s", value)
		}
	case ValueTypeDuration:
		_, err := time.ParseDuration(value)
		return err
	default:
		return fmt.Errorf("unsupported value type: %s", valueType)
	}
}

// GetConfigsByCategory retrieves all configurations in a specific category
func (s *DatabaseConfigService) GetConfigsByCategory(ctx context.Context, category string) (map[string]string, error) {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	result := make(map[string]string)
	for key, config := range s.cache {
		if config.Category == category {
			result[key] = config.Value
		}
	}

	return result, nil
}

// GetAllConfigs retrieves all configurations as a key-value map
func (s *DatabaseConfigService) GetAllConfigs(ctx context.Context) (map[string]string, error) {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	result := make(map[string]string)
	for key, config := range s.cache {
		result[key] = config.Value
	}

	return result, nil
}

// ValidateConfig validates a configuration key-value pair before storing
func (s *DatabaseConfigService) ValidateConfig(key, value string) error {
	// Basic validation rules
	if key == "" {
		return NewConfigError(key, "configuration key cannot be empty", nil)
	}

	if len(key) > 255 {
		return NewConfigError(key, "configuration key too long (max 255 characters)", nil)
	}

	if len(value) > 65535 {
		return NewConfigError(key, "configuration value too long (max 65535 characters)", nil)
	}

	// Additional validation based on key patterns
	if strings.HasSuffix(key, "_RATE_LIMIT_PER_MINUTE") || strings.HasSuffix(key, "_RATE_LIMIT_PER_DAY") {
		if _, err := strconv.Atoi(value); err != nil {
			return NewConfigError(key, "rate limit values must be integers", err)
		}
	}

	if strings.HasSuffix(key, "_ENABLED") {
		switch strings.ToLower(value) {
		case "true", "false", "1", "0", "yes", "no", "on", "off", "enabled", "disabled":
			// Valid boolean values
		default:
			return NewConfigError(key, "boolean configuration values must be true/false, 1/0, yes/no, on/off, or enabled/disabled", nil)
		}
	}

	return nil
}

// DeleteConfig removes a configuration entry
func (s *DatabaseConfigService) DeleteConfig(ctx context.Context, key string) error {
	// Remove from database
	if err := s.storageService.DeleteConfiguration(ctx, key); err != nil {
		return NewConfigError(key, "failed to delete configuration", err)
	}

	// Remove from cache
	s.cacheMutex.Lock()
	oldValue := ""
	if oldConfig, exists := s.cache[key]; exists {
		oldValue = oldConfig.Value
		delete(s.cache, key)
	}
	s.cacheMutex.Unlock()

	// Notify listeners
	if oldValue != "" {
		s.listenerMutex.RLock()
		for _, listener := range s.listeners {
			listener.OnConfigChanged(key, oldValue, "")
		}
		s.listenerMutex.RUnlock()
	}

	return nil
}

// HealthCheck verifies that the configuration service is working properly
func (s *DatabaseConfigService) HealthCheck(ctx context.Context) error {
	// Check storage service health
	if err := s.storageService.HealthCheck(ctx); err != nil {
		return NewConfigError("", "storage service health check failed", err)
	}

	// Verify cache is populated
	s.cacheMutex.RLock()
	cacheSize := len(s.cache)
	s.cacheMutex.RUnlock()

	if cacheSize == 0 {
		// Empty cache might be normal for a new installation
		// Try to reload to verify database connectivity
		if err := s.ReloadConfigs(ctx); err != nil {
			return NewConfigError("", "failed to reload configurations", err)
		}
	}

	return nil
}

// StartAutoReload begins automatic configuration reloading at specified interval
func (s *DatabaseConfigService) StartAutoReload(interval time.Duration) error {
	if s.autoReloadStop != nil {
		return fmt.Errorf("auto-reload already running")
	}

	s.autoReloadStop = make(chan struct{})
	s.autoReloadDone = make(chan struct{})

	go func() {
		defer close(s.autoReloadDone)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := s.ReloadConfigs(ctx); err != nil {
					// Log error but continue
					_ = fmt.Sprintf("Auto-reload configuration error: %v", err)
				}
				cancel()
			case <-s.autoReloadStop:
				return
			}
		}
	}()

	return nil
}

// StopAutoReload stops automatic configuration reloading
func (s *DatabaseConfigService) StopAutoReload() {
	if s.autoReloadStop != nil {
		close(s.autoReloadStop)
		<-s.autoReloadDone
		s.autoReloadStop = nil
		s.autoReloadDone = nil
	}
}

// AddConfigChangeListener adds a listener for configuration changes
func (s *DatabaseConfigService) AddConfigChangeListener(listener ConfigChangeListener) {
	s.listenerMutex.Lock()
	defer s.listenerMutex.Unlock()
	s.listeners = append(s.listeners, listener)
}

// RemoveConfigChangeListener removes a configuration change listener
func (s *DatabaseConfigService) RemoveConfigChangeListener(listener ConfigChangeListener) {
	s.listenerMutex.Lock()
	defer s.listenerMutex.Unlock()

	for i, l := range s.listeners {
		if l == listener {
			s.listeners = append(s.listeners[:i], s.listeners[i+1:]...)
			break
		}
	}
}
