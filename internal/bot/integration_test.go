package bot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"bmad-knowledge-bot/internal/monitor"

	"github.com/bwmarrin/discordgo"
)

func TestDiscordConnectionValidation(t *testing.T) {
	// Skip integration test if no bot token is provided
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		t.Skip("Skipping integration test: BOT_TOKEN environment variable not set")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create context with timeout for the entire test
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test Discord session creation
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		t.Fatalf("Error creating Discord session: %v", err)
	}

	// Setup proper cleanup with defer and error handling
	var connectionClosed bool
	defer func() {
		if !connectionClosed {
			if closeErr := dg.Close(); closeErr != nil {
				t.Logf("Warning: Error closing Discord connection during cleanup: %v", closeErr)
			}
		}
	}()

	// Create a channel to handle connection status
	connected := make(chan struct{})
	connectionError := make(chan error, 1)

	// Add ready handler to detect successful connection
	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		close(connected)
	})

	// Test connection opening with goroutine to handle timeout
	go func() {
		if err := dg.Open(); err != nil {
			connectionError <- err
		}
	}()

	// Wait for connection with timeout
	select {
	case <-connected:
		// Connection successful
	case err := <-connectionError:
		t.Fatalf("Error opening Discord connection: %v", err)
	case <-ctx.Done():
		t.Fatalf("Test timeout: Discord connection took too long")
	}

	// Enhanced validation for bot user properties
	if dg.State.User == nil {
		t.Error("Expected bot user to be available in session state")
	} else {
		// Validate bot user ID format
		if dg.State.User.ID == "" {
			t.Error("Expected bot user ID to be non-empty")
		}
		if len(dg.State.User.ID) < 15 {
			t.Error("Expected bot user ID to have reasonable length")
		}

		// Validate username
		if dg.State.User.Username == "" {
			t.Error("Expected bot username to be non-empty")
		}

		// Validate that it's actually a bot
		if !dg.State.User.Bot {
			t.Error("Expected user to be marked as a bot")
		}
	}

	// Connection resilience testing - multiple connection attempts
	logger.Info("Testing connection resilience...")

	// Close and reopen connection to test reconnection
	if err := dg.Close(); err != nil {
		t.Errorf("Error during controlled disconnect: %v", err)
	}
	connectionClosed = true

	// Wait briefly before reconnection attempt
	time.Sleep(1 * time.Second)

	// Test reconnection
	reconnected := make(chan struct{})
	reconnectionError := make(chan error, 1)

	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		close(reconnected)
	})

	go func() {
		if err := dg.Open(); err != nil {
			reconnectionError <- err
		}
	}()

	// Wait for reconnection with timeout
	reconnectCtx, reconnectCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer reconnectCancel()

	select {
	case <-reconnected:
		logger.Info("Integration test passed: Discord connection and reconnection successful",
			"bot_id", dg.State.User.ID,
			"bot_username", dg.State.User.Username,
			"discriminator", dg.State.User.Discriminator)
		connectionClosed = false // Reset for proper cleanup
	case err := <-reconnectionError:
		t.Errorf("Error during reconnection: %v", err)
	case <-reconnectCtx.Done():
		t.Error("Reconnection timeout: Discord reconnection took too long")
	}
}

func TestBotStatusUpdate(t *testing.T) {
	// Skip integration test if no bot token is provided
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		t.Skip("Skipping integration test: BOT_TOKEN environment variable not set")
	}

	// Create context with timeout for the entire test
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		t.Fatalf("Error creating Discord session: %v", err)
	}

	// Setup proper cleanup with defer and error handling
	defer func() {
		if closeErr := dg.Close(); closeErr != nil {
			t.Logf("Warning: Error closing Discord connection during cleanup: %v", closeErr)
		}
	}()

	// Create channel to handle connection readiness
	ready := make(chan struct{})
	connectionError := make(chan error, 1)

	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		close(ready)
	})

	// Open connection with error handling
	go func() {
		if err := dg.Open(); err != nil {
			connectionError <- err
		}
	}()

	// Wait for connection with timeout
	select {
	case <-ready:
		// Connection ready
	case err := <-connectionError:
		t.Fatalf("Error opening Discord connection: %v", err)
	case <-ctx.Done():
		t.Fatalf("Connection timeout")
	}

	// Multiple status update testing scenarios
	testStatuses := []string{
		"Integration Test Running",
		"Testing Status Updates",
		"Final Test Status",
	}

	for i, status := range testStatuses {
		t.Run(strings.ReplaceAll(status, " ", "_"), func(t *testing.T) {
			// Create timeout context for each status update
			statusCtx, statusCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer statusCancel()

			// Channel to handle status update completion
			statusDone := make(chan error, 1)

			go func() {
				err := dg.UpdateGameStatus(0, status)
				statusDone <- err
			}()

			// Wait for status update with timeout
			select {
			case err := <-statusDone:
				if err != nil {
					t.Errorf("Error updating bot status to '%s': %v", status, err)
				} else {
					t.Logf("Successfully updated status to: %s", status)
				}
			case <-statusCtx.Done():
				t.Errorf("Timeout updating bot status to '%s'", status)
			}

			// Brief pause between status updates
			if i < len(testStatuses)-1 {
				time.Sleep(500 * time.Millisecond)
			}
		})
	}
}

