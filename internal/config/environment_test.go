package config

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestHybridConfigService_Initialize(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewHybridConfigService(mockStorage)

	err := service.Initialize(context.Background())
	if err != nil {
		t.Errorf("Initialize failed: %v", err)
	}
}

func TestHybridConfigService_SecureKeys(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewHybridConfigService(mockStorage)
	ctx := context.Background()

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Set environment variable for secure key
	os.Setenv("BOT_TOKEN", "test_token_value")
	defer os.Unsetenv("BOT_TOKEN")

	// Test that secure keys are read from environment variables
	value, err := service.GetConfig(ctx, "BOT_TOKEN")
	if err != nil {
		t.Errorf("GetConfig failed for secure key: %v", err)
	}
	if value != "test_token_value" {
		t.Errorf("Expected 'test_token_value', got '%s'", value)
	}

	// Test that secure keys cannot be set through ConfigService
	err = service.SetConfig(ctx, "BOT_TOKEN", "new_token", "security", "Bot token")
	if err == nil {
		t.Error("Should not be able to set secure configuration keys")
	}

	// Test that secure keys cannot be deleted through ConfigService
	err = service.DeleteConfig(ctx, "BOT_TOKEN")
	if err == nil {
		t.Error("Should not be able to delete secure configuration keys")
	}
}

func TestHybridConfigService_DatabaseFirst(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewHybridConfigService(mockStorage)
	ctx := context.Background()

	// Add configuration to database
	mockStorage.AddTestConfiguration("test_key", "database_value", "string", "test", "Test configuration")

	// Set environment variable with different value
	os.Setenv("test_key", "env_value")
	defer os.Unsetenv("test_key")

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test that database value takes precedence
	value, err := service.GetConfig(ctx, "test_key")
	if err != nil {
		t.Errorf("GetConfig failed: %v", err)
	}
	if value != "database_value" {
		t.Errorf("Expected 'database_value' from database, got '%s'", value)
	}
}

func TestHybridConfigService_EnvironmentFallback(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewHybridConfigService(mockStorage)
	ctx := context.Background()

	// Set environment variable
	os.Setenv("env_only_key", "env_value")
	defer os.Unsetenv("env_only_key")

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test that environment variable is used when not in database
	value, err := service.GetConfig(ctx, "env_only_key")
	if err != nil {
		t.Errorf("GetConfig failed: %v", err)
	}
	if value != "env_value" {
		t.Errorf("Expected 'env_value' from environment, got '%s'", value)
	}
}

func TestHybridConfigService_DatabaseUnavailable(t *testing.T) {
	mockStorage := NewMockStorageService()
	// Simulate database initialization failure
	mockStorage.SetGetError(NewConfigError("", "database unavailable", nil))

	service := NewHybridConfigService(mockStorage)
	ctx := context.Background()

	// Set environment variable
	os.Setenv("fallback_key", "fallback_value")
	defer os.Unsetenv("fallback_key")

	// Initialize service (should not fail even if database is unavailable)
	err := service.Initialize(ctx)
	if err != nil {
		t.Errorf("Initialize should not fail when database is unavailable: %v", err)
	}

	// Test that environment fallback works
	value, err := service.GetConfig(ctx, "fallback_key")
	if err != nil {
		t.Errorf("GetConfig failed with database unavailable: %v", err)
	}
	if value != "fallback_value" {
		t.Errorf("Expected 'fallback_value' from environment fallback, got '%s'", value)
	}
}

