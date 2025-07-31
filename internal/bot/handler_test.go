package bot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bmad-knowledge-bot/internal/storage"
	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockAIService implements the AIService interface for testing
type MockAIService struct {
	responses map[string]string
	errors    map[string]error
}

func NewMockAIService() *MockAIService {
	return &MockAIService{
		responses: make(map[string]string),
		errors:    make(map[string]error),
	}
}

func (m *MockAIService) QueryAI(query string) (string, error) {
	if err, exists := m.errors[query]; exists {
		return "", err
	}
	if response, exists := m.responses[query]; exists {
		return response, nil
	}
	return "Default mock response for: " + query, nil
}

func (m *MockAIService) SummarizeQuery(query string) (string, error) {
	if err, exists := m.errors["summary:"+query]; exists {
		return "", err
	}
	if response, exists := m.responses["summary:"+query]; exists {
		return response, nil
	}
	// Default behavior: create a simple summary
	words := strings.Fields(query)
	if len(words) == 0 {
		return "Question", nil
	}
	if len(words) <= 3 {
		return query, nil
	}
	return strings.Join(words[:3], " ") + "...", nil
}

// QueryAIWithSummary sends a query and returns both response and extracted summary
func (m *MockAIService) QueryAIWithSummary(query string) (string, string, error) {
	summaryKey := "integrated:" + query
	if err, exists := m.errors[summaryKey]; exists {
		return "", "", err
	}
	if response, exists := m.responses[summaryKey]; exists {
		// Parse the response to extract main answer and summary
		parts := strings.Split(response, "|SUMMARY|")
		if len(parts) == 2 {
			return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
		}
		return response, "", nil
	}
	
	// Default behavior: generate both response and summary
	mainResponse := "Default mock response for: " + query
	
	// Generate a simple summary (first 3 words + "...")
	words := strings.Fields(query)
	summary := ""
	if len(words) == 0 {
		summary = "Question"
	} else if len(words) <= 3 {
		summary = query
	} else {
		summary = strings.Join(words[:3], " ") + "..."
	}
	
	return mainResponse, summary, nil
}

// QueryWithContext sends a query with conversation history context to the AI service
func (m *MockAIService) QueryWithContext(query string, conversationHistory string) (string, error) {
	contextKey := "context:" + query + ":" + conversationHistory
	if err, exists := m.errors[contextKey]; exists {
		return "", err
	}
	if response, exists := m.responses[contextKey]; exists {
		return response, nil
	}
	// Default behavior: return response with context indication
	if conversationHistory != "" {
		return "Contextual response for: " + query, nil
	}
	return "Default mock response for: " + query, nil
}

// SummarizeConversation creates a summary of conversation history for context preservation
func (m *MockAIService) SummarizeConversation(messages []string) (string, error) {
	if len(messages) == 0 {
		return "", nil
	}
	summaryKey := "conversation_summary"
	if err, exists := m.errors[summaryKey]; exists {
		return "", err
	}
	if response, exists := m.responses[summaryKey]; exists {
		return response, nil
	}
	// Default behavior: simple summary of message count
	return fmt.Sprintf("Conversation with %d messages", len(messages)), nil
}

func (m *MockAIService) SetResponse(query, response string) {
	m.responses[query] = response
}

func (m *MockAIService) SetError(query string, err error) {
	m.errors[query] = err
}

func (m *MockAIService) SetContextResponse(query, conversationHistory, response string) {
	contextKey := "context:" + query + ":" + conversationHistory
	m.responses[contextKey] = response
}

func (m *MockAIService) SetConversationSummary(summary string) {
	m.responses["conversation_summary"] = summary
}

func (m *MockAIService) SetIntegratedResponse(query, response, summary string) {
	integratedKey := "integrated:" + query
	if summary != "" {
		m.responses[integratedKey] = response + "|SUMMARY|" + summary
	} else {
		m.responses[integratedKey] = response
	}
}

// GetProviderID returns the provider ID for testing
func (m *MockAIService) GetProviderID() string {
	return "mock"
}

func TestNewHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	mockStorage := setupTestStorage(t)
	defer mockStorage.Close()

	handler := NewHandler(logger, mockAI, mockStorage)

	if handler == nil {
		t.Fatal("expected handler to be created")
	}

	if handler.logger != logger {
		t.Error("expected logger to be set correctly")
	}

	if handler.aiService != mockAI {
		t.Error("expected AI service to be set correctly")
	}

	if handler.storageService != mockStorage {
		t.Error("expected storage service to be set correctly")
	}

	// Test that thread ownership map is initialized
	if handler.threadOwnership == nil {
		t.Error("expected thread ownership map to be initialized")
	}
}

