package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"bmad-knowledge-bot/internal/bot"
	"bmad-knowledge-bot/internal/monitor"
	"bmad-knowledge-bot/internal/service"
	"bmad-knowledge-bot/internal/storage"

	"github.com/bwmarrin/discordgo"
)

func main() {
	// Handle health check flag for Docker containers
	if len(os.Args) > 1 && os.Args[1] == "--health-check" {
		// Simple health check - just exit successfully
		// In a real implementation, this would check service health
		os.Exit(0)
	}

	// Initialize structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("BMAD Knowledge Bot starting up...")

	// Read and validate bot token from environment variable
	token := os.Getenv("BOT_TOKEN")
	if err := validateToken(token); err != nil {
		slog.Error("Token validation failed", "error", err)
		os.Exit(1)
	}

	// Read AI provider selection from environment variable
	aiProvider := os.Getenv("AI_PROVIDER")
	if aiProvider == "" {
		aiProvider = "gemini" // Default to Gemini
	}

	// Validate AI provider selection
	if aiProvider != "gemini" && aiProvider != "ollama" {
		slog.Error("Invalid AI provider", "provider", aiProvider, "supported", []string{"gemini", "ollama"})
		os.Exit(1)
	}

	// Validate provider-specific requirements
	if aiProvider == "gemini" {
		geminiCLIPath := os.Getenv("GEMINI_CLI_PATH")
		if err := validateGeminiCLIPath(geminiCLIPath); err != nil {
			slog.Error("Gemini CLI validation failed", "error", err)
			os.Exit(1)
		}
	}

	// Read and validate rate limiting configuration
	rateLimitConfig, err := loadRateLimitConfig(aiProvider)
	if err != nil {
		slog.Error("Failed to load rate limit configuration", "error", err)
		os.Exit(1)
	}

	// Read and validate status management configuration
	statusEnabled, statusInterval, err := loadStatusConfig()
	if err != nil {
		slog.Error("Failed to load status configuration", "error", err)
		os.Exit(1)
	}

	// Read and validate database configuration
	databasePath, recoveryWindowMinutes, err := loadDatabaseConfig()
	if err != nil {
		slog.Error("Failed to load database configuration", "error", err)
		os.Exit(1)
	}

	// Read and validate knowledge base refresh configuration
	kbConfig, err := loadKnowledgeBaseConfig()
	if err != nil {
		slog.Error("Failed to load knowledge base configuration", "error", err)
		os.Exit(1)
	}

	// Initialize storage service
	storageService := storage.NewSQLiteStorageService(databasePath)
	if err := storageService.Initialize(context.Background()); err != nil {
		slog.Error("Failed to initialize storage service", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := storageService.Close(); err != nil {
			slog.Error("Error closing storage service", "error", err)
		}
	}()

	slog.Info("Storage service initialized successfully", "database_path", databasePath)

	// Initialize rate limit manager with provider configurations
	rateLimitManager := monitor.NewRateLimitManager(logger, []monitor.ProviderConfig{rateLimitConfig})
	slog.Info("Rate limit manager initialized",
		"provider", rateLimitConfig.ProviderID,
		"limits", rateLimitConfig.Limits)

	// Initialize AI service based on provider selection
	var aiService service.AIService
	switch aiProvider {
	case "gemini":
		geminiCLIPath := os.Getenv("GEMINI_CLI_PATH")
		geminiService, err := service.NewGeminiCLIService(geminiCLIPath, logger)
		if err != nil {
			slog.Error("Failed to initialize Gemini AI service", "error", err)
			os.Exit(1)
		}
		geminiService.SetRateLimiter(rateLimitManager)
		aiService = geminiService
		slog.Info("Gemini AI service initialized successfully",
			"cli_path", geminiCLIPath,
			"provider", aiService.GetProviderID())
	case "ollama":
		ollamaService, err := service.NewOllamaAIService(logger)
		if err != nil {
			slog.Error("Failed to initialize Ollama AI service", "error", err)
			os.Exit(1)
		}
		ollamaService.SetRateLimiter(rateLimitManager)
		aiService = ollamaService
		slog.Info("Ollama AI service initialized successfully",
			"provider", aiService.GetProviderID())
	}

	slog.Info("Rate limiter configured for AI service", "provider", aiService.GetProviderID())

	// Setup graceful shutdown with context and timeout
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize knowledge base updater if enabled
	var knowledgeUpdater service.KnowledgeUpdater
	if kbConfig.Enabled {
		knowledgeUpdater = service.NewHTTPKnowledgeUpdater(*kbConfig, logger)
		if err := knowledgeUpdater.Start(ctx); err != nil {
			slog.Error("Failed to start knowledge base updater", "error", err)
			os.Exit(1)
		}
		slog.Info("Knowledge base refresh service started",
			"remote_url", kbConfig.RemoteURL,
			"interval", kbConfig.RefreshInterval)
	} else {
		slog.Info("Knowledge base refresh service disabled")
	}

	// Create bot handler with AI service and storage service
	handler := bot.NewHandler(logger, aiService, storageService)

	// Create Discord session
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		slog.Error("Error creating Discord session", "error", err)
		os.Exit(1)
	}

	// Add event handlers
	dg.AddHandler(ready)
	dg.AddHandler(handler.HandleMessageCreate)

	// Set bot intents to include message content, mention parsing, and thread access
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent | discordgo.IntentsDirectMessages

	// Open connection to Discord
	err = dg.Open()
	if err != nil {
		slog.Error("Error opening Discord connection", "error", err)
		os.Exit(1)
	}

	// Load BMAD statuses for rotation
	statusFilePath := "data/bmad_statuses.txt"
	if err := bot.LoadBMADStatuses(statusFilePath, logger); err != nil {
		slog.Warn("Failed to load BMAD statuses", "error", err, "file", statusFilePath)
		slog.Info("Using fallback status system")
	} else {
		slog.Info("BMAD statuses loaded successfully", "count", bot.GetStatusCount())
	}

	// Read BMAD status rotation configuration
	bmadStatusEnabled, bmadStatusInterval, err := loadBMADStatusConfig()
	if err != nil {
		slog.Error("Failed to load BMAD status configuration", "error", err)
		os.Exit(1)
	}

	// Initialize BMAD status rotation if enabled
	var statusRotator *bot.StatusRotator
	if bmadStatusEnabled {
		statusRotator = bot.NewStatusRotator(dg, logger)
		statusRotator.SetInterval(bmadStatusInterval)
		statusRotator.Start(ctx)
		slog.Info("BMAD status rotation started", 
			"enabled", bmadStatusEnabled,
			"interval", bmadStatusInterval,
			"total_statuses", bot.GetStatusCount())
	} else {
		slog.Info("BMAD status rotation disabled by configuration")
	}

	// Initialize legacy status management if enabled
	if statusEnabled {
		// Create bot session wrapper
		botSession := bot.NewSession(token, logger)
		botSession.SetDiscordSession(dg)

		// Create status manager
		statusManager := bot.NewDiscordStatusManager(botSession, logger)
		statusManager.SetDebounceInterval(statusInterval)

		// Register status callback with rate limiter
		statusCallback := func(providerID, status string) {
			err := statusManager.UpdateStatusFromRateLimit(providerID, status)
			if err != nil {
				slog.Warn("Failed to update Discord status from rate limit",
					"provider", providerID,
					"status", status,
					"error", err)
			}
		}
		rateLimitManager.RegisterStatusCallback(statusCallback)

		// Set initial status only if BMAD rotation is disabled
		if !bmadStatusEnabled {
			err = statusManager.SetOnline("API: Ready")
			if err != nil {
				slog.Warn("Failed to set initial Discord status", "error", err)
			} else {
				slog.Info("Legacy status management initialized successfully",
					"enabled", statusEnabled,
					"debounce_interval", statusInterval)
			}
		}
	} else {
		slog.Info("Legacy status management disabled by configuration")
	}

	// Perform message recovery for missed messages during downtime
	slog.Info("Starting message recovery process", "recovery_window_minutes", recoveryWindowMinutes)
	if err := handler.RecoverMissedMessages(dg, recoveryWindowMinutes); err != nil {
		slog.Warn("Message recovery completed with errors", "error", err)
	} else {
		slog.Info("Message recovery completed successfully")
	}

	// Log thread-related capabilities
	slog.Info("Bot is now running with thread creation capabilities. Press CTRL+C to exit.")
	slog.Info("Thread permissions note: Ensure bot has 'Create Public Threads' permission in target channels")

	// Wait for CTRL+C or other term signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	select {
	case <-sc:
		slog.Info("Shutdown signal received, initiating graceful shutdown...")
	case <-ctx.Done():
		slog.Info("Context cancelled, shutting down...")
	}

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	done := make(chan struct{})
	go func() {
		defer close(done)

		// Stop BMAD status rotator
		if statusRotator != nil && statusRotator.IsRunning() {
			statusRotator.Stop()
			slog.Info("BMAD status rotator stopped successfully")
		}

		// Stop knowledge base updater
		if knowledgeUpdater != nil {
			if err := knowledgeUpdater.Stop(); err != nil {
				slog.Error("Error stopping knowledge base updater", "error", err)
			} else {
				slog.Info("Knowledge base updater stopped successfully")
			}
		}

		if err := dg.Close(); err != nil {
			slog.Error("Error during Discord session cleanup", "error", err)
		} else {
			slog.Info("Discord session closed successfully")
		}
	}()

	select {
	case <-done:
		slog.Info("Bot shutdown completed successfully")
	case <-shutdownCtx.Done():
		slog.Warn("Shutdown timeout exceeded, forcing exit")
	}
}

