package bot

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"bmad-knowledge-bot/internal/service"
)

// ThreadOwnership tracks metadata for bot-created threads
type ThreadOwnership struct {
	OriginalUserID string
	CreatedBy      string // Bot ID that created the thread
	CreationTime   int64  // Unix timestamp
}

// Handler manages Discord event handling
type Handler struct {
	logger         *slog.Logger
	aiService      service.AIService
	threadOwnership map[string]*ThreadOwnership // threadID -> ownership info
}

// NewHandler creates a new bot event handler
func NewHandler(logger *slog.Logger, aiService service.AIService) *Handler {
	return &Handler{
		logger:          logger,
		aiService:       aiService,
		threadOwnership: make(map[string]*ThreadOwnership),
	}
}

// HandleMessageCreate processes incoming Discord messages and responds to mentions
func (h *Handler) HandleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore messages from the bot itself to prevent loops
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Detect thread context for proper handling
	isInThread := h.isMessageInThread(s, m.ChannelID)
	
	// Check if this is a bot-owned thread and if user should get auto-response
	shouldAutoRespond := false
	if isInThread {
		shouldAutoRespond = h.shouldAutoRespondInThread(s, m.ChannelID, m.Author.ID, s.State.User.ID)
	}
	
	// Log incoming message for debugging including thread context
	h.logger.Info("Received message",
		"author", m.Author.Username,
		"channel", m.ChannelID,
		"content_length", len(m.Content),
		"content", m.Content,
		"mentions_count", len(m.Mentions),
		"in_thread", isInThread,
		"should_auto_respond", shouldAutoRespond)

	// Log each mention for debugging
	for i, mention := range m.Mentions {
		h.logger.Info("Found mention",
			"index", i,
			"mention_id", mention.ID,
			"mention_username", mention.Username,
			"bot_id", s.State.User.ID)
	}

	// Check if the bot is mentioned in the message
	botMentioned := false
	var queryText string
	
	// Check for bot mentions in the message
	for _, mention := range m.Mentions {
		if mention.ID == s.State.User.ID {
			botMentioned = true
			h.logger.Info("Bot mention detected!", "bot_id", s.State.User.ID)
			break
		}
	}

	// Also log if no mention was found
	if !botMentioned {
		h.logger.Info("No bot mention found",
			"bot_id", s.State.User.ID,
			"mentions_count", len(m.Mentions),
			"should_auto_respond", shouldAutoRespond)
	}

	// Determine if we should process this message
	shouldProcess := botMentioned || shouldAutoRespond
	
	if shouldProcess {
		// Extract query text based on whether it's a mention or auto-response
		if botMentioned {
			queryText = h.extractQueryFromMention(m.Content, s.State.User.ID)
		} else {
			// For auto-response, use the entire message content as the query
			queryText = strings.TrimSpace(m.Content)
		}
		
		if strings.TrimSpace(queryText) == "" {
			h.logger.Info("Message processed but no query text found",
				"message_id", m.ID,
				"bot_mentioned", botMentioned,
				"auto_respond", shouldAutoRespond)
			return
		}

		h.logger.Info("Processing message",
			"query_length", len(queryText),
			"message_id", m.ID,
			"bot_mentioned", botMentioned,
			"auto_respond", shouldAutoRespond)

		// Process the AI query and respond (pass thread context)
		h.processAIQuery(s, m, queryText, isInThread)
	}
}

// isMessageInThread checks if a message is posted in a Discord thread
func (h *Handler) isMessageInThread(s *discordgo.Session, channelID string) bool {
	// Check for nil session to prevent panic in tests
	if s == nil || s.Ratelimiter == nil {
		h.logger.Error("Session or ratelimiter is nil, cannot check thread status", "channel_id", channelID)
		return false
	}
	
	// Get channel information to determine if it's a thread
	channel, err := s.Channel(channelID)
	if err != nil {
		h.logger.Error("Failed to get channel information", "error", err, "channel_id", channelID)
		return false
	}
	
	// Check if the channel type indicates it's a thread
	return channel.Type == discordgo.ChannelTypeGuildPublicThread ||
		   channel.Type == discordgo.ChannelTypeGuildPrivateThread ||
		   channel.Type == discordgo.ChannelTypeGuildNewsThread
}

