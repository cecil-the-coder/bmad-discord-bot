package config

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"bmad-knowledge-bot/internal/storage"
)

// MockStorageService implements storage.StorageService for testing
type MockStorageService struct {
	mu             sync.RWMutex
	configurations map[string]*storage.Configuration
	healthError    error
	getError       error
	upsertError    error
	deleteError    error
}

func NewMockStorageService() *MockStorageService {
	return &MockStorageService{
		configurations: make(map[string]*storage.Configuration),
	}
}

// Storage service interface methods (partial implementation for testing)
func (m *MockStorageService) Initialize(ctx context.Context) error {
	return nil
}

func (m *MockStorageService) Close() error {
	return nil
}

func (m *MockStorageService) GetMessageState(ctx context.Context, channelID string, threadID *string) (*storage.MessageState, error) {
	return nil, nil
}

func (m *MockStorageService) UpsertMessageState(ctx context.Context, state *storage.MessageState) error {
	return nil
}

func (m *MockStorageService) GetAllMessageStates(ctx context.Context) ([]*storage.MessageState, error) {
	return nil, nil
}

func (m *MockStorageService) GetMessageStatesWithinWindow(ctx context.Context, windowDuration time.Duration) ([]*storage.MessageState, error) {
	return nil, nil
}

func (m *MockStorageService) HealthCheck(ctx context.Context) error {
	return m.healthError
}

func (m *MockStorageService) GetThreadOwnership(ctx context.Context, threadID string) (*storage.ThreadOwnership, error) {
	return nil, nil
}

func (m *MockStorageService) UpsertThreadOwnership(ctx context.Context, ownership *storage.ThreadOwnership) error {
	return nil
}

func (m *MockStorageService) GetAllThreadOwnerships(ctx context.Context) ([]*storage.ThreadOwnership, error) {
	return nil, nil
}

func (m *MockStorageService) CleanupOldThreadOwnerships(ctx context.Context, maxAge int64) error {
	return nil
}

// Status message methods (required by StorageService interface)
func (m *MockStorageService) GetStatusMessagesBatch(ctx context.Context, limit int) ([]*storage.StatusMessage, error) {
	return nil, nil
}

func (m *MockStorageService) AddStatusMessage(ctx context.Context, activityType, statusText string, enabled bool) error {
	return nil
}

func (m *MockStorageService) UpdateStatusMessage(ctx context.Context, id int64, enabled bool) error {
	return nil
}

func (m *MockStorageService) GetAllStatusMessages(ctx context.Context) ([]*storage.StatusMessage, error) {
	return nil, nil
}

func (m *MockStorageService) GetEnabledStatusMessagesCount(ctx context.Context) (int, error) {
	return 0, nil
}

// Configuration methods for testing
func (m *MockStorageService) GetConfiguration(ctx context.Context, key string) (*storage.Configuration, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	config, exists := m.configurations[key]
	if !exists {
		return nil, nil
	}
	return config, nil
}

func (m *MockStorageService) UpsertConfiguration(ctx context.Context, config *storage.Configuration) error {
	if m.upsertError != nil {
		return m.upsertError
	}
	if config.CreatedAt == 0 {
		config.CreatedAt = time.Now().Unix()
	}
	config.UpdatedAt = time.Now().Unix()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configurations[config.Key] = config
	return nil
}

func (m *MockStorageService) GetConfigurationsByCategory(ctx context.Context, category string) ([]*storage.Configuration, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*storage.Configuration
	for _, config := range m.configurations {
		if config.Category == category {
			result = append(result, config)
		}
	}
	return result, nil
}

func (m *MockStorageService) GetAllConfigurations(ctx context.Context) ([]*storage.Configuration, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*storage.Configuration
	for _, config := range m.configurations {
		result = append(result, config)
	}
	return result, nil
}

