package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"bmad-knowledge-bot/internal/monitor"
)

// GeminiCLIService implements AIService interface using Google Gemini CLI
type GeminiCLIService struct {
	cliPath     string
	timeout     time.Duration
	logger      *slog.Logger
	rateLimiter monitor.AIProviderRateLimiter
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
		cliPath:     cliPath,
		timeout:     30 * time.Second, // Default 30 second timeout
		logger:      logger,
		rateLimiter: nil, // Will be set via SetRateLimiter
	}, nil
}

// SetRateLimiter sets the rate limiter for this service
func (g *GeminiCLIService) SetRateLimiter(rateLimiter monitor.AIProviderRateLimiter) {
	g.rateLimiter = rateLimiter
}

// QueryAI sends a query to the Gemini CLI and returns the response
func (g *GeminiCLIService) QueryAI(query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	// Check rate limit before proceeding
	if err := g.checkRateLimit(); err != nil {
		return "", err
	}

	g.logger.Info("Sending query to Gemini CLI",
		"provider", g.GetProviderID(),
		"query_length", len(query))

	// Register the API call for rate limiting
	if g.rateLimiter != nil {
		if err := g.rateLimiter.RegisterCall(g.GetProviderID()); err != nil {
			g.logger.Warn("Failed to register API call for rate limiting", "error", err)
		}
	}

	// Create context with timeout for command execution
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	// Execute gemini-cli command with the user's query using -p flag
	cmd := exec.CommandContext(ctx, g.cliPath, "-p", query)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		g.logger.Error("Gemini CLI execution failed",
			"provider", g.GetProviderID(),
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
		g.logger.Warn("Gemini CLI returned empty response", "provider", g.GetProviderID())
		return "I received an empty response from the AI service.", nil
	}

	g.logger.Info("Gemini CLI response received",
		"provider", g.GetProviderID(),
		"response_length", len(responseText))

	return responseText, nil
}

// SummarizeQuery creates a summarized version of a user query suitable for Discord thread titles
func (g *GeminiCLIService) SummarizeQuery(query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	// Check rate limit before proceeding
	if err := g.checkRateLimit(); err != nil {
		return "", err
	}

	g.logger.Info("Creating query summary",
		"provider", g.GetProviderID(),
		"query_length", len(query))

	// Register the API call for rate limiting
	if g.rateLimiter != nil {
		if err := g.rateLimiter.RegisterCall(g.GetProviderID()); err != nil {
			g.logger.Warn("Failed to register API call for rate limiting", "error", err)
		}
	}

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
			"provider", g.GetProviderID(),
			"error", err,
			"output", string(output))

		// Check if it's a timeout error
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("gemini CLI summarization timed out after %v", g.timeout)
		}

		// Fallback to simple truncation if AI summarization fails
		g.logger.Warn("AI summarization failed, using fallback", "provider", g.GetProviderID(), "error", err)
		return g.fallbackSummarize(query), nil
	}

	summary := strings.TrimSpace(string(output))
	if summary == "" {
		g.logger.Warn("Gemini CLI returned empty summary, using fallback", "provider", g.GetProviderID())
		return g.fallbackSummarize(query), nil
	}

	// Ensure summary fits Discord's 100 character limit for thread titles
	if len(summary) > 100 {
		summary = summary[:97] + "..."
	}

	g.logger.Info("Query summary created",
		"provider", g.GetProviderID(),
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

// QueryWithContext sends a query with conversation history context to the AI service
func (g *GeminiCLIService) QueryWithContext(query string, conversationHistory string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	// Check rate limit before proceeding
	if err := g.checkRateLimit(); err != nil {
		return "", err
	}

	g.logger.Info("Sending contextual query to Gemini CLI",
		"provider", g.GetProviderID(),
		"query_length", len(query),
		"history_length", len(conversationHistory))

	// Register the API call for rate limiting
	if g.rateLimiter != nil {
		if err := g.rateLimiter.RegisterCall(g.GetProviderID()); err != nil {
			g.logger.Warn("Failed to register API call for rate limiting", "error", err)
		}
	}

	// Create a contextual prompt that includes conversation history
	var prompt string
	if strings.TrimSpace(conversationHistory) != "" {
		prompt = fmt.Sprintf(`You are continuing an ongoing conversation. Here is the conversation history:

%s

The user just asked: "%s"

Please respond to this follow-up question in the context of the previous conversation. If the question refers to something mentioned earlier (like "that city", "it", "there"), use the conversation history to understand what they're referring to. Maintain continuity with the previous discussion.`, conversationHistory, query)
	} else {
		// Fallback to regular query if no history
		prompt = query
	}

	// Create context with timeout for command execution
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	// Execute gemini-cli command with the contextual prompt
	cmd := exec.CommandContext(ctx, g.cliPath, "-p", prompt)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		g.logger.Error("Gemini CLI contextual execution failed",
			"provider", g.GetProviderID(),
			"error", err,
			"output", string(output))

		// Check if it's a timeout error
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("gemini CLI contextual request timed out after %v", g.timeout)
		}

		return "", fmt.Errorf("gemini CLI contextual error: %w", err)
	}

	responseText := strings.TrimSpace(string(output))
	if responseText == "" {
		g.logger.Warn("Gemini CLI returned empty contextual response", "provider", g.GetProviderID())
		return "I received an empty response from the AI service.", nil
	}

	g.logger.Info("Gemini CLI contextual response received",
		"provider", g.GetProviderID(),
		"response_length", len(responseText))

	return responseText, nil
}