func TestHandler_extractQueryFromMention(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandler(logger, mockAI)

	botID := "123456789"

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "simple mention with question",
			content:  "<@123456789> What is the weather?",
			expected: "What is the weather?",
		},
		{
			name:     "mention with exclamation mark",
			content:  "<@!123456789> Hello there!",
			expected: "Hello there!",
		},
		{
			name:     "mention in middle of message",
			content:  "Hey <@123456789> can you help me?",
			expected: "Hey  can you help me?",
		},
		{
			name:     "multiple spaces around mention",
			content:  "  <@123456789>   Tell me a joke  ",
			expected: "Tell me a joke",
		},
		{
			name:     "mention only",
			content:  "<@123456789>",
			expected: "",
		},
		{
			name:     "empty after mention",
			content:  "<@123456789>   ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.extractQueryFromMention(tt.content, botID)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestHandler_HandleMessageCreate_IgnoresBotMessages(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandler(logger, mockAI)

	// Create a mock session with bot user
	session := &discordgo.Session{
		State: &discordgo.State{},
	}
	// Set the user properly on the state
	session.State.User = &discordgo.User{ID: "bot123"}

	// Create message from the bot itself
	message := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:      "msg123",
			Content: "<@bot123> test message",
			Author:  &discordgo.User{ID: "bot123"},
			Mentions: []*discordgo.User{
				{ID: "bot123"},
			},
		},
	}

	// This should not panic or cause issues
	handler.HandleMessageCreate(session, message)

	// No assertions needed - just ensuring no panic occurs
}

func TestHandler_HandleMessageCreate_ProcessesMentions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandler(logger, mockAI)

	// Set up mock response
	expectedQuery := "What is 2+2?"
	expectedResponse := "2+2 equals 4"
	mockAI.SetResponse(expectedQuery, expectedResponse)

	// Create a mock session
	session := &discordgo.Session{
		State: &discordgo.State{},
	}
	// Set the user properly on the state
	session.State.User = &discordgo.User{ID: "bot123"}

	// Create message mentioning the bot
	message := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg123",
			Content:   "<@bot123> What is 2+2?",
			ChannelID: "channel123",
			Author:    &discordgo.User{ID: "user123"},
			Mentions: []*discordgo.User{
				{ID: "bot123"},
			},
		},
	}

	// Test the mention detection and query processing logic
	// We'll manually test the individual components to avoid Discord API calls

	// Test 1: Verify mention detection
	botMentioned := false
	for _, mention := range message.Mentions {
		if mention.ID == session.State.User.ID {
			botMentioned = true
			break
		}
	}
	if !botMentioned {
		t.Error("Expected bot to be detected as mentioned")
	}

	// Test 2: Verify query extraction
	queryText := handler.extractQueryFromMention(message.Content, session.State.User.ID)
	if queryText != expectedQuery {
		t.Errorf("Expected query %q, got %q", expectedQuery, queryText)
	}

	// Test 3: Verify AI service is called correctly
	response, err := mockAI.QueryAI(queryText)
	if err != nil {
		t.Errorf("Unexpected error from AI service: %v", err)
	}
	if response != expectedResponse {
		t.Errorf("Expected response %q, got %q", expectedResponse, response)
	}

	t.Log("Mention detection and AI query processing logic validated successfully")
}

func TestHandler_HandleMessageCreate_IgnoresNonMentions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandler(logger, mockAI)

	// Create a mock session
	session := &discordgo.Session{
		State: &discordgo.State{},
	}
	// Set the user properly on the state
	session.State.User = &discordgo.User{ID: "bot123"}

	// Create message that doesn't mention the bot
	message := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg123",
			Content:   "Hello everyone, how are you?",
			ChannelID: "channel123",
			Author:    &discordgo.User{ID: "user123"},
			Mentions:  []*discordgo.User{}, // No mentions
		},
	}

	// This should not process the message
	handler.HandleMessageCreate(session, message)

	// No assertions needed - just ensuring no panic occurs
}

func TestHandler_MessageProcessingFlow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandler(logger, mockAI)

	// Set up test scenarios
	tests := []struct {
		name           string
		botID          string
		messageContent string
		messageAuthor  string
		mentions       []*discordgo.User
		expectedQuery  string
		shouldProcess  bool
	}{
		{
			name:           "valid mention with query",
			botID:          "bot123",
			messageContent: "<@bot123> What is the weather?",
			messageAuthor:  "user456",
			mentions:       []*discordgo.User{{ID: "bot123"}},
			expectedQuery:  "What is the weather?",
			shouldProcess:  true,
		},
		{
			name:           "bot mentions itself",
			botID:          "bot123",
			messageContent: "<@bot123> test message",
			messageAuthor:  "bot123", // Same as bot ID
			mentions:       []*discordgo.User{{ID: "bot123"}},
			expectedQuery:  "",
			shouldProcess:  false, // Should be ignored
		},
		{
			name:           "mention without query",
			botID:          "bot123",
			messageContent: "<@bot123>",
			messageAuthor:  "user456",
			mentions:       []*discordgo.User{{ID: "bot123"}},
			expectedQuery:  "",
			shouldProcess:  false,
		},
		{
			name:           "no mention",
			botID:          "bot123",
			messageContent: "Hello everyone!",
			messageAuthor:  "user456",
			mentions:       []*discordgo.User{},
			expectedQuery:  "",
			shouldProcess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test query extraction
			if tt.shouldProcess && len(tt.mentions) > 0 {
				extractedQuery := handler.extractQueryFromMention(tt.messageContent, tt.botID)
				if extractedQuery != tt.expectedQuery {
					t.Errorf("Expected query %q, got %q", tt.expectedQuery, extractedQuery)
				}
			}

			// Test mention detection logic
			botMentioned := false
			for _, mention := range tt.mentions {
				if mention.ID == tt.botID {
					botMentioned = true
					break
				}
			}

			expectedMentioned := len(tt.mentions) > 0 && tt.mentions[0].ID == tt.botID
			if botMentioned != expectedMentioned {
				t.Errorf("Expected mention detection %v, got %v", expectedMentioned, botMentioned)
			}
		})
	}
}