func TestHybridConfigService_GetConfigsByCategory(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewHybridConfigService(mockStorage)
	ctx := context.Background()

	// Add database configuration
	mockStorage.AddTestConfiguration("db_rate_limit", "30", "int", "rate_limiting", "Database rate limit")

	// Set environment variables
	os.Setenv("AI_PROVIDER_GEMINI_RATE_LIMIT_PER_MINUTE", "60")
	os.Setenv("SOME_OTHER_CONFIG", "other_value")
	defer func() {
		os.Unsetenv("AI_PROVIDER_GEMINI_RATE_LIMIT_PER_MINUTE")
		os.Unsetenv("SOME_OTHER_CONFIG")
	}()

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test getting rate limiting configurations
	rateLimitConfigs, err := service.GetConfigsByCategory(ctx, "rate_limiting")
	if err != nil {
		t.Errorf("GetConfigsByCategory failed: %v", err)
	}

	// Should include both database and environment configurations
	if len(rateLimitConfigs) < 2 {
		t.Errorf("Expected at least 2 rate limiting configurations, got %d", len(rateLimitConfigs))
	}

	// Database value should be included
	if _, exists := rateLimitConfigs["db_rate_limit"]; !exists {
		t.Error("Database configuration should be included")
	}

	// Environment variable matching pattern should be included
	if _, exists := rateLimitConfigs["AI_PROVIDER_GEMINI_RATE_LIMIT_PER_MINUTE"]; !exists {
		t.Error("Environment rate limit configuration should be included")
	}

	// Non-matching environment variable should not be included
	if _, exists := rateLimitConfigs["SOME_OTHER_CONFIG"]; exists {
		t.Error("Non-matching environment configuration should not be included")
	}
}

func TestHybridConfigService_GetAllConfigs(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewHybridConfigService(mockStorage)
	ctx := context.Background()

	// Add database configuration
	mockStorage.AddTestConfiguration("db_config", "db_value", "string", "test", "Database configuration")

	// Set environment variables (non-secure)
	os.Setenv("ENV_CONFIG", "env_value")
	os.Setenv("BOT_TOKEN", "secret_token") // This should be excluded from GetAllConfigs
	defer func() {
		os.Unsetenv("ENV_CONFIG")
		os.Unsetenv("BOT_TOKEN")
	}()

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test getting all configurations
	allConfigs, err := service.GetAllConfigs(ctx)
	if err != nil {
		t.Errorf("GetAllConfigs failed: %v", err)
	}

	// Database configuration should be included
	if _, exists := allConfigs["db_config"]; !exists {
		t.Error("Database configuration should be included")
	}

	// Non-secure environment configuration should be included
	if _, exists := allConfigs["ENV_CONFIG"]; !exists {
		t.Error("Non-secure environment configuration should be included")
	}

	// Secure environment configuration should not be included
	if _, exists := allConfigs["BOT_TOKEN"]; exists {
		t.Error("Secure configuration should not be included in GetAllConfigs")
	}
}

func TestHybridConfigService_IsDatabaseAvailable(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewHybridConfigService(mockStorage)

	// Before initialization
	if service.IsDatabaseAvailable() {
		t.Error("Database should not be available before initialization")
	}

	// After successful initialization
	err := service.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if !service.IsDatabaseAvailable() {
		t.Error("Database should be available after successful initialization")
	}

	// Test with failed initialization
	mockStorage2 := NewMockStorageService()
	mockStorage2.SetGetError(NewConfigError("", "database error", nil))
	service2 := NewHybridConfigService(mockStorage2)

	err = service2.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize should not fail: %v", err)
	}

	if service2.IsDatabaseAvailable() {
		t.Error("Database should not be available when initialization fails")
	}
}

func TestHybridConfigService_SetEnvironmentFallback(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewHybridConfigService(mockStorage)
	ctx := context.Background()

	// Set environment variable
	os.Setenv("test_fallback", "env_value")
	defer os.Unsetenv("test_fallback")

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test with environment fallback enabled (default)
	value, err := service.GetConfig(ctx, "test_fallback")
	if err != nil {
		t.Errorf("GetConfig failed with fallback enabled: %v", err)
	}
	if value != "env_value" {
		t.Errorf("Expected 'env_value', got '%s'", value)
	}

	// Disable environment fallback
	service.SetEnvironmentFallback(false)

	// Test with environment fallback disabled
	_, err = service.GetConfig(ctx, "test_fallback")
	if err == nil {
		t.Error("GetConfig should fail with fallback disabled")
	}

	// Re-enable environment fallback
	service.SetEnvironmentFallback(true)

	// Test with environment fallback re-enabled
	value, err = service.GetConfig(ctx, "test_fallback")
	if err != nil {
		t.Errorf("GetConfig failed with fallback re-enabled: %v", err)
	}
	if value != "env_value" {
		t.Errorf("Expected 'env_value', got '%s'", value)
	}
}