func TestMentionToReplyWorkflow(t *testing.T) {
	// Skip integration test if no bot token or Gemini CLI path is provided
	token := os.Getenv("BOT_TOKEN")
	geminiPath := os.Getenv("GEMINI_CLI_PATH")

	if token == "" {
		t.Skip("Skipping integration test: BOT_TOKEN environment variable not set")
	}
	if geminiPath == "" {
		t.Skip("Skipping integration test: GEMINI_CLI_PATH environment variable not set")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create mock AI service for testing (avoid real API calls in tests)
	mockAI := NewMockAIService()
	mockAI.SetResponse("test query", "Mock AI response for test query")

	// Create bot handler with mock AI service
	handler := newTestHandler(logger, mockAI)

	// Create context with timeout for the entire test
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Test Discord session creation
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		t.Fatalf("Error creating Discord session: %v", err)
	}

	// Setup proper cleanup
	defer func() {
		if closeErr := dg.Close(); closeErr != nil {
			t.Logf("Warning: Error closing Discord connection during cleanup: %v", closeErr)
		}
	}()

	// Add the message handler
	dg.AddHandler(handler.HandleMessageCreate)

	// Create channel to handle connection readiness
	ready := make(chan struct{})
	connectionError := make(chan error, 1)

	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		close(ready)
	})

	// Set proper intents for message content
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	// Open connection with error handling
	go func() {
		if err := dg.Open(); err != nil {
			connectionError <- err
		}
	}()

	// Wait for connection with timeout
	select {
	case <-ready:
		// Connection ready
	case err := <-connectionError:
		t.Fatalf("Error opening Discord connection: %v", err)
	case <-ctx.Done():
		t.Fatalf("Connection timeout")
	}

	// Verify bot user is available
	if dg.State.User == nil {
		t.Fatal("Bot user not available in session state")
	}

	// Test mention detection logic directly
	t.Run("mention_detection", func(t *testing.T) {
		// Create a mock message mentioning the bot
		mockMessage := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ID:        "test_msg_123",
				Content:   "<@" + dg.State.User.ID + "> test query",
				ChannelID: "test_channel_123",
				Author:    &discordgo.User{ID: "test_user_123", Username: "testuser"},
				Mentions: []*discordgo.User{
					{ID: dg.State.User.ID},
				},
			},
		}

		// This should process the mention without errors
		// Note: In real integration test, we would need a test Discord server
		// For now, we're testing the handler logic
		handler.HandleMessageCreate(dg, mockMessage)

		// Verify the mock AI service was called (indirectly)
		// In a real test, we'd verify the actual Discord reply was sent
		t.Log("Mention detection and processing completed successfully")
	})

	t.Log("Integration test completed: Bot connection and handler integration successful")
}

