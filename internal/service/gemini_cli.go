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

// ModelState tracks the state of a specific Gemini model
type ModelState struct {
	Name           string
	RateLimited    bool
	RateLimitTime  time.Time
	QuotaExhausted bool
	QuotaResetTime time.Time
	LastUsed       time.Time
	Mutex          sync.RWMutex
}

// GeminiCLIService implements AIService interface using Google Gemini CLI
type GeminiCLIService struct {
	cliPath           string
	timeout           time.Duration
	logger            *slog.Logger
	rateLimiter       monitor.AIProviderRateLimiter
	bmadKnowledgeBase string
	bmadPromptPath    string
	knowledgeBaseMu   sync.RWMutex
	primaryModel      *ModelState
	fallbackModel     *ModelState
	modelsMu          sync.RWMutex
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

	// Get model configuration from environment variables with defaults
	primaryModelName := os.Getenv("GEMINI_PRIMARY_MODEL")
	if primaryModelName == "" {
		primaryModelName = "gemini-2.5-pro"
	}

	fallbackModelName := os.Getenv("GEMINI_FALLBACK_MODEL")
	if fallbackModelName == "" {
		fallbackModelName = "gemini-2.5-flash-lite"
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
		primaryModel: &ModelState{
			Name: primaryModelName,
		},
		fallbackModel: &ModelState{
			Name: fallbackModelName,
		},
	}

	// AC 2.3.1: Log configured models
	logger.Info("Gemini models configured",
		"primary_model", primaryModelName,
		"fallback_model", fallbackModelName)

	// AC 2.3.1: Validate that both models are supported by CLI
	if err := service.validateModels(); err != nil {
		return nil, fmt.Errorf("model validation failed: %w", err)
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

// validateModels validates that both primary and fallback models are supported by the CLI
func (g *GeminiCLIService) validateModels() error {
	// Test primary model
	if err := g.testModel(g.primaryModel.Name); err != nil {
		g.logger.Error("Primary model validation failed",
			"model", g.primaryModel.Name,
			"error", err)
		return fmt.Errorf("primary model %s is not supported: %w", g.primaryModel.Name, err)
	}

	// Test fallback model
	if err := g.testModel(g.fallbackModel.Name); err != nil {
		g.logger.Error("Fallback model validation failed",
			"model", g.fallbackModel.Name,
			"error", err)
		return fmt.Errorf("fallback model %s is not supported: %w", g.fallbackModel.Name, err)
	}

	g.logger.Info("Model validation completed successfully",
		"primary_model", g.primaryModel.Name,
		"fallback_model", g.fallbackModel.Name)

	return nil
}

// testModel tests if a specific model is supported by the CLI
func (g *GeminiCLIService) testModel(modelName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test with a simple prompt to validate model availability
	cmd := exec.CommandContext(ctx, g.cliPath, "--model", modelName, "-p", "test")
	output, err := cmd.CombinedOutput()

	if err != nil {
		outputStr := string(output)
		// Check for model-specific errors
		if strings.Contains(outputStr, "model") && strings.Contains(outputStr, "not found") {
			return fmt.Errorf("model not found")
		}
		if strings.Contains(outputStr, "invalid model") {
			return fmt.Errorf("invalid model")
		}
		// For quota/rate limit errors, we consider the model valid but unavailable
		if strings.Contains(outputStr, "429") || strings.Contains(outputStr, "quota") {
			g.logger.Warn("Model validation hit rate limit, but model appears valid",
				"model", modelName)
			return nil // Model exists but is rate limited
		}
		return fmt.Errorf("CLI test failed: %w", err)
	}

	return nil
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

IMPORTANT: Answer ONLY based on the information provided in the BMAD knowledge base above. If the question cannot be answered from the knowledge base, politely indicate that the information is not available in the BMAD knowledge base. Maintain any citation markers (e.g., [cite: 123]) from the source text in your response.

After your main answer, provide a concise, 8-word or less topic summary of this question for Discord thread titles, prefixed with "[SUMMARY]:". This summary should focus on the BMAD topic or concept being asked about. Example: "[SUMMARY]: BMAD Roles and Responsibilities".`, g.bmadKnowledgeBase, userQuery)
}

// QueryAI sends a query to the Gemini CLI and returns the response
func (g *GeminiCLIService) QueryAI(query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	// AC 2.3.5: Check and restore models that may have recovered
	g.checkAndRestoreModels()

	// Check rate limit and daily quota before proceeding
	if err := g.checkRateLimit(); err != nil {
		// If the error indicates quota exhaustion, return a specific user-friendly message
		if strings.Contains(err.Error(), "daily quota exhausted") {
			providerState, exists := g.rateLimiter.GetProviderState(g.GetProviderID())
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

	// AC 2.3.6: Check if all models are unavailable
	modelStatus := g.getModelStatus()
	if modelStatus["primary"] != "Available" && modelStatus["fallback"] != "Available" {
		g.logger.Error("All AI models are currently unavailable",
			"primary_status", modelStatus["primary"],
			"fallback_status", modelStatus["fallback"])
		return "All AI models are currently rate limited. Please try again later.", nil
	}

	// AC 2.3.3: Get the best available model
	currentModel := g.getCurrentModel()

	g.logger.Info("Sending query to Gemini CLI",
		"provider", g.GetProviderID(),
		"model", currentModel.Name,
		"query_length", len(query))

	// Register the API call for rate limiting
	if g.rateLimiter != nil {
		if err := g.rateLimiter.RegisterCall(g.GetProviderID()); err != nil {
			g.logger.Warn("Failed to register API call for rate limiting", "error", err)
		}
	}

	// Build BMAD-constrained prompt
	bmadPrompt := g.buildBMADPrompt(query)

	response, err := g.executeModelQuery(currentModel, bmadPrompt)
	if err != nil {
		return "", err
	}

	// Clean citations from the response
	return g.cleanCitations(response), nil
}

// QueryAIWithSummary sends a query to the Gemini CLI and returns both the response and extracted summary
// Returns (response, summary, error) where summary is extracted from the integrated response
func (g *GeminiCLIService) QueryAIWithSummary(query string) (string, string, error) {
	if strings.TrimSpace(query) == "" {
		return "", "", fmt.Errorf("query cannot be empty")
	}

	// AC 2.3.5: Check and restore models that may have recovered
	g.checkAndRestoreModels()

	// Check rate limit and daily quota before proceeding
	if err := g.checkRateLimit(); err != nil {
		// If the error indicates quota exhaustion, return a specific user-friendly message
		if strings.Contains(err.Error(), "daily quota exhausted") {
			providerState, exists := g.rateLimiter.GetProviderState(g.GetProviderID())
			if exists && !providerState.DailyQuotaResetTime.IsZero() {
				g.logger.Info("Request blocked due to daily quota exhaustion",
					"provider", g.GetProviderID(),
					"query_length", len(query),
					"reset_time", providerState.DailyQuotaResetTime)
				errorMsg := fmt.Sprintf("I've reached my daily quota for AI processing. Service will be restored tomorrow at %s UTC.", providerState.DailyQuotaResetTime.Format("15:04"))
				return errorMsg, "", nil
			}
			g.logger.Info("Request blocked due to daily quota exhaustion",
				"provider", g.GetProviderID(),
				"query_length", len(query))
			return "I've reached my daily quota for AI processing. Service will be restored tomorrow at midnight UTC.", "", nil
		}
		return "", "", err
	}

	// AC 2.3.6: Check if all models are unavailable
	modelStatus := g.getModelStatus()
	if modelStatus["primary"] != "Available" && modelStatus["fallback"] != "Available" {
		g.logger.Error("All AI models are currently unavailable",
			"primary_status", modelStatus["primary"],
			"fallback_status", modelStatus["fallback"])
		return "All AI models are currently rate limited. Please try again later.", "", nil
	}

	// AC 2.3.3: Get the best available model
	currentModel := g.getCurrentModel()

	g.logger.Info("Sending query to Gemini CLI with integrated summarization",
		"provider", g.GetProviderID(),
		"model", currentModel.Name,
		"query_length", len(query))

	// Register the API call for rate limiting
	if g.rateLimiter != nil {
		if err := g.rateLimiter.RegisterCall(g.GetProviderID()); err != nil {
			g.logger.Warn("Failed to register API call for rate limiting", "error", err)
		}
	}

	// Build BMAD-constrained prompt with summary instructions
	bmadPrompt := g.buildBMADPrompt(query)

	// Execute the query
	fullResponse, err := g.executeModelQuery(currentModel, bmadPrompt)
	if err != nil {
		return "", "", err
	}

	// Parse the response to extract main answer and summary
	mainAnswer, summary, parseErr := g.parseResponseWithSummary(fullResponse)
	if parseErr != nil {
		g.logger.Warn("Failed to parse response with summary, returning full response",
			"error", parseErr)
		return fullResponse, "", nil
	}

	return mainAnswer, summary, nil
}

// executeModelQuery executes a query using the specified model
func (g *GeminiCLIService) executeModelQuery(model *ModelState, prompt string) (string, error) {
	// Create context with timeout for command execution
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	// AC 2.3.3: Execute gemini-cli command with the --model parameter
	cmd := exec.CommandContext(ctx, g.cliPath, "--model", model.Name, "-p", prompt)

	// Update last used time
	model.Mutex.Lock()
	model.LastUsed = time.Now()
	model.Mutex.Unlock()

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := string(output)
		g.logger.Error("Gemini CLI execution failed",
			"provider", g.GetProviderID(),
			"model", model.Name,
			"error", err,
			"output", errMsg)

		// AC 2.3.2: Model-specific rate limit detection
		if g.isModelRateLimited(errMsg) {
			resetTime := g.calculateResetTime(errMsg)
			g.markModelRateLimited(model.Name, resetTime)
			g.logger.Warn("Model rate limit detected",
				"model", model.Name,
				"reset_time", resetTime.Format(time.RFC3339))

			// AC 2.3.3: Try fallback if primary failed
			if model.Name == g.primaryModel.Name {
				g.logger.Info("Attempting fallback model due to primary model rate limit",
					"primary_model", g.primaryModel.Name,
					"fallback_model", g.fallbackModel.Name)
				return g.executeModelQuery(g.fallbackModel, prompt)
			}

			return "", fmt.Errorf("model %s is rate limited. Service will be restored at %s UTC", model.Name, resetTime.Format("15:04"))
		}

		// AC 2.3.2: Model-specific daily quota detection
		if g.isDailyQuotaExhausted(errMsg) {
			// Calculate next UTC midnight for reset time
			now := time.Now().UTC()
			resetTime := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)

			g.markModelQuotaExhausted(model.Name, resetTime)

			// AC 2.2.2: Update global quota state for compatibility
			if g.rateLimiter != nil {
				g.rateLimiter.SetQuotaExhausted(g.GetProviderID(), resetTime)
				g.logger.Error("Daily quota exhausted for model",
					"provider", g.GetProviderID(),
					"model", model.Name,
					"reset_time", resetTime.Format(time.RFC3339),
					"service_impact", "Model operations blocked until reset time.")
			}

			// AC 2.3.3: Try fallback if primary failed
			if model.Name == g.primaryModel.Name {
				g.logger.Info("Attempting fallback model due to primary model quota exhaustion",
					"primary_model", g.primaryModel.Name,
					"fallback_model", g.fallbackModel.Name)
				return g.executeModelQuery(g.fallbackModel, prompt)
			}

			return "", fmt.Errorf("daily quota exhausted for Gemini API. Service will be restored at %s UTC", resetTime.Format("15:04"))
		}

		// Check if it's a timeout error
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("gemini CLI request timed out after %v for model %s", g.timeout, model.Name)
		}

		return "", fmt.Errorf("gemini CLI error for model %s: %w", model.Name, err)
	}

	responseText := strings.TrimSpace(string(output))
	if responseText == "" {
		g.logger.Warn("Gemini CLI returned empty response",
			"provider", g.GetProviderID(),
			"model", model.Name)
		return "I received an empty response from the AI service.", nil
	}

	g.logger.Info("Gemini CLI response received",
		"provider", g.GetProviderID(),
		"model", model.Name,
		"response_length", len(responseText))

	return responseText, nil
}

// parseResponseWithSummary extracts the main answer and summary from an integrated response
// Returns (mainAnswer, summary, error)
func (g *GeminiCLIService) parseResponseWithSummary(response string) (string, string, error) {
	if response == "" {
		return "", "", fmt.Errorf("empty response")
	}

	// Look for the [SUMMARY]: delimiter
	summaryMarker := "[SUMMARY]:"
	summaryIndex := strings.LastIndex(response, summaryMarker)

	if summaryIndex == -1 {
		// No summary found, return the full response as main answer with empty summary
		g.logger.Warn("No summary marker found in response, summary extraction failed")
		return strings.TrimSpace(response), "", nil
	}

	// Extract main answer (everything before [SUMMARY]:)
	mainAnswer := strings.TrimSpace(response[:summaryIndex])

	// Extract summary (everything after [SUMMARY]:)
	summaryStart := summaryIndex + len(summaryMarker)
	summary := strings.TrimSpace(response[summaryStart:])

	// Validate summary length (Discord thread title limit is 100 characters)
	if len(summary) > 100 {
		g.logger.Warn("Summary too long, truncating",
			"original_length", len(summary),
			"summary", summary)
		summary = summary[:97] + "..."
	}

	// Validate summary is not empty
	if summary == "" {
		g.logger.Warn("Empty summary extracted")
		return mainAnswer, "", nil
	}

	g.logger.Info("Response parsed successfully",
		"main_answer_length", len(mainAnswer),
		"summary_length", len(summary),
		"summary", summary)

	// Clean citations from both main answer and summary
	mainAnswer = g.cleanCitations(mainAnswer)
	summary = g.cleanCitations(summary)

	return mainAnswer, summary, nil
}

// cleanCitations removes citation markers like [cite: 1, 2] from response text
func (g *GeminiCLIService) cleanCitations(text string) string {
	// Remove citation patterns like [cite: 1], [cite: 1, 2], [cite: 1,2,3], etc.
	citationPattern := `\[cite:[^\]]*\]`
	re := regexp.MustCompile(citationPattern)
	cleaned := re.ReplaceAllString(text, "")

	// Clean up any double spaces that might be left after removing citations
	cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")

	return strings.TrimSpace(cleaned)
}

// isModelRateLimited checks if the error message indicates model-specific rate limiting (but not daily quota)
func (g *GeminiCLIService) isModelRateLimited(errMsg string) bool {
	// First check if it's daily quota exhaustion - if so, it's not regular rate limiting
	if g.isDailyQuotaExhausted(errMsg) {
		return false
	}

	rateLimitPatterns := []string{
		"rate limit",
		"Rate limit",
		"too many requests",
		"Too Many Requests",
		"throttling",
		"Throttling",
	}

	for _, pattern := range rateLimitPatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// isDailyQuotaExhausted checks if the error message indicates daily quota exhaustion
func (g *GeminiCLIService) isDailyQuotaExhausted(errMsg string) bool {
	// Must contain 429 Too Many Requests and quota exceeded with 'per day' limit
	if !strings.Contains(errMsg, "429 Too Many Requests") {
		return false
	}

	// Check for quota exceeded pattern with 'per day' limit
	dailyQuotaPattern := `Quota exceeded for quota metric '.*' and limit '.*per day.*'`
	matched, err := regexp.MatchString(dailyQuotaPattern, errMsg)
	if err != nil {
		g.logger.Warn("Error matching daily quota pattern", "error", err)
		return false
	}

	return matched
}

// calculateResetTime calculates when the rate limit will reset based on error message
func (g *GeminiCLIService) calculateResetTime(errMsg string) time.Time {
	// For now, assume 1 minute reset for rate limits
	// This could be enhanced to parse specific reset times from error messages
	return time.Now().Add(1 * time.Minute)
}

// getCurrentModel returns the best available model to use
func (g *GeminiCLIService) getCurrentModel() *ModelState {
	g.modelsMu.RLock()
	defer g.modelsMu.RUnlock()

	// Check if primary model is available
	g.primaryModel.Mutex.RLock()
	primaryAvailable := !g.primaryModel.RateLimited && !g.primaryModel.QuotaExhausted
	g.primaryModel.Mutex.RUnlock()

	if primaryAvailable {
		return g.primaryModel
	}

	// Check if fallback model is available
	g.fallbackModel.Mutex.RLock()
	fallbackAvailable := !g.fallbackModel.RateLimited && !g.fallbackModel.QuotaExhausted
	g.fallbackModel.Mutex.RUnlock()

	if fallbackAvailable {
		g.logger.Info("Using fallback model due to primary model unavailability",
			"primary_model", g.primaryModel.Name,
			"fallback_model", g.fallbackModel.Name)
		return g.fallbackModel
	}

	// AC 2.3.6: Both models unavailable - return primary for error handling
	g.logger.Warn("Both models unavailable, returning primary for error handling",
		"primary_model", g.primaryModel.Name,
		"fallback_model", g.fallbackModel.Name)
	return g.primaryModel
}

// markModelRateLimited marks a model as rate limited
func (g *GeminiCLIService) markModelRateLimited(modelName string, resetTime time.Time) {
	g.modelsMu.Lock()
	defer g.modelsMu.Unlock()

	var targetModel *ModelState
	if g.primaryModel.Name == modelName {
		targetModel = g.primaryModel
	} else if g.fallbackModel.Name == modelName {
		targetModel = g.fallbackModel
	} else {
		g.logger.Warn("Attempted to mark unknown model as rate limited", "model", modelName)
		return
	}

	targetModel.Mutex.Lock()
	defer targetModel.Mutex.Unlock()

	targetModel.RateLimited = true
	targetModel.RateLimitTime = resetTime

	g.logger.Warn("Model marked as rate limited",
		"model", modelName,
		"reset_time", resetTime.Format(time.RFC3339))
}

// markModelQuotaExhausted marks a model as quota exhausted
func (g *GeminiCLIService) markModelQuotaExhausted(modelName string, resetTime time.Time) {
	g.modelsMu.Lock()
	defer g.modelsMu.Unlock()

	var targetModel *ModelState
	if g.primaryModel.Name == modelName {
		targetModel = g.primaryModel
	} else if g.fallbackModel.Name == modelName {
		targetModel = g.fallbackModel
	} else {
		g.logger.Warn("Attempted to mark unknown model as quota exhausted", "model", modelName)
		return
	}

	targetModel.Mutex.Lock()
	defer targetModel.Mutex.Unlock()

	targetModel.QuotaExhausted = true
	targetModel.QuotaResetTime = resetTime

	g.logger.Error("Model marked as quota exhausted",
		"model", modelName,
		"reset_time", resetTime.Format(time.RFC3339))
}

// checkAndRestoreModels checks if any models can be restored from rate limits
func (g *GeminiCLIService) checkAndRestoreModels() {
	now := time.Now()

	g.modelsMu.Lock()
	defer g.modelsMu.Unlock()

	// Check primary model
	g.primaryModel.Mutex.Lock()
	if g.primaryModel.RateLimited && now.After(g.primaryModel.RateLimitTime) {
		g.primaryModel.RateLimited = false
		g.primaryModel.RateLimitTime = time.Time{}
		g.logger.Info("Primary model rate limit restored",
			"model", g.primaryModel.Name)
	}
	if g.primaryModel.QuotaExhausted && now.After(g.primaryModel.QuotaResetTime) {
		g.primaryModel.QuotaExhausted = false
		g.primaryModel.QuotaResetTime = time.Time{}
		g.logger.Info("Primary model quota exhaustion restored",
			"model", g.primaryModel.Name)
	}
	g.primaryModel.Mutex.Unlock()

	// Check fallback model
	g.fallbackModel.Mutex.Lock()
	if g.fallbackModel.RateLimited && now.After(g.fallbackModel.RateLimitTime) {
		g.fallbackModel.RateLimited = false
		g.fallbackModel.RateLimitTime = time.Time{}
		g.logger.Info("Fallback model rate limit restored",
			"model", g.fallbackModel.Name)
	}
	if g.fallbackModel.QuotaExhausted && now.After(g.fallbackModel.QuotaResetTime) {
		g.fallbackModel.QuotaExhausted = false
		g.fallbackModel.QuotaResetTime = time.Time{}
		g.logger.Info("Fallback model quota exhaustion restored",
			"model", g.fallbackModel.Name)
	}
	g.fallbackModel.Mutex.Unlock()
}

// getModelStatus returns status information for both models
func (g *GeminiCLIService) getModelStatus() map[string]string {
	g.modelsMu.RLock()
	defer g.modelsMu.RUnlock()

	status := make(map[string]string)

	g.primaryModel.Mutex.RLock()
	if g.primaryModel.QuotaExhausted {
		status["primary"] = "Quota Exhausted"
	} else if g.primaryModel.RateLimited {
		status["primary"] = "Rate Limited"
	} else {
		status["primary"] = "Available"
	}
	g.primaryModel.Mutex.RUnlock()

	g.fallbackModel.Mutex.RLock()
	if g.fallbackModel.QuotaExhausted {
		status["fallback"] = "Quota Exhausted"
	} else if g.fallbackModel.RateLimited {
		status["fallback"] = "Rate Limited"
	} else {
		status["fallback"] = "Available"
	}
	g.fallbackModel.Mutex.RUnlock()

	return status
}

// SummarizeQuery creates a summarized version of a user query suitable for Discord thread titles
func (g *GeminiCLIService) SummarizeQuery(query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	// AC 2.3.5: Check and restore models that may have recovered
	g.checkAndRestoreModels()

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

	// AC 2.3.3: Get the best available model
	currentModel := g.getCurrentModel()

	// AC 2.3.6: Check if all models are unavailable
	modelStatus := g.getModelStatus()
	if modelStatus["primary"] != "Available" && modelStatus["fallback"] != "Available" {
		g.logger.Error("All AI models are currently unavailable for summarization",
			"primary_status", modelStatus["primary"],
			"fallback_status", modelStatus["fallback"])
		g.logger.Warn("AI summarization failed due to model unavailability, using fallback", "provider", g.GetProviderID())
		return g.fallbackSummarize(query), nil
	}

	g.logger.Info("Creating query summary",
		"provider", g.GetProviderID(),
		"model", currentModel.Name,
		"query_length", len(query))

	// Register the API call for rate limiting
	if g.rateLimiter != nil {
		if err := g.rateLimiter.RegisterCall(g.GetProviderID()); err != nil {
			g.logger.Warn("Failed to register API call for rate limiting", "error", err)
		}
	}

	// Create a specialized prompt for BMAD-focused summarization
	prompt := fmt.Sprintf("Create a concise summary of this BMAD-METHOD related question in 8 words or less, suitable for a Discord thread title. Focus on the BMAD topic or concept being asked about. Do not include quotes or formatting. Question: %s", query)

	summary, err := g.executeModelQuery(currentModel, prompt)
	if err != nil {
		// Fallback to simple truncation if AI summarization fails
		g.logger.Warn("AI summarization failed, using fallback", "provider", g.GetProviderID(), "error", err)
		return g.fallbackSummarize(query), nil
	}

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
		"model", currentModel.Name,
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

	// AC 2.3.5: Check and restore models that may have recovered
	g.checkAndRestoreModels()

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

	// AC 2.3.3: Get the best available model
	currentModel := g.getCurrentModel()

	// AC 2.3.6: Check if all models are unavailable
	modelStatus := g.getModelStatus()
	if modelStatus["primary"] != "Available" && modelStatus["fallback"] != "Available" {
		g.logger.Error("All AI models are currently unavailable for contextual query",
			"primary_status", modelStatus["primary"],
			"fallback_status", modelStatus["fallback"])
		return "All AI models are currently rate limited. Please try again later.", nil
	}

	g.logger.Info("Sending contextual query to Gemini CLI",
		"provider", g.GetProviderID(),
		"model", currentModel.Name,
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

	response, err := g.executeModelQuery(currentModel, prompt)
	if err != nil {
		return "", err
	}

	// Clean citations from the response
	return g.cleanCitations(response), nil
}

// SummarizeConversation creates a summary of conversation history for context preservation
func (g *GeminiCLIService) SummarizeConversation(messages []string) (string, error) {
	if len(messages) == 0 {
		return "", nil
	}

	// AC 2.3.5: Check and restore models that may have recovered
	g.checkAndRestoreModels()

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

	// AC 2.3.3: Get the best available model
	currentModel := g.getCurrentModel()

	// AC 2.3.6: Check if all models are unavailable
	modelStatus := g.getModelStatus()
	if modelStatus["primary"] != "Available" && modelStatus["fallback"] != "Available" {
		g.logger.Error("All AI models are currently unavailable for conversation summarization",
			"primary_status", modelStatus["primary"],
			"fallback_status", modelStatus["fallback"])
		g.logger.Warn("AI conversation summarization failed due to model unavailability, using fallback", "provider", g.GetProviderID())
		return g.fallbackConversationSummary(messages), nil
	}

	g.logger.Info("Summarizing conversation",
		"provider", g.GetProviderID(),
		"model", currentModel.Name,
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

	summary, err := g.executeModelQuery(currentModel, prompt)
	if err != nil {
		// Fallback to truncated conversation if AI summarization fails
		g.logger.Warn("AI conversation summarization failed, using fallback", "provider", g.GetProviderID(), "error", err)
		return g.fallbackConversationSummary(messages), nil
	}

	if summary == "" {
		g.logger.Warn("Gemini CLI returned empty conversation summary, using fallback", "provider", g.GetProviderID())
		return g.fallbackConversationSummary(messages), nil
	}

	g.logger.Info("Conversation summary created",
		"provider", g.GetProviderID(),
		"model", currentModel.Name,
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
