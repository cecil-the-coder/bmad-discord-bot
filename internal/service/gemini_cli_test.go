package service

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"bmad-knowledge-bot/internal/monitor"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRateLimiter implements AIProviderRateLimiter interface for testing
type mockRateLimiter struct {
	status string
	usage  int
	limit  int
}

func (m *mockRateLimiter) RegisterCall(providerID string) error {
	return nil
}

func (m *mockRateLimiter) GetProviderStatus(providerID string) string {
	return m.status
}

func (m *mockRateLimiter) GetProviderUsage(providerID string) (usage int, limit int) {
	return m.usage, m.limit
}

func (m *mockRateLimiter) CleanupOldCalls(providerID string) {
	// No-op for testing
}

func (m *mockRateLimiter) GetProviderState(providerID string) (*monitor.ProviderRateLimitState, bool) {
	// Return nil for testing
	return nil, false
}

// setupTestServiceWithBMAD creates a test service with a temporary BMAD prompt file
func setupTestServiceWithBMAD(t *testing.T) (*GeminiCLIService, string) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create a temporary BMAD prompt file
	bmadPath := filepath.Join(tmpDir, "bmadprompt.md")
	bmadContent := "Test BMAD knowledge base content"
	err := os.WriteFile(bmadPath, []byte(bmadContent), 0644)
	require.NoError(t, err)

	// Set environment variable
	t.Setenv("BMAD_PROMPT_PATH", bmadPath)

	// Create a temporary executable
	execPath := filepath.Join(tmpDir, "gemini-cli")
	err = os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0755)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service, err := NewGeminiCLIService(execPath, logger)
	require.NoError(t, err)

	return service, tmpDir
}

func TestGeminiCLIService_GetProviderID(t *testing.T) {
	service, _ := setupTestServiceWithBMAD(t)

	providerID := service.GetProviderID()
	if providerID != "gemini" {
		t.Errorf("Expected provider ID 'gemini', got '%s'", providerID)
	}
}

func TestGeminiCLIService_RateLimitIntegration(t *testing.T) {
	service, _ := setupTestServiceWithBMAD(t)

	// Create a mock rate limiter
	mockRateLimiter := &mockRateLimiter{
		status: "Normal",
		usage:  5,
		limit:  10,
	}

	// Set the rate limiter
	service.SetRateLimiter(mockRateLimiter)

	// Verify the rate limiter is set and functional
	if service.rateLimiter == nil {
		t.Error("Rate limiter was not set")
	}

	// Test that rate limit check is performed
	err := service.checkRateLimit()
	if err != nil {
		t.Errorf("Expected no error for Normal status, got: %v", err)
	}

	// Test throttled state
	mockRateLimiter.status = "Throttled"
	err = service.checkRateLimit()
	if err == nil {
		t.Error("Expected error for Throttled status, got nil")
	}
}

func TestGeminiCLIService_RateLimitGracefulDegradation(t *testing.T) {
	service, _ := setupTestServiceWithBMAD(t)

	// Test without rate limiter (nil)
	err := service.checkRateLimit()
	if err != nil {
		t.Errorf("Expected no error when rate limiter is nil, got: %v", err)
	}

	// Test QueryAI without rate limiter should work
	// Note: This would normally execute the CLI, but our test executable just echoes
	response, err := service.QueryAI("test query")
	if err != nil {
		t.Errorf("Expected QueryAI to work without rate limiter, got error: %v", err)
	}
	if response == "" {
		t.Error("Expected non-empty response")
	}
}

func TestGeminiCLIService_SetRateLimiter(t *testing.T) {
	service, _ := setupTestServiceWithBMAD(t)

	// Initially, rate limiter should be nil
	if service.rateLimiter != nil {
		t.Error("Expected rate limiter to be nil initially")
	}

	// Create and set a mock rate limiter
	mockRateLimiter := &mockRateLimiter{
		status: "Normal",
		usage:  0,
		limit:  10,
	}

	service.SetRateLimiter(mockRateLimiter)

	// Verify it was set
	if service.rateLimiter == nil {
		t.Error("Expected rate limiter to be set")
	}
}

