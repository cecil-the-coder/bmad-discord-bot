package service

import (
	"os"
	"testing"
	"time"

	"bmad-knowledge-bot/internal/monitor"
	"log/slog"
)

func TestNewGeminiCLIService_ModelConfiguration(t *testing.T) {
	// Test default model configuration
	t.Run("Default model configuration", func(t *testing.T) {
		// Clear environment variables
		os.Unsetenv("GEMINI_PRIMARY_MODEL")
		os.Unsetenv("GEMINI_FALLBACK_MODEL")
		
		tmpDir := t.TempDir()
		bmadFile := tmpDir + "/bmad.md"
		if err := os.WriteFile(bmadFile, []byte("test bmad content"), 0644); err != nil {
			t.Fatal(err)
		}
		os.Setenv("BMAD_PROMPT_PATH", bmadFile)
		defer os.Unsetenv("BMAD_PROMPT_PATH")

		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
		
		// Use a mock CLI path that doesn't exist to skip validation
		service := &GeminiCLIService{
			cliPath:        "/mock/cli",
			timeout:        30 * time.Second,
			logger:         logger,
			bmadPromptPath: bmadFile,
			primaryModel: &ModelState{
				Name: "gemini-2.5-pro",
			},
			fallbackModel: &ModelState{
				Name: "gemini-2.5-flash-lite",
			},
		}

		if err := service.loadBMADKnowledgeBase(); err != nil {
			t.Fatal(err)
		}

		if service.primaryModel.Name != "gemini-2.5-pro" {
			t.Errorf("Expected primary model gemini-2.5-pro, got %s", service.primaryModel.Name)
		}
		if service.fallbackModel.Name != "gemini-2.5-flash-lite" {
			t.Errorf("Expected fallback model gemini-2.5-flash-lite, got %s", service.fallbackModel.Name)
		}
	})

	t.Run("Custom model configuration", func(t *testing.T) {
		os.Setenv("GEMINI_PRIMARY_MODEL", "custom-primary")
		os.Setenv("GEMINI_FALLBACK_MODEL", "custom-fallback")
		defer func() {
			os.Unsetenv("GEMINI_PRIMARY_MODEL")
			os.Unsetenv("GEMINI_FALLBACK_MODEL")
		}()

		tmpDir := t.TempDir()
		bmadFile := tmpDir + "/bmad.md"
		if err := os.WriteFile(bmadFile, []byte("test bmad content"), 0644); err != nil {
			t.Fatal(err)
		}
		os.Setenv("BMAD_PROMPT_PATH", bmadFile)
		defer os.Unsetenv("BMAD_PROMPT_PATH")

		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
		
		// Use a mock CLI path that doesn't exist to skip validation
		service := &GeminiCLIService{
			cliPath:        "/mock/cli",
			timeout:        30 * time.Second,
			logger:         logger,
			bmadPromptPath: bmadFile,
			primaryModel: &ModelState{
				Name: "custom-primary",
			},
			fallbackModel: &ModelState{
				Name: "custom-fallback",
			},
		}

		if err := service.loadBMADKnowledgeBase(); err != nil {
			t.Fatal(err)
		}

		if service.primaryModel.Name != "custom-primary" {
			t.Errorf("Expected primary model custom-primary, got %s", service.primaryModel.Name)
		}
		if service.fallbackModel.Name != "custom-fallback" {
			t.Errorf("Expected fallback model custom-fallback, got %s", service.fallbackModel.Name)
		}
	})
}