// SummarizeConversation creates a summary of conversation history for context preservation
func (g *GeminiCLIService) SummarizeConversation(messages []string) (string, error) {
	if len(messages) == 0 {
		return "", nil
	}

	// Check rate limit before proceeding
	if err := g.checkRateLimit(); err != nil {
		return "", err
	}

	g.logger.Info("Summarizing conversation",
		"provider", g.GetProviderID(),
		"message_count", len(messages))

	// Register the API call for rate limiting
	if g.rateLimiter != nil {
		if err := g.rateLimiter.RegisterCall(g.GetProviderID()); err != nil {
			g.logger.Warn("Failed to register API call for rate limiting", "error", err)
		}
	}

	// Join messages into a single conversation text
	conversationText := strings.Join(messages, "\n")

	// Create a specialized prompt for conversation summarization
	prompt := fmt.Sprintf("Summarize this conversation in a concise way that preserves the key context and topics discussed. Focus on the main questions asked and important information shared. Keep it under 500 words:\n\n%s", conversationText)

	// Create context with timeout for command execution
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	// Execute gemini-cli command with the summarization prompt
	cmd := exec.CommandContext(ctx, g.cliPath, "-p", prompt)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		g.logger.Error("Gemini CLI conversation summarization failed",
			"provider", g.GetProviderID(),
			"error", err,
			"output", string(output))

		// Check if it's a timeout error
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("gemini CLI conversation summarization timed out after %v", g.timeout)
		}

		// Fallback to truncated conversation if AI summarization fails
		g.logger.Warn("AI conversation summarization failed, using fallback", "provider", g.GetProviderID(), "error", err)
		return g.fallbackConversationSummary(messages), nil
	}

	summary := strings.TrimSpace(string(output))
	if summary == "" {
		g.logger.Warn("Gemini CLI returned empty conversation summary, using fallback", "provider", g.GetProviderID())
		return g.fallbackConversationSummary(messages), nil
	}

	g.logger.Info("Conversation summary created",
		"provider", g.GetProviderID(),
		"summary_length", len(summary))

	return summary, nil
}

// fallbackConversationSummary provides a simple fallback when AI summarization fails
func (g *GeminiCLIService) fallbackConversationSummary(messages []string) string {
	if len(messages) == 0 {
		return ""
	}

	// Simple fallback: take the last few messages and truncate if needed
	const maxMessages = 5
	const maxLength = 1000

	startIdx := 0
	if len(messages) > maxMessages {
		startIdx = len(messages) - maxMessages
	}

	recentMessages := messages[startIdx:]
	summary := strings.Join(recentMessages, "\n")

	if len(summary) > maxLength {
		summary = summary[:maxLength-3] + "..."
	}

	return summary
}

// GetProviderID returns the unique identifier for this AI provider
func (g *GeminiCLIService) GetProviderID() string {
	return "gemini"
}

// checkRateLimit validates that the provider is not rate limited before making a call
func (g *GeminiCLIService) checkRateLimit() error {
	if g.rateLimiter == nil {
		// Rate limiting not configured - allow the call
		return nil
	}

	status := g.rateLimiter.GetProviderStatus(g.GetProviderID())
	if status == "Throttled" {
		usage, limit := g.rateLimiter.GetProviderUsage(g.GetProviderID())
		g.logger.Warn("Rate limit exceeded for provider",
			"provider", g.GetProviderID(),
			"status", status,
			"usage", usage,
			"limit", limit)
		return fmt.Errorf("rate limit exceeded for provider %s: %d/%d requests",
			g.GetProviderID(), usage, limit)
	}

	// Log warning status but don't block the call
	if status == "Warning" {
		usage, limit := g.rateLimiter.GetProviderUsage(g.GetProviderID())
		g.logger.Warn("Rate limit warning for provider",
			"provider", g.GetProviderID(),
			"status", status,
			"usage", usage,
			"limit", limit)
	}

	return nil
}

// SetTimeout allows customizing the CLI execution timeout
func (g *GeminiCLIService) SetTimeout(timeout time.Duration) {
	g.timeout = timeout
}