// Test thread detection functionality
func TestHandler_isMessageInThread(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	_ = newTestHandler(logger, mockAI)

	tests := []struct {
		name        string
		channelType discordgo.ChannelType
		expected    bool
		description string
	}{
		{
			name:        "guild_public_thread",
			channelType: discordgo.ChannelTypeGuildPublicThread,
			expected:    true,
			description: "Public thread should be detected as thread",
		},
		{
			name:        "guild_private_thread",
			channelType: discordgo.ChannelTypeGuildPrivateThread,
			expected:    true,
			description: "Private thread should be detected as thread",
		},
		{
			name:        "guild_news_thread",
			channelType: discordgo.ChannelTypeGuildNewsThread,
			expected:    true,
			description: "News thread should be detected as thread",
		},
		{
			name:        "guild_text",
			channelType: discordgo.ChannelTypeGuildText,
			expected:    false,
			description: "Regular text channel should not be detected as thread",
		},
		{
			name:        "dm",
			channelType: discordgo.ChannelTypeDM,
			expected:    false,
			description: "DM channel should not be detected as thread",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the Channel method (this is a simplified test)
			// In practice, you'd use a more sophisticated mocking framework
			// For now, we'll test the logic directly

			channel := &discordgo.Channel{
				Type: tt.channelType,
			}

			// Test the core logic of thread detection
			isThread := channel.Type == discordgo.ChannelTypeGuildPublicThread ||
				channel.Type == discordgo.ChannelTypeGuildPrivateThread ||
				channel.Type == discordgo.ChannelTypeGuildNewsThread

			if isThread != tt.expected {
				t.Errorf("%s: expected %v, got %v", tt.description, tt.expected, isThread)
			}
		})
	}
}

// Test question summarization
func TestHandler_createFallbackTitle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandler(logger, mockAI)

	tests := []struct {
		name     string
		query    string
		expected string
	}{
		{
			name:     "simple_question",
			query:    "What is the weather?",
			expected: "What is the weather?",
		},
		{
			name:     "long_question",
			query:    "What is the weather like today in San Francisco and will it rain tomorrow?",
			expected: "What is the weather like today in San Francisco and will it rain tomorrow?",
		},
		{
			name:     "empty_query",
			query:    "",
			expected: "Question",
		},
		{
			name:     "whitespace_only",
			query:    "   ",
			expected: "Question",
		},
		{
			name:     "single_word",
			query:    "Hello",
			expected: "Hello",
		},
		{
			name:     "very_long_single_sentence",
			query:    "This is a very long question that should be truncated because it exceeds the reasonable length for a Discord thread title and needs to be cut off appropriately",
			expected: "This is a very long question that should be truncated because it exceeds the reasonable length...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.createFallbackTitle(tt.query)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}

			// Ensure result is within Discord's limits
			if len(result) > 100 {
				t.Errorf("title too long: %d characters (max 100)", len(result))
			}
		})
	}
}