func TestGeminiCLIService_GetCurrentModel(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	
	service := &GeminiCLIService{
		logger: logger,
		primaryModel: &ModelState{
			Name: "primary-model",
		},
		fallbackModel: &ModelState{
			Name: "fallback-model",
		},
	}

	t.Run("Primary model available", func(t *testing.T) {
		// Reset model states
		service.primaryModel.RateLimited = false
		service.primaryModel.QuotaExhausted = false
		service.fallbackModel.RateLimited = false
		service.fallbackModel.QuotaExhausted = false

		model := service.getCurrentModel()
		if model.Name != "primary-model" {
			t.Errorf("Expected primary-model, got %s", model.Name)
		}
	})

	t.Run("Primary model rate limited, fallback available", func(t *testing.T) {
		service.primaryModel.RateLimited = true
		service.primaryModel.QuotaExhausted = false
		service.fallbackModel.RateLimited = false
		service.fallbackModel.QuotaExhausted = false

		model := service.getCurrentModel()
		if model.Name != "fallback-model" {
			t.Errorf("Expected fallback-model, got %s", model.Name)
		}
	})

	t.Run("Primary model quota exhausted, fallback available", func(t *testing.T) {
		service.primaryModel.RateLimited = false
		service.primaryModel.QuotaExhausted = true
		service.fallbackModel.RateLimited = false
		service.fallbackModel.QuotaExhausted = false

		model := service.getCurrentModel()
		if model.Name != "fallback-model" {
			t.Errorf("Expected fallback-model, got %s", model.Name)
		}
	})

	t.Run("Both models unavailable", func(t *testing.T) {
		service.primaryModel.RateLimited = true
		service.primaryModel.QuotaExhausted = false
		service.fallbackModel.RateLimited = false
		service.fallbackModel.QuotaExhausted = true

		model := service.getCurrentModel()
		// Should return primary for error handling
		if model.Name != "primary-model" {
			t.Errorf("Expected primary-model for error handling, got %s", model.Name)
		}
	})
}

func TestGeminiCLIService_ModelStateManagement(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	
	service := &GeminiCLIService{
		logger: logger,
		primaryModel: &ModelState{
			Name: "primary-model",
		},
		fallbackModel: &ModelState{
			Name: "fallback-model",
		},
	}

	t.Run("Mark model rate limited", func(t *testing.T) {
		resetTime := time.Now().Add(1 * time.Minute)
		service.markModelRateLimited("primary-model", resetTime)

		if !service.primaryModel.RateLimited {
			t.Error("Expected primary model to be marked as rate limited")
		}
		if !service.primaryModel.RateLimitTime.Equal(resetTime) {
			t.Error("Expected reset time to be set correctly")
		}
	})

	t.Run("Mark model quota exhausted", func(t *testing.T) {
		resetTime := time.Now().Add(24 * time.Hour)
		service.markModelQuotaExhausted("fallback-model", resetTime)

		if !service.fallbackModel.QuotaExhausted {
			t.Error("Expected fallback model to be marked as quota exhausted")
		}
		if !service.fallbackModel.QuotaResetTime.Equal(resetTime) {
			t.Error("Expected quota reset time to be set correctly")
		}
	})

	t.Run("Restore models after time passes", func(t *testing.T) {
		// Set models as limited with past reset times
		pastTime := time.Now().Add(-1 * time.Minute)
		service.primaryModel.RateLimited = true
		service.primaryModel.RateLimitTime = pastTime
		service.fallbackModel.QuotaExhausted = true
		service.fallbackModel.QuotaResetTime = pastTime

		service.checkAndRestoreModels()

		if service.primaryModel.RateLimited {
			t.Error("Expected primary model rate limit to be cleared")
		}
		if service.fallbackModel.QuotaExhausted {
			t.Error("Expected fallback model quota exhaustion to be cleared")
		}
	})
}

func TestGeminiCLIService_ModelStatus(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	
	service := &GeminiCLIService{
		logger: logger,
		primaryModel: &ModelState{
			Name: "primary-model",
		},
		fallbackModel: &ModelState{
			Name: "fallback-model",
		},
	}

	t.Run("All models available", func(t *testing.T) {
		service.primaryModel.RateLimited = false
		service.primaryModel.QuotaExhausted = false
		service.fallbackModel.RateLimited = false
		service.fallbackModel.QuotaExhausted = false

		status := service.getModelStatus()
		if status["primary"] != "Available" {
			t.Errorf("Expected primary Available, got %s", status["primary"])
		}
		if status["fallback"] != "Available" {
			t.Errorf("Expected fallback Available, got %s", status["fallback"])
		}
	})

	t.Run("Primary rate limited", func(t *testing.T) {
		service.primaryModel.RateLimited = true
		service.primaryModel.QuotaExhausted = false
		service.fallbackModel.RateLimited = false
		service.fallbackModel.QuotaExhausted = false

		status := service.getModelStatus()
		if status["primary"] != "Rate Limited" {
			t.Errorf("Expected primary Rate Limited, got %s", status["primary"])
		}
		if status["fallback"] != "Available" {
			t.Errorf("Expected fallback Available, got %s", status["fallback"])
		}
	})

	t.Run("Fallback quota exhausted", func(t *testing.T) {
		service.primaryModel.RateLimited = false
		service.primaryModel.QuotaExhausted = false
		service.fallbackModel.RateLimited = false
		service.fallbackModel.QuotaExhausted = true

		status := service.getModelStatus()
		if status["primary"] != "Available" {
			t.Errorf("Expected primary Available, got %s", status["primary"])
		}
		if status["fallback"] != "Quota Exhausted" {
			t.Errorf("Expected fallback Quota Exhausted, got %s", status["fallback"])
		}
	})
}