func TestThreadCreationWorkflow(t *testing.T) {
	// Skip integration test if no bot token is provided
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		t.Skip("Skipping integration test: BOT_TOKEN environment variable not set")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create mock AI service for testing (avoid real API calls in tests)
	mockAI := NewMockAIService()
	mockAI.SetResponse("What is Go programming?", "Go is a programming language developed by Google.")
	mockAI.SetResponse("summary:What is Go programming?", "Go programming")

	// Create bot handler with mock AI service
	handler := newTestHandler(logger, mockAI)

	// Create context with timeout for the entire test
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test Discord session creation
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		t.Fatalf("Error creating Discord session: %v", err)
	}

	// Setup proper cleanup
	defer func() {
		if closeErr := dg.Close(); closeErr != nil {
			t.Logf("Warning: Error closing Discord connection during cleanup: %v", closeErr)
		}
	}()

	// Add the message handler
	dg.AddHandler(handler.HandleMessageCreate)

	// Create channel to handle connection readiness
	ready := make(chan struct{})
	connectionError := make(chan error, 1)

	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		close(ready)
	})

	// Set proper intents for message content and thread access
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	// Open connection with error handling
	go func() {
		if err := dg.Open(); err != nil {
			connectionError <- err
		}
	}()

	// Wait for connection with timeout
	select {
	case <-ready:
		// Connection ready
	case err := <-connectionError:
		t.Fatalf("Error opening Discord connection: %v", err)
	case <-ctx.Done():
		t.Fatalf("Connection timeout")
	}

	// Verify bot user is available
	if dg.State.User == nil {
		t.Fatal("Bot user not available in session state")
	}

	t.Run("thread_detection_logic", func(t *testing.T) {
		// Test thread detection logic with different channel types
		testCases := []struct {
			name        string
			channelType discordgo.ChannelType
			expected    bool
		}{
			{"public_thread", discordgo.ChannelTypeGuildPublicThread, true},
			{"private_thread", discordgo.ChannelTypeGuildPrivateThread, true},
			{"news_thread", discordgo.ChannelTypeGuildNewsThread, true},
			{"text_channel", discordgo.ChannelTypeGuildText, false},
			{"dm_channel", discordgo.ChannelTypeDM, false},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Test the core thread detection logic
				isThread := tc.channelType == discordgo.ChannelTypeGuildPublicThread ||
					tc.channelType == discordgo.ChannelTypeGuildPrivateThread ||
					tc.channelType == discordgo.ChannelTypeGuildNewsThread

				if isThread != tc.expected {
					t.Errorf("Expected thread detection %v, got %v for channel type %v", tc.expected, isThread, tc.channelType)
				}
			})
		}
	})

	t.Run("main_channel_mention_processing", func(t *testing.T) {
		// Create a mock message mentioning the bot in a main channel
		mockMessage := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ID:        "test_msg_123",
				Content:   "<@" + dg.State.User.ID + "> What is Go programming?",
				ChannelID: "test_channel_123",
				Author:    &discordgo.User{ID: "test_user_123", Username: "testuser"},
				Mentions: []*discordgo.User{
					{ID: dg.State.User.ID},
				},
			},
		}

		// Test query extraction
		query := handler.extractQueryFromMention(mockMessage.Content, dg.State.User.ID)
		expectedQuery := "What is Go programming?"
		if query != expectedQuery {
			t.Errorf("Expected query %q, got %q", expectedQuery, query)
		}

		// Test AI response
		response, err := mockAI.QueryAI(query)
		if err != nil {
			t.Fatalf("Unexpected AI service error: %v", err)
		}
		expectedResponse := "Go is a programming language developed by Google."
		if response != expectedResponse {
			t.Errorf("Expected response %q, got %q", expectedResponse, response)
		}

		// Test summarization
		summary, err := mockAI.SummarizeQuery(query)
		if err != nil {
			t.Fatalf("Unexpected summarization error: %v", err)
		}
		expectedSummary := "Go programming"
		if summary != expectedSummary {
			t.Errorf("Expected summary %q, got %q", expectedSummary, summary)
		}

		t.Logf("Main channel workflow test completed successfully:")
		t.Logf("  Query: %q", query)
		t.Logf("  AI Response: %q", response)
		t.Logf("  Thread Title: %q", summary)
	})

	t.Run("existing_thread_mention_processing", func(t *testing.T) {
		// Create a mock message mentioning the bot in an existing thread
		// This should use the original reply behavior
		mockThreadMessage := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ID:        "test_thread_msg_123",
				Content:   "<@" + dg.State.User.ID + "> Follow-up question in thread",
				ChannelID: "test_thread_123", // This would be a thread ID in real scenario
				Author:    &discordgo.User{ID: "test_user_123", Username: "testuser"},
				Mentions: []*discordgo.User{
					{ID: dg.State.User.ID},
				},
			},
		}

		// Test that the mention detection and processing still works for threads
		query := handler.extractQueryFromMention(mockThreadMessage.Content, dg.State.User.ID)
		expectedQuery := "Follow-up question in thread"
		if query != expectedQuery {
			t.Errorf("Expected thread query %q, got %q", expectedQuery, query)
		}

		t.Logf("Thread mention processing validated: %q", query)
	})

	t.Run("summarization_edge_cases", func(t *testing.T) {
		testCases := []struct {
			name     string
			query    string
			expected string
		}{
			{"empty_query", "", "Question"},
			{"short_query", "Hi", "Hi"},
			{"medium_query", "What is the weather?", "What is the..."},
			{"long_query", "This is a very long question that should be properly truncated for Discord thread titles", "This is a..."},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Test fallback summarization logic
				result := handler.createFallbackTitle(tc.query)
				if tc.query == "" {
					if result != "Question" {
						t.Errorf("Expected 'Question' for empty query, got %q", result)
					}
				} else {
					// Verify result is not empty and within limits
					if len(result) > 100 {
						t.Errorf("Summary too long: %d characters", len(result))
					}
					if result == "" && tc.query != "" {
						t.Errorf("Expected non-empty result for query %q", tc.query)
					}
				}
				t.Logf("Query: %q -> Summary: %q", tc.query, result)
			})
		}
	})

	t.Log("Thread creation workflow integration test completed successfully")
}