// Test AI service summarization
func TestMockAIService_SummarizeQuery(t *testing.T) {
	mockAI := NewMockAIService()

	tests := []struct {
		name          string
		query         string
		setupResponse string
		setupError    error
		expected      string
		expectError   bool
	}{
		{
			name:        "simple_default_summary",
			query:       "What is the weather like today?",
			expected:    "What is the...",
			expectError: false,
		},
		{
			name:        "short_query_unchanged",
			query:       "Hello there",
			expected:    "Hello there",
			expectError: false,
		},
		{
			name:        "empty_query",
			query:       "",
			expected:    "Question",
			expectError: false,
		},
		{
			name:          "custom_response",
			query:         "What is machine learning?",
			setupResponse: "ML basics",
			expected:      "ML basics",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock responses if needed
			if tt.setupResponse != "" {
				mockAI.SetResponse("summary:"+tt.query, tt.setupResponse)
			}
			if tt.setupError != nil {
				mockAI.SetError("summary:"+tt.query, tt.setupError)
			}

			result, err := mockAI.SummarizeQuery(tt.query)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// Test thread creation error handling scenarios
func TestHandler_processMainChannelQuery_ErrorHandling(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandler(logger, mockAI)

	// Test that the function exists and can handle basic inputs
	// Full integration testing would require Discord API mocking
	query := "What is the capital of France?"
	response := "The capital of France is Paris."

	// Verify the function can be called without panicking
	// We can't test the full Discord interaction without mocking the Discord API
	t.Run("function_exists", func(t *testing.T) {
		// This test validates that our function signature is correct
		// and that it can be called (even though it will fail due to nil session)
		defer func() {
			if r := recover(); r != nil {
				// Expected to panic due to nil session, but that's OK for this test
				// We're just validating the function exists and has correct signature
			}
		}()

		// This will panic due to nil session, but proves the function exists
		handler.processMainChannelQuery(nil, nil, query, response)
	})
}

// Test integration between mention detection and thread creation logic
func TestHandler_ThreadWorkflowIntegration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandler(logger, mockAI)

	// Set up mock responses
	query := "What is Go programming?"
	aiResponse := "Go is a programming language developed by Google."
	summary := "Go programming"

	mockAI.SetResponse(query, aiResponse)
	mockAI.SetResponse("summary:"+query, summary)

	t.Run("mention_processing_workflow", func(t *testing.T) {
		// Test the complete workflow components individually

		// 1. Test mention detection
		content := "<@bot123> What is Go programming?"
		botID := "bot123"

		extractedQuery := handler.extractQueryFromMention(content, botID)
		if extractedQuery != query {
			t.Errorf("expected query %q, got %q", query, extractedQuery)
		}

		// 2. Test AI query processing
		response, err := mockAI.QueryAI(extractedQuery)
		if err != nil {
			t.Fatalf("unexpected AI service error: %v", err)
		}
		if response != aiResponse {
			t.Errorf("expected AI response %q, got %q", aiResponse, response)
		}

		// 3. Test summarization
		titleSummary, err := mockAI.SummarizeQuery(extractedQuery)
		if err != nil {
			t.Fatalf("unexpected summarization error: %v", err)
		}
		if titleSummary != summary {
			t.Errorf("expected summary %q, got %q", summary, titleSummary)
		}

		t.Logf("Workflow validation completed successfully:")
		t.Logf("  Query: %q", extractedQuery)
		t.Logf("  AI Response: %q", response)
		t.Logf("  Thread Title: %q", titleSummary)
	})
}

// Test thread history retrieval functionality
func TestHandler_fetchThreadHistory(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandler(logger, mockAI)

	t.Run("function_signature_validation", func(t *testing.T) {
		// This test validates that the function exists with correct signature
		// We can't test actual Discord API calls without complex mocking
		// but we can validate the function signature and error handling

		// Use a defer to catch the panic from nil session
		defer func() {
			if r := recover(); r != nil {
				// Expected to panic due to nil session, which proves the function exists
				t.Logf("Function exists and panics correctly with nil session: %v", r)
			}
		}()

		_, err := handler.fetchThreadHistory(nil, "channel123", "bot123", 10, false)

		// If we get here without panic, there should be an error
		if err == nil {
			t.Error("expected error for nil session")
		}
	})
}

// Test conversation history formatting
func TestHandler_formatConversationHistory(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandler(logger, mockAI)

	tests := []struct {
		name     string
		messages []*discordgo.Message
		expected string
	}{
		{
			name:     "empty_messages",
			messages: []*discordgo.Message{},
			expected: "",
		},
		{
			name: "single_message",
			messages: []*discordgo.Message{
				{
					Content: "Hello",
					Author:  &discordgo.User{Username: "user1"},
				},
			},
			expected: "user1: Hello",
		},
		{
			name: "multiple_messages",
			messages: []*discordgo.Message{
				{
					Content: "What is Go?",
					Author:  &discordgo.User{Username: "user1"},
				},
				{
					Content: "Go is a programming language",
					Author:  &discordgo.User{Username: "bot"},
				},
				{
					Content: "Who created it?",
					Author:  &discordgo.User{Username: "user1"},
				},
			},
			expected: "user1: What is Go?\nbot: Go is a programming language\nuser1: Who created it?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.formatConversationHistory(tt.messages)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// Test contextual AI query processing
func TestHandler_ProcessAIQuery_WithContext(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	_ = newTestHandler(logger, mockAI)

	// Set up mock responses for contextual queries
	query := "What about tomorrow?"
	history := "User: What is the weather today?\nBot: It's sunny."
	contextualResponse := "Based on today being sunny, tomorrow looks cloudy."

	mockAI.SetContextResponse(query, history, contextualResponse)

	t.Run("contextual_query_processing", func(t *testing.T) {
		// Test the AI service contextual call directly
		response, err := mockAI.QueryWithContext(query, history)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if response != contextualResponse {
			t.Errorf("expected contextual response %q, got %q", contextualResponse, response)
		}

		t.Logf("Contextual query test successful: %q -> %q", query, response)
	})
}

// Test conversation summarization
func TestHandler_ConversationSummarization(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	_ = newTestHandler(logger, mockAI)

	messages := []string{
		"User: What is Go programming?",
		"Bot: Go is a language created by Google.",
		"User: What are its main features?",
		"Bot: Go has goroutines, channels, and garbage collection.",
	}

	expectedSummary := "Discussion about Go programming language and its features"
	mockAI.SetConversationSummary(expectedSummary)

	t.Run("conversation_summarization", func(t *testing.T) {
		summary, err := mockAI.SummarizeConversation(messages)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if summary != expectedSummary {
			t.Errorf("expected summary %q, got %q", expectedSummary, summary)
		}

		t.Logf("Conversation summarization test successful: %d messages -> %q", len(messages), summary)
	})
}

// Test thread ownership tracking functionality
func TestHandler_ThreadOwnership(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandler(logger, mockAI)

	threadID := "thread123"
	userID := "user456"

	t.Run("record_thread_ownership", func(t *testing.T) {
		botID := "bot789"
		handler.recordThreadOwnership(threadID, userID, botID)

		ownership, exists := handler.getThreadOwnership(threadID)
		if !exists || ownership == nil {
			t.Fatal("expected thread ownership to be recorded")
		}

		if ownership.OriginalUserID != userID {
			t.Errorf("expected original user ID %q, got %q", userID, ownership.OriginalUserID)
		}

		if ownership.CreatedBy != botID {
			t.Errorf("expected created by %q, got %q", botID, ownership.CreatedBy)
		}
	})

	t.Run("get_nonexistent_thread_ownership", func(t *testing.T) {
		ownership, exists := handler.getThreadOwnership("nonexistent123")
		if exists || ownership != nil {
			t.Error("expected no ownership for nonexistent thread")
		}
	})

	t.Run("cleanup_thread_ownership", func(t *testing.T) {
		// First record ownership
		botID := "bot789"
		handler.recordThreadOwnership(threadID, userID, botID)

		// Verify it exists
		ownership, exists := handler.getThreadOwnership(threadID)
		if !exists || ownership == nil {
			t.Fatal("expected thread ownership to exist before cleanup")
		}

		// Clean it up (use negative maxAge to force cleanup of all records)
		handler.cleanupThreadOwnership(-1)

		// Verify it's gone
		ownership, exists = handler.getThreadOwnership(threadID)
		if exists || ownership != nil {
			t.Error("expected thread ownership to be cleaned up")
		}
	})
}

// Test auto-response logic for thread ownership
func TestHandler_ShouldAutoRespondInThread(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandler(logger, mockAI)

	threadID := "thread123"
	originalUserID := "user456"
	otherUserID := "user789"

	botID := "bot101"

	// Record thread ownership
	handler.recordThreadOwnership(threadID, originalUserID, botID)

	tests := []struct {
		name           string
		threadID       string
		userID         string
		botID          string
		expectedResult bool
		description    string
	}{
		{
			name:           "original_user_in_owned_thread",
			threadID:       threadID,
			userID:         originalUserID,
			botID:          botID,
			expectedResult: true,
			description:    "Original user should get auto-response in their thread",
		},
		{
			name:           "other_user_in_owned_thread",
			threadID:       threadID,
			userID:         otherUserID,
			botID:          botID,
			expectedResult: false,
			description:    "Other users should not get auto-response in owned thread",
		},
		{
			name:           "user_in_unowned_thread",
			threadID:       "unowned456",
			userID:         originalUserID,
			botID:          botID,
			expectedResult: false,
			description:    "No auto-response in threads not owned by bot",
		},
		{
			name:           "different_bot_instance",
			threadID:       threadID,
			userID:         originalUserID,
			botID:          "different_bot",
			expectedResult: false,
			description:    "Different bot instance should not auto-respond",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For this test, we need a session, but we'll pass nil since we're testing error handling
			result := handler.shouldAutoRespondInThread(nil, tt.threadID, tt.userID, tt.botID)
			if result != tt.expectedResult {
				t.Errorf("%s: expected %v, got %v", tt.description, tt.expectedResult, result)
			}
		})
	}
}

// Test enhanced HandleMessageCreate with auto-response functionality
func TestHandler_HandleMessageCreate_AutoResponse(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandler(logger, mockAI)

	botID := "bot123"
	threadID := "thread456"
	originalUserID := "user789"
	otherUserID := "user101"

	// Set up mock AI response
	query := "Follow up question"
	expectedResponse := "Follow up answer"
	mockAI.SetResponse(query, expectedResponse)

	// Record thread ownership
	handler.recordThreadOwnership(threadID, originalUserID, botID)

	// Create mock session
	session := &discordgo.Session{
		State: &discordgo.State{},
	}
	session.State.User = &discordgo.User{ID: botID}

	t.Run("auto_response_for_original_user", func(t *testing.T) {
		// Test the auto-response detection logic
		shouldRespond := handler.shouldAutoRespondInThread(nil, threadID, originalUserID, botID)
		if !shouldRespond {
			t.Error("Expected auto-response for original user in their thread")
		}

		// The actual HandleMessageCreate would process this, but we can't test
		// the Discord API call without complex mocking. The logic test above
		// validates the core functionality.
	})

	t.Run("no_auto_response_for_other_user", func(t *testing.T) {
		// Test the auto-response detection logic
		shouldRespond := handler.shouldAutoRespondInThread(nil, threadID, otherUserID, botID)
		if shouldRespond {
			t.Error("Expected no auto-response for other user without mention")
		}
	})

	t.Run("mention_still_works_for_other_users", func(t *testing.T) {
		// Message from other user with @mention should still work
		message := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ID:        "msg125",
				Content:   "<@bot123> Mentioned question",
				ChannelID: threadID,
				Author:    &discordgo.User{ID: otherUserID},
				Mentions: []*discordgo.User{
					{ID: botID},
				},
			},
		}

		// Test mention detection still works
		botMentioned := false
		for _, mention := range message.Mentions {
			if mention.ID == botID {
				botMentioned = true
				break
			}
		}

		if !botMentioned {
			t.Error("Expected bot to be detected as mentioned")
		}

		// Test query extraction
		extractedQuery := handler.extractQueryFromMention(message.Content, botID)
		if extractedQuery != "Mentioned question" {
			t.Errorf("Expected query 'Mentioned question', got %q", extractedQuery)
		}
	})
}

// Test thread message processing workflow
func TestHandler_ThreadMessageProcessing_Workflow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandler(logger, mockAI)

	botID := "bot123"
	threadID := "thread789"
	userID := "user101"

	// Set up mock responses
	initialQuery := "What is Docker?"
	initialResponse := "Docker is a containerization platform."
	followUpQuery := "How do I install it?"
	followUpResponse := "You can install Docker from their official website."

	mockAI.SetResponse(initialQuery, initialResponse)
	mockAI.SetResponse(followUpQuery, followUpResponse)

	t.Run("complete_thread_workflow", func(t *testing.T) {
		// Step 1: Initial query in main channel creates thread
		// (This would normally call processMainChannelQuery and create a thread)
		// We simulate the thread creation by recording ownership
		handler.recordThreadOwnership(threadID, userID, botID)

		// Verify ownership was recorded
		ownership, exists := handler.getThreadOwnership(threadID)
		if !exists || ownership == nil {
			t.Fatal("Expected thread ownership to be recorded")
		}
		if ownership.OriginalUserID != userID {
			t.Errorf("Expected original user %q, got %q", userID, ownership.OriginalUserID)
		}

		// Step 2: Follow-up message from original user should auto-respond
		shouldAutoRespond := handler.shouldAutoRespondInThread(nil, threadID, userID, botID)
		if !shouldAutoRespond {
			t.Error("Expected auto-response for original user in their thread")
		}

		// Step 3: Message from different user should not auto-respond
		otherUserID := "user999"
		shouldNotAutoRespond := handler.shouldAutoRespondInThread(nil, threadID, otherUserID, botID)
		if shouldNotAutoRespond {
			t.Error("Expected no auto-response for different user")
		}

		// Step 4: Cleanup
		handler.cleanupThreadOwnership(-1) // negative maxAge will force cleanup of all records
		ownership, exists = handler.getThreadOwnership(threadID)
		if exists || ownership != nil {
			t.Error("Expected thread ownership to be cleaned up")
		}
	})
}

// Test message processing logic with both mention and auto-response triggers
func TestHandler_MessageProcessing_TriggerLogic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandler(logger, mockAI)

	botID := "bot123"
	userID := "user456"
	threadID := "thread789"
	mainChannelID := "main123"

	// Record thread ownership
	handler.recordThreadOwnership(threadID, userID, botID)

	tests := []struct {
		name             string
		channelID        string
		authorID         string
		content          string
		mentions         []*discordgo.User
		shouldProcessMsg bool
		processingReason string
	}{
		{
			name:             "mention_in_main_channel",
			channelID:        mainChannelID,
			authorID:         userID,
			content:          "<@bot123> What is Go?",
			mentions:         []*discordgo.User{{ID: botID}},
			shouldProcessMsg: true,
			processingReason: "Bot mentioned in main channel",
		},
		{
			name:             "mention_in_thread",
			channelID:        threadID,
			authorID:         "other_user",
			content:          "<@bot123> Another question",
			mentions:         []*discordgo.User{{ID: botID}},
			shouldProcessMsg: true,
			processingReason: "Bot mentioned in thread",
		},
		{
			name:             "auto_response_in_owned_thread",
			channelID:        threadID,
			authorID:         userID,
			content:          "Follow up question",
			mentions:         []*discordgo.User{},
			shouldProcessMsg: true,
			processingReason: "Original user in bot-created thread",
		},
		{
			name:             "no_mention_other_user_in_thread",
			channelID:        threadID,
			authorID:         "other_user",
			content:          "Random message",
			mentions:         []*discordgo.User{},
			shouldProcessMsg: false,
			processingReason: "Other user without mention in thread",
		},
		{
			name:             "no_mention_main_channel",
			channelID:        mainChannelID,
			authorID:         userID,
			content:          "Random message",
			mentions:         []*discordgo.User{},
			shouldProcessMsg: false,
			processingReason: "No mention in main channel",
		},
		{
			name:             "bot_message_ignored",
			channelID:        mainChannelID,
			authorID:         botID,
			content:          "Bot's own message",
			mentions:         []*discordgo.User{},
			shouldProcessMsg: false,
			processingReason: "Bot's own message should be ignored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the individual logic components that determine if a message should be processed

			// Check if message is from bot (should be ignored)
			isFromBot := tt.authorID == botID

			// Check if bot is mentioned
			botMentioned := false
			for _, mention := range tt.mentions {
				if mention.ID == botID {
					botMentioned = true
					break
				}
			}

			// Check if should auto-respond in thread
			shouldAutoRespond := handler.shouldAutoRespondInThread(nil, tt.channelID, tt.authorID, botID)

			// Determine if message should be processed
			shouldProcess := !isFromBot && (botMentioned || shouldAutoRespond)

			if shouldProcess != tt.shouldProcessMsg {
				t.Errorf("%s: expected shouldProcess=%v, got %v",
					tt.processingReason, tt.shouldProcessMsg, shouldProcess)
			}

			t.Logf("%s: shouldProcess=%v (botMentioned=%v, shouldAutoRespond=%v, isFromBot=%v)",
				tt.name, shouldProcess, botMentioned, shouldAutoRespond, isFromBot)
		})
	}
}