// validateToken validates the Discord bot token format and content
func validateToken(token string) error {
	if token == "" {
		return fmt.Errorf("BOT_TOKEN environment variable is required")
	}

	// Basic token format validation
	token = strings.TrimSpace(token)
	if len(token) < 50 {
		return fmt.Errorf("token appears to be too short (expected at least 50 characters)")
	}

	// Discord bot tokens typically have specific patterns
	if !strings.Contains(token, ".") {
		return fmt.Errorf("token format appears invalid (missing expected separators)")
	}

	return nil
}

// loadRateLimitConfig loads rate limiting configuration from environment variables
func loadRateLimitConfig(aiProvider string) (monitor.ProviderConfig, error) {
	config := monitor.ProviderConfig{
		ProviderID: aiProvider,
		Limits:     make(map[string]int),
		Thresholds: make(map[string]float64),
	}

	// Load rate limit per minute with provider-specific or generic environment variables
	var perMinuteStr string
	if aiProvider == "gemini" {
		perMinuteStr = os.Getenv("AI_PROVIDER_GEMINI_RATE_LIMIT_PER_MINUTE")
	} else if aiProvider == "ollama" {
		perMinuteStr = os.Getenv("AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_MINUTE")
	}
	// Fallback to generic rate limit setting
	if perMinuteStr == "" {
		perMinuteStr = os.Getenv("AI_PROVIDER_RATE_LIMIT_PER_MINUTE")
	}
	if perMinuteStr == "" {
		perMinuteStr = "60" // Default value
	}
	perMinute, err := strconv.Atoi(perMinuteStr)
	if err != nil {
		return config, fmt.Errorf("invalid rate limit per minute for provider %s: %s", aiProvider, perMinuteStr)
	}
	if perMinute <= 0 {
		return config, fmt.Errorf("rate limit per minute must be positive for provider %s: %d", aiProvider, perMinute)
	}
	config.Limits["minute"] = perMinute

	// Load rate limit per day with provider-specific or generic environment variables
	var perDayStr string
	if aiProvider == "gemini" {
		perDayStr = os.Getenv("AI_PROVIDER_GEMINI_RATE_LIMIT_PER_DAY")
	} else if aiProvider == "ollama" {
		perDayStr = os.Getenv("AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_DAY")
	}
	// Fallback to generic rate limit setting
	if perDayStr == "" {
		perDayStr = os.Getenv("AI_PROVIDER_RATE_LIMIT_PER_DAY")
	}
	if perDayStr == "" {
		perDayStr = "1000" // Default value
	}
	perDay, err := strconv.Atoi(perDayStr)
	if err != nil {
		return config, fmt.Errorf("invalid rate limit per day for provider %s: %s", aiProvider, perDayStr)
	}
	if perDay <= 0 {
		return config, fmt.Errorf("rate limit per day must be positive for provider %s: %d", aiProvider, perDay)
	}
	config.Limits["day"] = perDay

	// Load warning threshold with provider-specific or generic environment variables
	var warningThresholdStr string
	if aiProvider == "gemini" {
		warningThresholdStr = os.Getenv("AI_PROVIDER_GEMINI_WARNING_THRESHOLD")
	} else if aiProvider == "ollama" {
		warningThresholdStr = os.Getenv("AI_PROVIDER_OLLAMA_WARNING_THRESHOLD")
	}
	// Fallback to generic threshold setting
	if warningThresholdStr == "" {
		warningThresholdStr = os.Getenv("AI_PROVIDER_WARNING_THRESHOLD")
	}
	if warningThresholdStr == "" {
		warningThresholdStr = "0.75" // Default value
	}
	warningThreshold, err := strconv.ParseFloat(warningThresholdStr, 64)
	if err != nil {
		return config, fmt.Errorf("invalid warning threshold for provider %s: %s", aiProvider, warningThresholdStr)
	}
	if warningThreshold <= 0 || warningThreshold >= 1 {
		return config, fmt.Errorf("warning threshold must be between 0 and 1 for provider %s: %f", aiProvider, warningThreshold)
	}
	config.Thresholds["warning"] = warningThreshold

	// Load throttled threshold with provider-specific or generic environment variables
	var throttledThresholdStr string
	if aiProvider == "gemini" {
		throttledThresholdStr = os.Getenv("AI_PROVIDER_GEMINI_THROTTLED_THRESHOLD")
	} else if aiProvider == "ollama" {
		throttledThresholdStr = os.Getenv("AI_PROVIDER_OLLAMA_THROTTLED_THRESHOLD")
	}
	// Fallback to generic threshold setting
	if throttledThresholdStr == "" {
		throttledThresholdStr = os.Getenv("AI_PROVIDER_THROTTLED_THRESHOLD")
	}
	if throttledThresholdStr == "" {
		throttledThresholdStr = "1.0" // Default value
	}
	throttledThreshold, err := strconv.ParseFloat(throttledThresholdStr, 64)
	if err != nil {
		return config, fmt.Errorf("invalid throttled threshold for provider %s: %s", aiProvider, throttledThresholdStr)
	}
	if throttledThreshold <= 0 || throttledThreshold > 1 {
		return config, fmt.Errorf("throttled threshold must be between 0 and 1 for provider %s: %f", aiProvider, throttledThreshold)
	}
	config.Thresholds["throttled"] = throttledThreshold

	// Validate that warning threshold is less than throttled threshold
	if config.Thresholds["warning"] >= config.Thresholds["throttled"] {
		return config, fmt.Errorf("warning threshold (%f) must be less than throttled threshold (%f)",
			config.Thresholds["warning"], config.Thresholds["throttled"])
	}

	slog.Info("Rate limit configuration loaded",
		"provider", config.ProviderID,
		"minute_limit", config.Limits["minute"],
		"day_limit", config.Limits["day"],
		"warning_threshold", config.Thresholds["warning"],
		"throttled_threshold", config.Thresholds["throttled"])

	return config, nil
}