// extractQueryFromMention extracts the query text from a message that mentions the bot
func (h *Handler) extractQueryFromMention(content string, botID string) string {
	// Remove bot mention patterns like <@botID> or <@!botID>
	mentionPatterns := []string{
		"<@" + botID + ">",
		"<@!" + botID + ">",
	}
	
	cleanedContent := content
	for _, pattern := range mentionPatterns {
		cleanedContent = strings.ReplaceAll(cleanedContent, pattern, "")
	}
	
	// Trim whitespace and return the remaining query
	return strings.TrimSpace(cleanedContent)
}

// processAIQuery sends the query to the AI service and replies with the response
func (h *Handler) processAIQuery(s *discordgo.Session, m *discordgo.MessageCreate, query string, isInThread bool) {
	h.logger.Info("Processing AI query", "query", query, "in_thread", isInThread)

	var response string
	var err error

	// If in thread, fetch conversation history and use contextual query
	if isInThread {
		// Fetch thread history (limit to 50 messages for reasonable context window)
		const historyLimit = 50
		
		// FIXED: Always include bot messages for proper contextual conversation
		// Bot messages are essential for understanding follow-up questions with contextual references
		includeAllMessages := true
		
		// Still count participants for logging purposes
		participantCount, countErr := h.countThreadParticipants(s, m.ChannelID, s.State.User.ID)
		if countErr != nil {
			h.logger.Warn("Failed to count thread participants", "error", countErr, "channel_id", m.ChannelID)
			participantCount = 1 // Assume single user on error
		}
		
		threadMessages, historyErr := h.fetchThreadHistory(s, m.ChannelID, s.State.User.ID, historyLimit, includeAllMessages)
		
		if historyErr != nil {
			h.logger.Error("Failed to fetch thread history, falling back to regular query",
				"error", historyErr, "channel_id", m.ChannelID)
			// Fallback to regular query if history retrieval fails
			response, err = h.aiService.QueryAI(query)
		} else {
			// Format conversation history for AI context
			conversationHistory := h.formatConversationHistory(threadMessages)
			
			h.logger.Info("Using contextual query with thread history",
				"history_messages", len(threadMessages),
				"history_length", len(conversationHistory),
				"participant_count", participantCount,
				"include_all_messages", includeAllMessages)
			
			// Use contextual query with conversation history
			response, err = h.aiService.QueryWithContext(query, conversationHistory)
		}
	} else {
		// For main channel messages, use regular query
		response, err = h.aiService.QueryAI(query)
	}

	if err != nil {
		h.logger.Error("AI service error", "error", err, "query", query)
		
		// Send user-friendly error message
		errorMsg := "I'm sorry, I encountered an error while processing your request. Please try again later."
		if _, err := s.ChannelMessageSendReply(m.ChannelID, errorMsg, m.Reference()); err != nil {
			h.logger.Error("Failed to send error reply", "error", err)
		}
		return
	}

	// If message is in main channel (not a thread), create a new thread for the conversation
	if !isInThread {
		h.processMainChannelQuery(s, m, query, response)
	} else {
		// If already in a thread, reply directly with contextual response
		if _, err := s.ChannelMessageSendReply(m.ChannelID, response, m.Reference()); err != nil {
			h.logger.Error("Failed to send AI response in thread", "error", err)
		} else {
			h.logger.Info("AI contextual response sent successfully in existing thread",
				"response_length", len(response),
				"message_id", m.ID)
		}
	}
}