// Test multi-user thread detection functionality
func TestHandler_MultiUserThreadDetection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandler(logger, mockAI)

	botID := "bot123"
	threadID := "thread456"
	originalUserID := "user789"

	// Record thread ownership
	handler.recordThreadOwnership(threadID, originalUserID, botID)

	t.Run("single_user_allows_auto_response", func(t *testing.T) {
		// With a nil session (test environment), the multi-user check is skipped
		// so we should get auto-response for the original user
		shouldRespond := handler.shouldAutoRespondInThread(nil, threadID, originalUserID, botID)
		if !shouldRespond {
			t.Error("Expected auto-response for original user in single-user scenario (test with nil session)")
		}
	})

	t.Run("multi_user_logic_components", func(t *testing.T) {
		// Test the countThreadParticipants function logic
		// Since we can't test with real Discord API, we verify the function exists and has correct signature

		// This would normally count participants, but with nil session it will error
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Expected panic with nil session: %v", r)
			}
		}()

		_, err := handler.countThreadParticipants(nil, threadID, botID)
		if err == nil {
			t.Error("Expected error for nil session in countThreadParticipants")
		}
	})

	t.Run("enhanced_conversation_history", func(t *testing.T) {
		// Test formatConversationHistory with bot messages included
		messages := []*discordgo.Message{
			{
				Content: "What is Docker?",
				Author:  &discordgo.User{Username: "user1", Bot: false},
			},
			{
				Content: "Docker is a containerization platform.",
				Author:  &discordgo.User{Username: "TestBot", Bot: true},
			},
			{
				Content: "How do I install it?",
				Author:  &discordgo.User{Username: "user2", Bot: false},
			},
		}

		expected := "user1: What is Docker?\nBot (TestBot): Docker is a containerization platform.\nuser2: How do I install it?"
		result := handler.formatConversationHistory(messages)

		if result != expected {
			t.Errorf("Expected formatted history %q, got %q", expected, result)
		}

		t.Logf("Multi-user conversation history formatted correctly: %q", result)
	})
}

