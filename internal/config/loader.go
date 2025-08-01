package config

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// ServiceConfigListener defines services that need to be notified of configuration changes
type ServiceConfigListener struct {
	Name     string
	OnReload func(configs map[string]string) error
}

// ConfigurationLoader manages configuration loading and coordinates service notifications
type ConfigurationLoader struct {
	configService ConfigService
	listeners     []ServiceConfigListener
	listenerMutex sync.RWMutex
}

// NewConfigurationLoader creates a new configuration loader
func NewConfigurationLoader(configService ConfigService) *ConfigurationLoader {
	return &ConfigurationLoader{
		configService: configService,
		listeners:     make([]ServiceConfigListener, 0),
	}
}

// Initialize sets up the configuration loader
func (l *ConfigurationLoader) Initialize(ctx context.Context) error {
	return l.configService.Initialize(ctx)
}

// RegisterServiceListener registers a service to receive configuration change notifications
func (l *ConfigurationLoader) RegisterServiceListener(listener ServiceConfigListener) {
	l.listenerMutex.Lock()
	defer l.listenerMutex.Unlock()
	l.listeners = append(l.listeners, listener)
}

// UnregisterServiceListener removes a service listener
func (l *ConfigurationLoader) UnregisterServiceListener(name string) {
	l.listenerMutex.Lock()
	defer l.listenerMutex.Unlock()

	for i, listener := range l.listeners {
		if listener.Name == name {
			l.listeners = append(l.listeners[:i], l.listeners[i+1:]...)
			break
		}
	}
}

// ReloadAndNotifyServices reloads configuration and notifies all registered services
func (l *ConfigurationLoader) ReloadAndNotifyServices(ctx context.Context) error {
	// Reload configuration from database
	if err := l.configService.ReloadConfigs(ctx); err != nil {
		return fmt.Errorf("failed to reload configurations: %w", err)
	}

	// Get all configurations
	allConfigs, err := l.configService.GetAllConfigs(ctx)
	if err != nil {
		return fmt.Errorf("failed to get all configurations: %w", err)
	}

	// Notify all services
	l.listenerMutex.RLock()
	defer l.listenerMutex.RUnlock()

	for _, listener := range l.listeners {
		if err := listener.OnReload(allConfigs); err != nil {
			// Log error but continue with other services
			_ = fmt.Sprintf("Service %s configuration reload failed: %v", listener.Name, err)
		}
	}

	return nil
}

// GetConfigService returns the underlying configuration service
func (l *ConfigurationLoader) GetConfigService() ConfigService {
	return l.configService
}

// Close shuts down the configuration loader
func (l *ConfigurationLoader) Close() error {
	return l.configService.Close()
}

// StartAutoReloadWithServiceNotification starts auto-reload with service notifications
func (l *ConfigurationLoader) StartAutoReloadWithServiceNotification(interval time.Duration) error {
	// Add a configuration change listener that notifies services
	if hybridService, ok := l.configService.(*HybridConfigService); ok {
		changeListener := &serviceNotificationListener{loader: l}
		hybridService.AddConfigChangeListener(changeListener)
	}

	return l.configService.StartAutoReload(interval)
}

// serviceNotificationListener implements ConfigChangeListener to notify services of changes
type serviceNotificationListener struct {
	loader *ConfigurationLoader
}

func (s *serviceNotificationListener) OnConfigChanged(key, oldValue, newValue string) {
	// Create a background context for service notifications
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get all current configurations
	allConfigs, err := s.loader.configService.GetAllConfigs(ctx)
	if err != nil {
		// Log error but don't fail
		_ = fmt.Sprintf("Failed to get all configurations for service notification: %v", err)
		return
	}

	// Notify all services
	s.loader.listenerMutex.RLock()
	defer s.loader.listenerMutex.RUnlock()

	for _, listener := range s.loader.listeners {
		if err := listener.OnReload(allConfigs); err != nil {
			// Log error but continue with other services
			_ = fmt.Sprintf("Service %s configuration change notification failed for key %s: %v", listener.Name, key, err)
		}
	}
}

// ConfigurationMigrator handles migrating environment variables to database
type ConfigurationMigrator struct {
	configService ConfigService
}

// NewConfigurationMigrator creates a new configuration migrator
func NewConfigurationMigrator(configService ConfigService) *ConfigurationMigrator {
	return &ConfigurationMigrator{
		configService: configService,
	}
}

