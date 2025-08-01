package config

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// MockConfigChangeListener implements ConfigChangeListener for testing
type MockConfigChangeListener struct {
	changes []ConfigChange
}

type ConfigChange struct {
	Key      string
	OldValue string
	NewValue string
}

func (m *MockConfigChangeListener) OnConfigChanged(key, oldValue, newValue string) {
	m.changes = append(m.changes, ConfigChange{
		Key:      key,
		OldValue: oldValue,
		NewValue: newValue,
	})
}

func (m *MockConfigChangeListener) GetChanges() []ConfigChange {
	return m.changes
}

func (m *MockConfigChangeListener) ClearChanges() {
	m.changes = nil
}

// MockServiceListener implements service configuration reload for testing
type MockServiceListener struct {
	reloadCount int
	lastConfigs map[string]string
	reloadError error
}

func (m *MockServiceListener) OnReload(configs map[string]string) error {
	m.reloadCount++
	m.lastConfigs = configs
	return m.reloadError
}

func (m *MockServiceListener) GetReloadCount() int {
	return m.reloadCount
}

func (m *MockServiceListener) GetLastConfigs() map[string]string {
	return m.lastConfigs
}

func (m *MockServiceListener) SetError(err error) {
	m.reloadError = err
}

func TestConfigurationLoader_Initialize(t *testing.T) {
	mockStorage := NewMockStorageService()
	configService := NewHybridConfigService(mockStorage)
	loader := NewConfigurationLoader(configService)

	err := loader.Initialize(context.Background())
	if err != nil {
		t.Errorf("Initialize failed: %v", err)
	}
}

func TestConfigurationLoader_RegisterServiceListener(t *testing.T) {
	mockStorage := NewMockStorageService()
	configService := NewHybridConfigService(mockStorage)
	loader := NewConfigurationLoader(configService)
	ctx := context.Background()

	// Initialize loader
	err := loader.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Create mock service listener
	mockService := &MockServiceListener{}
	serviceListener := ServiceConfigListener{
		Name:     "test_service",
		OnReload: mockService.OnReload,
	}

	// Register service listener
	loader.RegisterServiceListener(serviceListener)

	// Add configuration and trigger reload
	mockStorage.AddTestConfiguration("test_key", "test_value", "string", "test", "Test configuration")
	err = loader.ReloadAndNotifyServices(ctx)
	if err != nil {
		t.Errorf("ReloadAndNotifyServices failed: %v", err)
	}

	// Verify service was notified
	if mockService.GetReloadCount() != 1 {
		t.Errorf("Expected 1 reload notification, got %d", mockService.GetReloadCount())
	}

	configs := mockService.GetLastConfigs()
	if len(configs) == 0 {
		t.Error("Service should have received configurations")
	}

	if value, exists := configs["test_key"]; !exists || value != "test_value" {
		t.Errorf("Service should have received test configuration")
	}
}

func TestConfigurationLoader_UnregisterServiceListener(t *testing.T) {
	mockStorage := NewMockStorageService()
	configService := NewHybridConfigService(mockStorage)
	loader := NewConfigurationLoader(configService)
	ctx := context.Background()

	// Initialize loader
	err := loader.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Create and register mock service listener
	mockService := &MockServiceListener{}
	serviceListener := ServiceConfigListener{
		Name:     "test_service",
		OnReload: mockService.OnReload,
	}
	loader.RegisterServiceListener(serviceListener)

	// Trigger reload to verify registration
	err = loader.ReloadAndNotifyServices(ctx)
	if err != nil {
		t.Errorf("ReloadAndNotifyServices failed: %v", err)
	}
	if mockService.GetReloadCount() != 1 {
		t.Error("Service should have been notified after registration")
	}

	// Unregister service listener
	loader.UnregisterServiceListener("test_service")

	// Reset counter and trigger reload again
	mockService.reloadCount = 0
	err = loader.ReloadAndNotifyServices(ctx)
	if err != nil {
		t.Errorf("ReloadAndNotifyServices failed: %v", err)
	}

	// Verify service was not notified after unregistration
	if mockService.GetReloadCount() != 0 {
		t.Errorf("Service should not be notified after unregistration, got %d notifications", mockService.GetReloadCount())
	}
}