// processMainChannelQuery handles AI queries from main channels by creating threads
func (h *Handler) processMainChannelQuery(s *discordgo.Session, m *discordgo.MessageCreate, query string, response string) {
	h.logger.Info("Processing main channel query, creating thread", "query", query)

	// Get a summarized title for the thread using AI service
	threadTitle, err := h.aiService.SummarizeQuery(query)
	if err != nil {
		h.logger.Error("Failed to summarize query for thread title", "error", err)
		// Use fallback title if summarization fails
		threadTitle = h.createFallbackTitle(query)
	}

	h.logger.Info("Thread title created", "title", threadTitle, "length", len(threadTitle))

	// Create a public thread in the channel
	thread, err := s.ThreadStart(m.ChannelID, threadTitle, discordgo.ChannelTypeGuildPublicThread, 60) // 60 minute auto-archive
	if err != nil {
		h.logger.Error("Failed to create thread", "error", err, "channel_id", m.ChannelID)
		
		// Fallback: reply in main channel if thread creation fails
		errorMsg := "I encountered an issue creating a thread for our conversation. Here's my response:"
		fallbackResponse := errorMsg + "\n\n" + response
		if _, err := s.ChannelMessageSendReply(m.ChannelID, fallbackResponse, m.Reference()); err != nil {
			h.logger.Error("Failed to send fallback response", "error", err)
		}
		return
	}

	h.logger.Info("Thread created successfully",
		"thread_id", thread.ID,
		"thread_name", thread.Name,
		"parent_channel", m.ChannelID)

	// Record thread ownership for auto-response functionality (AC 1.4.5)
	h.recordThreadOwnership(thread.ID, m.Author.ID, s.State.User.ID)

	// Post the AI response as the first message in the newly created thread
	if _, err := s.ChannelMessageSend(thread.ID, response); err != nil {
		h.logger.Error("Failed to send AI response in new thread", "error", err, "thread_id", thread.ID)
		
		// If we can't post in the thread, try to reply in main channel as fallback
		errorMsg := "I created a thread but couldn't post my response there. Here's my answer:"
		fallbackResponse := errorMsg + "\n\n" + response
		if _, err := s.ChannelMessageSendReply(m.ChannelID, fallbackResponse, m.Reference()); err != nil {
			h.logger.Error("Failed to send fallback response after thread creation", "error", err)
		}
	} else {
		h.logger.Info("AI response posted successfully in new thread",
			"response_length", len(response),
			"thread_id", thread.ID,
			"message_id", m.ID,
			"thread_owner", m.Author.ID)
	}
}

// createFallbackTitle creates a simple fallback title when AI summarization fails
func (h *Handler) createFallbackTitle(query string) string {
	// Simple fallback: take first few words
	words := strings.Fields(strings.TrimSpace(query))
	if len(words) == 0 {
		return "Question"
	}
	
	title := ""
	for _, word := range words {
		testTitle := title + " " + word
		if len(strings.TrimSpace(testTitle)) > 95 { // Leave room for "..."
			break
		}
		title = testTitle
	}
	
	title = strings.TrimSpace(title)
	if title == "" {
		return "Question"
	}
	
	// Add ellipsis if we truncated
	if len(words) > len(strings.Fields(title)) {
		title += "..."
	}
	
	return title
}

// fetchThreadHistory retrieves message history from a Discord thread
// Returns messages in chronological order, optionally including bot messages for multi-user threads
func (h *Handler) fetchThreadHistory(s *discordgo.Session, channelID string, botID string, limit int, includeAllMessages bool) ([]*discordgo.Message, error) {
	h.logger.Info("Fetching thread history",
		"channel_id", channelID,
		"limit", limit,
		"include_all_messages", includeAllMessages)
	
	// Fetch messages from the thread (Discord returns in reverse chronological order)
	messages, err := s.ChannelMessages(channelID, limit, "", "", "")
	if err != nil {
		h.logger.Error("Failed to fetch thread messages", "error", err, "channel_id", channelID)
		return nil, fmt.Errorf("failed to fetch thread messages: %w", err)
	}
	
	// Filter messages and reverse to chronological order
	var filteredMessages []*discordgo.Message
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		
		if includeAllMessages {
			// Include all messages for multi-user threads (better context)
			filteredMessages = append(filteredMessages, msg)
		} else {
			// Skip bot's own messages to avoid circular context (single-user threads)
			if msg.Author.ID != botID {
				filteredMessages = append(filteredMessages, msg)
			}
		}
	}
	
	h.logger.Info("Thread history retrieved",
		"total_messages", len(messages),
		"filtered_messages", len(filteredMessages),
		"channel_id", channelID,
		"include_all_messages", includeAllMessages)
	
	return filteredMessages, nil
}

