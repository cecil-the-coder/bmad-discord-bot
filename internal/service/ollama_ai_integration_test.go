package service

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"bmad-knowledge-bot/internal/monitor"
)

// TestOllamaAIServiceIntegration tests the complete integration of Ollama AI service
func TestOllamaAIServiceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a comprehensive mock Ollama server that simulates real API behavior
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}

		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req OllamaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Simulate different responses based on the prompt content
		var responseText string
		if strings.Contains(req.Prompt, "test") {
			responseText = "test response"
		} else if strings.Contains(req.Prompt, "Create a concise summary") && strings.Contains(req.Prompt, "development process") {
			responseText = "BMAD Development Process"
		} else if strings.Contains(req.Prompt, "Summarize this BMAD-METHOD conversation") {
			responseText = "The conversation covered BMAD agents, their roles, and development workflows."
		} else if strings.Contains(req.Prompt, "CONVERSATION HISTORY") {
			responseText = "Based on the previous discussion about agents, here's more information about BMAD roles."
		} else if strings.Contains(req.Prompt, "BMAD") {
			responseText = "BMAD-METHOD is a framework for AI-driven development. [SUMMARY]: BMAD Framework Overview"
		} else {
			responseText = "Based on the BMAD knowledge base, here is the information requested."
		}

		response := OllamaResponse{
			Model:    req.Model,
			Response: responseText,
			Done:     true,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	// Create a test BMAD knowledge base file
	tempFile, err := os.CreateTemp("", "integration_bmad_*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	testKnowledge := `# BMAD Knowledge Base

## Overview
BMAD-METHOD is a framework that combines AI agents with Agile development methodologies.

## Agents
- PM: Product Manager
- Dev: Developer
- Architect: Solution Architect
- QA: Quality Assurance

## Workflows
The system follows structured workflows for development projects.`

	if _, err := tempFile.WriteString(testKnowledge); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tempFile.Close()

	// Set up environment for integration test
	os.Setenv("OLLAMA_HOST", mockServer.URL)
	os.Setenv("OLLAMA_MODEL", "devstral")
	os.Setenv("OLLAMA_TIMEOUT", "30")
	os.Setenv("BMAD_PROMPT_PATH", tempFile.Name())
	defer func() {
		os.Unsetenv("OLLAMA_HOST")
		os.Unsetenv("OLLAMA_MODEL")
		os.Unsetenv("OLLAMA_TIMEOUT")
		os.Unsetenv("BMAD_PROMPT_PATH")
	}()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Test service initialization
	service, err := NewOllamaAIService(logger)
	if err != nil {
		t.Fatalf("Failed to create Ollama AI service: %v", err)
	}

	// Test rate limiter integration
	rateLimiterConfigs := []monitor.ProviderConfig{
		{
			ProviderID: "ollama",
			Limits: map[string]int{
				"minute": 60,
				"day":    1000,
			},
			Thresholds: map[string]float64{
				"warning":   0.75,
				"throttled": 1.0,
			},
		},
	}

	rateLimitManager := monitor.NewRateLimitManager(logger, rateLimiterConfigs)
	service.SetRateLimiter(rateLimitManager)

	// Test 1: Basic Query
	t.Run("BasicQuery", func(t *testing.T) {
		response, err := service.QueryAI("What is BMAD?")
		if err != nil {
			t.Fatalf("QueryAI failed: %v", err)
		}

		if !strings.Contains(response, "BMAD-METHOD") {
			t.Errorf("Response should contain 'BMAD-METHOD', got: %s", response)
		}

		// Verify citations are cleaned
		if strings.Contains(response, "[cite:") {
			t.Errorf("Citations should be cleaned from response")
		}
	})

	// Test 2: Query with Summary
	t.Run("QueryWithSummary", func(t *testing.T) {
		mainAnswer, summary, err := service.QueryAIWithSummary("Tell me about BMAD agents")
		if err != nil {
			t.Fatalf("QueryAIWithSummary failed: %v", err)
		}

		if mainAnswer == "" {
			t.Errorf("Main answer should not be empty")
		}

		if summary != "BMAD Framework Overview" {
			t.Errorf("Expected specific summary, got: %s", summary)
		}

		// Verify summary length constraint
		if len(summary) > 100 {
			t.Errorf("Summary should be <= 100 characters, got %d characters", len(summary))
		}
	})

	// Test 3: Query Summarization
	t.Run("SummarizeQuery", func(t *testing.T) {
		summary, err := service.SummarizeQuery("How do BMAD agents work in the development process?")
		if err != nil {
			t.Fatalf("SummarizeQuery failed: %v", err)
		}

		if summary == "" {
			t.Errorf("Summary should not be empty")
		}

		if len(summary) > 100 {
			t.Errorf("Summary should be <= 100 characters, got %d characters", len(summary))
		}

		expectedSummary := "BMAD Development Process"
		if summary != expectedSummary {
			t.Errorf("Expected summary '%s', got '%s'", expectedSummary, summary)
		}
	})

	// Test 4: Contextual Query
	t.Run("QueryWithContext", func(t *testing.T) {
		conversationHistory := "User: What are BMAD agents?\nBot: BMAD agents are specialized AI roles."

		response, err := service.QueryWithContext("Tell me more about their workflows", conversationHistory)
		if err != nil {
			t.Fatalf("QueryWithContext failed: %v", err)
		}

		if response == "" {
			t.Errorf("Response should not be empty")
		}

		// The mock server should return contextual response for queries with history
		if !strings.Contains(response, "Based on the previous discussion") {
			t.Errorf("Response should reference previous discussion, got: %s", response)
		}
	})

	// Test 5: Conversation Summarization
	t.Run("SummarizeConversation", func(t *testing.T) {
		messages := []string{
			"User: What is BMAD?",
			"Bot: BMAD is a framework for AI-driven development.",
			"User: How do the agents work?",
			"Bot: Agents have specialized roles like PM, Dev, and Architect.",
		}

		summary, err := service.SummarizeConversation(messages)
		if err != nil {
			t.Fatalf("SummarizeConversation failed: %v", err)
		}

		if summary == "" {
			t.Errorf("Summary should not be empty")
		}

		expectedContent := "BMAD agents"
		if !strings.Contains(summary, expectedContent) {
			t.Errorf("Summary should contain '%s', got: %s", expectedContent, summary)
		}
	})

	// Test 6: Rate Limiting Integration
	t.Run("RateLimitingIntegration", func(t *testing.T) {
		// Get initial usage count
		initialUsage, _ := rateLimitManager.GetProviderUsage("ollama")

		// Make multiple rapid calls to test rate limiting
		for i := 0; i < 5; i++ {
			_, err := service.QueryAI("Test query " + string(rune(i+'1')))
			if err != nil {
				t.Fatalf("Query %d failed: %v", i+1, err)
			}
		}

		// Check rate limiter state (should be initial + 5)
		usage, limit := rateLimitManager.GetProviderUsage("ollama")
		expectedUsage := initialUsage + 5
		if usage != expectedUsage {
			t.Errorf("Expected usage of %d, got %d", expectedUsage, usage)
		}

		if limit != 60 {
			t.Errorf("Expected limit of 60, got %d", limit)
		}

		status := rateLimitManager.GetProviderStatus("ollama")
		if status != "Normal" {
			t.Errorf("Expected status 'Normal', got '%s'", status)
		}
	})

	// Test 7: Error Handling and Recovery
	t.Run("ErrorHandlingAndRecovery", func(t *testing.T) {
		// Test empty query
		_, err := service.QueryAI("")
		if err == nil {
			t.Errorf("Expected error for empty query")
		}

		// Test service recovery after error
		response, err := service.QueryAI("Recovery test")
		if err != nil {
			t.Fatalf("Service should recover after error: %v", err)
		}

		if response == "" {
			t.Errorf("Response should not be empty after recovery")
		}
	})

	// Test 8: BMAD Knowledge Base Integration
	t.Run("BMADKnowledgeBaseIntegration", func(t *testing.T) {
		// The service should have loaded the BMAD knowledge base
		if service.bmadKnowledgeBase == "" {
			t.Errorf("BMAD knowledge base should be loaded")
		}

		if !strings.Contains(service.bmadKnowledgeBase, "BMAD-METHOD") {
			t.Errorf("BMAD knowledge base should contain expected content")
		}

		// Test that queries include the knowledge base
		response, err := service.QueryAI("What frameworks are available?")
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}

		// The mock server should return content that shows the knowledge base was included
		if response == "" {
			t.Errorf("Response should not be empty")
		}
	})

	// Test 9: Provider ID and Configuration
	t.Run("ProviderConfiguration", func(t *testing.T) {
		if service.GetProviderID() != "ollama" {
			t.Errorf("Expected provider ID 'ollama', got '%s'", service.GetProviderID())
		}

		// Test timeout configuration
		originalTimeout := service.timeout
		service.SetTimeout(45 * time.Second)
		if service.timeout != 45*time.Second {
			t.Errorf("Expected timeout 45s, got %v", service.timeout)
		}

		// Restore original timeout
		service.SetTimeout(originalTimeout)
	})

	// Test 10: Fallback Mechanisms
	t.Run("FallbackMechanisms", func(t *testing.T) {
		// Test fallback summarization
		longQuery := strings.Repeat("This is a very long query that exceeds normal length limits ", 10)
		summary := service.fallbackSummarize(longQuery)

		if len(summary) > 100 {
			t.Errorf("Fallback summary should be <= 100 characters, got %d", len(summary))
		}

		if summary == "" {
			t.Errorf("Fallback summary should not be empty")
		}

		// Test fallback conversation summary
		longMessages := make([]string, 20)
		for i := range longMessages {
			longMessages[i] = "Message " + string(rune(i+'1')) + ": This is a test message."
		}

		convSummary := service.fallbackConversationSummary(longMessages)
		if convSummary == "" {
			t.Errorf("Fallback conversation summary should not be empty")
		}

		if len(convSummary) > 1000 {
			t.Errorf("Fallback conversation summary should be <= 1000 characters, got %d", len(convSummary))
		}
	})
}

// TestOllamaAIServiceRealEndpoint tests against the actual Ollama endpoint (optional)
func TestOllamaAIServiceRealEndpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real endpoint test in short mode")
	}

	// This test is optional and only runs if explicitly enabled
	if os.Getenv("TEST_REAL_OLLAMA") != "true" {
		t.Skip("Skipping real Ollama endpoint test (set TEST_REAL_OLLAMA=true to enable)")
	}

	// Create a temporary BMAD knowledge base file
	tempFile, err := os.CreateTemp("", "real_bmad_*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	testKnowledge := "# BMAD Knowledge Base\n\nBMAD-METHOD is a framework for AI-driven development."
	if _, err := tempFile.WriteString(testKnowledge); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tempFile.Close()

	// Set environment variables for real endpoint
	os.Setenv("OLLAMA_HOST", "https://ollama")
	os.Setenv("OLLAMA_MODEL", "devstral")
	os.Setenv("OLLAMA_TIMEOUT", "60")
	os.Setenv("BMAD_PROMPT_PATH", tempFile.Name())
	defer func() {
		os.Unsetenv("OLLAMA_HOST")
		os.Unsetenv("OLLAMA_MODEL")
		os.Unsetenv("OLLAMA_TIMEOUT")
		os.Unsetenv("BMAD_PROMPT_PATH")
	}()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	service, err := NewOllamaAIService(logger)
	if err != nil {
		t.Fatalf("Failed to create service for real endpoint: %v", err)
	}

	// Test a simple query
	response, err := service.QueryAI("What is BMAD in one sentence?")
	if err != nil {
		t.Fatalf("Real endpoint query failed: %v", err)
	}

	if response == "" {
		t.Errorf("Response should not be empty")
	}

	t.Logf("Real endpoint response: %s", response)

	// Test query with summary
	mainAnswer, summary, err := service.QueryAIWithSummary("Explain BMAD agents briefly")
	if err != nil {
		t.Fatalf("Real endpoint query with summary failed: %v", err)
	}

	if mainAnswer == "" {
		t.Errorf("Main answer should not be empty")
	}

	t.Logf("Real endpoint main answer: %s", mainAnswer)
	t.Logf("Real endpoint summary: %s", summary)
}