func TestConfigurationLoader_ReloadAndNotifyServices_WithError(t *testing.T) {
	mockStorage := NewMockStorageService()
	configService := NewHybridConfigService(mockStorage)
	loader := NewConfigurationLoader(configService)
	ctx := context.Background()

	// Initialize loader
	err := loader.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Create mock service listener that returns an error
	mockService := &MockServiceListener{}
	mockService.SetError(NewConfigError("", "service reload failed", nil))
	serviceListener := ServiceConfigListener{
		Name:     "failing_service",
		OnReload: mockService.OnReload,
	}
	loader.RegisterServiceListener(serviceListener)

	// Trigger reload - should not fail even if service returns error
	err = loader.ReloadAndNotifyServices(ctx)
	if err != nil {
		t.Errorf("ReloadAndNotifyServices should not fail due to service error: %v", err)
	}

	// Verify service was still called
	if mockService.GetReloadCount() != 1 {
		t.Errorf("Expected 1 reload attempt despite error, got %d", mockService.GetReloadCount())
	}
}

func TestConfigurationMigrator_MigrateEnvironmentVariables(t *testing.T) {
	mockStorage := NewMockStorageService()
	configService := NewHybridConfigService(mockStorage)
	migrator := NewConfigurationMigrator(configService)
	ctx := context.Background()

	// Initialize config service
	err := configService.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Set environment variables that should be migrated
	testEnvVars := map[string]string{
		"AI_PROVIDER_GEMINI_RATE_LIMIT_PER_MINUTE": "30",
		"BMAD_KB_REFRESH_ENABLED":                  "true",
		"OLLAMA_HOST":                              "http://localhost:11434",
		"BOT_STATUS_UPDATE_INTERVAL":               "30s",
	}

	for key, value := range testEnvVars {
		os.Setenv(key, value)
	}
	defer func() {
		for key := range testEnvVars {
			os.Unsetenv(key)
		}
	}()

	// Run migration
	err = migrator.MigrateEnvironmentVariables(ctx)
	if err != nil {
		t.Errorf("MigrateEnvironmentVariables failed: %v", err)
	}

	// Verify configurations were migrated
	for key, expectedValue := range testEnvVars {
		// Skip secure keys (they shouldn't be migrated)
		if SecureConfigKeys[key] {
			continue
		}

		value, err := configService.GetConfig(ctx, key)
		if err != nil {
			t.Errorf("Failed to get migrated configuration %s: %v", key, err)
		}
		if value != expectedValue {
			t.Errorf("Expected migrated value '%s' for key %s, got '%s'", expectedValue, key, value)
		}
	}
}

func TestConfigurationMigrator_MigrateEnvironmentVariables_SkipExisting(t *testing.T) {
	mockStorage := NewMockStorageService()
	configService := NewHybridConfigService(mockStorage)
	migrator := NewConfigurationMigrator(configService)
	ctx := context.Background()

	// Pre-populate database with existing configuration
	mockStorage.AddTestConfiguration("BMAD_KB_REFRESH_ENABLED", "false", "bool", "features", "Existing configuration")

	// Initialize config service
	err := configService.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Set environment variable with different value
	os.Setenv("BMAD_KB_REFRESH_ENABLED", "true")
	defer os.Unsetenv("BMAD_KB_REFRESH_ENABLED")

	// Run migration
	err = migrator.MigrateEnvironmentVariables(ctx)
	if err != nil {
		t.Errorf("MigrateEnvironmentVariables failed: %v", err)
	}

	// Verify existing database value was preserved
	value, err := configService.GetConfig(ctx, "BMAD_KB_REFRESH_ENABLED")
	if err != nil {
		t.Errorf("Failed to get configuration: %v", err)
	}
	if value != "false" {
		t.Errorf("Expected existing database value 'false', got '%s'", value)
	}
}