// loadStatusConfig loads status management configuration from environment variables
func loadStatusConfig() (bool, time.Duration, error) {
	// Load status update enabled flag (default: true)
	enabledStr := os.Getenv("BOT_STATUS_UPDATE_ENABLED")
	if enabledStr == "" {
		enabledStr = "true" // Default value
	}

	enabled, err := strconv.ParseBool(enabledStr)
	if err != nil {
		return false, 0, fmt.Errorf("invalid BOT_STATUS_UPDATE_ENABLED: %s", enabledStr)
	}

	// Load status update interval (default: 30s)
	intervalStr := os.Getenv("BOT_STATUS_UPDATE_INTERVAL")
	if intervalStr == "" {
		intervalStr = "30s" // Default value
	}

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		return false, 0, fmt.Errorf("invalid BOT_STATUS_UPDATE_INTERVAL: %s", intervalStr)
	}

	if interval < time.Second {
		return false, 0, fmt.Errorf("BOT_STATUS_UPDATE_INTERVAL must be at least 1 second: %s", intervalStr)
	}

	slog.Info("Status management configuration loaded",
		"enabled", enabled,
		"update_interval", interval)

	return enabled, interval, nil
}

// validateGeminiCLIPath validates the Gemini CLI path and accessibility
func validateGeminiCLIPath(cliPath string) error {
	if cliPath == "" {
		return fmt.Errorf("GEMINI_CLI_PATH environment variable is required")
	}

	// Check if the file exists and is accessible
	if _, err := os.Stat(cliPath); os.IsNotExist(err) {
		return fmt.Errorf("gemini CLI not found at path: %s", cliPath)
	}

	return nil
}