func (m *MockStorageService) DeleteConfiguration(ctx context.Context, key string) error {
	if m.deleteError != nil {
		return m.deleteError
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.configurations[key]; !exists {
		return NewConfigError(key, "configuration not found", nil)
	}
	delete(m.configurations, key)
	return nil
}

// Test helper methods
func (m *MockStorageService) SetHealthError(err error) {
	m.healthError = err
}

func (m *MockStorageService) SetGetError(err error) {
	m.getError = err
}

func (m *MockStorageService) SetUpsertError(err error) {
	m.upsertError = err
}

func (m *MockStorageService) SetDeleteError(err error) {
	m.deleteError = err
}

func (m *MockStorageService) AddTestConfiguration(key, value, valueType, category, description string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configurations[key] = &storage.Configuration{
		Key:         key,
		Value:       value,
		Type:        valueType,
		Category:    category,
		Description: description,
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}
}

// Test DatabaseConfigService
func TestDatabaseConfigService_Initialize(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)

	err := service.Initialize(context.Background())
	if err != nil {
		t.Errorf("Initialize failed: %v", err)
	}
}

func TestDatabaseConfigService_GetConfig(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Add test configuration
	mockStorage.AddTestConfiguration("test_key", "test_value", "string", "test", "Test configuration")

	// Initialize service to load configurations
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test getting existing configuration
	value, err := service.GetConfig(ctx, "test_key")
	if err != nil {
		t.Errorf("GetConfig failed: %v", err)
	}
	if value != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", value)
	}

	// Test getting non-existent configuration
	_, err = service.GetConfig(ctx, "non_existent")
	if err == nil {
		t.Error("Expected error for non-existent configuration")
	}
}

func TestDatabaseConfigService_GetConfigWithDefault(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Add test configuration
	mockStorage.AddTestConfiguration("existing_key", "existing_value", "string", "test", "Test configuration")

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test getting existing configuration
	value := service.GetConfigWithDefault(ctx, "existing_key", "default_value")
	if value != "existing_value" {
		t.Errorf("Expected 'existing_value', got '%s'", value)
	}

	// Test getting non-existent configuration with default
	value = service.GetConfigWithDefault(ctx, "non_existent", "default_value")
	if value != "default_value" {
		t.Errorf("Expected 'default_value', got '%s'", value)
	}
}

func TestDatabaseConfigService_GetConfigInt(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Add test configurations
	mockStorage.AddTestConfiguration("valid_int", "42", "int", "test", "Valid integer")
	mockStorage.AddTestConfiguration("invalid_int", "not_a_number", "string", "test", "Invalid integer")

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test getting valid integer
	value, err := service.GetConfigInt(ctx, "valid_int")
	if err != nil {
		t.Errorf("GetConfigInt failed: %v", err)
	}
	if value != 42 {
		t.Errorf("Expected 42, got %d", value)
	}

	// Test getting invalid integer
	_, err = service.GetConfigInt(ctx, "invalid_int")
	if err == nil {
		t.Error("Expected error for invalid integer")
	}

	// Test with default
	value = service.GetConfigIntWithDefault(ctx, "non_existent", 100)
	if value != 100 {
		t.Errorf("Expected 100, got %d", value)
	}
}

func TestDatabaseConfigService_GetConfigBool(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Add test configurations
	testCases := map[string]bool{
		"true":     true,
		"false":    false,
		"1":        true,
		"0":        false,
		"yes":      true,
		"no":       false,
		"on":       true,
		"off":      false,
		"enabled":  true,
		"disabled": false,
	}

	for key := range testCases {
		mockStorage.AddTestConfiguration(key, key, "bool", "test", "Test boolean")
	}
	mockStorage.AddTestConfiguration("invalid_bool", "maybe", "bool", "test", "Invalid boolean")

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test valid boolean values
	for key, expected := range testCases {
		value, err := service.GetConfigBool(ctx, key)
		if err != nil {
			t.Errorf("GetConfigBool failed for '%s': %v", key, err)
		}
		if value != expected {
			t.Errorf("For key '%s', expected %t, got %t", key, expected, value)
		}
	}

	// Test invalid boolean
	_, err = service.GetConfigBool(ctx, "invalid_bool")
	if err == nil {
		t.Error("Expected error for invalid boolean")
	}

	// Test with default
	value := service.GetConfigBoolWithDefault(ctx, "non_existent", true)
	if value != true {
		t.Errorf("Expected true, got %t", value)
	}
}