func TestConfigurationMigrator_SeedDefaultConfigurations(t *testing.T) {
	mockStorage := NewMockStorageService()
	configService := NewHybridConfigService(mockStorage)
	migrator := NewConfigurationMigrator(configService)
	ctx := context.Background()

	// Initialize config service
	err := configService.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Run seeding
	err = migrator.SeedDefaultConfigurations(ctx)
	if err != nil {
		t.Errorf("SeedDefaultConfigurations failed: %v", err)
	}

	// Verify some default configurations were seeded
	defaultChecks := map[string]string{
		"AI_PROVIDER_GEMINI_RATE_LIMIT_PER_MINUTE": "30",
		"BMAD_KB_REFRESH_ENABLED":                  "true",
		"OLLAMA_HOST":                              "http://localhost:11434",
		"BOT_STATUS_UPDATE_ENABLED":                "true",
	}

	for key, expectedValue := range defaultChecks {
		value, err := configService.GetConfig(ctx, key)
		if err != nil {
			t.Errorf("Failed to get seeded configuration %s: %v", key, err)
		}
		if value != expectedValue {
			t.Errorf("Expected default value '%s' for key %s, got '%s'", expectedValue, key, value)
		}
	}
}

func TestConfigurationMigrator_SeedDefaultConfigurations_SkipExisting(t *testing.T) {
	mockStorage := NewMockStorageService()
	configService := NewHybridConfigService(mockStorage)
	migrator := NewConfigurationMigrator(configService)
	ctx := context.Background()

	// Pre-populate database with existing configuration
	mockStorage.AddTestConfiguration("BMAD_KB_REFRESH_ENABLED", "false", "bool", "features", "Existing configuration")

	// Initialize config service
	err := configService.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Run seeding
	err = migrator.SeedDefaultConfigurations(ctx)
	if err != nil {
		t.Errorf("SeedDefaultConfigurations failed: %v", err)
	}

	// Verify existing configuration was preserved
	value, err := configService.GetConfig(ctx, "BMAD_KB_REFRESH_ENABLED")
	if err != nil {
		t.Errorf("Failed to get configuration: %v", err)
	}
	if value != "false" {
		t.Errorf("Expected existing value 'false', got '%s'", value)
	}
}

func TestGetEnvWithDefault(t *testing.T) {
	testCases := []struct {
		key          string
		envValue     string
		defaultValue string
		expected     string
		setEnv       bool
	}{
		{"TEST_KEY", "env_value", "default_value", "env_value", true},
		{"MISSING_KEY", "", "default_value", "default_value", false},
		{"EMPTY_KEY", "", "default_value", "default_value", true},
	}

	for _, tc := range testCases {
		if tc.setEnv {
			os.Setenv(tc.key, tc.envValue)
			defer os.Unsetenv(tc.key)
		}

		result := GetEnvWithDefault(tc.key, tc.defaultValue)
		if result != tc.expected {
			t.Errorf("GetEnvWithDefault(%s, %s) = %s, expected %s", tc.key, tc.defaultValue, result, tc.expected)
		}
	}
}

func TestConfigurationLoader_StartAutoReloadWithServiceNotification(t *testing.T) {
	mockStorage := NewMockStorageService()
	configService := NewHybridConfigService(mockStorage)
	loader := NewConfigurationLoader(configService)
	ctx := context.Background()

	// Initialize loader
	err := loader.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Create and register mock service listener
	mockService := &MockServiceListener{}
	serviceListener := ServiceConfigListener{
		Name:     "test_service",
		OnReload: mockService.OnReload,
	}
	loader.RegisterServiceListener(serviceListener)

	// Start auto-reload with short interval for testing
	err = loader.StartAutoReloadWithServiceNotification(100 * time.Millisecond)
	if err != nil {
		t.Errorf("StartAutoReloadWithServiceNotification failed: %v", err)
	}

	// Add configuration to trigger notification
	mockStorage.AddTestConfiguration("auto_reload_key", "auto_reload_value", "string", "test", "Auto reload test")

	// Manually trigger the change notification to simulate auto-reload
	// In a real scenario, this would happen automatically through the database service
	if configService.IsDatabaseAvailable() {
		// Reload configurations to trigger change detection
		err = configService.ReloadConfigs(ctx)
		if err != nil {
			t.Errorf("ReloadConfigs failed: %v", err)
		}
	}

	// Give some time for async operations
	time.Sleep(50 * time.Millisecond)

	// Clean up
	defer func() {
		configService.StopAutoReload()
		loader.Close()
	}()
}