// formatConversationHistory converts Discord messages to text format for AI context
func (h *Handler) formatConversationHistory(messages []*discordgo.Message) string {
	if len(messages) == 0 {
		return ""
	}
	
	var conversationText strings.Builder
	for _, msg := range messages {
		// Format: "Username: Message content" (with special handling for bot messages)
		if msg.Author.Bot {
			// Mark bot messages clearly in the conversation history
			conversationText.WriteString(fmt.Sprintf("Bot (%s): %s\n", msg.Author.Username, msg.Content))
		} else {
			conversationText.WriteString(fmt.Sprintf("%s: %s\n", msg.Author.Username, msg.Content))
		}
	}
	
	return strings.TrimSpace(conversationText.String())
}

// recordThreadOwnership stores thread ownership information for auto-response functionality
func (h *Handler) recordThreadOwnership(threadID string, originalUserID string, botID string) {
	ownership := &ThreadOwnership{
		OriginalUserID: originalUserID,
		CreatedBy:      botID,
		CreationTime:   time.Now().Unix(),
	}
	
	h.threadOwnership[threadID] = ownership
	
	h.logger.Info("Thread ownership recorded",
		"thread_id", threadID,
		"original_user", originalUserID,
		"created_by", botID)
}

// countThreadParticipants counts the number of unique users who have participated in a thread
func (h *Handler) countThreadParticipants(s *discordgo.Session, threadID string, botID string) (int, error) {
	// Fetch recent messages to count participants (limit to 100 for performance)
	messages, err := s.ChannelMessages(threadID, 100, "", "", "")
	if err != nil {
		return 0, fmt.Errorf("failed to fetch thread messages for participant count: %w", err)
	}
	
	// Use map to track unique participants (excluding the bot)
	participants := make(map[string]bool)
	for _, msg := range messages {
		if msg.Author.ID != botID {
			participants[msg.Author.ID] = true
		}
	}
	
	return len(participants), nil
}

// shouldAutoRespondInThread checks if the bot should auto-respond to a message in a thread
// Returns true if the message is from the original user in a bot-created thread AND there's only one participant
func (h *Handler) shouldAutoRespondInThread(s *discordgo.Session, threadID string, authorID string, botID string) bool {
	ownership, exists := h.threadOwnership[threadID]
	if !exists {
		// Thread not tracked as bot-created
		return false
	}
	
	// Check if the message author is the original user who started the conversation
	if ownership.OriginalUserID != authorID {
		h.logger.Info("Message from different user in bot thread, requiring mention",
			"thread_id", threadID,
			"message_author", authorID,
			"original_user", ownership.OriginalUserID)
		return false
	}
	
	// Check if the thread was created by this bot instance
	if ownership.CreatedBy != botID {
		h.logger.Info("Thread created by different bot, requiring mention",
			"thread_id", threadID,
			"created_by", ownership.CreatedBy,
			"current_bot", botID)
		return false
	}
	
	// NEW: Check if there are multiple participants in the thread
	// Skip participant count check if session is nil (for tests)
	if s != nil {
		participantCount, err := h.countThreadParticipants(s, threadID, botID)
		if err != nil {
			h.logger.Error("Failed to count thread participants, requiring mention as fallback",
				"error", err, "thread_id", threadID)
			return false
		}
		
		// If there are multiple participants, require @mention even from original user
		if participantCount > 1 {
			h.logger.Info("Multiple participants in thread, requiring mention from all users",
				"thread_id", threadID,
				"participant_count", participantCount,
				"user_id", authorID)
			return false
		}
	}
	
	h.logger.Info("Auto-response triggered for original user in bot thread",
		"thread_id", threadID,
		"user_id", authorID)
	
	return true
}

// getThreadOwnership retrieves ownership information for a thread
func (h *Handler) getThreadOwnership(threadID string) (*ThreadOwnership, bool) {
	ownership, exists := h.threadOwnership[threadID]
	return ownership, exists
}

// cleanupThreadOwnership removes old thread ownership records (called periodically)
func (h *Handler) cleanupThreadOwnership(maxAge int64) {
	currentTime := time.Now().Unix()
	cutoffTime := currentTime - maxAge
	
	for threadID, ownership := range h.threadOwnership {
		// If maxAge is negative, force cleanup of all records
		// Otherwise, clean up records older than cutoffTime
		if maxAge < 0 || ownership.CreationTime < cutoffTime {
			delete(h.threadOwnership, threadID)
			h.logger.Info("Cleaned up old thread ownership record",
				"thread_id", threadID,
				"age_seconds", currentTime-ownership.CreationTime)
		}
	}
}