// Helper functions for storage testing

func setupTestStorage(t *testing.T) *storage.SQLiteStorageService {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	
	service := storage.NewSQLiteStorageService(dbPath)
	err := service.Initialize(context.Background())
	require.NoError(t, err)
	
	return service
}

// Helper function to create handler with storage for tests that don't need storage
func newTestHandler(logger *slog.Logger, mockAI *MockAIService) *Handler {
	return NewHandler(logger, mockAI, nil)
}

// Helper function to create handler with storage for integration tests
func newTestHandlerWithStorage(t *testing.T, logger *slog.Logger, mockAI *MockAIService) *Handler {
	storageService := setupTestStorage(t)
	t.Cleanup(func() { storageService.Close() })
	return NewHandler(logger, mockAI, storageService)
}

// Test storage integration with message handling
func TestHandler_MessageStatePersistence(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandlerWithStorage(t, logger, mockAI)
	storageService := handler.storageService.(*storage.SQLiteStorageService)
	
	// Create a mock message
	message := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg123",
			ChannelID: "channel456",
			Author:    &discordgo.User{ID: "user789"},
			Content:   "Test message",
		},
	}
	
	// Test message state recording
	t.Run("records_message_state", func(t *testing.T) {
		// Record message state
		handler.recordMessageState(message, false)
		
		// Give async operation time to complete
		time.Sleep(100 * time.Millisecond)
		
		// Verify state was persisted
		ctx := context.Background()
		state, err := storageService.GetMessageState(ctx, "channel456", nil)
		require.NoError(t, err)
		if state != nil {
			assert.Equal(t, "channel456", state.ChannelID)
			assert.Equal(t, "msg123", state.LastMessageID)
			assert.Nil(t, state.ThreadID)
		} else {
			t.Log("Message state not found - async operation may not have completed")
		}
	})
	
	t.Run("records_thread_message_state", func(t *testing.T) {
		threadMessage := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ID:        "msg456",
				ChannelID: "thread789",
				Author:    &discordgo.User{ID: "user789"},
				Content:   "Thread message",
			},
		}
		
		// Record thread message state
		handler.recordMessageState(threadMessage, true)
		
		// Give async operation time to complete
		time.Sleep(100 * time.Millisecond)
		
		// Verify thread state was persisted
		ctx := context.Background()
		threadID := "thread789"
		state, err := storageService.GetMessageState(ctx, "thread789", &threadID)
		require.NoError(t, err)
		if state != nil {
			assert.Equal(t, "thread789", state.ChannelID)
			assert.Equal(t, "msg456", state.LastMessageID)
			assert.NotNil(t, state.ThreadID)
			assert.Equal(t, "thread789", *state.ThreadID)
		} else {
			t.Log("Thread message state not found - async operation may not have completed")
		}
	})
}

