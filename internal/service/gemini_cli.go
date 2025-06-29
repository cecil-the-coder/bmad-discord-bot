package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp" // Added for regex matching on error messages
	"strings"
	"sync"
	"time"

	"bmad-knowledge-bot/internal/monitor"
)

// GeminiCLIService implements AIService interface using Google Gemini CLI
type GeminiCLIService struct {
	cliPath           string
	timeout           time.Duration
	logger            *slog.Logger
	rateLimiter       monitor.AIProviderRateLimiter
	bmadKnowledgeBase string
	bmadPromptPath    string
	knowledgeBaseMu   sync.RWMutex
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

	// Get BMAD prompt path from environment or use default
	bmadPromptPath := os.Getenv("BMAD_PROMPT_PATH")
	if bmadPromptPath == "" {
		bmadPromptPath = "internal/knowledge/bmad.md"
	}

	service := &GeminiCLIService{
		cliPath:        cliPath,
		timeout:        30 * time.Second, // Default 30 second timeout
		logger:         logger,
		rateLimiter:    nil, // Will be set via SetRateLimiter
		bmadPromptPath: bmadPromptPath,
	}

	// Load BMAD knowledge base at startup
	if err := service.loadBMADKnowledgeBase(); err != nil {
		logger.Error("Failed to load BMAD knowledge base",
			"path", bmadPromptPath,
			"error", err)
		return nil, fmt.Errorf("failed to load BMAD knowledge base: %w", err)
	}

	logger.Info("BMAD knowledge base loaded successfully",
		"path", bmadPromptPath,
		"size", len(service.bmadKnowledgeBase))

	return service, nil
}

// loadBMADKnowledgeBase loads the BMAD prompt file into memory
func (g *GeminiCLIService) loadBMADKnowledgeBase() error {
	g.knowledgeBaseMu.Lock()
	defer g.knowledgeBaseMu.Unlock()

	content, err := os.ReadFile(g.bmadPromptPath)
	if err != nil {
		return fmt.Errorf("failed to read BMAD prompt file: %w", err)
	}

	g.bmadKnowledgeBase = string(content)
	return nil
}

// SetRateLimiter sets the rate limiter for this service
func (g *GeminiCLIService) SetRateLimiter(rateLimiter monitor.AIProviderRateLimiter) {
	g.rateLimiter = rateLimiter
}

// buildBMADPrompt creates a prompt that includes the BMAD knowledge base and constraints
func (g *GeminiCLIService) buildBMADPrompt(userQuery string) string {
	g.knowledgeBaseMu.RLock()
	defer g.knowledgeBaseMu.RUnlock()

	return fmt.Sprintf(`%s

-----

USER QUESTION: %s

IMPORTANT: Answer ONLY based on the information provided in the BMAD knowledge base above. If the question cannot be answered from the knowledge base, politely indicate that the information is not available in the BMAD knowledge base. Maintain any citation markers (e.g., [cite: 123]) from the source text in your response.`, g.bmadKnowledgeBase, userQuery)
}

