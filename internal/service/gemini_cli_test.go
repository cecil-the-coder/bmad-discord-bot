package service

import (
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNewGeminiCLIService(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tests := []struct {
		name        string
		cliPath     string
		expectError bool
	}{
		{
			name:        "empty CLI path",
			cliPath:     "",
			expectError: true,
		},
		{
			name:        "non-existent CLI path",
			cliPath:     "/non/existent/path",
			expectError: true,
		},
		{
			name:        "valid CLI path (this binary)",
			cliPath:     os.Args[0], // Use the test binary itself as a valid file
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewGeminiCLIService(tt.cliPath, logger)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				if service != nil {
					t.Errorf("expected nil service but got %v", service)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if service == nil {
					t.Errorf("expected service but got nil")
				} else {
					// Verify default timeout is set
					if service.timeout != 30*time.Second {
						t.Errorf("expected default timeout 30s, got %v", service.timeout)
					}
				}
			}
		})
	}
}

func TestGeminiCLIService_QueryAI(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	// Create a mock script for testing
	mockScript := createMockGeminiScript(t)
	defer os.Remove(mockScript)
	
	service, err := NewGeminiCLIService(mockScript, logger)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	tests := []struct {
		name        string
		query       string
		expectError bool
		expectedMsg string
	}{
		{
			name:        "empty query",
			query:       "",
			expectError: true,
		},
		{
			name:        "whitespace only query",
			query:       "   \n\t  ",
			expectError: true,
		},
		{
			name:        "valid query",
			query:       "Hello, how are you?",
			expectError: false,
			expectedMsg: "Mock response for: Hello, how are you?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := service.QueryAI(tt.query)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if !strings.Contains(response, tt.expectedMsg) {
					t.Errorf("expected response to contain %q, got %q", tt.expectedMsg, response)
				}
			}
		})
	}
}

func TestGeminiCLIService_SetTimeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service, err := NewGeminiCLIService(os.Args[0], logger)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	newTimeout := 5 * time.Second
	service.SetTimeout(newTimeout)
	
	if service.timeout != newTimeout {
		t.Errorf("expected timeout %v, got %v", newTimeout, service.timeout)
	}
}

// createMockGeminiScript creates a temporary script that mimics gemini-cli behavior
func createMockGeminiScript(t *testing.T) string {
	// Create a temporary shell script that echoes the input
	scriptContent := `#!/bin/bash
echo "Mock response for: $2"
`
	
	tmpFile, err := os.CreateTemp("", "mock-gemini-*.sh")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	
	if _, err := tmpFile.WriteString(scriptContent); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}
	
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}
	
	// Make the script executable
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		t.Fatalf("failed to make script executable: %v", err)
	}
	
	return tmpFile.Name()
}

func TestGeminiCLIService_SummarizeQuery(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	// Create a mock script for testing summarization
	mockScript := createMockSummarizationScript(t)
	defer os.Remove(mockScript)
	
	service, err := NewGeminiCLIService(mockScript, logger)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	tests := []struct {
		name        string
		query       string
		expectError bool
		expectFallback bool
	}{
		{
			name:        "empty query",
			query:       "",
			expectError: true,
		},
		{
			name:        "whitespace only query",
			query:       "   \n\t  ",
			expectError: true,
		},
		{
			name:        "valid query",
			query:       "What is the weather like today?",
			expectError: false,
		},
		{
			name:        "short query",
			query:       "Hello",
			expectError: false,
		},
		{
			name:        "very long query",
			query:       "This is a very long question that should be summarized appropriately for Discord thread titles and should not exceed the 100 character limit that Discord imposes on thread names",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary, err := service.SummarizeQuery(tt.query)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				
				// Verify summary meets Discord requirements
				if len(summary) > 100 {
					t.Errorf("summary too long: %d characters (max 100)", len(summary))
				}
				
				if summary == "" {
					t.Errorf("summary should not be empty for valid input")
				}
				
				t.Logf("Query: %q -> Summary: %q", tt.query, summary)
			}
		})
	}
}

func TestGeminiCLIService_SummarizeQuery_FallbackBehavior(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	// Create a mock script that fails to test fallback behavior
	mockScript := createFailingMockScript(t)
	defer os.Remove(mockScript)
	
	service, err := NewGeminiCLIService(mockScript, logger)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	tests := []struct {
		name         string
		query        string
		expectedFallback string
	}{
		{
			name:         "simple question",
			query:        "What is Go?",
			expectedFallback: "What is Go?",
		},
		{
			name:         "long question uses fallback",
			query:        "This is a very long question that will trigger the fallback summarization logic because the AI service fails",
			expectedFallback: "This is a very long question that will trigger the fallback summarization logic because the AI...",
		},
		{
			name:         "empty query fallback",
			query:        "",
			expectedFallback: "", // Will error before fallback
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary, err := service.SummarizeQuery(tt.query)
			
			if tt.query == "" {
				// Should error before reaching fallback
				if err == nil {
					t.Errorf("expected error for empty query")
				}
				return
			}
			
			// Should not error (fallback handles the failure)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			
			// Should get fallback summary
			if summary != tt.expectedFallback {
				t.Errorf("expected fallback %q, got %q", tt.expectedFallback, summary)
			}
			
			t.Logf("Fallback test - Query: %q -> Summary: %q", tt.query, summary)
		})
	}
}

func TestGeminiCLIService_FallbackSummarize(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service, err := NewGeminiCLIService(os.Args[0], logger)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	tests := []struct {
		name     string
		query    string
		expected string
	}{
		{
			name:     "empty query",
			query:    "",
			expected: "Question",
		},
		{
			name:     "single word",
			query:    "Hello",
			expected: "Hello",
		},
		{
			name:     "short question",
			query:    "What is Go?",
			expected: "What is Go?",
		},
		{
			name:     "long question with truncation",
			query:    "What is the best way to learn programming and become a software engineer in the modern technology industry",
			expected: "What is the best way to learn programming and become a software engineer in the modern...",
		},
		{
			name:     "whitespace only",
			query:    "   \t\n  ",
			expected: "Question",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.fallbackSummarize(tt.query)
			
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
			
			// Verify length constraints
			if len(result) > 100 {
				t.Errorf("result too long: %d characters", len(result))
			}
		})
	}
}

// createMockSummarizationScript creates a script that returns a summary
func createMockSummarizationScript(t *testing.T) string {
	scriptContent := `#!/bin/bash
# Check if the prompt contains summarization request
if [[ "$2" == *"Create a concise summary"* ]]; then
    echo "Summary of question"
else
    echo "Mock response for: $2"
fi
`
	
	tmpFile, err := os.CreateTemp("", "mock-summary-*.sh")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	
	if _, err := tmpFile.WriteString(scriptContent); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}
	
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}
	
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		t.Fatalf("failed to make script executable: %v", err)
	}
	
	return tmpFile.Name()
}

// createFailingMockScript creates a script that always fails
func createFailingMockScript(t *testing.T) string {
	scriptContent := `#!/bin/bash
exit 1
`
	
	tmpFile, err := os.CreateTemp("", "mock-fail-*.sh")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	
	if _, err := tmpFile.WriteString(scriptContent); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}
	
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}
	
	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		t.Fatalf("failed to make script executable: %v", err)
	}
	
	return tmpFile.Name()
}