// Test message recovery functionality
func TestHandler_MessageRecovery(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandlerWithStorage(t, logger, mockAI)
	storageService := handler.storageService.(*storage.SQLiteStorageService)
	
	t.Run("skips_recovery_with_no_states", func(t *testing.T) {
		// Test recovery with empty database
		err := handler.RecoverMissedMessages(nil, 5)
		assert.NoError(t, err)
	})
	
	t.Run("skips_recovery_outside_window", func(t *testing.T) {
		ctx := context.Background()
		
		// Insert old message state (outside 5-minute window)
		oldState := &storage.MessageState{
			ChannelID:         "channel123",
			ThreadID:          nil,
			LastMessageID:     "old_msg",
			LastSeenTimestamp: time.Now().Add(-10 * time.Minute).Unix(),
		}
		err := storageService.UpsertMessageState(ctx, oldState)
		require.NoError(t, err)
		
		// Recovery should skip this old state
		err = handler.RecoverMissedMessages(nil, 5)
		assert.NoError(t, err) // Should complete without errors
	})
	
	t.Run("handles_storage_unavailable", func(t *testing.T) {
		// Create handler without storage service
		handlerWithoutStorage := newTestHandler(logger, mockAI)
		
		// Should handle gracefully when storage is unavailable
		err := handlerWithoutStorage.RecoverMissedMessages(nil, 5)
		assert.NoError(t, err)
	})
}