// QueryAI sends a query to the Gemini CLI and returns the response
func (g *GeminiCLIService) QueryAI(query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	// Check rate limit and daily quota before proceeding
	if err := g.checkRateLimit(); err != nil {
		// If the error indicates quota exhaustion, return a specific user-friendly message
		if strings.Contains(err.Error(), "daily quota exhausted") {
			providerState, exists := g.rateLimiter.GetProviderState(g.GetProviderID()) // Assuming this will return the state
			if exists && !providerState.DailyQuotaResetTime.IsZero() {
				g.logger.Info("Request blocked due to daily quota exhaustion",
					"provider", g.GetProviderID(),
					"query_length", len(query),
					"reset_time", providerState.DailyQuotaResetTime)
				return fmt.Sprintf("I've reached my daily quota for AI processing. Service will be restored tomorrow at %s UTC.", providerState.DailyQuotaResetTime.Format("15:04")), nil
			}
			g.logger.Info("Request blocked due to daily quota exhaustion",
				"provider", g.GetProviderID(),
				"query_length", len(query))
			return "I've reached my daily quota for AI processing. Service will be restored tomorrow at midnight UTC.", nil
		}
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

	// Build BMAD-constrained prompt
	bmadPrompt := g.buildBMADPrompt(query)

	// Create context with timeout for command execution
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	// Execute gemini-cli command with the BMAD-constrained prompt
	cmd := exec.CommandContext(ctx, g.cliPath, "-p", bmadPrompt)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := string(output)
		g.logger.Error("Gemini CLI execution failed",
			"provider", g.GetProviderID(),
			"error", err,
			"output", errMsg)

		// AC 2.2.1: Daily Quota Detection
		dailyQuotaPattern := "Quota exceeded for quota metric '.*[Rr]equests.*' and limit '.*per day.*'"
		if strings.Contains(errMsg, "429 Too Many Requests") && regexp.MustCompile(dailyQuotaPattern).MatchString(errMsg) {
			// Calculate next UTC midnight for reset time
			now := time.Now().UTC()
			resetTime := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)

			// AC 2.2.2: Quota State Management & AC 2.2.4: Administrative Notifications
			if g.rateLimiter != nil {
				g.rateLimiter.SetQuotaExhausted(g.GetProviderID(), resetTime)
				g.logger.Error("Daily quota exhausted for Gemini API",
					"provider", g.GetProviderID(),
					"reset_time", resetTime.Format(time.RFC3339),
					"service_impact", "All Gemini operations blocked until reset time.")
			}
			return "", fmt.Errorf("daily quota exhausted for Gemini API. Service will be restored at %s UTC", resetTime.Format("15:04"))
		}

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

	// Check rate limit and daily quota before proceeding
	if err := g.checkRateLimit(); err != nil {
		// If the error indicates quota exhaustion, return a specific user-friendly message
		if strings.Contains(err.Error(), "daily quota exhausted") {
			providerState, exists := g.rateLimiter.GetProviderState(g.GetProviderID())
			if exists && !providerState.DailyQuotaResetTime.IsZero() {
				g.logger.Info("Summarization request blocked due to daily quota exhaustion",
					"provider", g.GetProviderID(),
					"query_length", len(query),
					"reset_time", providerState.DailyQuotaResetTime)
				return fmt.Sprintf("AI summarization is temporarily unavailable. Service will be restored tomorrow at %s UTC.", providerState.DailyQuotaResetTime.Format("15:04")), nil
			}
			g.logger.Info("Summarization request blocked due to daily quota exhaustion",
				"provider", g.GetProviderID(),
				"query_length", len(query))
			return "AI summarization is temporarily unavailable. Service will be restored tomorrow at midnight UTC.", nil
		}
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

	// Create a specialized prompt for BMAD-focused summarization
	prompt := fmt.Sprintf("Create a concise summary of this BMAD-METHOD related question in 8 words or less, suitable for a Discord thread title. Focus on the BMAD topic or concept being asked about. Do not include quotes or formatting. Question: %s", query)

	// Create context with timeout for command execution
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	// Execute gemini-cli command with the summarization prompt
	cmd := exec.CommandContext(ctx, g.cliPath, "-p", prompt)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := string(output)
		g.logger.Error("Gemini CLI summarization failed",
			"provider", g.GetProviderID(),
			"error", err,
			"output", errMsg)

		// AC 2.2.1: Daily Quota Detection
		dailyQuotaPattern := "Quota exceeded for quota metric '.*[Rr]equests.*' and limit '.*per day.*'"
		if strings.Contains(errMsg, "429 Too Many Requests") && regexp.MustCompile(dailyQuotaPattern).MatchString(errMsg) {
			// Calculate next UTC midnight for reset time
			now := time.Now().UTC()
			resetTime := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)

			// AC 2.2.2: Quota State Management & AC 2.2.4: Administrative Notifications
			if g.rateLimiter != nil {
				g.rateLimiter.SetQuotaExhausted(g.GetProviderID(), resetTime)
				g.logger.Error("Daily quota exhausted for Gemini API during summarization",
					"provider", g.GetProviderID(),
					"reset_time", resetTime.Format(time.RFC3339),
					"service_impact", "AI summarization blocked until reset time.")
			}
			return fmt.Sprintf("AI summarization is temporarily unavailable. Service will be restored tomorrow at %s UTC.", resetTime.Format("15:04")), nil
		}

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

	// Check rate limit and daily quota before proceeding
	if err := g.checkRateLimit(); err != nil {
		// If the error indicates quota exhaustion, return a specific user-friendly message
		if strings.Contains(err.Error(), "daily quota exhausted") {
			providerState, exists := g.rateLimiter.GetProviderState(g.GetProviderID())
			if exists && !providerState.DailyQuotaResetTime.IsZero() {
				g.logger.Info("Contextual query blocked due to daily quota exhaustion",
					"provider", g.GetProviderID(),
					"query_length", len(query),
					"history_length", len(conversationHistory),
					"reset_time", providerState.DailyQuotaResetTime)
				return fmt.Sprintf("I've reached my daily quota for AI processing. Service will be restored tomorrow at %s UTC.", providerState.DailyQuotaResetTime.Format("15:04")), nil
			}
			g.logger.Info("Contextual query blocked due to daily quota exhaustion",
				"provider", g.GetProviderID(),
				"query_length", len(query),
				"history_length", len(conversationHistory))
			return "I've reached my daily quota for AI processing. Service will be restored tomorrow at midnight UTC.", nil
		}
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

	// Build BMAD-constrained contextual prompt
	g.knowledgeBaseMu.RLock()
	bmadKnowledge := g.bmadKnowledgeBase
	g.knowledgeBaseMu.RUnlock()

	// Create a contextual prompt that includes BMAD knowledge base and conversation history
	var prompt string
	if strings.TrimSpace(conversationHistory) != "" {
		prompt = fmt.Sprintf(`%s

-----

CONVERSATION HISTORY:
%s

USER QUESTION: %s

IMPORTANT: You are continuing a conversation about BMAD-METHOD. Answer ONLY based on the information provided in the BMAD knowledge base above. If the follow-up question refers to something mentioned earlier in the conversation, use the conversation history to understand the context. However, your answer must still be grounded in the BMAD knowledge base. If the question cannot be answered from the knowledge base, politely indicate that the information is not available in the BMAD knowledge base. Maintain any citation markers (e.g., [cite: 123]) from the source text in your response.

After your main answer, provide a concise, 8-word or less topic summary of this conversation for Discord thread titles, prefixed with "[SUMMARY]:". This summary should focus on the BMAD topic or concept discussed. Example: "[SUMMARY]: BMAD Roles and Responsibilities".`, bmadKnowledge, conversationHistory, query)
	} else {
		// Fallback to regular BMAD query if no history
		prompt = g.buildBMADPrompt(query)
	}

	// Create context with timeout for command execution
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	// Execute gemini-cli command with the contextual prompt
	cmd := exec.CommandContext(ctx, g.cliPath, "-p", prompt)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := string(output)
		g.logger.Error("Gemini CLI contextual execution failed",
			"provider", g.GetProviderID(),
			"error", err,
			"output", errMsg)

		// AC 2.2.1: Daily Quota Detection
		dailyQuotaPattern := "Quota exceeded for quota metric '.*[Rr]equests.*' and limit '.*per day.*'"
		if strings.Contains(errMsg, "429 Too Many Requests") && regexp.MustCompile(dailyQuotaPattern).MatchString(errMsg) {
			// Calculate next UTC midnight for reset time
			now := time.Now().UTC()
			resetTime := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)

			// AC 2.2.2: Quota State Management & AC 2.2.4: Administrative Notifications
			if g.rateLimiter != nil {
				g.rateLimiter.SetQuotaExhausted(g.GetProviderID(), resetTime)
				g.logger.Error("Daily quota exhausted for Gemini API during contextual query",
					"provider", g.GetProviderID(),
					"reset_time", resetTime.Format(time.RFC3339),
					"service_impact", "All Gemini operations blocked until reset time.")
			}
			return "", fmt.Errorf("daily quota exhausted for Gemini API. Service will be restored at %s UTC", resetTime.Format("15:04"))
		}

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

	// Check rate limit and daily quota before proceeding
	if err := g.checkRateLimit(); err != nil {
		// If the error indicates quota exhaustion, return a specific user-friendly message
		if strings.Contains(err.Error(), "daily quota exhausted") {
			providerState, exists := g.rateLimiter.GetProviderState(g.GetProviderID())
			if exists && !providerState.DailyQuotaResetTime.IsZero() {
				g.logger.Info("Conversation summarization blocked due to daily quota exhaustion",
					"provider", g.GetProviderID(),
					"message_count", len(messages),
					"reset_time", providerState.DailyQuotaResetTime)
				return fmt.Sprintf("AI conversation summarization is temporarily unavailable. Service will be restored tomorrow at %s UTC.", providerState.DailyQuotaResetTime.Format("15:04")), nil
			}
			g.logger.Info("Conversation summarization blocked due to daily quota exhaustion",
				"provider", g.GetProviderID(),
				"message_count", len(messages))
			return "AI conversation summarization is temporarily unavailable. Service will be restored tomorrow at midnight UTC.", nil
		}
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

	// Create a specialized prompt for BMAD conversation summarization
	prompt := fmt.Sprintf("Summarize this BMAD-METHOD conversation in a concise way that preserves the key BMAD concepts and topics discussed. Focus on the BMAD-related questions asked and important BMAD information shared. Keep it under 500 words:\n\n%s", conversationText)

	// Create context with timeout for command execution
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	// Execute gemini-cli command with the summarization prompt
	cmd := exec.CommandContext(ctx, g.cliPath, "-p", prompt)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := string(output)
		g.logger.Error("Gemini CLI conversation summarization failed",
			"provider", g.GetProviderID(),
			"error", err,
			"output", errMsg)

		// AC 2.2.1: Daily Quota Detection
		dailyQuotaPattern := "Quota exceeded for quota metric '.*[Rr]equests.*' and limit '.*per day.*'"
		if strings.Contains(errMsg, "429 Too Many Requests") && regexp.MustCompile(dailyQuotaPattern).MatchString(errMsg) {
			// Calculate next UTC midnight for reset time
			now := time.Now().UTC()
			resetTime := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)

			// AC 2.2.2: Quota State Management & AC 2.2.4: Administrative Notifications
			if g.rateLimiter != nil {
				g.rateLimiter.SetQuotaExhausted(g.GetProviderID(), resetTime)
				g.logger.Error("Daily quota exhausted for Gemini API during conversation summarization",
					"provider", g.GetProviderID(),
					"reset_time", resetTime.Format(time.RFC3339),
					"service_impact", "AI conversation summarization blocked until reset time.")
			}
			return fmt.Sprintf("AI conversation summarization is temporarily unavailable. Service will be restored tomorrow at %s UTC.", resetTime.Format("15:04")), nil
		}

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

// checkRateLimit validates that the provider is not rate limited or quota exhausted before making a call
func (g *GeminiCLIService) checkRateLimit() error {
	if g.rateLimiter == nil {
		// Rate limiting not configured - allow the call
		return nil
	}

	providerID := g.GetProviderID()
	status := g.rateLimiter.GetProviderStatus(providerID)
	providerState, exists := g.rateLimiter.GetProviderState(providerID)

	// AC 2.2.5: Graceful Service Restoration
	// If the quota was exhausted but the reset time has passed, clear the flag
	if exists && providerState.DailyQuotaExhausted && time.Now().After(providerState.DailyQuotaResetTime) {
		g.rateLimiter.ClearQuotaExhaustion(providerID)
		status = g.rateLimiter.GetProviderStatus(providerID) // Re-evaluate status after clearing
		g.logger.Info("Daily quota exhaustion cleared and service restored for Gemini API",
			"provider", providerID,
			"old_reset_time", providerState.DailyQuotaResetTime)
	}

	if status == "Quota Exhausted" {
		resetTime := "midnight UTC"
		if exists && !providerState.DailyQuotaResetTime.IsZero() {
			resetTime = providerState.DailyQuotaResetTime.Format("15:04 UTC")
		}
		g.logger.Warn("Daily quota exhausted for provider, blocking call",
			"provider", providerID,
			"reset_time", resetTime)
		return fmt.Errorf("daily quota exhausted for provider %s. Service will be restored tomorrow at %s.",
			providerID, resetTime)
	}

	if status == "Throttled" {
		usage, limit := g.rateLimiter.GetProviderUsage(providerID)
		g.logger.Warn("Rate limit exceeded for provider",
			"provider", providerID,
			"status", status,
			"usage", usage,
			"limit", limit)
		return fmt.Errorf("rate limit exceeded for provider %s: %d/%d requests",
			providerID, usage, limit)
	}

	// Log warning status but don't block the call
	if status == "Warning" {
		usage, limit := g.rateLimiter.GetProviderUsage(providerID)
		g.logger.Warn("Rate limit warning for provider",
			"provider", providerID,
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