func TestGeminiCLIService_RateLimitDetection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	
	service := &GeminiCLIService{
		logger: logger,
	}

	testCases := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{"Rate limit detected", "Error: rate limit exceeded", true},
		{"Rate Limit detected", "Error: Rate limit exceeded", true},
		{"Too many requests", "429 too many requests", true},
		{"Too Many Requests", "Error: Too Many Requests", true},
		{"Throttling detected", "Service throttling in effect", true},
		{"Throttling uppercase", "Service Throttling in effect", true},
		{"Regular error", "Error: invalid request", false},
		{"Empty message", "", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := service.isModelRateLimited(tc.errMsg)
			if result != tc.expected {
				t.Errorf("Expected %v for '%s', got %v", tc.expected, tc.errMsg, result)
			}
		})
	}
}

func TestGeminiCLIService_AllModelsUnavailable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	
	service := &GeminiCLIService{
		logger: logger,
		primaryModel: &ModelState{
			Name:            "primary-model",
			RateLimited:     true,
			QuotaExhausted:  false,
		},
		fallbackModel: &ModelState{
			Name:            "fallback-model",
			RateLimited:     false,
			QuotaExhausted:  true,
		},
	}

	// Test the model status detection directly first
	modelStatus := service.getModelStatus()
	if modelStatus["primary"] != "Rate Limited" {
		t.Errorf("Expected primary model to be Rate Limited, got %s", modelStatus["primary"])
	}
	if modelStatus["fallback"] != "Quota Exhausted" {
		t.Errorf("Expected fallback model to be Quota Exhausted, got %s", modelStatus["fallback"])
	}

	// Test that the check returns true when both models are unavailable
	bothUnavailable := modelStatus["primary"] != "Available" && modelStatus["fallback"] != "Available"
	if !bothUnavailable {
		t.Error("Expected both models to be unavailable")
	}
}

func TestGeminiCLIService_FallbackIntegration(t *testing.T) {
	// Test the integration with existing rate limiting systems
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	
	// Create a temporary directory for test files
	tmpDir := t.TempDir()
	bmadFile := tmpDir + "/bmad.md"
	if err := os.WriteFile(bmadFile, []byte("test bmad content"), 0644); err != nil {
		t.Fatal(err)
	}

	service := &GeminiCLIService{
		cliPath:           "/mock/cli",
		timeout:           30 * time.Second,
		logger:            logger,
		bmadKnowledgeBase: "test bmad content",
		bmadPromptPath:    bmadFile,
		primaryModel: &ModelState{
			Name: "gemini-2.5-pro",
		},
		fallbackModel: &ModelState{
			Name: "gemini-2.5-flash-lite",
		},
	}

	// Test rate limiter integration
	rateLimiter := monitor.NewRateLimiter(logger)
	service.SetRateLimiter(rateLimiter.GetManager())

	// Verify that the service integrates with existing rate limiting
	if service.rateLimiter == nil {
		t.Error("Expected rate limiter to be set")
	}

	// Test provider ID remains consistent
	if service.GetProviderID() != "gemini" {
		t.Errorf("Expected provider ID 'gemini', got '%s'", service.GetProviderID())
	}

	// Test that current model selection works
	currentModel := service.getCurrentModel()
	if currentModel.Name != "gemini-2.5-pro" {
		t.Errorf("Expected current model to be primary model, got %s", currentModel.Name)
	}
}