func TestDatabaseConfigService_GetConfigDuration(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Add test configurations
	mockStorage.AddTestConfiguration("valid_duration", "5m", "duration", "test", "Valid duration")
	mockStorage.AddTestConfiguration("invalid_duration", "not_a_duration", "duration", "test", "Invalid duration")

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test getting valid duration
	duration, err := service.GetConfigDuration(ctx, "valid_duration")
	if err != nil {
		t.Errorf("GetConfigDuration failed: %v", err)
	}
	if duration != 5*time.Minute {
		t.Errorf("Expected 5m, got %v", duration)
	}

	// Test getting invalid duration
	_, err = service.GetConfigDuration(ctx, "invalid_duration")
	if err == nil {
		t.Error("Expected error for invalid duration")
	}

	// Test with default
	duration = service.GetConfigDurationWithDefault(ctx, "non_existent", 10*time.Second)
	if duration != 10*time.Second {
		t.Errorf("Expected 10s, got %v", duration)
	}
}

func TestDatabaseConfigService_SetConfig(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test setting new configuration
	err = service.SetConfig(ctx, "new_key", "new_value", "test", "New test configuration")
	if err != nil {
		t.Errorf("SetConfig failed: %v", err)
	}

	// Verify the configuration was set
	value, err := service.GetConfig(ctx, "new_key")
	if err != nil {
		t.Errorf("GetConfig failed after SetConfig: %v", err)
	}
	if value != "new_value" {
		t.Errorf("Expected 'new_value', got '%s'", value)
	}

	// Test updating existing configuration
	err = service.SetConfig(ctx, "new_key", "updated_value", "test", "Updated test configuration")
	if err != nil {
		t.Errorf("SetConfig update failed: %v", err)
	}

	// Verify the configuration was updated
	value, err = service.GetConfig(ctx, "new_key")
	if err != nil {
		t.Errorf("GetConfig failed after SetConfig update: %v", err)
	}
	if value != "updated_value" {
		t.Errorf("Expected 'updated_value', got '%s'", value)
	}
}

func TestDatabaseConfigService_SetConfigTyped(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test setting valid typed configurations
	testCases := []struct {
		key       string
		value     string
		valueType string
		valid     bool
	}{
		{"string_key", "string_value", "string", true},
		{"int_key", "42", "int", true},
		{"bool_key", "true", "bool", true},
		{"duration_key", "5m", "duration", true},
		{"invalid_int", "not_a_number", "int", false},
		{"invalid_bool", "maybe", "bool", false},
		{"invalid_duration", "not_a_duration", "duration", false},
		{"unsupported_type", "value", "unsupported", false},
	}

	for _, tc := range testCases {
		err = service.SetConfigTyped(ctx, tc.key, tc.value, tc.valueType, "test", "Test configuration")
		if tc.valid && err != nil {
			t.Errorf("SetConfigTyped failed for valid case %s: %v", tc.key, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("SetConfigTyped should have failed for invalid case %s", tc.key)
		}
	}
}

func TestDatabaseConfigService_ValidateConfig(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)

	testCases := []struct {
		key     string
		value   string
		valid   bool
		message string
	}{
		{"valid_key", "valid_value", true, "Valid configuration"},
		{"", "value", false, "Empty key should be invalid"},
		{"key", "", true, "Empty value should be valid"},
		{"very_long_key_" + string(make([]byte, 260)), "value", false, "Key too long"},
		{"key", string(make([]byte, 70000)), false, "Value too long"},
		{"SOME_RATE_LIMIT_PER_MINUTE", "30", true, "Valid rate limit"},
		{"SOME_RATE_LIMIT_PER_MINUTE", "not_a_number", false, "Invalid rate limit"},
		{"FEATURE_ENABLED", "true", true, "Valid boolean feature flag"},
		{"FEATURE_ENABLED", "maybe", false, "Invalid boolean feature flag"},
	}

	for _, tc := range testCases {
		err := service.ValidateConfig(tc.key, tc.value)
		if tc.valid && err != nil {
			t.Errorf("ValidateConfig failed for valid case '%s': %v (%s)", tc.key, err, tc.message)
		}
		if !tc.valid && err == nil {
			t.Errorf("ValidateConfig should have failed for invalid case '%s' (%s)", tc.key, tc.message)
		}
	}
}

