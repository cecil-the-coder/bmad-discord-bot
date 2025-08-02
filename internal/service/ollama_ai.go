package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"bmad-knowledge-bot/internal/monitor"
)

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// OllamaRequest represents the request payload for Ollama API
type OllamaRequest struct {
	Model   string                 `json:"model"`
	Prompt  string                 `json:"prompt"`
	Stream  bool                   `json:"stream"`
	Options map[string]interface{} `json:"options,omitempty"`
}

// OllamaResponse represents the response from Ollama API
type OllamaResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
	Context  []int  `json:"context,omitempty"`
	Error    string `json:"error,omitempty"`
}

// QualityScore represents the quality assessment of a response
type QualityScore struct {
	BMADCoverageScore      float64  // 0-1: How well response covers BMAD concepts
	KnowledgeBoundaryScore float64  // 0-1: How well response stays within knowledge base
	ContentQualityScore    float64  // 0-1: Overall content quality assessment
	OverallScore           float64  // 0-1: Weighted average of all scores
	Issues                 []string // List of quality issues found
	BMADTermsFound         []string // BMAD-specific terms detected
	Warnings               []string // Warnings about potential issues
}

// QualityMetrics tracks response quality over time
type QualityMetrics struct {
	TotalResponses       int64     `json:"total_responses"`
	AverageOverallScore  float64   `json:"average_overall_score"`
	AverageBMADScore     float64   `json:"average_bmad_score"`
	AverageBoundaryScore float64   `json:"average_boundary_score"`
	AverageContentScore  float64   `json:"average_content_score"`
	LowQualityResponses  int64     `json:"low_quality_responses"`
	EmptyResponses       int64     `json:"empty_responses"`
	OffTopicResponses    int64     `json:"off_topic_responses"`
	LastUpdated          time.Time `json:"last_updated"`
	mutex                sync.RWMutex
}

// OllamaAIService implements AIService interface using Ollama API
type OllamaAIService struct {
	client             *http.Client
	baseURL            string
	modelName          string
	timeout            time.Duration
	logger             *slog.Logger
	rateLimiter        monitor.AIProviderRateLimiter
	bmadKnowledgeBase  string
	ephemeralCachePath string
	knowledgeBaseMu    sync.RWMutex
	qualityMetrics     *QualityMetrics
	bmadTerms          []string
	qualityEnabled     bool
}

// NewOllamaAIService creates a new Ollama AI service instance
func NewOllamaAIService(logger *slog.Logger) (*OllamaAIService, error) {
	// Get configuration from environment variables with defaults
	baseURL := os.Getenv("OLLAMA_HOST")
	if baseURL == "" {
		baseURL = "https://ollama"
	}

	modelName := os.Getenv("OLLAMA_MODEL")
	if modelName == "" {
		modelName = "devstral"
	}

	timeoutStr := os.Getenv("OLLAMA_TIMEOUT")
	timeout := 30 * time.Second // Default 30 second timeout
	if timeoutStr != "" {
		if parsedTimeout, err := time.ParseDuration(timeoutStr + "s"); err == nil {
			timeout = parsedTimeout
		}
	}

	// BMAD knowledge base is now fetched from remote URL and cached ephemerally
	// No persistent file path needed - content stored in /tmp

	// Check if quality monitoring is enabled
	qualityEnabled := os.Getenv("OLLAMA_QUALITY_MONITORING_ENABLED")
	if qualityEnabled == "" {
		qualityEnabled = "true" // Default to enabled
	}
	qualityEnabledBool := qualityEnabled == "true"

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: timeout,
	}

	service := &OllamaAIService{
		client:      client,
		baseURL:     baseURL,
		modelName:   modelName,
		timeout:     timeout,
		logger:      logger,
		rateLimiter: nil, // Will be set via SetRateLimiter
		qualityMetrics: &QualityMetrics{
			LastUpdated: time.Now(),
		},
		qualityEnabled: qualityEnabledBool,
		bmadTerms: []string{
			"BMAD", "BMAD-METHOD", "bmad", "bmad-method",
			"agent", "agents", "PM", "Developer", "Architect", "QA", "UX", "UX Expert",
			"Scrum Master", "Product Owner", "SM", "PO", "Dev",
			"story", "stories", "epic", "epics", "PRD", "architecture",
			"workflow", "workflows", "vibe CEO", "CEO", "orchestrator",
			"bmad-master", "bmad-orchestrator", "shard", "sharding",
			"greenfield", "brownfield", "template", "templates",
			"checklist", "checklists", "task", "tasks",
		},
	}

	// Log configured settings
	logger.Info("Ollama service configured",
		"base_url", baseURL,
		"model", modelName,
		"timeout", timeout)

	// Validate that the model is available
	if err := service.validateModel(); err != nil {
		return nil, fmt.Errorf("model validation failed: %w", err)
	}

	// Load BMAD knowledge base from remote URL and cache ephemerally
	if err := service.loadBMADKnowledgeBaseFromURL(); err != nil {
		logger.Error("Failed to load BMAD knowledge base from remote URL",
			"error", err)
		return nil, fmt.Errorf("failed to load BMAD knowledge base: %w", err)
	}

	logger.Info("BMAD knowledge base loaded successfully from remote URL",
		"cache_path", service.ephemeralCachePath,
		"size", len(service.bmadKnowledgeBase))

	return service, nil
}

// validateModel validates that the devstral model is available on the Ollama server
func (o *OllamaAIService) validateModel() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test with a simple prompt to validate model availability
	testRequest := OllamaRequest{
		Model:  o.modelName,
		Prompt: "test",
		Stream: false,
	}

	jsonData, err := json.Marshal(testRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal test request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/generate", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Ollama server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(body), "model") && strings.Contains(string(body), "not found") {
			return fmt.Errorf("model '%s' not found on Ollama server", o.modelName)
		}
		return fmt.Errorf("ollama server returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to check for errors
	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return fmt.Errorf("failed to decode test response: %w", err)
	}

	if ollamaResp.Error != "" {
		return fmt.Errorf("ollama API error: %s", ollamaResp.Error)
	}

	o.logger.Info("Model validation completed successfully",
		"model", o.modelName)

	return nil
}