func TestGeminiCLIService_CheckRateLimit(t *testing.T) {
	service, _ := setupTestServiceWithBMAD(t)

	testCases := []struct {
		name          string
		status        string
		expectError   bool
		errorContains string
	}{
		{
			name:        "Normal status",
			status:      "Normal",
			expectError: false,
		},
		{
			name:        "Warning status",
			status:      "Warning",
			expectError: false,
		},
		{
			name:          "Throttled status",
			status:        "Throttled",
			expectError:   true,
			errorContains: "rate limit exceeded",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRateLimiter := &mockRateLimiter{
				status: tc.status,
				usage:  8,
				limit:  10,
			}
			service.SetRateLimiter(mockRateLimiter)

			err := service.checkRateLimit()

			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if !contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error to contain '%s', got: %s", tc.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
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

func TestNewGeminiCLIService_BMADKnowledgeBase(t *testing.T) {
	// Create a temporary BMAD prompt file
	tmpDir := t.TempDir()
	bmadPath := filepath.Join(tmpDir, "bmadprompt.md")
	bmadContent := `You are an expert on the BMAD-METHOD. Your knowledge is based solely on the provided text below.

# BMAD Knowledge Base

## Overview
[cite_start]BMAD-METHOD (Breakthrough Method of Agile AI-driven Development) is a framework that combines AI agents with Agile development methodologies. [cite: 85]`

	err := os.WriteFile(bmadPath, []byte(bmadContent), 0644)
	require.NoError(t, err)

	// Set environment variable
	t.Setenv("BMAD_PROMPT_PATH", bmadPath)

	// Create a temporary executable
	execPath := filepath.Join(tmpDir, "gemini-cli")
	err = os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0755)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service, err := NewGeminiCLIService(execPath, logger)

	assert.NoError(t, err)
	assert.NotNil(t, service)
	assert.Equal(t, bmadPath, service.bmadPromptPath)
	assert.Contains(t, service.bmadKnowledgeBase, "BMAD-METHOD")
	assert.Contains(t, service.bmadKnowledgeBase, "[cite: 85]")
}

func TestNewGeminiCLIService_MissingBMADFile(t *testing.T) {
	// Set environment variable to non-existent file
	t.Setenv("BMAD_PROMPT_PATH", "/non/existent/bmadprompt.md")

	// Create a temporary executable
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "gemini-cli")
	err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0755)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service, err := NewGeminiCLIService(execPath, logger)

	assert.Error(t, err)
	assert.Nil(t, service)
	assert.Contains(t, err.Error(), "failed to load BMAD knowledge base")
}

func TestBuildBMADPrompt(t *testing.T) {
	service := &GeminiCLIService{
		bmadKnowledgeBase: "Test BMAD knowledge base content",
	}

	userQuery := "What is BMAD-METHOD?"
	prompt := service.buildBMADPrompt(userQuery)

	assert.Contains(t, prompt, "Test BMAD knowledge base content")
	assert.Contains(t, prompt, "USER QUESTION: What is BMAD-METHOD?")
	assert.Contains(t, prompt, "Answer ONLY based on the information provided in the BMAD knowledge base")
	assert.Contains(t, prompt, "Maintain any citation markers")
}

func TestQueryAI_WithBMADConstraints(t *testing.T) {
	// Create a temporary script that simulates gemini-cli
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "gemini-cli")

	// Script that checks if BMAD prompt is included
	script := `#!/bin/sh
if echo "$2" | grep -q "BMAD knowledge base" && echo "$2" | grep -q "USER QUESTION:"; then
    echo "BMAD-METHOD is a framework [cite: 85]"
else
    echo "General response without BMAD context"
fi
`
	err := os.WriteFile(execPath, []byte(script), 0755)
	require.NoError(t, err)

	// Create BMAD prompt file
	bmadPath := filepath.Join(tmpDir, "bmadprompt.md")
	bmadContent := "You are an expert on the BMAD-METHOD."
	err = os.WriteFile(bmadPath, []byte(bmadContent), 0644)
	require.NoError(t, err)

	t.Setenv("BMAD_PROMPT_PATH", bmadPath)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service, err := NewGeminiCLIService(execPath, logger)
	require.NoError(t, err)

	response, err := service.QueryAI("What is BMAD?")

	assert.NoError(t, err)
	assert.Contains(t, response, "BMAD-METHOD is a framework")
	assert.Contains(t, response, "[cite: 85]")
}

func TestQueryWithContext_BMADConstraints(t *testing.T) {
	// Create a temporary script that simulates gemini-cli
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "gemini-cli")

	// Script that checks for BMAD context and conversation history
	script := `#!/bin/sh
if echo "$2" | grep -q "BMAD knowledge base" && echo "$2" | grep -q "CONVERSATION HISTORY:"; then
    echo "Based on BMAD knowledge and previous context [cite: 90]"
else
    echo "Response without proper context"
fi
`
	err := os.WriteFile(execPath, []byte(script), 0755)
	require.NoError(t, err)

	// Create BMAD prompt file
	bmadPath := filepath.Join(tmpDir, "bmadprompt.md")
	bmadContent := "You are an expert on the BMAD-METHOD."
	err = os.WriteFile(bmadPath, []byte(bmadContent), 0644)
	require.NoError(t, err)

	t.Setenv("BMAD_PROMPT_PATH", bmadPath)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service, err := NewGeminiCLIService(execPath, logger)
	require.NoError(t, err)

	conversationHistory := "User: What is BMAD?\nBot: BMAD is a framework for AI-driven development."
	response, err := service.QueryWithContext("Tell me more about that", conversationHistory)

	assert.NoError(t, err)
	assert.Contains(t, response, "Based on BMAD knowledge")
	assert.Contains(t, response, "[cite: 90]")
}