// Integration test combining multiple components
func TestConfigurationIntegration(t *testing.T) {
	mockStorage := NewMockStorageService()
	configService := NewHybridConfigService(mockStorage)
	loader := NewConfigurationLoader(configService)
	migrator := NewConfigurationMigrator(configService)
	ctx := context.Background()

	// Set up environment variables
	os.Setenv("INTEGRATION_TEST_KEY", "env_value")
	os.Setenv("AI_PROVIDER_GEMINI_RATE_LIMIT_PER_MINUTE", "45")
	defer func() {
		os.Unsetenv("INTEGRATION_TEST_KEY")
		os.Unsetenv("AI_PROVIDER_GEMINI_RATE_LIMIT_PER_MINUTE")
	}()

	// Initialize all components
	err := loader.Initialize(ctx)
	if err != nil {
		t.Fatalf("Loader initialize failed: %v", err)
	}
	defer loader.Close()

	// Run migration and seeding
	err = migrator.MigrateEnvironmentVariables(ctx)
	if err != nil {
		t.Errorf("Migration failed: %v", err)
	}

	err = migrator.SeedDefaultConfigurations(ctx)
	if err != nil {
		t.Errorf("Seeding failed: %v", err)
	}

	// Register service listener
	mockService := &MockServiceListener{}
	serviceListener := ServiceConfigListener{
		Name:     "integration_test_service",
		OnReload: mockService.OnReload,
	}
	loader.RegisterServiceListener(serviceListener)

	// Test configuration access
	// Environment variable fallback
	value, err := configService.GetConfig(ctx, "INTEGRATION_TEST_KEY")
	if err != nil {
		t.Errorf("Failed to get environment configuration: %v", err)
	}
	if value != "env_value" {
		t.Errorf("Expected 'env_value', got '%s'", value)
	}

	// Migrated configuration
	value, err = configService.GetConfig(ctx, "AI_PROVIDER_GEMINI_RATE_LIMIT_PER_MINUTE")
	if err != nil {
		t.Errorf("Failed to get migrated configuration: %v", err)
	}
	if value != "45" {
		t.Errorf("Expected '45', got '%s'", value)
	}

	// Seeded default configuration
	value = configService.GetConfigWithDefault(ctx, "BMAD_KB_REFRESH_ENABLED", "false")
	if value != "true" {
		t.Errorf("Expected seeded default 'true', got '%s'", value)
	}

	// Set new configuration and reload services
	err = configService.SetConfig(ctx, "dynamic_key", "dynamic_value", "test", "Dynamic configuration")
	if err != nil {
		t.Errorf("Failed to set dynamic configuration: %v", err)
	}

	// Trigger service notification
	err = loader.ReloadAndNotifyServices(ctx)
	if err != nil {
		t.Errorf("ReloadAndNotifyServices failed: %v", err)
	}

	// Verify service was notified
	if mockService.GetReloadCount() != 1 {
		t.Errorf("Expected 1 service notification, got %d", mockService.GetReloadCount())
	}

	configs := mockService.GetLastConfigs()
	if _, exists := configs["dynamic_key"]; !exists {
		t.Error("Service should have received dynamic configuration")
	}
}

// Benchmark tests
func BenchmarkConfigurationLoader_ReloadAndNotifyServices(b *testing.B) {
	mockStorage := NewMockStorageService()
	configService := NewHybridConfigService(mockStorage)
	loader := NewConfigurationLoader(configService)
	ctx := context.Background()

	// Initialize and add test configurations
	loader.Initialize(ctx)
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("benchmark_key_%d", i)
		mockStorage.AddTestConfiguration(key, "benchmark_value", "string", "benchmark", "Benchmark configuration")
	}

	// Register multiple service listeners
	for i := 0; i < 10; i++ {
		mockService := &MockServiceListener{}
		serviceListener := ServiceConfigListener{
			Name:     fmt.Sprintf("benchmark_service_%d", i),
			OnReload: mockService.OnReload,
		}
		loader.RegisterServiceListener(serviceListener)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = loader.ReloadAndNotifyServices(ctx)
	}
}