// loadBMADKnowledgeBaseFromURL fetches BMAD knowledge base from remote URL and caches ephemerally
func (o *OllamaAIService) loadBMADKnowledgeBaseFromURL() error {
	o.knowledgeBaseMu.Lock()
	defer o.knowledgeBaseMu.Unlock()

	// Get remote URL from environment variable
	remoteURL := os.Getenv("BMAD_KB_REMOTE_URL")
	if remoteURL == "" {
		remoteURL = "https://github.com/bmadcode/BMAD-METHOD/raw/refs/heads/main/bmad-core/data/bmad-kb.md"
	}

	// Set ephemeral cache path in /tmp
	o.ephemeralCachePath = "/tmp/bmad-kb-cache.md"

	// Try to read from ephemeral cache first
	if content, err := os.ReadFile(o.ephemeralCachePath); err == nil {
		o.bmadKnowledgeBase = string(content)
		o.logger.Info("BMAD knowledge base loaded from ephemeral cache",
			"cache_path", o.ephemeralCachePath,
			"size", len(content))
		return nil
	}

	// If cache doesn't exist or is invalid, fetch from remote URL
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", remoteURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch BMAD knowledge base from %s: %w", remoteURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d when fetching BMAD knowledge base from %s", resp.StatusCode, remoteURL)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Cache content ephemerally
	if err := os.WriteFile(o.ephemeralCachePath, content, 0644); err != nil {
		o.logger.Warn("Failed to write ephemeral cache",
			"cache_path", o.ephemeralCachePath,
			"error", err)
	}

	o.bmadKnowledgeBase = string(content)
	o.logger.Info("BMAD knowledge base fetched from remote URL and cached",
		"remote_url", remoteURL,
		"cache_path", o.ephemeralCachePath,
		"size", len(content))

	return nil
}

// RefreshKnowledgeBase refreshes the knowledge base from ephemeral cache
func (o *OllamaAIService) RefreshKnowledgeBase() error {
	o.knowledgeBaseMu.Lock()
	defer o.knowledgeBaseMu.Unlock()

	// Try to read from ephemeral cache
	if content, err := os.ReadFile(o.ephemeralCachePath); err == nil {
		o.bmadKnowledgeBase = string(content)
		o.logger.Info("BMAD knowledge base refreshed from ephemeral cache",
			"cache_path", o.ephemeralCachePath,
			"size", len(content))
		return nil
	} else {
		o.logger.Warn("Failed to refresh knowledge base from ephemeral cache",
			"cache_path", o.ephemeralCachePath,
			"error", err)
		return err
	}
}

// SetRateLimiter sets the rate limiter for this service
func (o *OllamaAIService) SetRateLimiter(rateLimiter monitor.AIProviderRateLimiter) {
	o.rateLimiter = rateLimiter
}

// analyzeResponseQuality performs comprehensive quality analysis on a response
func (o *OllamaAIService) analyzeResponseQuality(query, response string) *QualityScore {
	if !o.qualityEnabled {
		return &QualityScore{OverallScore: 1.0} // Default to perfect if disabled
	}

	score := &QualityScore{
		Issues:         make([]string, 0),
		BMADTermsFound: make([]string, 0),
		Warnings:       make([]string, 0),
	}

	// 1. BMAD Coverage Analysis
	score.BMADCoverageScore = o.calculateBMADCoverage(response, score)

	// 2. Knowledge Boundary Analysis
	score.KnowledgeBoundaryScore = o.calculateKnowledgeBoundary(query, response, score)

	// 3. Content Quality Analysis
	score.ContentQualityScore = o.calculateContentQuality(response, score)

	// 4. Calculate overall weighted score
	score.OverallScore = (score.BMADCoverageScore*0.4 +
		score.KnowledgeBoundaryScore*0.35 +
		score.ContentQualityScore*0.25)

	// 5. Log quality issues if any
	if len(score.Issues) > 0 || len(score.Warnings) > 0 {
		o.logger.Warn("Response quality issues detected",
			"overall_score", score.OverallScore,
			"bmad_score", score.BMADCoverageScore,
			"boundary_score", score.KnowledgeBoundaryScore,
			"content_score", score.ContentQualityScore,
			"issues", score.Issues,
			"warnings", score.Warnings,
			"bmad_terms", score.BMADTermsFound)
	}

	return score
}

// calculateBMADCoverage analyzes how well the response covers BMAD concepts
func (o *OllamaAIService) calculateBMADCoverage(response string, score *QualityScore) float64 {
	responseLower := strings.ToLower(response)

	// Return minimal score for empty responses
	if len(strings.TrimSpace(response)) == 0 {
		score.Issues = append(score.Issues, "No BMAD-specific terminology found in response")
		return 0.0
	}

	termsFound := 0
	uniqueTerms := make(map[string]bool)

	for _, term := range o.bmadTerms {
		if strings.Contains(responseLower, strings.ToLower(term)) {
			if !uniqueTerms[strings.ToLower(term)] {
				termsFound++
				uniqueTerms[strings.ToLower(term)] = true
				score.BMADTermsFound = append(score.BMADTermsFound, term)
			}
		}
	}

	// Base score on number of unique BMAD terms found
	coverageScore := float64(termsFound) / 3.0 // Expect at least 3 terms for good coverage
	if coverageScore > 1.0 {
		coverageScore = 1.0
	}

	// Penalty for no BMAD terms at all
	if termsFound == 0 {
		score.Issues = append(score.Issues, "No BMAD-specific terminology found in response")
		coverageScore = 0.05 // Very low but not zero
	} else if termsFound == 1 {
		score.Warnings = append(score.Warnings, "Limited BMAD terminology usage")
	}

	// Check for key BMAD concepts (more specific than general terms)
	keyConceptsFound := 0
	keyConcepts := []string{"bmad", "agent", "workflow", "story", "epic", "architecture"}
	for _, concept := range keyConcepts {
		if strings.Contains(responseLower, concept) {
			keyConceptsFound++
		}
	}

	if keyConceptsFound == 0 && termsFound > 0 {
		score.Issues = append(score.Issues, "No key BMAD concepts referenced")
		coverageScore *= 0.5
	}

	return coverageScore
}