func TestHybridConfigService_ConfigTypeConversions(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewHybridConfigService(mockStorage)
	ctx := context.Background()

	// Set environment variables with different types
	os.Setenv("TEST_INT", "42")
	os.Setenv("TEST_BOOL", "true")
	os.Setenv("TEST_DURATION", "5m")
	defer func() {
		os.Unsetenv("TEST_INT")
		os.Unsetenv("TEST_BOOL")
		os.Unsetenv("TEST_DURATION")
	}()

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test integer conversion
	intValue := service.GetConfigIntWithDefault(ctx, "TEST_INT", 0)
	if intValue != 42 {
		t.Errorf("Expected 42, got %d", intValue)
	}

	// Test boolean conversion
	boolValue := service.GetConfigBoolWithDefault(ctx, "TEST_BOOL", false)
	if !boolValue {
		t.Errorf("Expected true, got %t", boolValue)
	}

	// Test duration conversion
	durationValue := service.GetConfigDurationWithDefault(ctx, "TEST_DURATION", 0)
	if durationValue != 5*time.Minute {
		t.Errorf("Expected 5m, got %v", durationValue)
	}

	// Test defaults for non-existent keys
	defaultInt := service.GetConfigIntWithDefault(ctx, "NON_EXISTENT_INT", 100)
	if defaultInt != 100 {
		t.Errorf("Expected default 100, got %d", defaultInt)
	}

	defaultBool := service.GetConfigBoolWithDefault(ctx, "NON_EXISTENT_BOOL", true)
	if !defaultBool {
		t.Errorf("Expected default true, got %t", defaultBool)
	}

	defaultDuration := service.GetConfigDurationWithDefault(ctx, "NON_EXISTENT_DURATION", 10*time.Second)
	if defaultDuration != 10*time.Second {
		t.Errorf("Expected default 10s, got %v", defaultDuration)
	}
}

func TestHybridConfigService_HealthCheck(t *testing.T) {
	mockStorage := NewMockStorageService()
	service := NewHybridConfigService(mockStorage)
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

	// Test health check with database unavailable (should still pass due to fallback)
	mockStorage.SetHealthError(NewConfigError("", "database health check failed", nil))
	err = service.HealthCheck(ctx)
	if err != nil {
		t.Errorf("HealthCheck should pass even with database issues: %v", err)
	}
}

// Benchmark tests for hybrid service
func BenchmarkHybridConfigService_GetConfig_Database(b *testing.B) {
	mockStorage := NewMockStorageService()
	service := NewHybridConfigService(mockStorage)
	ctx := context.Background()

	// Add test configuration to database
	mockStorage.AddTestConfiguration("benchmark_key", "benchmark_value", "string", "benchmark", "Benchmark configuration")

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		b.Fatalf("Initialize failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.GetConfig(ctx, "benchmark_key")
	}
}

func BenchmarkHybridConfigService_GetConfig_Environment(b *testing.B) {
	mockStorage := NewMockStorageService()
	service := NewHybridConfigService(mockStorage)
	ctx := context.Background()

	// Set environment variable
	os.Setenv("BENCHMARK_ENV_KEY", "benchmark_value")
	defer os.Unsetenv("BENCHMARK_ENV_KEY")

	// Initialize service
	err := service.Initialize(ctx)
	if err != nil {
		b.Fatalf("Initialize failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = service.GetConfig(ctx, "BENCHMARK_ENV_KEY")
	}
}
