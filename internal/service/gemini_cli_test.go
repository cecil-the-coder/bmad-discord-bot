package service

import (
	"log/slog"
	"os"
	"testing"

	"bmad-knowledge-bot/internal/monitor"
)

func TestGeminiCLIService_GetProviderID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	// Create a temporary executable file for testing
	tmpFile, err := os.CreateTemp("", "gemini-cli-test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()
	
	service, err := NewGeminiCLIService(tmpFile.Name(), logger)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	providerID := service.GetProviderID()
	if providerID != "gemini" {
		t.Errorf("Expected provider ID to be 'gemini', got '%s'", providerID)
	}
}

func TestGeminiCLIService_RateLimitIntegration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	// Create a temporary executable file for testing
	tmpFile, err := os.CreateTemp("", "gemini-cli-test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()
	
	// Create service
	service, err := NewGeminiCLIService(tmpFile.Name(), logger)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	// Create rate limiter with very low limits for testing
	config := monitor.ProviderConfig{
		ProviderID: "gemini",
		Limits: map[string]int{
			"minute": 1, // Only 1 call per minute
		},
		Thresholds: map[string]float64{
			"warning":   0.75,
			"throttled": 1.0,
		},
	}
	
	rateLimitManager := monitor.NewRateLimitManager(logger, []monitor.ProviderConfig{config})
	service.SetRateLimiter(rateLimitManager)
	
	// First, manually register a call to reach the limit
	err = rateLimitManager.RegisterCall("gemini")
	if err != nil {
		t.Fatalf("Failed to register call: %v", err)
	}
	
	// Verify the service is now throttled
	status := rateLimitManager.GetProviderStatus("gemini")
	if status != "Throttled" {
		t.Errorf("Expected status to be Throttled, got %s", status)
	}
	
	// Test that QueryAI is blocked when rate limited
	_, err = service.QueryAI("test query")
	if err == nil {
		t.Error("Expected QueryAI to fail when rate limited, but it succeeded")
	}
	
	expectedErrorSubstring := "rate limit exceeded"
	if err != nil && len(err.Error()) > 0 {
		if !contains(err.Error(), expectedErrorSubstring) {
			t.Errorf("Expected error to contain '%s', got: %s", expectedErrorSubstring, err.Error())
		}
	}
}

func TestGeminiCLIService_RateLimitGracefulDegradation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	// Create a temporary executable file for testing
	tmpFile, err := os.CreateTemp("", "gemini-cli-test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()
	
	// Create service without rate limiter
	service, err := NewGeminiCLIService(tmpFile.Name(), logger)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	// Test that service works without rate limiter (graceful degradation)
	// This should not panic or fail due to missing rate limiter
	providerID := service.GetProviderID()
	if providerID != "gemini" {
		t.Errorf("Expected provider ID to be 'gemini', got '%s'", providerID)
	}
	
	// checkRateLimit should return nil when no rate limiter is set
	err = service.checkRateLimit()
	if err != nil {
		t.Errorf("Expected checkRateLimit to return nil when no rate limiter set, got: %v", err)
	}
}

func TestGeminiCLIService_SetRateLimiter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	// Create a temporary executable file for testing
	tmpFile, err := os.CreateTemp("", "gemini-cli-test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()
	
	service, err := NewGeminiCLIService(tmpFile.Name(), logger)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	// Verify initial rate limiter is nil
	if service.rateLimiter != nil {
		t.Error("Expected initial rate limiter to be nil")
	}
	
	// Create and set rate limiter
	config := monitor.ProviderConfig{
		ProviderID: "gemini",
		Limits: map[string]int{
			"minute": 60,
		},
		Thresholds: map[string]float64{
			"warning":   0.75,
			"throttled": 1.0,
		},
	}
	
	rateLimitManager := monitor.NewRateLimitManager(logger, []monitor.ProviderConfig{config})
	service.SetRateLimiter(rateLimitManager)
	
	// Verify rate limiter is set
	if service.rateLimiter == nil {
		t.Error("Expected rate limiter to be set after SetRateLimiter call")
	}
}

func TestGeminiCLIService_CheckRateLimit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	// Create a temporary executable file for testing
	tmpFile, err := os.CreateTemp("", "gemini-cli-test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()
	
	service, err := NewGeminiCLIService(tmpFile.Name(), logger)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	
	// Test checkRateLimit with Normal status
	config := monitor.ProviderConfig{
		ProviderID: "gemini",
		Limits: map[string]int{
			"minute": 10,
		},
		Thresholds: map[string]float64{
			"warning":   0.75,
			"throttled": 1.0,
		},
	}
	
	rateLimitManager := monitor.NewRateLimitManager(logger, []monitor.ProviderConfig{config})
	service.SetRateLimiter(rateLimitManager)
	
	// Should pass when status is Normal
	err = service.checkRateLimit()
	if err != nil {
		t.Errorf("Expected checkRateLimit to pass with Normal status, got error: %v", err)
	}
	
	// Register calls to reach warning threshold (7.5/10 = 75%)
	for i := 0; i < 7; i++ {
		rateLimitManager.RegisterCall("gemini")
	}
	
	// Should still pass with Warning status (doesn't block)
	err = service.checkRateLimit()
	if err != nil {
		t.Errorf("Expected checkRateLimit to pass with Warning status, got error: %v", err)
	}
	
	// Register more calls to reach throttled threshold
	for i := 0; i < 3; i++ {
		rateLimitManager.RegisterCall("gemini")
	}
	
	// Should fail with Throttled status
	err = service.checkRateLimit()
	if err == nil {
		t.Error("Expected checkRateLimit to fail with Throttled status")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && 
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		 func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		 }())))
}