// calculateKnowledgeBoundary analyzes if response stays within BMAD knowledge base
func (o *OllamaAIService) calculateKnowledgeBoundary(query, response string, score *QualityScore) float64 {
	boundaryScore := 1.0
	responseLower := strings.ToLower(response)

	// Check for common signs of going outside knowledge base
	offTopicPhrases := []string{
		"i don't know", "i'm not sure", "i cannot find", "not mentioned in",
		"outside of", "beyond the scope", "not covered in the documentation",
		"based on my general knowledge", "from what i understand generally",
		"in general software development", "typically in software projects",
	}

	for _, phrase := range offTopicPhrases {
		if strings.Contains(responseLower, phrase) {
			if strings.Contains(phrase, "not mentioned") || strings.Contains(phrase, "not covered") {
				// These are good - staying within bounds
				boundaryScore += 0.1
			} else {
				score.Warnings = append(score.Warnings, fmt.Sprintf("Potential boundary violation: '%s'", phrase))
				boundaryScore -= 0.2
			}
		}
	}

	// Check for good boundary behavior
	goodBoundaryPhrases := []string{
		"based on the bmad knowledge base", "according to bmad",
		"the bmad documentation states", "bmad framework specifies",
		"not available in the bmad knowledge base",
	}

	goodBoundaryFound := false
	for _, phrase := range goodBoundaryPhrases {
		if strings.Contains(responseLower, phrase) {
			goodBoundaryFound = true
			break
		}
	}

	if goodBoundaryFound {
		boundaryScore += 0.2
	}

	// Check for hallucination indicators
	hallucinationIndicators := []string{
		"version 2.0", "version 3.0", "latest update", "recent changes",
		"new feature", "upcoming release", "will be added in",
		"according to chatgpt", "based on my training", "as an ai",
	}

	for _, indicator := range hallucinationIndicators {
		if strings.Contains(responseLower, indicator) {
			score.Issues = append(score.Issues, fmt.Sprintf("Potential hallucination: '%s'", indicator))
			boundaryScore -= 0.3
		}
	}

	// Ensure score stays within bounds
	if boundaryScore > 1.0 {
		boundaryScore = 1.0
	}
	if boundaryScore < 0.0 {
		boundaryScore = 0.0
	}

	return boundaryScore
}

// calculateContentQuality analyzes overall content quality
func (o *OllamaAIService) calculateContentQuality(response string, score *QualityScore) float64 {
	contentScore := 1.0

	// Length analysis
	responseLen := len(strings.TrimSpace(response))
	if responseLen == 0 {
		score.Issues = append(score.Issues, "Empty response")
		return 0.0
	}

	if responseLen < 20 {
		score.Issues = append(score.Issues, "Response too short (< 20 characters)")
		contentScore -= 0.4
	} else if responseLen < 50 {
		score.Warnings = append(score.Warnings, "Response quite short (< 50 characters)")
		contentScore -= 0.2
	}

	// Check for repetitive content
	words := strings.Fields(strings.ToLower(response))
	if len(words) > 10 {
		wordFreq := make(map[string]int)
		for _, word := range words {
			if len(word) > 3 { // Skip short words
				wordFreq[word]++
			}
		}

		maxRepetition := 0
		for _, freq := range wordFreq {
			if freq > maxRepetition {
				maxRepetition = freq
			}
		}

		if maxRepetition > len(words)/4 { // More than 25% repetition
			score.Issues = append(score.Issues, "Highly repetitive content detected")
			contentScore -= 0.3
		} else if maxRepetition > len(words)/6 { // More than 16% repetition
			score.Warnings = append(score.Warnings, "Some repetitive content detected")
			contentScore -= 0.1
		}
	}

	// Check for coherence (simple sentence structure analysis)
	sentences := strings.Split(response, ".")
	if len(sentences) > 1 {
		avgSentenceLength := float64(responseLen) / float64(len(sentences))
		if avgSentenceLength < 10 {
			score.Warnings = append(score.Warnings, "Very short sentences (may indicate fragmented response)")
			contentScore -= 0.1
		} else if avgSentenceLength > 200 {
			score.Warnings = append(score.Warnings, "Very long sentences (may indicate run-on text)")
			contentScore -= 0.1
		}
	}

	// Check for generic/template responses
	genericPhrases := []string{
		"i hope this helps", "let me know if you need", "feel free to ask",
		"here is the information", "based on the information provided",
		"to answer your question", "in summary", "in conclusion",
	}

	genericCount := 0
	for _, phrase := range genericPhrases {
		if strings.Contains(strings.ToLower(response), phrase) {
			genericCount++
		}
	}

	if genericCount > 2 {
		score.Warnings = append(score.Warnings, "Response contains multiple generic phrases")
		contentScore -= 0.15
	}

	// Ensure score stays within bounds
	if contentScore < 0.0 {
		contentScore = 0.0
	}

	return contentScore
}