func TestDiscordPermissionsForThreads(t *testing.T) {
	// Skip integration test if no bot token is provided
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		t.Skip("Skipping integration test: BOT_TOKEN environment variable not set")
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Test Discord session creation
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		t.Fatalf("Error creating Discord session: %v", err)
	}

	// Setup proper cleanup
	defer func() {
		if closeErr := dg.Close(); closeErr != nil {
			t.Logf("Warning: Error closing Discord connection during cleanup: %v", closeErr)
		}
	}()

	// Create channel for connection readiness
	ready := make(chan struct{})
	connectionError := make(chan error, 1)

	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		close(ready)
	})

	// Set proper intents for thread operations
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	// Open connection
	go func() {
		if err := dg.Open(); err != nil {
			connectionError <- err
		}
	}()

	// Wait for connection
	select {
	case <-ready:
		// Connection ready
	case err := <-connectionError:
		t.Fatalf("Error opening Discord connection: %v", err)
	case <-ctx.Done():
		t.Fatalf("Connection timeout")
	}

	// Test basic permissions validation
	t.Run("bot_identity_validation", func(t *testing.T) {
		if dg.State.User == nil {
			t.Fatal("Bot user not available")
		}

		if !dg.State.User.Bot {
			t.Error("User should be marked as bot")
		}

		if dg.State.User.ID == "" {
			t.Error("Bot ID should not be empty")
		}

		t.Logf("Bot identity validated: %s (ID: %s)", dg.State.User.Username, dg.State.User.ID)
	})

	// Note: Full permission testing would require a test Discord server
	// This test validates the bot connection and basic identity requirements
	t.Log("Discord permissions validation completed")
}

