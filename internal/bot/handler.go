package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"bmad-knowledge-bot/internal/service"
	"bmad-knowledge-bot/internal/storage"
	"github.com/bwmarrin/discordgo"
)

// ThreadOwnership tracks metadata for bot-created threads
type ThreadOwnership struct {
	OriginalUserID string
	CreatedBy      string // Bot ID that created the thread
	CreationTime   int64  // Unix timestamp
}

// Handler manages Discord event handling
type Handler struct {
	logger          *slog.Logger
	aiService       service.AIService
	storageService  storage.StorageService
	threadOwnership map[string]*ThreadOwnership // threadID -> ownership info
}

// NewHandler creates a new bot event handler
func NewHandler(logger *slog.Logger, aiService service.AIService, storageService storage.StorageService) *Handler {
	return &Handler{
		logger:          logger,
		aiService:       aiService,
		storageService:  storageService,
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

		// Record message state before processing (AC 2.5.2)
		h.recordMessageState(m, isInThread)

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
		// For main channel messages, we'll get the response in processMainChannelQuery
		// using integrated summarization to avoid duplicate API calls
		response = "" // Will be handled by processMainChannelQuery
		err = nil
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
		// Handle Discord's 2000 character limit by chunking if necessary
		if err := h.sendResponseInChunks(s, m.ChannelID, response); err != nil {
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
	h.logger.Info("Processing main channel query, creating thread with integrated summarization", "query", query)

	// Use integrated query with summary to get both response and thread title in one API call
	aiResponse, summary, err := h.aiService.QueryAIWithSummary(query)
	if err != nil {
		h.logger.Error("Failed to get AI response with summary", "error", err)
		// Fallback: reply in main channel if AI query fails
		errorMsg := "I'm sorry, I encountered an error while processing your request. Please try again later."
		if _, err := s.ChannelMessageSendReply(m.ChannelID, errorMsg, m.Reference()); err != nil {
			h.logger.Error("Failed to send error reply", "error", err)
		}
		return
	}

	// Determine thread title from extracted summary
	var threadTitle string
	if summary != "" {
		threadTitle = summary
		h.logger.Info("Using extracted summary as thread title", "summary", summary, "length", len(summary))
	} else {
		// Fallback to manual title creation if summary extraction failed
		h.logger.Warn("No summary extracted, using fallback title generation")
		threadTitle = h.createFallbackTitle(query)
	}

	h.logger.Info("Thread title determined", "title", threadTitle, "length", len(threadTitle), "api_calls_saved", 1)

	// Create a public thread in the channel
	thread, err := s.ThreadStart(m.ChannelID, threadTitle, discordgo.ChannelTypeGuildPublicThread, 60) // 60 minute auto-archive
	if err != nil {
		h.logger.Error("Failed to create thread", "error", err, "channel_id", m.ChannelID)

		// Fallback: reply in main channel if thread creation fails
		errorMsg := "I encountered an issue creating a thread for our conversation. Here's my response:"
		fallbackResponse := errorMsg + "\n\n" + aiResponse
		if err := h.sendResponseInChunks(s, m.ChannelID, fallbackResponse); err != nil {
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
	// Handle Discord's 2000 character limit by chunking if necessary
	if err := h.sendResponseInChunks(s, thread.ID, aiResponse); err != nil {
		h.logger.Error("Failed to send AI response in new thread", "error", err, "thread_id", thread.ID)

		// If we can't post in the thread, try to reply in main channel as fallback
		errorMsg := "I created a thread but couldn't post my response there. Here's my answer:"
		fallbackResponse := errorMsg + "\n\n" + aiResponse
		// Also chunk the fallback response if needed
		if err := h.sendResponseInChunks(s, m.ChannelID, fallbackResponse); err != nil {
			h.logger.Error("Failed to send fallback response after thread creation", "error", err)
		}
	} else {
		h.logger.Info("AI response posted successfully in new thread with integrated summarization",
			"response_length", len(aiResponse),
			"thread_id", thread.ID,
			"message_id", m.ID,
			"thread_owner", m.Author.ID,
			"api_calls_made", 1)
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

// sendResponseInChunks sends a response message, splitting it into chunks if it exceeds Discord's 2000 character limit
func (h *Handler) sendResponseInChunks(s *discordgo.Session, channelID string, response string) error {
	const maxDiscordMessageLength = 2000

	// If response fits in one message, send it directly
	if len(response) <= maxDiscordMessageLength {
		_, err := s.ChannelMessageSend(channelID, response)
		return err
	}

	h.logger.Info("Response exceeds Discord limit, chunking message",
		"response_length", len(response),
		"max_length", maxDiscordMessageLength,
		"channel_id", channelID)

	// Split response into chunks at word boundaries to avoid breaking sentences
	chunks := h.splitResponseIntoChunks(response, maxDiscordMessageLength)

	for i, chunk := range chunks {
		// Add chunk indicator for multi-part messages
		var messageContent string
		if len(chunks) > 1 {
			messageContent = fmt.Sprintf("**[Part %d/%d]**\n%s", i+1, len(chunks), chunk)
		} else {
			messageContent = chunk
		}

		if _, err := s.ChannelMessageSend(channelID, messageContent); err != nil {
			h.logger.Error("Failed to send message chunk",
				"error", err,
				"chunk", i+1,
				"total_chunks", len(chunks),
				"channel_id", channelID)
			return fmt.Errorf("failed to send chunk %d/%d: %w", i+1, len(chunks), err)
		}

		h.logger.Info("Message chunk sent successfully",
			"chunk", i+1,
			"total_chunks", len(chunks),
			"chunk_length", len(messageContent))

		// Add small delay between chunks to avoid rate limiting
		if i < len(chunks)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	return nil
}

// splitResponseIntoChunks splits a long response into chunks at word boundaries
func (h *Handler) splitResponseIntoChunks(response string, maxLength int) []string {
	// Reserve space for chunk headers like "**[Part 1/X]**\n"
	const headerReserve = 20
	chunkSize := maxLength - headerReserve

	if len(response) <= chunkSize {
		return []string{response}
	}

	var chunks []string
	remaining := response

	for len(remaining) > chunkSize {
		// Find the last space within the chunk size to avoid breaking words
		cutPoint := chunkSize
		for cutPoint > 0 && remaining[cutPoint] != ' ' && remaining[cutPoint] != '\n' {
			cutPoint--
		}

		// If no space found in reasonable distance, cut at chunk boundary
		if cutPoint < chunkSize/2 {
			cutPoint = chunkSize
		}

		chunks = append(chunks, strings.TrimSpace(remaining[:cutPoint]))
		remaining = strings.TrimSpace(remaining[cutPoint:])
	}

	// Add the remaining text as the final chunk
	if len(remaining) > 0 {
		chunks = append(chunks, remaining)
	}

	return chunks
}

// recordMessageState persists the last seen message state to the database
func (h *Handler) recordMessageState(m *discordgo.MessageCreate, isInThread bool) {
	if h.storageService == nil {
		h.logger.Warn("Storage service not available, skipping message state persistence")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var threadID *string
	if isInThread {
		threadID = &m.ChannelID
	}

	// For threads, we need to get the parent channel ID
	var channelID string
	if isInThread {
		// Try to get parent channel from thread ownership or default to channel ID
		channelID = m.ChannelID // This will be the thread ID for threads
		// We'll use the thread ID as both channel and thread for simplicity
		threadID = &m.ChannelID
		channelID = m.ChannelID // Use thread ID as identifier
	} else {
		channelID = m.ChannelID
		threadID = nil
	}

	messageState := &storage.MessageState{
		ChannelID:         channelID,
		ThreadID:          threadID,
		LastMessageID:     m.ID,
		LastSeenTimestamp: time.Now().Unix(),
	}

	// Attempt to persist state asynchronously to avoid blocking message processing
	go func() {
		err := h.storageService.UpsertMessageState(ctx, messageState)
		if err != nil {
			h.logger.Error("Failed to persist message state",
				"error", err,
				"channel_id", channelID,
				"thread_id", threadID,
				"message_id", m.ID)
		} else {
			h.logger.Debug("Message state persisted successfully",
				"channel_id", channelID,
				"thread_id", threadID,
				"message_id", m.ID)
		}
	}()
}

// RecoverMissedMessages retrieves and processes messages that were sent while the bot was offline
func (h *Handler) RecoverMissedMessages(s *discordgo.Session, recoveryWindowMinutes int) error {
	if h.storageService == nil {
		h.logger.Warn("Storage service not available, skipping message recovery")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get message states within the recovery window
	windowDuration := time.Duration(recoveryWindowMinutes) * time.Minute
	states, err := h.storageService.GetMessageStatesWithinWindow(ctx, windowDuration)
	if err != nil {
		h.logger.Error("Failed to get message states for recovery", "error", err)
		return fmt.Errorf("failed to get message states: %w", err)
	}

	h.logger.Info("Starting message recovery",
		"recovery_window_minutes", recoveryWindowMinutes,
		"tracked_channels", len(states))

	recoveredCount := 0
	for _, state := range states {
		count, err := h.recoverChannelMessages(s, state, windowDuration)
		if err != nil {
			h.logger.Error("Failed to recover messages for channel",
				"error", err,
				"channel_id", state.ChannelID,
				"thread_id", state.ThreadID)
			continue
		}
		recoveredCount += count
	}

	h.logger.Info("Message recovery completed",
		"total_recovered", recoveredCount,
		"channels_processed", len(states))

	return nil
}

// recoverChannelMessages recovers missed messages for a specific channel/thread
func (h *Handler) recoverChannelMessages(s *discordgo.Session, state *storage.MessageState, windowDuration time.Duration) (int, error) {
	channelID := state.ChannelID
	if state.ThreadID != nil {
		channelID = *state.ThreadID
	}

	// Calculate time window
	cutoffTime := time.Now().Add(-windowDuration)
	lastSeenTime := time.Unix(state.LastSeenTimestamp, 0)

	// Skip if last seen is outside recovery window
	if lastSeenTime.Before(cutoffTime) {
		h.logger.Info("Skipping recovery for channel outside window",
			"channel_id", state.ChannelID,
			"thread_id", state.ThreadID,
			"last_seen", lastSeenTime,
			"cutoff", cutoffTime)
		return 0, nil
	}

	// Fetch recent messages from Discord
	messages, err := s.ChannelMessages(channelID, 50, "", state.LastMessageID, "")
	if err != nil {
		return 0, fmt.Errorf("failed to fetch messages from Discord: %w", err)
	}

	// Filter messages that are:
	// 1. After the last seen message
	// 2. Within the recovery window
	// 3. Not from the bot itself
	var missedMessages []*discordgo.Message
	for i := len(messages) - 1; i >= 0; i-- { // Reverse to chronological order
		msg := messages[i]

		// Skip bot's own messages
		if msg.Author.ID == s.State.User.ID {
			continue
		}

		// Parse message timestamp
		msgTime := msg.Timestamp

		// Check if message is within recovery window and after last seen
		if msgTime.After(lastSeenTime) && msgTime.After(cutoffTime) {
			missedMessages = append(missedMessages, msg)
		}
	}

	// Process missed messages
	processedCount := 0
	for _, msg := range missedMessages {
		// Convert to MessageCreate event for processing
		messageCreate := &discordgo.MessageCreate{Message: msg}

		h.logger.Info("Processing recovered message",
			"message_id", msg.ID,
			"author", msg.Author.Username,
			"channel_id", channelID,
			"content_length", len(msg.Content))

		// Process the message through normal handler logic
		h.HandleMessageCreate(s, messageCreate)
		processedCount++

		// Add delay to avoid rate limiting
		time.Sleep(500 * time.Millisecond)
	}

	h.logger.Info("Channel recovery completed",
		"channel_id", state.ChannelID,
		"thread_id", state.ThreadID,
		"messages_processed", processedCount)

	return processedCount, nil
}