// MigrateEnvironmentVariables migrates non-secure environment variables to database configuration
func (m *ConfigurationMigrator) MigrateEnvironmentVariables(ctx context.Context) error {
	// Define migration mappings from environment variables to database configuration
	migrationMappings := []struct {
		EnvKey      string
		Category    string
		Description string
		ValueType   string
	}{
		// Rate limiting configuration
		{"AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_MINUTE", "rate_limiting", "Ollama API rate limit per minute", "int"},
		{"AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_DAY", "rate_limiting", "Ollama API rate limit per day", "int"},

		// Feature flags
		{"BMAD_KB_REFRESH_ENABLED", "features", "Enable knowledge base refresh functionality", "bool"},
		{"REACTION_TRIGGER_ENABLED", "features", "Enable reaction trigger functionality", "bool"},
		{"BOT_STATUS_UPDATE_ENABLED", "features", "Enable bot status updates", "bool"},

		// AI service configuration
		{"OLLAMA_HOST", "ai_services", "Ollama service host address", "string"},
		{"OLLAMA_MODEL", "ai_services", "Default Ollama model to use", "string"},

		// System configuration
		{"BOT_STATUS_UPDATE_INTERVAL", "system", "Interval for bot status updates", "duration"},
		{"CONFIG_RELOAD_INTERVAL", "system", "Configuration reload interval", "duration"},
	}

	migratedCount := 0
	for _, mapping := range migrationMappings {
		// Skip secure configuration keys
		if SecureConfigKeys[mapping.EnvKey] {
			continue
		}

		// Check if configuration already exists in database
		_, err := m.configService.GetConfig(ctx, mapping.EnvKey)
		if err == nil {
			// Configuration already exists, skip migration
			continue
		}

		// Get value from environment variable
		envValue := GetEnvWithDefault(mapping.EnvKey, "")
		if envValue == "" {
			// Environment variable not set, skip
			continue
		}

		// Migrate to database
		err = m.configService.SetConfigTyped(ctx, mapping.EnvKey, envValue, mapping.ValueType, mapping.Category, mapping.Description)
		if err != nil {
			return fmt.Errorf("failed to migrate %s: %w", mapping.EnvKey, err)
		}

		migratedCount++
	}

	if migratedCount > 0 {
		_ = fmt.Sprintf("Migrated %d environment variables to database configuration", migratedCount)
	}

	return nil
}

// SeedDefaultConfigurations creates default configuration values if they don't exist
func (m *ConfigurationMigrator) SeedDefaultConfigurations(ctx context.Context) error {
	defaultConfigurations := []struct {
		Key         string
		Value       string
		ValueType   string
		Category    string
		Description string
	}{
		// Default rate limits
		{"AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_MINUTE", "60", "int", "rate_limiting", "Default Ollama API rate limit per minute"},
		{"AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_DAY", "2000", "int", "rate_limiting", "Default Ollama API rate limit per day"},

		// Default feature flags
		{"BMAD_KB_REFRESH_ENABLED", "true", "bool", "features", "Enable knowledge base refresh functionality by default"},
		{"REACTION_TRIGGER_ENABLED", "true", "bool", "features", "Enable reaction trigger functionality by default"},
		{"BOT_STATUS_UPDATE_ENABLED", "true", "bool", "features", "Enable bot status updates by default"},

		// Default AI service configuration
		{"OLLAMA_HOST", "http://localhost:11434", "string", "ai_services", "Default Ollama service host"},
		{"OLLAMA_MODEL", "llama2", "string", "ai_services", "Default Ollama model"},

		// Default system configuration
		{"BOT_STATUS_UPDATE_INTERVAL", "5m", "duration", "system", "Default bot status update interval"},
		{"CONFIG_RELOAD_INTERVAL", "1m", "duration", "system", "Default configuration reload interval"},
	}

	seededCount := 0
	for _, config := range defaultConfigurations {
		// Check if configuration already exists
		_, err := m.configService.GetConfig(ctx, config.Key)
		if err == nil {
			// Configuration already exists, skip seeding
			continue
		}

		// Create default configuration
		err = m.configService.SetConfigTyped(ctx, config.Key, config.Value, config.ValueType, config.Category, config.Description)
		if err != nil {
			return fmt.Errorf("failed to seed default configuration %s: %w", config.Key, err)
		}

		seededCount++
	}

	if seededCount > 0 {
		_ = fmt.Sprintf("Seeded %d default configurations", seededCount)
	}

	return nil
}

// GetEnvWithDefault gets an environment variable with a default value
func GetEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