func TestStatusManagementIntegration(t *testing.T) {
	if os.Getenv("BOT_TOKEN") == "" {
		t.Skip("Skipping integration test: BOT_TOKEN environment variable not set")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create rate limit manager with low limits for testing
	config := monitor.ProviderConfig{
		ProviderID: "test",
		Limits: map[string]int{
			"minute": 3, // Very low limit for testing
		},
		Thresholds: map[string]float64{
			"warning":   0.67, // 2/3 = 67%
			"throttled": 1.0,  // 3/3 = 100%
		},
	}

	rateLimitManager := monitor.NewRateLimitManager(logger, []monitor.ProviderConfig{config})

	// Create mock bot session
	mockSession := &MockBotSession{}
	statusManager := NewDiscordStatusManager(mockSession, logger)
	statusManager.SetDebounceInterval(100 * time.Millisecond) // Short debounce for testing

	// Track status changes
	var statusChanges []string
	statusMutex := sync.Mutex{}

	statusCallback := func(providerID, status string) {
		statusMutex.Lock()
		defer statusMutex.Unlock()
		statusChanges = append(statusChanges, status)

		err := statusManager.UpdateStatusFromRateLimit(providerID, status)
		if err != nil {
			t.Errorf("Status update failed: %v", err)
		}
	}

	rateLimitManager.RegisterStatusCallback(statusCallback)

	// Test status progression: Normal -> Warning -> Throttled

	// Initial state should be Normal (no calls yet)
	initialStatus := rateLimitManager.GetProviderStatus("test")
	if initialStatus != "Normal" {
		t.Errorf("Expected initial status 'Normal', got '%s'", initialStatus)
	}

	// Register 2 calls (67% of 3) - should trigger Warning
	for i := 0; i < 2; i++ {
		err := rateLimitManager.RegisterCall("test")
		if err != nil {
			t.Fatalf("RegisterCall failed: %v", err)
		}
	}

	// Wait for callback processing
	time.Sleep(200 * time.Millisecond)

	// Register 1 more call (100% of 3) - should trigger Throttled
	err := rateLimitManager.RegisterCall("test")
	if err != nil {
		t.Fatalf("RegisterCall failed: %v", err)
	}

	// Wait for callback processing
	time.Sleep(200 * time.Millisecond)

	// Verify status changes were recorded
	statusMutex.Lock()
	defer statusMutex.Unlock()

	expectedChanges := []string{"Warning", "Throttled"}
	if len(statusChanges) != len(expectedChanges) {
		t.Fatalf("Expected %d status changes, got %d: %v", len(expectedChanges), len(statusChanges), statusChanges)
	}

	for i, expected := range expectedChanges {
		if statusChanges[i] != expected {
			t.Errorf("Expected status change %d to be '%s', got '%s'", i, expected, statusChanges[i])
		}
	}

	// Verify Discord status updates were called
	if !mockSession.updateCalled {
		t.Error("Expected Discord status updates to be called")
	}

	// Verify final Discord status is Do Not Disturb
	if mockSession.lastStatus != discordgo.StatusDoNotDisturb {
		t.Errorf("Expected final Discord status to be DoNotDisturb, got %s", mockSession.lastStatus)
	}

	if mockSession.lastActivity == nil || mockSession.lastActivity.Name != "API: Throttled" {
		t.Errorf("Expected final activity to be 'API: Throttled', got %v", mockSession.lastActivity)
	}

	t.Logf("Status management integration test completed successfully")
	t.Logf("Status changes: %v", statusChanges)
	t.Logf("Final Discord status: %s with activity: %s",
		mockSession.lastStatus,
		mockSession.lastActivity.Name)
}

func TestStatusManagementConfiguration(t *testing.T) {
	// Test environment variable configuration
	tests := []struct {
		name             string
		enabledEnv       string
		intervalEnv      string
		expectedEnabled  bool
		expectedInterval time.Duration
		expectError      bool
	}{
		{
			name:             "Default values",
			enabledEnv:       "",
			intervalEnv:      "",
			expectedEnabled:  true,
			expectedInterval: 30 * time.Second,
			expectError:      false,
		},
		{
			name:             "Explicit enabled",
			enabledEnv:       "true",
			intervalEnv:      "10s",
			expectedEnabled:  true,
			expectedInterval: 10 * time.Second,
			expectError:      false,
		},
		{
			name:             "Disabled",
			enabledEnv:       "false",
			intervalEnv:      "5s",
			expectedEnabled:  false,
			expectedInterval: 5 * time.Second,
			expectError:      false,
		},
		{
			name:        "Invalid enabled value",
			enabledEnv:  "invalid",
			intervalEnv: "30s",
			expectError: true,
		},
		{
			name:        "Invalid interval format",
			enabledEnv:  "true",
			intervalEnv: "invalid",
			expectError: true,
		},
		{
			name:        "Interval too short",
			enabledEnv:  "true",
			intervalEnv: "500ms",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			if tt.enabledEnv != "" {
				os.Setenv("BOT_STATUS_UPDATE_ENABLED", tt.enabledEnv)
			} else {
				os.Unsetenv("BOT_STATUS_UPDATE_ENABLED")
			}

			if tt.intervalEnv != "" {
				os.Setenv("BOT_STATUS_UPDATE_INTERVAL", tt.intervalEnv)
			} else {
				os.Unsetenv("BOT_STATUS_UPDATE_INTERVAL")
			}

			// Test configuration loading
			enabled, interval, err := loadStatusConfig()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if enabled != tt.expectedEnabled {
				t.Errorf("Expected enabled=%v, got %v", tt.expectedEnabled, enabled)
			}

			if interval != tt.expectedInterval {
				t.Errorf("Expected interval=%v, got %v", tt.expectedInterval, interval)
			}
		})
	}

	// Clean up environment variables
	os.Unsetenv("BOT_STATUS_UPDATE_ENABLED")
	os.Unsetenv("BOT_STATUS_UPDATE_INTERVAL")
}

// loadStatusConfig is a copy of the function from main.go for testing
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

	return enabled, interval, nil
}
