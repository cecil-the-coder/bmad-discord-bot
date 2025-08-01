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
	"bmad-knowledge-bot/internal/config"
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
		aiProvider = "ollama" // Default to Ollama
	}

	// Validate AI provider selection
	if aiProvider != "ollama" {
		slog.Error("Invalid AI provider", "provider", aiProvider, "supported", []string{"ollama"})
		os.Exit(1)
	}

	// No provider-specific requirements for Ollama

	// Read and validate rate limiting configuration
	rateLimitConfig, err := loadRateLimitConfig(aiProvider)
	if err != nil {
		slog.Error("Failed to load rate limit configuration", "error", err)
		os.Exit(1)
	}

	// Placeholder for status configuration - will be loaded after ConfigService
	var statusEnabled bool
	var statusInterval time.Duration

	// Read and validate database configuration
	databaseType, databasePath, recoveryWindowMinutes, err := loadDatabaseConfig()
	if err != nil {
		slog.Error("Failed to load database configuration", "error", err)
		os.Exit(1)
	}

	// Placeholder for configuration service - will be initialized after storage service
	var kbConfig *service.Config

	// Read and validate reply mention configuration
	replyMentionConfig, err := loadReplyMentionConfig()
	if err != nil {
		slog.Error("Failed to load reply mention configuration", "error", err)
		os.Exit(1)
	}

	// Read and validate reaction trigger configuration
	reactionTriggerConfig, err := loadReactionTriggerConfig()
	if err != nil {
		slog.Error("Failed to load reaction trigger configuration", "error", err)
		os.Exit(1)
	}

	// Initialize storage service
	var storageService storage.StorageService
	if databaseType == "mysql" {
		mysqlConfig, err := loadMySQLConfig()
		if err != nil {
			slog.Error("Failed to load MySQL configuration", "error", err)
			os.Exit(1)
		}
		storageService = storage.NewMySQLStorageService(mysqlConfig)
		slog.Info("Using MySQL storage service",
			"host", mysqlConfig.Host,
			"port", mysqlConfig.Port,
			"database", mysqlConfig.Database)
	} else {
		storageService = storage.NewSQLiteStorageService(databasePath)
		slog.Info("Using SQLite storage service", "database_path", databasePath)
	}

	if err := storageService.Initialize(context.Background()); err != nil {
		slog.Error("Failed to initialize storage service", "error", err, "type", databaseType)
		os.Exit(1)
	}
	defer func() {
		if err := storageService.Close(); err != nil {
			slog.Error("Error closing storage service", "error", err)
		}
	}()

	slog.Info("Storage service initialized successfully", "type", databaseType)

	// Initialize configuration service with database backend and environment fallback
	configService := config.NewHybridConfigService(storageService)
	if err := configService.Initialize(context.Background()); err != nil {
		slog.Warn("Failed to initialize database configuration service, falling back to environment variables only", "error", err)
	} else {
		slog.Info("Configuration service initialized successfully with database backend")
	}
	defer func() {
		if err := configService.Close(); err != nil {
			slog.Error("Error closing configuration service", "error", err)
		}
	}()

	// Initialize configuration loader and migrator
	configLoader := config.NewConfigurationLoader(configService)
	if err := configLoader.Initialize(context.Background()); err != nil {
		slog.Error("Failed to initialize configuration loader", "error", err)
		os.Exit(1)
	}

	// Run configuration migration on first startup
	migrator := config.NewConfigurationMigrator(configService)
	if err := migrator.MigrateEnvironmentVariables(context.Background()); err != nil {
		slog.Warn("Configuration migration completed with warnings", "error", err)
	} else {
		slog.Info("Configuration migration completed successfully")
	}

	// Seed default configurations
	if err := migrator.SeedDefaultConfigurations(context.Background()); err != nil {
		slog.Warn("Configuration seeding completed with warnings", "error", err)
	} else {
		slog.Info("Configuration seeding completed successfully")
	}

	// Start configuration auto-reload
	if err := configLoader.StartAutoReloadWithServiceNotification(1 * time.Minute); err != nil {
		slog.Warn("Failed to start configuration auto-reload", "error", err)
	} else {
		slog.Info("Configuration auto-reload started", "interval", "1m")
	}

	// Update rate limiting configuration to use ConfigService
	rateLimitConfig, err = loadRateLimitConfigFromService(aiProvider, configService)
	if err != nil {
		slog.Error("Failed to load rate limit configuration from service", "error", err)
		os.Exit(1)
	}

	// Load knowledge base configuration using ConfigService
	kbConfig, err = loadKnowledgeBaseConfigFromService(configService)
	if err != nil {
		slog.Error("Failed to load knowledge base configuration from service", "error", err)
		os.Exit(1)
	}

	// Load status configuration using ConfigService
	statusEnabled = configService.GetConfigBoolWithDefault(context.Background(), "BOT_STATUS_UPDATE_ENABLED", true)
	statusIntervalStr := configService.GetConfigWithDefault(context.Background(), "BOT_STATUS_UPDATE_INTERVAL", "30s")
	statusInterval, err = time.ParseDuration(statusIntervalStr)
	if err != nil {
		slog.Error("Failed to parse bot status update interval", "error", err, "value", statusIntervalStr)
		os.Exit(1)
	}
	if statusInterval < time.Second {
		slog.Error("Bot status update interval too short", "interval", statusInterval, "minimum", "1s")
		os.Exit(1)
	}

	// Initialize rate limit manager with provider configurations
	rateLimitManager := monitor.NewRateLimitManager(logger, []monitor.ProviderConfig{rateLimitConfig})
	slog.Info("Rate limit manager initialized",
		"provider", rateLimitConfig.ProviderID,
		"limits", rateLimitConfig.Limits)

	// Initialize AI service (Ollama only)
	aiService, err := service.NewOllamaAIService(logger)
	if err != nil {
		slog.Error("Failed to initialize Ollama AI service", "error", err)
		os.Exit(1)
	}
	aiService.SetRateLimiter(rateLimitManager)
	slog.Info("Ollama AI service initialized successfully",
		"provider", aiService.GetProviderID())

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

	// Create bot handler with AI service, storage service, and full configuration
	handler := bot.NewHandlerWithFullConfig(logger, aiService, storageService,
		bot.ReplyMentionConfig{
			DeleteReplyMessage: replyMentionConfig.DeleteReplyMessage,
		},
		bot.ReactionTriggerConfig{
			Enabled:           reactionTriggerConfig.Enabled,
			TriggerEmoji:      reactionTriggerConfig.TriggerEmoji,
			ApprovedUserIDs:   reactionTriggerConfig.ApprovedUserIDs,
			ApprovedRoleNames: reactionTriggerConfig.ApprovedRoleNames,
			RequireReaction:   reactionTriggerConfig.RequireReaction,
		})

	// Create Discord session
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		slog.Error("Error creating Discord session", "error", err)
		os.Exit(1)
	}

	// Add event handlers
	dg.AddHandler(ready)
	dg.AddHandler(handler.HandleMessageCreate)
	dg.AddHandler(handler.HandleMessageReactionAdd)

	// Set bot intents to include message content, mention parsing, thread access, and reactions
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent | discordgo.IntentsDirectMessages | discordgo.IntentsGuildMessageReactions

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

	// Read BMAD status rotation configuration using ConfigService
	bmadStatusEnabled := configService.GetConfigBoolWithDefault(context.Background(), "BMAD_STATUS_ROTATION_ENABLED", true)
	bmadStatusIntervalStr := configService.GetConfigWithDefault(context.Background(), "BMAD_STATUS_ROTATION_INTERVAL", "5m")
	bmadStatusInterval, err := time.ParseDuration(bmadStatusIntervalStr)
	if err != nil {
		slog.Error("Failed to parse BMAD status rotation interval", "error", err, "value", bmadStatusIntervalStr)
		os.Exit(1)
	}
	if bmadStatusInterval < 30*time.Second {
		slog.Error("BMAD status rotation interval too short", "interval", bmadStatusInterval, "minimum", "30s")
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

	// Perform thread ownership recovery for auto-response functionality
	slog.Info("Starting thread ownership recovery process")
	if err := handler.RecoverThreadOwnership(context.Background()); err != nil {
		slog.Warn("Thread ownership recovery completed with errors", "error", err)
	} else {
		slog.Info("Thread ownership recovery completed successfully")
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

	// Load rate limit per minute for Ollama
	perMinuteStr := os.Getenv("AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_MINUTE")
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

	// Load rate limit per day for Ollama
	perDayStr := os.Getenv("AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_DAY")
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

	// Load warning threshold for Ollama
	warningThresholdStr := os.Getenv("AI_PROVIDER_OLLAMA_WARNING_THRESHOLD")
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

	// Load throttled threshold for Ollama
	throttledThresholdStr := os.Getenv("AI_PROVIDER_OLLAMA_THROTTLED_THRESHOLD")
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
func loadDatabaseConfig() (string, string, int, error) {
	// Load database type (default: "sqlite")
	databaseType := os.Getenv("DATABASE_TYPE")
	if databaseType == "" {
		databaseType = "sqlite" // Default value for backward compatibility
	}

	// Validate database type
	if databaseType != "sqlite" && databaseType != "mysql" {
		return "", "", 0, fmt.Errorf("invalid DATABASE_TYPE: %s (supported: sqlite, mysql)", databaseType)
	}

	// Load database path (only needed for SQLite, default: "./data/bot_state.db")
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
		return "", "", 0, fmt.Errorf("invalid MESSAGE_RECOVERY_WINDOW_MINUTES: %s", recoveryWindowStr)
	}

	if recoveryWindowMinutes < 0 {
		return "", "", 0, fmt.Errorf("MESSAGE_RECOVERY_WINDOW_MINUTES must be non-negative: %d", recoveryWindowMinutes)
	}

	slog.Info("Database configuration loaded",
		"database_type", databaseType,
		"database_path", databasePath,
		"recovery_window_minutes", recoveryWindowMinutes)

	return databaseType, databasePath, recoveryWindowMinutes, nil
}

// loadMySQLConfig loads MySQL-specific configuration from environment variables
func loadMySQLConfig() (storage.MySQLConfig, error) {
	config := storage.MySQLConfig{}

	// Load MySQL host (default: "localhost")
	config.Host = os.Getenv("MYSQL_HOST")
	if config.Host == "" {
		config.Host = "localhost"
	}

	// Load MySQL port (default: "3306")
	config.Port = os.Getenv("MYSQL_PORT")
	if config.Port == "" {
		config.Port = "3306"
	}

	// Load MySQL database name (default: "bmad_bot")
	config.Database = os.Getenv("MYSQL_DATABASE")
	if config.Database == "" {
		config.Database = "bmad_bot"
	}

	// Load MySQL username (required)
	config.Username = os.Getenv("MYSQL_USERNAME")
	if config.Username == "" {
		return config, fmt.Errorf("MYSQL_USERNAME environment variable is required")
	}

	// Load MySQL password (required)
	config.Password = os.Getenv("MYSQL_PASSWORD")
	if config.Password == "" {
		return config, fmt.Errorf("MYSQL_PASSWORD environment variable is required")
	}

	// Load MySQL timeout (default: "30s")
	config.Timeout = os.Getenv("MYSQL_TIMEOUT")
	if config.Timeout == "" {
		config.Timeout = "30s"
	}

	// Validate timeout format
	if _, err := time.ParseDuration(config.Timeout); err != nil {
		return config, fmt.Errorf("invalid MYSQL_TIMEOUT format: %s", config.Timeout)
	}

	slog.Info("MySQL configuration loaded",
		"host", config.Host,
		"port", config.Port,
		"database", config.Database,
		"username", config.Username,
		"timeout", config.Timeout)

	return config, nil
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

// ReplyMentionConfig holds configuration for reply mention behavior
type ReplyMentionConfig struct {
	DeleteReplyMessage bool // Whether to delete the reply message that mentioned the bot
}

// ReactionTriggerConfig holds configuration for reaction-based bot triggers
type ReactionTriggerConfig struct {
	Enabled               bool     // Whether reaction triggers are enabled
	TriggerEmoji          string   // Emoji that triggers the bot (e.g., "❓" or "🤖")
	ApprovedUserIDs       []string // List of user IDs authorized to use reaction triggers
	ApprovedRoleNames     []string // List of role names authorized to use reaction triggers
	RequireReaction       bool     // Whether to add a confirmation reaction when processing
	RemoveTriggerReaction bool     // Whether to remove the trigger reaction after processing
}

// loadReplyMentionConfig loads reply mention configuration from environment variables
func loadReplyMentionConfig() (ReplyMentionConfig, error) {
	config := ReplyMentionConfig{}

	// Load delete reply message flag (default: false for safer behavior)
	deleteReplyStr := os.Getenv("REPLY_MENTION_DELETE_MESSAGE")
	if deleteReplyStr == "" {
		deleteReplyStr = "false" // Default value - safer to not delete by default
	}

	deleteReply, err := strconv.ParseBool(deleteReplyStr)
	if err != nil {
		return config, fmt.Errorf("invalid REPLY_MENTION_DELETE_MESSAGE: %s", deleteReplyStr)
	}

	config.DeleteReplyMessage = deleteReply

	slog.Info("Reply mention configuration loaded",
		"delete_reply_message", deleteReply)

	return config, nil
}

// loadReactionTriggerConfig loads reaction trigger configuration from environment variables
func loadReactionTriggerConfig() (ReactionTriggerConfig, error) {
	config := ReactionTriggerConfig{}

	// Load enabled flag (default: false for safer behavior)
	enabledStr := os.Getenv("REACTION_TRIGGER_ENABLED")
	if enabledStr == "" {
		enabledStr = "false" // Default value - safer to be disabled by default
	}

	enabled, err := strconv.ParseBool(enabledStr)
	if err != nil {
		return config, fmt.Errorf("invalid REACTION_TRIGGER_ENABLED: %s", enabledStr)
	}
	config.Enabled = enabled

	// Load trigger emoji (default: "❓")
	triggerEmoji := os.Getenv("REACTION_TRIGGER_EMOJI")
	if triggerEmoji == "" {
		triggerEmoji = "❓" // Default value
	}
	config.TriggerEmoji = triggerEmoji

	// Load approved user IDs (comma-separated list)
	approvedUserIDsStr := os.Getenv("REACTION_TRIGGER_APPROVED_USER_IDS")
	if approvedUserIDsStr != "" {
		config.ApprovedUserIDs = strings.Split(approvedUserIDsStr, ",")
		// Trim whitespace from each ID
		for i, id := range config.ApprovedUserIDs {
			config.ApprovedUserIDs[i] = strings.TrimSpace(id)
		}
	}

	// Load approved role names (comma-separated list)
	approvedRoleNamesStr := os.Getenv("REACTION_TRIGGER_APPROVED_ROLE_NAMES")
	if approvedRoleNamesStr != "" {
		config.ApprovedRoleNames = strings.Split(approvedRoleNamesStr, ",")
		// Trim whitespace from each role name
		for i, roleName := range config.ApprovedRoleNames {
			config.ApprovedRoleNames[i] = strings.TrimSpace(roleName)
		}
	}

	// Load require reaction flag (default: true)
	requireReactionStr := os.Getenv("REACTION_TRIGGER_REQUIRE_REACTION")
	if requireReactionStr == "" {
		requireReactionStr = "true" // Default value
	}

	requireReaction, err := strconv.ParseBool(requireReactionStr)
	if err != nil {
		return config, fmt.Errorf("invalid REACTION_TRIGGER_REQUIRE_REACTION: %s", requireReactionStr)
	}
	config.RequireReaction = requireReaction

	// Load remove trigger reaction flag (default: false)
	removeTriggerReactionStr := os.Getenv("REACTION_TRIGGER_REMOVE_REACTION")
	if removeTriggerReactionStr == "" {
		removeTriggerReactionStr = "false" // Default value - safer to leave reactions
	}

	removeTriggerReaction, err := strconv.ParseBool(removeTriggerReactionStr)
	if err != nil {
		return config, fmt.Errorf("invalid REACTION_TRIGGER_REMOVE_REACTION: %s", removeTriggerReactionStr)
	}
	config.RemoveTriggerReaction = removeTriggerReaction

	slog.Info("Reaction trigger configuration loaded",
		"enabled", enabled,
		"trigger_emoji", triggerEmoji,
		"approved_user_count", len(config.ApprovedUserIDs),
		"approved_role_count", len(config.ApprovedRoleNames),
		"require_reaction", requireReaction,
		"remove_trigger_reaction", removeTriggerReaction)

	return config, nil
}

// loadRateLimitConfigFromService loads rate limiting configuration using ConfigService
func loadRateLimitConfigFromService(aiProvider string, configService config.ConfigService) (monitor.ProviderConfig, error) {
	ctx := context.Background()
	config := monitor.ProviderConfig{
		ProviderID: aiProvider,
		Limits:     make(map[string]int),
		Thresholds: make(map[string]float64),
	}

	// Load rate limit per minute for Ollama
	perMinuteKey := "AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_MINUTE"

	perMinute := configService.GetConfigIntWithDefault(ctx, perMinuteKey, 60)
	if perMinute <= 0 {
		return config, fmt.Errorf("rate limit per minute must be positive for provider %s: %d", aiProvider, perMinute)
	}
	config.Limits["minute"] = perMinute

	// Load rate limit per day for Ollama
	perDayKey := "AI_PROVIDER_OLLAMA_RATE_LIMIT_PER_DAY"

	perDay := configService.GetConfigIntWithDefault(ctx, perDayKey, 1000)
	if perDay <= 0 {
		return config, fmt.Errorf("rate limit per day must be positive for provider %s: %d", aiProvider, perDay)
	}
	config.Limits["day"] = perDay

	// Load warning threshold for Ollama
	warningThresholdKey := "AI_PROVIDER_OLLAMA_WARNING_THRESHOLD"

	warningThresholdStr := configService.GetConfigWithDefault(ctx, warningThresholdKey, "0.75")
	warningThreshold, err := strconv.ParseFloat(warningThresholdStr, 64)
	if err != nil {
		return config, fmt.Errorf("invalid warning threshold for provider %s: %s", aiProvider, warningThresholdStr)
	}
	if warningThreshold <= 0 || warningThreshold >= 1 {
		return config, fmt.Errorf("warning threshold must be between 0 and 1 for provider %s: %f", aiProvider, warningThreshold)
	}
	config.Thresholds["warning"] = warningThreshold

	// Load throttled threshold for Ollama
	throttledThresholdKey := "AI_PROVIDER_OLLAMA_THROTTLED_THRESHOLD"

	throttledThresholdStr := configService.GetConfigWithDefault(ctx, throttledThresholdKey, "1.0")
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

	slog.Info("Rate limit configuration loaded from ConfigService",
		"provider", config.ProviderID,
		"minute_limit", config.Limits["minute"],
		"day_limit", config.Limits["day"],
		"warning_threshold", config.Thresholds["warning"],
		"throttled_threshold", config.Thresholds["throttled"])

	return config, nil
}

// loadKnowledgeBaseConfigFromService loads knowledge base configuration using ConfigService
func loadKnowledgeBaseConfigFromService(configService config.ConfigService) (*service.Config, error) {
	ctx := context.Background()

	// Load enabled flag
	enabled := configService.GetConfigBoolWithDefault(ctx, "BMAD_KB_REFRESH_ENABLED", true)

	// Load refresh interval in hours
	intervalHours := configService.GetConfigIntWithDefault(ctx, "BMAD_KB_REFRESH_INTERVAL_HOURS", 6)
	if intervalHours <= 0 {
		return nil, fmt.Errorf("BMAD_KB_REFRESH_INTERVAL_HOURS must be positive: %d", intervalHours)
	}

	// Load remote URL
	remoteURL := configService.GetConfigWithDefault(ctx, "BMAD_KB_REMOTE_URL",
		"https://github.com/bmadcode/BMAD-METHOD/raw/refs/heads/main/bmad-core/data/bmad-kb.md")

	config := &service.Config{
		RemoteURL:       remoteURL,
		LocalFilePath:   "internal/knowledge/bmad.md",
		RefreshInterval: time.Duration(intervalHours) * time.Hour,
		Enabled:         enabled,
		HTTPTimeout:     30 * time.Second,
		RetryAttempts:   3,
		RetryDelay:      time.Second,
	}

	slog.Info("Knowledge base configuration loaded from ConfigService",
		"enabled", enabled,
		"remote_url", remoteURL,
		"local_file", config.LocalFilePath,
		"interval_hours", intervalHours,
		"http_timeout", config.HTTPTimeout)

	return config, nil
}
