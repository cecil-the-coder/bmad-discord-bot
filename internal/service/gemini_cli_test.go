package service

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	// Return a mock state for testing
	state := &monitor.ProviderRateLimitState{
		ProviderID:              providerID,
		DailyQuotaExhausted:     false,
		DailyQuotaResetTime:     time.Time{},
	}
	return state, true
}

func (m *mockRateLimiter) SetQuotaExhausted(providerID string, resetTime time.Time) {
	// Mock implementation - no-op for basic tests
}

func (m *mockRateLimiter) ClearQuotaExhaustion(providerID string) {
	// Mock implementation - no-op for basic tests
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

// ========== NEW TESTS FOR STORY 2.2 ==========

// mockQuotaRateLimiter extends mockRateLimiter to support quota exhaustion testing
type mockQuotaRateLimiter struct {
	*mockRateLimiter
	quotaExhausted bool
	resetTime      time.Time
}

func (m *mockQuotaRateLimiter) GetProviderState(providerID string) (*monitor.ProviderRateLimitState, bool) {
	state := &monitor.ProviderRateLimitState{
		ProviderID:              providerID,
		DailyQuotaExhausted:     m.quotaExhausted,
		DailyQuotaResetTime:     m.resetTime,
	}
	return state, true
}

func (m *mockQuotaRateLimiter) SetQuotaExhausted(providerID string, resetTime time.Time) {
	m.quotaExhausted = true
	m.resetTime = resetTime
}

func (m *mockQuotaRateLimiter) ClearQuotaExhaustion(providerID string) {
	m.quotaExhausted = false
	m.resetTime = time.Time{}
}

func (m *mockQuotaRateLimiter) GetProviderStatus(providerID string) string {
	if m.quotaExhausted {
		return "Quota Exhausted"
	}
	return m.mockRateLimiter.GetProviderStatus(providerID)
}

// Test AC 2.2.1: Daily Quota Detection
func TestGeminiCLIService_DailyQuotaDetection(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "gemini-cli")

	// Script that simulates 429 error with daily quota message
	script := `#!/bin/sh
echo "Error: 429 Too Many Requests - Quota exceeded for quota metric 'GeminiRequests' and limit '1000 per day'. Please try again later." >&2
exit 1
`
	err := os.WriteFile(execPath, []byte(script), 0755)
	require.NoError(t, err)

	// Setup service
	service, _ := setupTestServiceWithBMAD(t)
	service.cliPath = execPath

	// Setup mock rate limiter
	mockRL := &mockQuotaRateLimiter{
		mockRateLimiter: &mockRateLimiter{status: "Normal"},
	}
	service.SetRateLimiter(mockRL)

	// Test QueryAI - should detect daily quota exhaustion
	_, err = service.QueryAI("test query")

	// Verify error indicates daily quota exhaustion
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "daily quota exhausted for Gemini API")
	assert.Contains(t, err.Error(), "Service will be restored at")

	// Verify quota exhausted flag was set
	assert.True(t, mockRL.quotaExhausted)
	assert.False(t, mockRL.resetTime.IsZero())
}

// Test AC 2.2.2: Quota State Management
func TestGeminiCLIService_QuotaStateManagement(t *testing.T) {
	service, _ := setupTestServiceWithBMAD(t)

	mockRL := &mockQuotaRateLimiter{
		mockRateLimiter: &mockRateLimiter{status: "Normal"},
	}
	service.SetRateLimiter(mockRL)

	// Simulate quota exhaustion
	resetTime := time.Now().Add(24 * time.Hour)
	mockRL.SetQuotaExhausted("gemini", resetTime)

	// Test that calls are blocked when quota is exhausted
	err := service.checkRateLimit()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "daily quota exhausted")

	// Test automatic quota restoration after reset time
	mockRL.resetTime = time.Now().Add(-1 * time.Hour) // Past reset time
	err = service.checkRateLimit()
	assert.NoError(t, err)
	assert.False(t, mockRL.quotaExhausted) // Should be cleared
}

// Test AC 2.2.3: User-Facing Error Handling
func TestGeminiCLIService_UserFacingErrorHandling(t *testing.T) {
	service, _ := setupTestServiceWithBMAD(t)

	mockRL := &mockQuotaRateLimiter{
		mockRateLimiter: &mockRateLimiter{status: "Quota Exhausted"},
		quotaExhausted:  true,
		resetTime:       time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	}
	service.SetRateLimiter(mockRL)

	// Test QueryAI returns user-friendly message
	response, err := service.QueryAI("test query")
	assert.NoError(t, err)
	assert.Contains(t, response, "I've reached my daily quota for AI processing")
	assert.Contains(t, response, "Service will be restored tomorrow at")

	// Test SummarizeQuery returns user-friendly message
	response, err = service.SummarizeQuery("test query")
	assert.NoError(t, err)
	assert.Contains(t, response, "AI summarization is temporarily unavailable")

	// Test QueryWithContext returns user-friendly message
	response, err = service.QueryWithContext("test", "history")
	assert.NoError(t, err)
	assert.Contains(t, response, "I've reached my daily quota for AI processing")

	// Test SummarizeConversation returns user-friendly message
	response, err = service.SummarizeConversation([]string{"msg1", "msg2"})
	assert.NoError(t, err)
	assert.Contains(t, response, "AI conversation summarization is temporarily unavailable")
}

