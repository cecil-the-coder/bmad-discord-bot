package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// GeminiCLIService implements AIService interface using Google Gemini CLI
type GeminiCLIService struct {
	cliPath string
	timeout time.Duration
	logger  *slog.Logger
}

// NewGeminiCLIService creates a new Gemini CLI service instance
func NewGeminiCLIService(cliPath string, logger *slog.Logger) (*GeminiCLIService, error) {
	if cliPath == "" {
		return nil, fmt.Errorf("gemini CLI path cannot be empty")
	}

	// Validate that the CLI executable exists and is accessible
	if _, err := os.Stat(cliPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("gemini CLI not found at path: %s", cliPath)
	}

	return &GeminiCLIService{
		cliPath: cliPath,
		timeout: 30 * time.Second, // Default 30 second timeout
		logger:  logger,
	}, nil
}

// QueryAI sends a query to the Gemini CLI and returns the response
func (g *GeminiCLIService) QueryAI(query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	g.logger.Info("Sending query to Gemini CLI", "query_length", len(query))

	// Create context with timeout for command execution
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	// Execute gemini-cli command with the user's query using -p flag
	cmd := exec.CommandContext(ctx, g.cliPath, "-p", query)
	
	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		g.logger.Error("Gemini CLI execution failed", 
			"error", err, 
			"output", string(output))
		
		// Check if it's a timeout error
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("gemini CLI request timed out after %v", g.timeout)
		}
		
		return "", fmt.Errorf("gemini CLI error: %w", err)
	}

	responseText := strings.TrimSpace(string(output))
	if responseText == "" {
		g.logger.Warn("Gemini CLI returned empty response")
		return "I received an empty response from the AI service.", nil
	}

	g.logger.Info("Gemini CLI response received", 
		"response_length", len(responseText))

	return responseText, nil
}

// SummarizeQuery creates a summarized version of a user query suitable for Discord thread titles
func (g *GeminiCLIService) SummarizeQuery(query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	g.logger.Info("Creating query summary", "query_length", len(query))

	// Create a specialized prompt for summarization
	prompt := fmt.Sprintf("Create a concise summary of this question in 8 words or less, suitable for a Discord thread title. Focus on the main topic or question being asked. Do not include quotes or formatting. Question: %s", query)

	// Create context with timeout for command execution
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	// Execute gemini-cli command with the summarization prompt
	cmd := exec.CommandContext(ctx, g.cliPath, "-p", prompt)
	
	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		g.logger.Error("Gemini CLI summarization failed",
			"error", err,
			"output", string(output))
		
		// Check if it's a timeout error
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("gemini CLI summarization timed out after %v", g.timeout)
		}
		
		// Fallback to simple truncation if AI summarization fails
		g.logger.Warn("AI summarization failed, using fallback", "error", err)
		return g.fallbackSummarize(query), nil
	}

	summary := strings.TrimSpace(string(output))
	if summary == "" {
		g.logger.Warn("Gemini CLI returned empty summary, using fallback")
		return g.fallbackSummarize(query), nil
	}

	// Ensure summary fits Discord's 100 character limit for thread titles
	if len(summary) > 100 {
		summary = summary[:97] + "..."
	}

	g.logger.Info("Query summary created",
		"summary_length", len(summary),
		"summary", summary)

	return summary, nil
}

// fallbackSummarize provides a simple fallback summarization when AI fails
func (g *GeminiCLIService) fallbackSummarize(query string) string {
	// Simple fallback: take first few words and truncate to fit Discord limit
	words := strings.Fields(strings.TrimSpace(query))
	if len(words) == 0 {
		return "Question"
	}
	
	summary := ""
	for _, word := range words {
		testSummary := summary + " " + word
		if len(strings.TrimSpace(testSummary)) > 95 { // Leave room for "..."
			break
		}
		summary = testSummary
	}
	
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return "Question"
	}
	
	// Add ellipsis if we truncated
	if len(words) > len(strings.Fields(summary)) {
		summary += "..."
	}
	
	return summary
}

// SetTimeout allows customizing the CLI execution timeout
func (g *GeminiCLIService) SetTimeout(timeout time.Duration) {
	g.timeout = timeout
}