// ready event handler - called when bot connects and is ready
func ready(s *discordgo.Session, event *discordgo.Ready) {
	// Set bot status to "Online" upon successful connection
	err := s.UpdateGameStatus(0, "Ready to serve!")
	if err != nil {
		slog.Error("Error setting bot status", "error", err)
		return
	}

	slog.Info("Bot connected successfully",
		"username", event.User.Username,
		"discriminator", event.User.Discriminator,
		"status", "Online")
}

// loadDatabaseConfig loads database configuration from environment variables
func loadDatabaseConfig() (string, int, error) {
	// Load database path (default: "./data/bot_state.db")
	databasePath := os.Getenv("DATABASE_PATH")
	if databasePath == "" {
		databasePath = "./data/bot_state.db" // Default value
	}

	// Load message recovery window in minutes (default: 5)
	recoveryWindowStr := os.Getenv("MESSAGE_RECOVERY_WINDOW_MINUTES")
	if recoveryWindowStr == "" {
		recoveryWindowStr = "5" // Default value
	}

	recoveryWindowMinutes, err := strconv.Atoi(recoveryWindowStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid MESSAGE_RECOVERY_WINDOW_MINUTES: %s", recoveryWindowStr)
	}

	if recoveryWindowMinutes < 0 {
		return "", 0, fmt.Errorf("MESSAGE_RECOVERY_WINDOW_MINUTES must be non-negative: %d", recoveryWindowMinutes)
	}

	slog.Info("Database configuration loaded",
		"database_path", databasePath,
		"recovery_window_minutes", recoveryWindowMinutes)

	return databasePath, recoveryWindowMinutes, nil
}

