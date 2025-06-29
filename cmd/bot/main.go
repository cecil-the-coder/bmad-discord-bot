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

	// Read and validate Gemini CLI path from environment variable
	geminiCLIPath := os.Getenv("GEMINI_CLI_PATH")
	if err := validateGeminiCLIPath(geminiCLIPath); err != nil {
		slog.Error("Gemini CLI validation failed", "error", err)
		os.Exit(1)
	}

	// Read and validate rate limiting configuration
	rateLimitConfig, err := loadRateLimitConfig()
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

	// Initialize rate limit manager with provider configurations
	rateLimitManager := monitor.NewRateLimitManager(logger, []monitor.ProviderConfig{rateLimitConfig})
	slog.Info("Rate limit manager initialized",
		"provider", rateLimitConfig.ProviderID,
		"limits", rateLimitConfig.Limits)

	// Initialize AI service
	geminiService, err := service.NewGeminiCLIService(geminiCLIPath, logger)
	if err != nil {
		slog.Error("Failed to initialize AI service", "error", err)
		os.Exit(1)
	}

	// Set rate limiter for AI service
	geminiService.SetRateLimiter(rateLimitManager)

	// Cast to interface for use throughout the application
	var aiService service.AIService = geminiService
	slog.Info("Rate limiter configured for AI service", "provider", aiService.GetProviderID())

	slog.Info("AI service initialized successfully",
		"cli_path", geminiCLIPath,
		"provider", aiService.GetProviderID())

	// Create bot handler with AI service
	handler := bot.NewHandler(logger, aiService)

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

	// Initialize status management if enabled
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

		// Set initial status
		err = statusManager.SetOnline("API: Ready")
		if err != nil {
			slog.Warn("Failed to set initial Discord status", "error", err)
		} else {
			slog.Info("Status management initialized successfully",
				"enabled", statusEnabled,
				"debounce_interval", statusInterval)
		}
	} else {
		slog.Info("Status management disabled by configuration")
	}

	// Log thread-related capabilities
	slog.Info("Bot is now running with thread creation capabilities. Press CTRL+C to exit.")
	slog.Info("Thread permissions note: Ensure bot has 'Create Public Threads' permission in target channels")

	// Setup graceful shutdown with context and timeout
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
func loadRateLimitConfig() (monitor.ProviderConfig, error) {
	config := monitor.ProviderConfig{
		ProviderID: "gemini",
		Limits:     make(map[string]int),
		Thresholds: make(map[string]float64),
	}

	// Load rate limit per minute (default: 60)
	perMinuteStr := os.Getenv("AI_PROVIDER_GEMINI_RATE_LIMIT_PER_MINUTE")
	if perMinuteStr == "" {
		perMinuteStr = "60" // Default value
	}
	perMinute, err := strconv.Atoi(perMinuteStr)
	if err != nil {
		return config, fmt.Errorf("invalid AI_PROVIDER_GEMINI_RATE_LIMIT_PER_MINUTE: %s", perMinuteStr)
	}
	if perMinute <= 0 {
		return config, fmt.Errorf("AI_PROVIDER_GEMINI_RATE_LIMIT_PER_MINUTE must be positive: %d", perMinute)
	}
	config.Limits["minute"] = perMinute

	// Load rate limit per day (default: 1000)
	perDayStr := os.Getenv("AI_PROVIDER_GEMINI_RATE_LIMIT_PER_DAY")
	if perDayStr == "" {
		perDayStr = "1000" // Default value
	}
	perDay, err := strconv.Atoi(perDayStr)
	if err != nil {
		return config, fmt.Errorf("invalid AI_PROVIDER_GEMINI_RATE_LIMIT_PER_DAY: %s", perDayStr)
	}
	if perDay <= 0 {
		return config, fmt.Errorf("AI_PROVIDER_GEMINI_RATE_LIMIT_PER_DAY must be positive: %d", perDay)
	}
	config.Limits["day"] = perDay

	// Load warning threshold (default: 0.75)
	warningThresholdStr := os.Getenv("AI_PROVIDER_GEMINI_WARNING_THRESHOLD")
	if warningThresholdStr == "" {
		warningThresholdStr = "0.75" // Default value
	}
	warningThreshold, err := strconv.ParseFloat(warningThresholdStr, 64)
	if err != nil {
		return config, fmt.Errorf("invalid AI_PROVIDER_GEMINI_WARNING_THRESHOLD: %s", warningThresholdStr)
	}
	if warningThreshold <= 0 || warningThreshold >= 1 {
		return config, fmt.Errorf("AI_PROVIDER_GEMINI_WARNING_THRESHOLD must be between 0 and 1: %f", warningThreshold)
	}
	config.Thresholds["warning"] = warningThreshold

	// Load throttled threshold (default: 1.0)
	throttledThresholdStr := os.Getenv("AI_PROVIDER_GEMINI_THROTTLED_THRESHOLD")
	if throttledThresholdStr == "" {
		throttledThresholdStr = "1.0" // Default value
	}
	throttledThreshold, err := strconv.ParseFloat(throttledThresholdStr, 64)
	if err != nil {
		return config, fmt.Errorf("invalid AI_PROVIDER_GEMINI_THROTTLED_THRESHOLD: %s", throttledThresholdStr)
	}
	if throttledThreshold <= 0 || throttledThreshold > 1 {
		return config, fmt.Errorf("AI_PROVIDER_GEMINI_THROTTLED_THRESHOLD must be between 0 and 1: %f", throttledThreshold)
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