func TestDatabaseConfigService_GetConfigsByCategory(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Add test configurations
	mockStorage.AddTestConfiguration("rate_limit_1", "30", "int", "rate_limiting", "Rate limit 1")
	mockStorage.AddTestConfiguration("rate_limit_2", "60", "int", "rate_limiting", "Rate limit 2")
	mockStorage.AddTestConfiguration("feature_1", "true", "bool", "features", "Feature 1")
	mockStorage.AddTestConfiguration("other_config", "value", "string", "other", "Other config")

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test getting configurations by category
	rateLimitConfigs, err := service.GetConfigsByCategory(ctx, "rate_limiting")
	if err != nil {
		t.Errorf("GetConfigsByCategory failed: %v", err)
	}
	if len(rateLimitConfigs) != 2 {
		t.Errorf("Expected 2 rate limiting configurations, got %d", len(rateLimitConfigs))
	}

	featureConfigs, err := service.GetConfigsByCategory(ctx, "features")
	if err != nil {
		t.Errorf("GetConfigsByCategory failed: %v", err)
	}
	if len(featureConfigs) != 1 {
		t.Errorf("Expected 1 feature configuration, got %d", len(featureConfigs))
	}

	nonExistentConfigs, err := service.GetConfigsByCategory(ctx, "non_existent")
	if err != nil {
		t.Errorf("GetConfigsByCategory failed: %v", err)
	}
	if len(nonExistentConfigs) != 0 {
		t.Errorf("Expected 0 non-existent configurations, got %d", len(nonExistentConfigs))
	}
}

func TestDatabaseConfigService_DeleteConfig(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Add test configuration
	mockStorage.AddTestConfiguration("delete_me", "value", "string", "test", "Configuration to delete")

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Verify configuration exists
	_, err = service.GetConfig(ctx, "delete_me")
	if err != nil {
		t.Errorf("Configuration should exist before deletion: %v", err)
	}

	// Delete configuration
	err = service.DeleteConfig(ctx, "delete_me")
	if err != nil {
		t.Errorf("DeleteConfig failed: %v", err)
	}

	// Verify configuration no longer exists
	_, err = service.GetConfig(ctx, "delete_me")
	if err == nil {
		t.Error("Configuration should not exist after deletion")
	}

	// Test deleting non-existent configuration
	err = service.DeleteConfig(ctx, "non_existent")
	if err == nil {
		t.Error("Expected error when deleting non-existent configuration")
	}
}

func TestDatabaseConfigService_ReloadConfigs(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Add initial configuration
	mockStorage.AddTestConfiguration("initial_key", "initial_value", "string", "test", "Initial configuration")

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Verify initial configuration
	value, err := service.GetConfig(ctx, "initial_key")
	if err != nil {
		t.Errorf("GetConfig failed: %v", err)
	}
	if value != "initial_value" {
		t.Errorf("Expected 'initial_value', got '%s'", value)
	}

	// Add new configuration directly to storage (simulating external change)
	mockStorage.AddTestConfiguration("new_key", "new_value", "string", "test", "New configuration")

	// Before reload, new configuration should not be available
	_, err = service.GetConfig(ctx, "new_key")
	if err == nil {
		t.Error("New configuration should not be available before reload")
	}

	// Reload configurations
	err = service.ReloadConfigs(ctx)
	if err != nil {
		t.Errorf("ReloadConfigs failed: %v", err)
	}

	// After reload, new configuration should be available
	value, err = service.GetConfig(ctx, "new_key")
	if err != nil {
		t.Errorf("GetConfig failed after reload: %v", err)
	}
	if value != "new_value" {
		t.Errorf("Expected 'new_value', got '%s'", value)
	}
}