// updateQualityMetrics updates the running quality metrics with a new score
func (o *OllamaAIService) updateQualityMetrics(score *QualityScore) {
	if !o.qualityEnabled {
		return
	}

	o.qualityMetrics.mutex.Lock()
	defer o.qualityMetrics.mutex.Unlock()

	o.qualityMetrics.TotalResponses++

	// Update running averages
	n := float64(o.qualityMetrics.TotalResponses)
	o.qualityMetrics.AverageOverallScore = ((o.qualityMetrics.AverageOverallScore * (n - 1)) + score.OverallScore) / n
	o.qualityMetrics.AverageBMADScore = ((o.qualityMetrics.AverageBMADScore * (n - 1)) + score.BMADCoverageScore) / n
	o.qualityMetrics.AverageBoundaryScore = ((o.qualityMetrics.AverageBoundaryScore * (n - 1)) + score.KnowledgeBoundaryScore) / n
	o.qualityMetrics.AverageContentScore = ((o.qualityMetrics.AverageContentScore * (n - 1)) + score.ContentQualityScore) / n

	// Update counters
	if score.OverallScore < 0.6 {
		o.qualityMetrics.LowQualityResponses++
	}

	if len(score.Issues) > 0 {
		for _, issue := range score.Issues {
			if strings.Contains(issue, "Empty response") {
				o.qualityMetrics.EmptyResponses++
			}
			if strings.Contains(issue, "No BMAD") {
				o.qualityMetrics.OffTopicResponses++
			}
		}
	}

	o.qualityMetrics.LastUpdated = time.Now()

	// Log quality metrics periodically
	if o.qualityMetrics.TotalResponses%10 == 0 {
		o.logger.Info("Quality metrics update",
			"total_responses", o.qualityMetrics.TotalResponses,
			"avg_overall_score", fmt.Sprintf("%.3f", o.qualityMetrics.AverageOverallScore),
			"avg_bmad_score", fmt.Sprintf("%.3f", o.qualityMetrics.AverageBMADScore),
			"avg_boundary_score", fmt.Sprintf("%.3f", o.qualityMetrics.AverageBoundaryScore),
			"avg_content_score", fmt.Sprintf("%.3f", o.qualityMetrics.AverageContentScore),
			"low_quality_responses", o.qualityMetrics.LowQualityResponses,
			"off_topic_responses", o.qualityMetrics.OffTopicResponses)
	}
}

// GetQualityMetrics returns a copy of the current quality metrics
func (o *OllamaAIService) GetQualityMetrics() QualityMetrics {
	o.qualityMetrics.mutex.RLock()
	defer o.qualityMetrics.mutex.RUnlock()

	// Return a copy to avoid race conditions
	return QualityMetrics{
		TotalResponses:       o.qualityMetrics.TotalResponses,
		AverageOverallScore:  o.qualityMetrics.AverageOverallScore,
		AverageBMADScore:     o.qualityMetrics.AverageBMADScore,
		AverageBoundaryScore: o.qualityMetrics.AverageBoundaryScore,
		AverageContentScore:  o.qualityMetrics.AverageContentScore,
		LowQualityResponses:  o.qualityMetrics.LowQualityResponses,
		EmptyResponses:       o.qualityMetrics.EmptyResponses,
		OffTopicResponses:    o.qualityMetrics.OffTopicResponses,
		LastUpdated:          o.qualityMetrics.LastUpdated,
	}
}

// buildBMADPrompt creates a prompt that includes the BMAD knowledge base and constraints
func (o *OllamaAIService) buildBMADPrompt(userQuery string) string {
	o.knowledgeBaseMu.RLock()
	defer o.knowledgeBaseMu.RUnlock()

	// Get prompt template preference from environment
	promptStyle := os.Getenv("OLLAMA_PROMPT_STYLE")
	if promptStyle == "" {
		promptStyle = "structured" // Default to structured
	}

	switch promptStyle {
	case "simple":
		return o.buildSimplePrompt(userQuery)
	case "detailed":
		return o.buildDetailedPrompt(userQuery)
	case "chain_of_thought":
		return o.buildChainOfThoughtPrompt(userQuery)
	default:
		return o.buildStructuredPrompt(userQuery)
	}
}

// buildStructuredPrompt creates a highly structured prompt for better model guidance
func (o *OllamaAIService) buildStructuredPrompt(userQuery string) string {
	return fmt.Sprintf(`# BMAD-METHOD KNOWLEDGE BASE
%s

---

# YOUR IDENTITY
You are bmadhelper, the BMAD-METHOD assistant agent on Discord.

# TASK
Answer the user's question using ONLY the BMAD knowledge base above.

# USER QUESTION
%s

# INSTRUCTIONS
1. READ the knowledge base carefully
2. FIND relevant information for the question
3. PROVIDE a clear, specific answer using BMAD terminology
4. USE proper BMAD concepts (agents, workflows, stories, epics, etc.)
5. If information is NOT in the knowledge base, say "This information is not available in the BMAD knowledge base"
6. If asked about release dates, updates, ETAs, or future features, remind the user: "I only have access to current BMAD documentation and cannot provide information about future updates or release schedules."

# RESPONSE FORMAT
Write your answer with proper paragraph breaks for Discord readability. Use double line breaks (blank lines) between paragraphs. Structure your response clearly with:
- Introduction paragraph (double line break after)
- Main content paragraphs (double line break between each)
- Conclusion or summary paragraph (if needed)

[Your answer here using BMAD terminology - remember to use double line breaks between paragraphs]

[SUMMARY]: [6-8 word summary for Discord thread title]

# REMEMBER
- Stay within BMAD knowledge base boundaries
- Use BMAD-specific terms when possible
- Be concise but comprehensive
- Focus on BMAD methodology and concepts`, o.bmadKnowledgeBase, userQuery)
}