// Test AC 2.2.5: Graceful Service Restoration
func TestGeminiCLIService_GracefulServiceRestoration(t *testing.T) {
	service, _ := setupTestServiceWithBMAD(t)

	mockRL := &mockQuotaRateLimiter{
		mockRateLimiter: &mockRateLimiter{status: "Normal"},
		quotaExhausted:  true,
		resetTime:       time.Now().Add(-1 * time.Hour), // Past reset time
	}
	service.SetRateLimiter(mockRL)

	// Call checkRateLimit - should detect expired quota and clear it
	err := service.checkRateLimit()
	assert.NoError(t, err)
	assert.False(t, mockRL.quotaExhausted)
	assert.True(t, mockRL.resetTime.IsZero())
}

// Test AC 2.2.6: Integration with Rate Limiting
func TestGeminiCLIService_QuotaExhaustedStatus(t *testing.T) {
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
		{
			name:          "Quota Exhausted status",
			status:        "Quota Exhausted",
			expectError:   true,
			errorContains: "daily quota exhausted",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockRL := &mockQuotaRateLimiter{
				mockRateLimiter: &mockRateLimiter{
					status: tc.status,
					usage:  8,
					limit:  10,
				},
			}
			if tc.status == "Quota Exhausted" {
				mockRL.quotaExhausted = true
				mockRL.resetTime = time.Now().Add(1 * time.Hour)
			}
			service.SetRateLimiter(mockRL)

			err := service.checkRateLimit()

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test daily quota pattern detection using integration approach
func TestGeminiCLIService_QuotaPatternDetection(t *testing.T) {
	testCases := []struct {
		name          string
		errorMessage  string
		shouldDetect  bool
	}{
		{
			name:         "Standard daily quota error",
			errorMessage: "429 Too Many Requests - Quota exceeded for quota metric 'GeminiRequests' and limit '1000 per day'",
			shouldDetect: true,
		},
		{
			name:         "Alternative daily quota format",
			errorMessage: "429 Too Many Requests - Quota exceeded for quota metric 'requests' and limit 'default per day'",
			shouldDetect: true,
		},
		{
			name:         "Per minute limit - should not detect",
			errorMessage: "429 Too Many Requests - Quota exceeded for quota metric 'requests' and limit '60 per minute'",
			shouldDetect: false,
		},
		{
			name:         "Per hour limit - should not detect", 
			errorMessage: "429 Too Many Requests - Quota exceeded for quota metric 'requests' and limit '1000 per hour'",
			shouldDetect: false,
		},
		{
			name:         "Regular rate limit - should not detect",
			errorMessage: "Too many requests, please try again later",
			shouldDetect: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			execPath := filepath.Join(tmpDir, "gemini-cli")

			// Create a script that simulates the specific error
			script := fmt.Sprintf(`#!/bin/sh
echo "%s" >&2
exit 1
`, tc.errorMessage)
			err := os.WriteFile(execPath, []byte(script), 0755)
			require.NoError(t, err)

			// Setup service
			service, _ := setupTestServiceWithBMAD(t)
			service.cliPath = execPath

			// Setup mock rate limiter
			mockRL := &mockQuotaRateLimiter{
				mockRateLimiter: &mockRateLimiter{status: "Normal"},
			}
			service.SetRateLimiter(mockRL)

			// Execute QueryAI and check if quota exhaustion was detected
			_, err = service.QueryAI("test query")

			if tc.shouldDetect {
				// Should detect daily quota and set the flag
				assert.True(t, mockRL.quotaExhausted, "Expected quota exhaustion to be detected for: %s", tc.errorMessage)
			} else {
				// Should not detect daily quota
				assert.False(t, mockRL.quotaExhausted, "Expected quota exhaustion NOT to be detected for: %s", tc.errorMessage)
			}
		})
	}
}

// Test quota reset time calculation through the actual logic
func TestGeminiCLIService_QuotaResetTimeCalculation(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "gemini-cli")

	// Script that simulates daily quota error
	script := `#!/bin/sh
echo "429 Too Many Requests - Quota exceeded for quota metric 'GeminiRequests' and limit '1000 per day'" >&2
exit 1
`
	err := os.WriteFile(execPath, []byte(script), 0755)
	require.NoError(t, err)

	// Setup service
	service, _ := setupTestServiceWithBMAD(t)
	service.cliPath = execPath

	// Setup mock rate limiter
	mockRL := &mockQuotaRateLimiter{
		mockRateLimiter: &mockRateLimiter{status: "Normal"},
	}
	service.SetRateLimiter(mockRL)

	// Execute QueryAI to trigger quota detection
	_, err = service.QueryAI("test query")
	
	// Verify the reset time was calculated correctly (should be next day at midnight UTC)
	assert.True(t, mockRL.quotaExhausted)
	assert.False(t, mockRL.resetTime.IsZero())
	
	// The reset time should be after now and should be at midnight UTC
	now := time.Now().UTC()
	assert.True(t, mockRL.resetTime.After(now))
	assert.Equal(t, 0, mockRL.resetTime.Hour())
	assert.Equal(t, 0, mockRL.resetTime.Minute())
	assert.Equal(t, 0, mockRL.resetTime.Second())
}