func TestDatabaseConfigService_HealthCheck(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test successful health check
	err = service.HealthCheck(ctx)
	if err != nil {
		t.Errorf("HealthCheck failed: %v", err)
	}

	// Test health check with storage error
	mockStorage.SetHealthError(NewConfigError("", "storage health check failed", nil))
	err = service.HealthCheck(ctx)
	if err == nil {
		t.Error("HealthCheck should have failed with storage error")
	}
}

func TestDatabaseConfigService_AutoReload(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Start auto-reload with short interval for testing
	err = service.StartAutoReload(100 * time.Millisecond)
	if err != nil {
		t.Errorf("StartAutoReload failed: %v", err)
	}

	// Add configuration to storage
	mockStorage.AddTestConfiguration("auto_reload_test", "test_value", "string", "test", "Auto reload test")

	// Wait for auto-reload to trigger
	time.Sleep(200 * time.Millisecond)

	// Check if configuration was loaded
	value, err := service.GetConfig(ctx, "auto_reload_test")
	if err != nil {
		t.Errorf("Configuration should be available after auto-reload: %v", err)
	}
	if value != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", value)
	}

	// Stop auto-reload
	service.StopAutoReload()

	// Verify auto-reload stopped
	time.Sleep(100 * time.Millisecond) // Brief wait to ensure no more reloads
}

// Benchmark tests
func BenchmarkDatabaseConfigService_GetConfig(b *testing.B) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Add test configurations
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("benchmark_key_%d", i)
		mockStorage.AddTestConfiguration(key, "benchmark_value", "string", "benchmark", "Benchmark configuration")
	}

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		b.Fatalf("Initialize failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("benchmark_key_%d", i%1000)
		_, _ = service.GetConfig(ctx, key)
	}
}

func BenchmarkDatabaseConfigService_SetConfig(b *testing.B) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		b.Fatalf("Initialize failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("benchmark_set_key_%d", i)
		_ = service.SetConfig(ctx, key, "benchmark_value", "benchmark", "Benchmark set configuration")
	}
}

func TestDatabaseConfigService_GetConfigIntWithDefault_ErrorCase(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Initialize service first
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Set error after initialization
	mockStorage.getError = fmt.Errorf("database connection failed")

	// Test error case - should return default value
	result := service.GetConfigIntWithDefault(ctx, "nonexistent_key", 42)
	if result != 42 {
		t.Errorf("Expected default value 42, got %d", result)
	}
}

func TestDatabaseConfigService_GetConfigBoolWithDefault_ErrorCase(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Initialize service first
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Set error after initialization
	mockStorage.getError = fmt.Errorf("database connection failed")

	// Test error case - should return default value
	result := service.GetConfigBoolWithDefault(ctx, "nonexistent_key", true)
	if result != true {
		t.Errorf("Expected default value true, got %v", result)
	}
}

func TestDatabaseConfigService_NotifyConfigChanges_Scenarios(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewDatabaseConfigService(mockStorage)
	ctx := context.Background()

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test with valid configuration change
	configChange := &storage.Configuration{
		Key:         "test_notify_key",
		Value:       "test_notify_value",
		Type:        "string",
		Category:    "test",
		Description: "Test notification",
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}

	// This should not panic and should handle the notification gracefully
	oldConfigs := make(map[string]*storage.Configuration)
	newConfigs := map[string]*storage.Configuration{
		"test_notify_key": configChange,
	}
	service.notifyConfigChanges(newConfigs, oldConfigs)
}
