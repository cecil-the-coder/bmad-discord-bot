package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"bmad-knowledge-bot/internal/bot"
	"bmad-knowledge-bot/internal/service"
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

	// Initialize AI service
	aiService, err := service.NewGeminiCLIService(geminiCLIPath, logger)
	if err != nil {
		slog.Error("Failed to initialize AI service", "error", err)
		os.Exit(1)
	}
	slog.Info("AI service initialized successfully", "cli_path", geminiCLIPath)

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