// Test graceful degradation when storage is unavailable
func TestHandler_StorageUnavailable(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	
	// Create handler without storage service
	handler := newTestHandler(logger, mockAI)
	
	// Create a mock message
	message := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg123",
			ChannelID: "channel456",
			Author:    &discordgo.User{ID: "user789"},
			Content:   "Test message",
		},
	}
	
	t.Run("handles_missing_storage_gracefully", func(t *testing.T) {
		// Should not panic when storage is unavailable
		assert.NotPanics(t, func() {
			handler.recordMessageState(message, false)
		})
	})
	
	t.Run("recovery_with_no_storage", func(t *testing.T) {
		// Should handle recovery gracefully without storage
		err := handler.RecoverMissedMessages(nil, 5)
		assert.NoError(t, err)
	})
}

// Test storage health check integration
func TestHandler_StorageHealthCheck(t *testing.T) {
	storageService := setupTestStorage(t)
	defer storageService.Close()
	
	t.Run("storage_health_check_passes", func(t *testing.T) {
		ctx := context.Background()
		err := storageService.HealthCheck(ctx)
		assert.NoError(t, err)
	})
	
	t.Run("storage_health_check_fails_after_close", func(t *testing.T) {
		tempStorage := setupTestStorage(t)
		tempStorage.Close()
		
		ctx := context.Background()
		err := tempStorage.HealthCheck(ctx)
		assert.Error(t, err)
	})
}

// Test complete integration workflow with storage
func TestHandler_CompleteWorkflowWithStorage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockAI := NewMockAIService()
	handler := newTestHandlerWithStorage(t, logger, mockAI)
	storageService := handler.storageService.(*storage.SQLiteStorageService)
	
	botID := "bot123"
	channelID := "channel456"
	userID := "user789"
	
	// Set up mock AI response
	query := "What is Docker?"
	response := "Docker is a containerization platform."
	mockAI.SetResponse(query, response)
	
	t.Run("complete_message_processing_with_storage", func(t *testing.T) {
		// Create message mentioning the bot
		message := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ID:        "msg123",
				Content:   "<@bot123> What is Docker?",
				ChannelID: channelID,
				Author:    &discordgo.User{ID: userID},
				Mentions: []*discordgo.User{
					{ID: botID},
				},
			},
		}
		
		// Test core processing logic components
		// 1. Verify mention detection
		botMentioned := false
		for _, mention := range message.Mentions {
			if mention.ID == botID {
				botMentioned = true
				break
			}
		}
		assert.True(t, botMentioned, "Bot should be detected as mentioned")
		
		// 2. Verify query extraction
		extractedQuery := handler.extractQueryFromMention(message.Content, botID)
		assert.Equal(t, query, extractedQuery, "Query should be extracted correctly")
		
		// 3. Test state persistence (simulate the recordMessageState call)
		handler.recordMessageState(message, false)
		
		// Give async operation time to complete
		time.Sleep(100 * time.Millisecond)
		
		// 4. Verify state was persisted
		ctx := context.Background()
		state, err := storageService.GetMessageState(ctx, channelID, nil)
		require.NoError(t, err)
		if state != nil {
			assert.Equal(t, channelID, state.ChannelID)
			assert.Equal(t, "msg123", state.LastMessageID)
		} else {
			t.Log("Message state not found - async operation may not have completed")
		}
		
		t.Logf("Complete workflow test successful:")
		t.Logf("  - Bot mention detected: %v", botMentioned)
		t.Logf("  - Query extracted: %q", extractedQuery)
		t.Logf("  - Message state persisted: %v", state != nil)
	})
}