// buildSimplePrompt creates a simpler, more direct prompt
func (o *OllamaAIService) buildSimplePrompt(userQuery string) string {
	return fmt.Sprintf(`You are bmadhelper, the BMAD-METHOD assistant agent on Discord.

BMAD Knowledge Base:
%s

Question: %s

Answer using only BMAD knowledge base information. Use BMAD terms like agents, workflows, stories, and epics. If asked about release dates, updates, ETAs, or future features, remind the user that you only have access to current BMAD documentation. Format with proper paragraph breaks for Discord readability - use double line breaks (blank lines) between paragraphs. End with [SUMMARY]: brief title.`, o.bmadKnowledgeBase, userQuery)
}

// buildDetailedPrompt creates a more detailed prompt with examples
func (o *OllamaAIService) buildDetailedPrompt(userQuery string) string {
	return fmt.Sprintf(`# BMAD-METHOD EXPERT SYSTEM

## KNOWLEDGE BASE
%s

## YOUR ROLE
You are bmadhelper, the BMAD-METHOD assistant agent on Discord. Your job is to answer questions using ONLY the knowledge base above. You are a helpful AI assistant specializing in BMAD methodology.

## QUESTION
%s

## RESPONSE GUIDELINES
✓ USE BMAD terminology: agents, workflows, stories, epics, PRD, architecture
✓ REFERENCE specific BMAD concepts and processes
✓ EXPLAIN how things work within the BMAD framework
✓ BE specific about BMAD roles (PM, Dev, Architect, QA, UX, SM, PO)
✗ DON'T make up information not in the knowledge base
✗ DON'T use general software development advice
✗ DON'T reference external frameworks or methods
⚠️ IF asked about release dates, updates, ETAs, or future features, remind the user: "I only have access to current BMAD documentation and cannot provide information about future updates or release schedules."

## EXAMPLE GOOD RESPONSE
"In BMAD-METHOD, agents work in structured workflows. The SM agent creates stories from sharded PRD documents, while the Dev agent implements approved stories following the coding standards."

## YOUR RESPONSE
Format your answer with proper paragraph breaks for Discord readability - use double line breaks (blank lines) between paragraphs.

[Answer here with clear paragraph spacing - remember double line breaks between paragraphs]

[SUMMARY]: [Brief BMAD-focused title]`, o.bmadKnowledgeBase, userQuery)
}

// buildChainOfThoughtPrompt uses chain-of-thought reasoning for better responses
func (o *OllamaAIService) buildChainOfThoughtPrompt(userQuery string) string {
	return fmt.Sprintf(`# YOUR IDENTITY
You are bmadhelper, the BMAD-METHOD assistant agent on Discord.

# BMAD KNOWLEDGE BASE
%s

---

# QUESTION: %s

# REASONING PROCESS
Let me think step by step:

1. IDENTIFY: What BMAD concepts does this question relate to?
2. SEARCH: What information is available in the knowledge base?
3. CONNECT: How do these concepts work together in BMAD?
4. CHECK: Is this about future updates/releases? (If so, remind user I only have current documentation)
5. RESPOND: Provide a clear answer using BMAD terminology

# ANALYSIS
[Think through the question step by step]
- What BMAD concepts are relevant?
- What specific information is in the knowledge base?
- How should I structure my response?

# ANSWER
[Your detailed BMAD-focused response - use double line breaks (blank lines) between paragraphs for Discord readability]

[SUMMARY]: [Concise BMAD topic summary]`, o.bmadKnowledgeBase, userQuery)
}