// loadKnowledgeBaseConfig loads knowledge base refresh configuration from environment variables
func loadKnowledgeBaseConfig() (*service.Config, error) {
	// Load enabled flag (default: true)
	enabledStr := os.Getenv("BMAD_KB_REFRESH_ENABLED")
	if enabledStr == "" {
		enabledStr = "true" // Default value
	}

	enabled, err := strconv.ParseBool(enabledStr)
	if err != nil {
		return nil, fmt.Errorf("invalid BMAD_KB_REFRESH_ENABLED: %s", enabledStr)
	}

	// Load refresh interval in hours (default: 6)
	intervalHoursStr := os.Getenv("BMAD_KB_REFRESH_INTERVAL_HOURS")
	if intervalHoursStr == "" {
		intervalHoursStr = "6" // Default value
	}

	intervalHours, err := strconv.Atoi(intervalHoursStr)
	if err != nil {
		return nil, fmt.Errorf("invalid BMAD_KB_REFRESH_INTERVAL_HOURS: %s", intervalHoursStr)
	}

	if intervalHours <= 0 {
		return nil, fmt.Errorf("BMAD_KB_REFRESH_INTERVAL_HOURS must be positive: %d", intervalHours)
	}

	// Load remote URL (default: GitHub raw link)
	remoteURL := os.Getenv("BMAD_KB_REMOTE_URL")
	if remoteURL == "" {
		remoteURL = "https://github.com/bmadcode/BMAD-METHOD/raw/refs/heads/main/bmad-core/data/bmad-kb.md"
	}

	config := &service.Config{
		RemoteURL:       remoteURL,
		LocalFilePath:   "internal/knowledge/bmad.md",
		RefreshInterval: time.Duration(intervalHours) * time.Hour,
		Enabled:         enabled,
		HTTPTimeout:     30 * time.Second,
		RetryAttempts:   3,
		RetryDelay:      time.Second,
	}

	slog.Info("Knowledge base configuration loaded",
		"enabled", enabled,
		"remote_url", remoteURL,
		"local_file", config.LocalFilePath,
		"interval_hours", intervalHours,
		"http_timeout", config.HTTPTimeout)

	return config, nil
}

// loadBMADStatusConfig loads BMAD status rotation configuration from environment variables
func loadBMADStatusConfig() (bool, time.Duration, error) {
	// Load BMAD status rotation enabled flag (default: true)
	enabledStr := os.Getenv("BMAD_STATUS_ROTATION_ENABLED")
	if enabledStr == "" {
		enabledStr = "true" // Default value
	}

	enabled, err := strconv.ParseBool(enabledStr)
	if err != nil {
		return false, 0, fmt.Errorf("invalid BMAD_STATUS_ROTATION_ENABLED: %s", enabledStr)
	}

	// Load BMAD status rotation interval (default: 5m)
	intervalStr := os.Getenv("BMAD_STATUS_ROTATION_INTERVAL")
	if intervalStr == "" {
		intervalStr = "5m" // Default value
	}

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		return false, 0, fmt.Errorf("invalid BMAD_STATUS_ROTATION_INTERVAL: %s", intervalStr)
	}

	if interval < 30*time.Second {
		return false, 0, fmt.Errorf("BMAD_STATUS_ROTATION_INTERVAL must be at least 30 seconds: %s", intervalStr)
	}

	slog.Info("BMAD status rotation configuration loaded",
		"enabled", enabled,
		"rotation_interval", interval)

	return enabled, interval, nil
}