// executeQuery sends a request to the Ollama API and returns the response
func (o *OllamaAIService) executeQuery(prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()

	// Create request payload
	request := OllamaRequest{
		Model:  o.modelName,
		Prompt: prompt,
		Stream: false,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/generate", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := o.client.Do(req)
	if err != nil {
		o.logger.Error("Ollama API request failed",
			"provider", o.GetProviderID(),
			"model", o.modelName,
			"error", err)

		// Check for specific error types
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("ollama API request timed out after %v", o.timeout)
		}
		return "", fmt.Errorf("ollama API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		o.logger.Error("Ollama API returned error status",
			"provider", o.GetProviderID(),
			"status", resp.StatusCode,
			"response", string(body))
		return "", fmt.Errorf("ollama API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Check for API errors
	if ollamaResp.Error != "" {
		o.logger.Error("Ollama API returned error",
			"provider", o.GetProviderID(),
			"error", ollamaResp.Error)
		return "", fmt.Errorf("ollama API error: %s", ollamaResp.Error)
	}

	// Validate response
	response := strings.TrimSpace(ollamaResp.Response)
	if response == "" {
		o.logger.Warn("Ollama API returned empty response",
			"provider", o.GetProviderID(),
			"model", o.modelName)
		return "I received an empty response from the AI service.", nil
	}

	// Unescape common escape sequences for proper Discord formatting
	unescapedResponse := o.unescapeText(response)

	o.logger.Info("Ollama API response received",
		"provider", o.GetProviderID(),
		"model", o.modelName,
		"response_length", len(unescapedResponse),
		"has_newlines", strings.Contains(unescapedResponse, "\n"),
		"newline_count", strings.Count(unescapedResponse, "\n"))

	return unescapedResponse, nil
}

// QueryAI sends a query to the Ollama API and returns the response
func (o *OllamaAIService) QueryAI(query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	// Check rate limit before proceeding
	if err := o.checkRateLimit(); err != nil {
		return "", err
	}

	o.logger.Info("Sending query to Ollama API",
		"provider", o.GetProviderID(),
		"model", o.modelName,
		"query_length", len(query))

	// Register the API call for rate limiting
	if o.rateLimiter != nil {
		if err := o.rateLimiter.RegisterCall(o.GetProviderID()); err != nil {
			o.logger.Warn("Failed to register API call for rate limiting", "error", err)
		}
	}

	// Build BMAD-constrained prompt
	bmadPrompt := o.buildBMADPrompt(query)

	response, err := o.executeQuery(bmadPrompt)
	if err != nil {
		return "", err
	}

	// Clean citations from the response
	cleanedResponse := o.cleanCitations(response)

	// Remove summary markers if present (since QueryAI doesn't return summary separately)
	cleanedResponse = o.removeSummaryMarkers(cleanedResponse)

	// Perform quality analysis if enabled
	if o.qualityEnabled {
		score := o.analyzeResponseQuality(query, cleanedResponse)
		o.updateQualityMetrics(score)

		// Log low-quality responses for monitoring
		if score.OverallScore < 0.6 {
			previewLen := 100
			if len(cleanedResponse) < previewLen {
				previewLen = len(cleanedResponse)
			}
			o.logger.Warn("Low quality response detected",
				"query", query,
				"response_preview", cleanedResponse[:previewLen],
				"overall_score", score.OverallScore,
				"issues", score.Issues)
		}
	}

	return cleanedResponse, nil
}

// QueryAIWithSummary sends a query to the Ollama API and returns both the response and extracted summary
func (o *OllamaAIService) QueryAIWithSummary(query string) (string, string, error) {
	if strings.TrimSpace(query) == "" {
		return "", "", fmt.Errorf("query cannot be empty")
	}

	// Check rate limit before proceeding
	if err := o.checkRateLimit(); err != nil {
		return "", "", err
	}

	o.logger.Info("Sending query to Ollama API with integrated summarization",
		"provider", o.GetProviderID(),
		"model", o.modelName,
		"query_length", len(query))

	// Register the API call for rate limiting
	if o.rateLimiter != nil {
		if err := o.rateLimiter.RegisterCall(o.GetProviderID()); err != nil {
			o.logger.Warn("Failed to register API call for rate limiting", "error", err)
		}
	}

	// Build BMAD-constrained prompt with summary instructions
	bmadPrompt := o.buildBMADPrompt(query)

	// Execute the query
	fullResponse, err := o.executeQuery(bmadPrompt)
	if err != nil {
		return "", "", err
	}

	// Parse the response to extract main answer and summary
	mainAnswer, summary, parseErr := o.parseResponseWithSummary(fullResponse)
	if parseErr != nil {
		o.logger.Warn("Failed to parse response with summary, returning full response",
			"error", parseErr)
		return fullResponse, "", nil
	}

	return mainAnswer, summary, nil
}

// parseResponseWithSummary extracts the main answer and summary from an integrated response
func (o *OllamaAIService) parseResponseWithSummary(response string) (string, string, error) {
	if response == "" {
		return "", "", fmt.Errorf("empty response")
	}

	// Look for various summary markers the AI might use
	summaryMarkers := []string{"[SUMMARY]:", "### Summary", "##Summary", "Summary:", "SUMMARY:"}
	var summaryIndex int = -1
	var foundMarker string

	for _, marker := range summaryMarkers {
		if idx := strings.LastIndex(response, marker); idx != -1 {
			summaryIndex = idx
			foundMarker = marker
			break
		}
	}

	if summaryIndex == -1 {
		// No summary found, return the full response as main answer with empty summary
		o.logger.Warn("No summary marker found in response, summary extraction failed",
			"response_preview", response[len(response)-min(200, len(response)):]) // Show last 200 chars for debugging
		cleanedResponse := o.removeUnnecessaryHeaders(strings.TrimSpace(response))
		return cleanedResponse, "", nil
	}

	// Extract main answer (everything before the summary marker)
	mainAnswer := strings.TrimSpace(response[:summaryIndex])

	// Remove unnecessary headers from the main answer
	mainAnswer = o.removeUnnecessaryHeaders(mainAnswer)

	// Extract summary (everything after the found marker)
	summaryStart := summaryIndex + len(foundMarker)
	summary := strings.TrimSpace(response[summaryStart:])

	// Validate summary length (Discord thread title limit is 100 characters)
	if len(summary) > 100 {
		o.logger.Warn("Summary too long, truncating",
			"original_length", len(summary),
			"summary", summary)
		summary = summary[:97] + "..."
	}

	// Validate summary is not empty
	if summary == "" {
		o.logger.Warn("Empty summary extracted")
		return mainAnswer, "", nil
	}

	o.logger.Info("Response parsed successfully",
		"main_answer_length", len(mainAnswer),
		"summary_length", len(summary),
		"summary", summary)

	// Clean citations from both main answer and summary
	mainAnswer = o.cleanCitations(mainAnswer)
	summary = o.cleanCitations(summary)

	return mainAnswer, summary, nil
}

// unescapeText converts common escape sequences to their actual characters for Discord formatting
func (o *OllamaAIService) unescapeText(text string) string {
	// Replace common escape sequences
	text = strings.ReplaceAll(text, "\\n", "\n")  // Newlines
	text = strings.ReplaceAll(text, "\\t", "\t")  // Tabs
	text = strings.ReplaceAll(text, "\\r", "\r")  // Carriage returns
	text = strings.ReplaceAll(text, "\\\"", "\"") // Quotes
	text = strings.ReplaceAll(text, "\\'", "'")   // Single quotes
	text = strings.ReplaceAll(text, "\\\\", "\\") // Backslashes (do this last)
	return text
}

// removeSummaryMarkers removes summary markers and content from text
func (o *OllamaAIService) removeSummaryMarkers(text string) string {
	summaryMarkers := []string{"[SUMMARY]:", "### Summary", "##Summary", "Summary:", "SUMMARY:"}

	for _, marker := range summaryMarkers {
		if summaryIndex := strings.LastIndex(text, marker); summaryIndex != -1 {
			// Return everything before the summary marker, trimmed
			return strings.TrimSpace(text[:summaryIndex])
		}
	}

	return text // No summary marker found
}

// removeUnnecessaryHeaders removes unnecessary headers like "### Answer" from response
func (o *OllamaAIService) removeUnnecessaryHeaders(text string) string {
	unnecessaryHeaders := []string{"### Answer", "## Answer", "# Answer", "**Answer**", "Answer:", "ANSWER:"}

	lines := strings.Split(text, "\n")
	var filteredLines []string

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		isUnnecessaryHeader := false

		for _, header := range unnecessaryHeaders {
			if trimmedLine == header {
				isUnnecessaryHeader = true
				break
			}
		}

		if !isUnnecessaryHeader {
			filteredLines = append(filteredLines, line)
		}
	}

	return strings.Join(filteredLines, "\n")
}

// cleanCitations removes citation markers like [cite: 1, 2] from response text
func (o *OllamaAIService) cleanCitations(text string) string {
	// Remove citation patterns like [cite: 1], [cite: 1, 2], [cite: 1,2,3], etc.
	citationPattern := `\[cite:[^\]]*\]`
	re := regexp.MustCompile(citationPattern)
	cleaned := re.ReplaceAllString(text, "")

	// Clean up any double spaces that might be left after removing citations
	cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")

	return strings.TrimSpace(cleaned)
}

// SummarizeQuery creates a summarized version of a user query suitable for Discord thread titles
func (o *OllamaAIService) SummarizeQuery(query string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	// Check rate limit before proceeding
	if err := o.checkRateLimit(); err != nil {
		return o.fallbackSummarize(query), nil
	}

	o.logger.Info("Creating query summary",
		"provider", o.GetProviderID(),
		"model", o.modelName,
		"query_length", len(query))

	// Register the API call for rate limiting
	if o.rateLimiter != nil {
		if err := o.rateLimiter.RegisterCall(o.GetProviderID()); err != nil {
			o.logger.Warn("Failed to register API call for rate limiting", "error", err)
		}
	}

	// Create a specialized prompt for BMAD-focused summarization
	prompt := fmt.Sprintf("Create a concise summary of this BMAD-METHOD related question in 8 words or less, suitable for a Discord thread title. Focus on the BMAD topic or concept being asked about. Do not include quotes or formatting. Question: %s", query)

	summary, err := o.executeQuery(prompt)
	if err != nil {
		// Fallback to simple truncation if AI summarization fails
		o.logger.Warn("AI summarization failed, using fallback", "provider", o.GetProviderID(), "error", err)
		return o.fallbackSummarize(query), nil
	}

	if summary == "" {
		o.logger.Warn("Ollama API returned empty summary, using fallback", "provider", o.GetProviderID())
		return o.fallbackSummarize(query), nil
	}

	// Ensure summary fits Discord's 100 character limit for thread titles
	if len(summary) > 100 {
		summary = summary[:97] + "..."
	}

	o.logger.Info("Query summary created",
		"provider", o.GetProviderID(),
		"model", o.modelName,
		"summary_length", len(summary),
		"summary", summary)

	return summary, nil
}

// fallbackSummarize provides a simple fallback summarization when AI fails
func (o *OllamaAIService) fallbackSummarize(query string) string {
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
func (o *OllamaAIService) QueryWithContext(query string, conversationHistory string) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	// Check rate limit before proceeding
	if err := o.checkRateLimit(); err != nil {
		return "", err
	}

	o.logger.Info("Sending contextual query to Ollama API",
		"provider", o.GetProviderID(),
		"model", o.modelName,
		"query_length", len(query),
		"history_length", len(conversationHistory))

	// Register the API call for rate limiting
	if o.rateLimiter != nil {
		if err := o.rateLimiter.RegisterCall(o.GetProviderID()); err != nil {
			o.logger.Warn("Failed to register API call for rate limiting", "error", err)
		}
	}

	// Build BMAD-constrained contextual prompt
	o.knowledgeBaseMu.RLock()
	bmadKnowledge := o.bmadKnowledgeBase
	o.knowledgeBaseMu.RUnlock()

	// Create a contextual prompt that includes BMAD knowledge base and conversation history
	var prompt string
	if strings.TrimSpace(conversationHistory) != "" {
		prompt = fmt.Sprintf(`%s

-----

CONVERSATION HISTORY:
%s

USER QUESTION: %s

IMPORTANT: You are bmadhelper, the BMAD-METHOD assistant agent, continuing a conversation on Discord. Answer ONLY based on the information provided in the BMAD knowledge base above. If the follow-up question refers to something mentioned earlier in the conversation, use the conversation history to understand the context. However, your answer must still be grounded in the BMAD knowledge base. If the question cannot be answered from the knowledge base, politely indicate that the information is not available in your BMAD knowledge base. If asked about release dates, updates, ETAs, or future features, remind the user: "I only have access to current BMAD documentation and cannot provide information about future updates or release schedules." Maintain any citation markers (e.g., [cite: 123]) from the source text in your response.

FORMAT YOUR RESPONSE: Use double line breaks (blank lines) between paragraphs for proper Discord readability. Structure your answer clearly with proper paragraph spacing.

After your main answer, provide a concise, 8-word or less topic summary of this conversation for Discord thread titles, prefixed with "[SUMMARY]:". This summary should focus on the BMAD topic or concept discussed. Example: "[SUMMARY]: BMAD Roles and Responsibilities".`, bmadKnowledge, conversationHistory, query)
	} else {
		// Fallback to regular BMAD query if no history
		prompt = o.buildBMADPrompt(query)
	}

	response, err := o.executeQuery(prompt)
	if err != nil {
		return "", err
	}

	// Clean citations from the response
	return o.cleanCitations(response), nil
}

// SummarizeConversation creates a summary of conversation history for context preservation
func (o *OllamaAIService) SummarizeConversation(messages []string) (string, error) {
	if len(messages) == 0 {
		return "", nil
	}

	// Check rate limit before proceeding
	if err := o.checkRateLimit(); err != nil {
		return o.fallbackConversationSummary(messages), nil
	}

	o.logger.Info("Summarizing conversation",
		"provider", o.GetProviderID(),
		"model", o.modelName,
		"message_count", len(messages))

	// Register the API call for rate limiting
	if o.rateLimiter != nil {
		if err := o.rateLimiter.RegisterCall(o.GetProviderID()); err != nil {
			o.logger.Warn("Failed to register API call for rate limiting", "error", err)
		}
	}

	// Join messages into a single conversation text
	conversationText := strings.Join(messages, "\n")

	// Create a specialized prompt for BMAD conversation summarization
	prompt := fmt.Sprintf("Summarize this BMAD-METHOD conversation in a concise way that preserves the key BMAD concepts and topics discussed. Focus on the BMAD-related questions asked and important BMAD information shared. Keep it under 500 words:\n\n%s", conversationText)

	summary, err := o.executeQuery(prompt)
	if err != nil {
		// Fallback to truncated conversation if AI summarization fails
		o.logger.Warn("AI conversation summarization failed, using fallback", "provider", o.GetProviderID(), "error", err)
		return o.fallbackConversationSummary(messages), nil
	}

	if summary == "" {
		o.logger.Warn("Ollama API returned empty conversation summary, using fallback", "provider", o.GetProviderID())
		return o.fallbackConversationSummary(messages), nil
	}

	o.logger.Info("Conversation summary created",
		"provider", o.GetProviderID(),
		"model", o.modelName,
		"summary_length", len(summary))

	return summary, nil
}

// fallbackConversationSummary provides a simple fallback when AI summarization fails
func (o *OllamaAIService) fallbackConversationSummary(messages []string) string {
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
func (o *OllamaAIService) GetProviderID() string {
	return "ollama"
}

// checkRateLimit validates that the provider is not rate limited before making a call
func (o *OllamaAIService) checkRateLimit() error {
	if o.rateLimiter == nil {
		// Rate limiting not configured - allow the call
		return nil
	}

	providerID := o.GetProviderID()
	status := o.rateLimiter.GetProviderStatus(providerID)

	if status == "Throttled" {
		usage, limit := o.rateLimiter.GetProviderUsage(providerID)
		o.logger.Warn("Rate limit exceeded for provider",
			"provider", providerID,
			"status", status,
			"usage", usage,
			"limit", limit)
		return fmt.Errorf("rate limit exceeded for provider %s: %d/%d requests",
			providerID, usage, limit)
	}

	// Log warning status but don't block the call
	if status == "Warning" {
		usage, limit := o.rateLimiter.GetProviderUsage(providerID)
		o.logger.Warn("Rate limit warning for provider",
			"provider", providerID,
			"status", status,
			"usage", usage,
			"limit", limit)
	}

	return nil
}

// SetTimeout allows customizing the HTTP client timeout
func (o *OllamaAIService) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
	o.client.Timeout = timeout
}

// LogQualityReport logs a detailed quality report to the logger
func (o *OllamaAIService) LogQualityReport() {
	if !o.qualityEnabled {
		o.logger.Info("Quality monitoring is disabled")
		return
	}

	metrics := o.GetQualityMetrics()

	if metrics.TotalResponses == 0 {
		o.logger.Info("No quality data available yet")
		return
	}

	// Calculate percentages
	lowQualityPct := float64(metrics.LowQualityResponses) / float64(metrics.TotalResponses) * 100
	emptyPct := float64(metrics.EmptyResponses) / float64(metrics.TotalResponses) * 100
	offTopicPct := float64(metrics.OffTopicResponses) / float64(metrics.TotalResponses) * 100

	o.logger.Info("=== OLLAMA QUALITY REPORT ===",
		"provider", o.GetProviderID(),
		"model", o.modelName,
		"total_responses", metrics.TotalResponses,
		"last_updated", metrics.LastUpdated.Format("2006-01-02 15:04:05"))

	o.logger.Info("Quality Scores (0.0-1.0)",
		"overall_avg", fmt.Sprintf("%.3f", metrics.AverageOverallScore),
		"bmad_coverage_avg", fmt.Sprintf("%.3f", metrics.AverageBMADScore),
		"knowledge_boundary_avg", fmt.Sprintf("%.3f", metrics.AverageBoundaryScore),
		"content_quality_avg", fmt.Sprintf("%.3f", metrics.AverageContentScore))

	o.logger.Info("Issue Statistics",
		"low_quality_responses", fmt.Sprintf("%d (%.1f%%)", metrics.LowQualityResponses, lowQualityPct),
		"empty_responses", fmt.Sprintf("%d (%.1f%%)", metrics.EmptyResponses, emptyPct),
		"off_topic_responses", fmt.Sprintf("%d (%.1f%%)", metrics.OffTopicResponses, offTopicPct))

	// Quality assessment
	var assessment string
	switch {
	case metrics.AverageOverallScore >= 0.8:
		assessment = "EXCELLENT - Model performing very well with BMAD knowledge"
	case metrics.AverageOverallScore >= 0.7:
		assessment = "GOOD - Model generally stays on topic with decent BMAD coverage"
	case metrics.AverageOverallScore >= 0.6:
		assessment = "FAIR - Model has some issues, monitoring recommended"
	case metrics.AverageOverallScore >= 0.5:
		assessment = "POOR - Model struggling with BMAD knowledge, consider alternative AI provider"
	default:
		assessment = "CRITICAL - Model performing poorly, immediate attention required"
	}

	o.logger.Info("Quality Assessment", "assessment", assessment)
}
