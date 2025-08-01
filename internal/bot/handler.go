package bot

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
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

// Handler manages Discord event handling
type Handler struct {
	logger                *slog.Logger
	aiService             service.AIService
	storageService        storage.StorageService
	threadOwnership       map[string]*ThreadOwnership // threadID -> ownership info
	replyMentionConfig    ReplyMentionConfig          // Configuration for reply mention behavior
	reactionTriggerConfig ReactionTriggerConfig       // Configuration for reaction-based triggers
}

// NewHandler creates a new bot event handler with default configuration
func NewHandler(logger *slog.Logger, aiService service.AIService, storageService storage.StorageService) *Handler {
	return &Handler{
		logger:          logger,
		aiService:       aiService,
		storageService:  storageService,
		threadOwnership: make(map[string]*ThreadOwnership),
		replyMentionConfig: ReplyMentionConfig{
			DeleteReplyMessage: false, // Default to safer behavior
		},
		reactionTriggerConfig: ReactionTriggerConfig{
			Enabled: false, // Default to disabled for safety
		},
	}
}

// NewHandlerWithConfig creates a new bot event handler with custom reply mention configuration
func NewHandlerWithConfig(logger *slog.Logger, aiService service.AIService, storageService storage.StorageService, replyConfig ReplyMentionConfig) *Handler {
	return &Handler{
		logger:             logger,
		aiService:          aiService,
		storageService:     storageService,
		threadOwnership:    make(map[string]*ThreadOwnership),
		replyMentionConfig: replyConfig,
		reactionTriggerConfig: ReactionTriggerConfig{
			Enabled: false, // Default to disabled for safety
		},
	}
}

// NewHandlerWithFullConfig creates a new bot event handler with both reply mention and reaction trigger configuration
func NewHandlerWithFullConfig(logger *slog.Logger, aiService service.AIService, storageService storage.StorageService, replyConfig ReplyMentionConfig, reactionConfig ReactionTriggerConfig) *Handler {
	return &Handler{
		logger:                logger,
		aiService:             aiService,
		storageService:        storageService,
		threadOwnership:       make(map[string]*ThreadOwnership),
		replyMentionConfig:    replyConfig,
		reactionTriggerConfig: reactionConfig,
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

	// Check for reply mention scenario
	isReplyMention := false
	var referencedMessage *discordgo.Message
	var replyMentionError error

	if m.MessageReference != nil && m.MessageReference.MessageID != "" {
		h.logger.Info("Message is a reply, checking for bot mention",
			"reply_to_message_id", m.MessageReference.MessageID,
			"reply_channel_id", m.MessageReference.ChannelID,
			"mentions_count", len(m.Mentions))

		// Check if bot is mentioned in this reply message
		for _, mention := range m.Mentions {
			if mention.ID == s.State.User.ID {
				// Bot is mentioned in a reply - fetch the referenced message
				referencedMessage, replyMentionError = h.fetchReferencedMessage(s, m.MessageReference)
				if replyMentionError == nil && referencedMessage != nil {
					isReplyMention = true
					h.logger.Info("Reply mention detected - bot mentioned in reply to another message",
						"bot_id", s.State.User.ID,
						"referenced_message_id", referencedMessage.ID,
						"referenced_author", referencedMessage.Author.Username)
				} else {
					h.logger.Error("Failed to fetch referenced message for reply mention",
						"error", replyMentionError,
						"referenced_message_id", m.MessageReference.MessageID)
				}
				break
			}
		}
	}

	// Log incoming message for debugging including reply mention context
	h.logger.Info("Received message",
		"author", m.Author.Username,
		"channel", m.ChannelID,
		"content_length", len(m.Content),
		"content", m.Content,
		"mentions_count", len(m.Mentions),
		"in_thread", isInThread,
		"should_auto_respond", shouldAutoRespond,
		"is_reply", m.MessageReference != nil,
		"is_reply_mention", isReplyMention)

	// Log each mention for debugging
	for i, mention := range m.Mentions {
		h.logger.Info("Found mention",
			"index", i,
			"mention_id", mention.ID,
			"mention_username", mention.Username,
			"bot_id", s.State.User.ID)
	}

	// Check if the bot is mentioned in the message (direct mention)
	botMentioned := false
	var queryText string

	// Check for bot mentions in the message (both user and role mentions)
	for _, mention := range m.Mentions {
		if mention.ID == s.State.User.ID {
			botMentioned = true
			h.logger.Info("Bot mention detected!", "bot_id", s.State.User.ID)
			break
		}
	}

	// Check for role mentions that include the bot
	if !botMentioned {
		botMentioned = h.checkForBotRoleMention(s, m)
	}

	// Also log if no mention was found
	if !botMentioned && !isReplyMention {
		h.logger.Info("No bot mention found",
			"bot_id", s.State.User.ID,
			"mentions_count", len(m.Mentions),
			"should_auto_respond", shouldAutoRespond,
			"is_reply_mention", isReplyMention)
	}

	// Determine if we should process this message
	shouldProcess := botMentioned || shouldAutoRespond || isReplyMention

	if shouldProcess {
		// Extract query text based on processing type
		if isReplyMention {
			// For reply mentions, use the referenced message content as the query
			queryText = h.extractQueryFromReplyMention(referencedMessage, m.Author.Username)
			h.logger.Info("Using referenced message content as query for reply mention",
				"query_length", len(queryText),
				"referenced_author", referencedMessage.Author.Username,
				"reply_author", m.Author.Username)
		} else if botMentioned {
			queryText = h.extractQueryFromMention(m.Content, s.State.User.ID)
		} else {
			// For auto-response, use the entire message content as the query
			queryText = strings.TrimSpace(m.Content)
		}

		if strings.TrimSpace(queryText) == "" {
			h.logger.Info("Message processed but no query text found",
				"message_id", m.ID,
				"bot_mentioned", botMentioned,
				"auto_respond", shouldAutoRespond,
				"reply_mention", isReplyMention)
			return
		}

		h.logger.Info("Processing message",
			"query_length", len(queryText),
			"message_id", m.ID,
			"bot_mentioned", botMentioned,
			"auto_respond", shouldAutoRespond,
			"reply_mention", isReplyMention)

		// Record message state before processing (AC 2.5.2)
		h.recordMessageState(m, isInThread)

		// Process the AI query and respond (pass thread context and reply mention info)
		h.processAIQueryWithContext(s, m, queryText, isInThread, isReplyMention, referencedMessage)
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

// checkForBotRoleMention checks if any mentioned roles contain the bot
func (h *Handler) checkForBotRoleMention(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	if len(m.MentionRoles) == 0 || m.GuildID == "" {
		return false
	}

	// Get bot's member info to check its roles
	botMember, err := s.GuildMember(m.GuildID, s.State.User.ID)
	if err != nil {
		h.logger.Warn("Failed to get bot's guild member info for role mention check",
			"error", err, "guild_id", m.GuildID)
		return false
	}

	// Check if any mentioned role is assigned to the bot
	for _, mentionedRoleID := range m.MentionRoles {
		for _, botRoleID := range botMember.Roles {
			if mentionedRoleID == botRoleID {
				h.logger.Info("Bot role mention detected!",
					"role_id", mentionedRoleID,
					"bot_id", s.State.User.ID)
				return true
			}
		}
	}

	return false
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

	// Also remove role mention patterns like <@&roleID>
	// This handles cases where the bot was mentioned via a role
	rolePattern := `<@&\d+>`
	re := regexp.MustCompile(rolePattern)
	cleanedContent = re.ReplaceAllString(cleanedContent, "")

	// Trim whitespace and return the remaining query
	return strings.TrimSpace(cleanedContent)
}

// fetchReferencedMessage retrieves the referenced message from Discord API
func (h *Handler) fetchReferencedMessage(s *discordgo.Session, ref *discordgo.MessageReference) (*discordgo.Message, error) {
	if ref == nil || ref.MessageID == "" {
		return nil, fmt.Errorf("invalid message reference")
	}

	// Determine channel ID for the API call
	channelID := ref.ChannelID
	if channelID == "" {
		return nil, fmt.Errorf("missing channel ID in message reference")
	}

	h.logger.Info("Fetching referenced message",
		"message_id", ref.MessageID,
		"channel_id", channelID)

	// Fetch the referenced message from Discord
	message, err := s.ChannelMessage(channelID, ref.MessageID)
	if err != nil {
		h.logger.Error("Failed to fetch referenced message",
			"error", err,
			"message_id", ref.MessageID,
			"channel_id", channelID)
		return nil, fmt.Errorf("failed to fetch referenced message: %w", err)
	}

	// Validate the message
	if message == nil {
		return nil, fmt.Errorf("referenced message is nil")
	}

	// Skip if referenced message is from a bot to avoid processing bot responses
	if message.Author.Bot {
		h.logger.Info("Referenced message is from a bot, skipping",
			"referenced_author", message.Author.Username,
			"message_id", message.ID)
		return nil, fmt.Errorf("referenced message is from a bot")
	}

	// Check if referenced message has meaningful content
	if strings.TrimSpace(message.Content) == "" {
		h.logger.Info("Referenced message has no content",
			"message_id", message.ID,
			"author", message.Author.Username)
		return nil, fmt.Errorf("referenced message has no content")
	}

	h.logger.Info("Successfully fetched referenced message",
		"message_id", message.ID,
		"author", message.Author.Username,
		"content_length", len(message.Content))

	return message, nil
}

// extractQueryFromReplyMention extracts query text from a referenced message for reply mention processing
func (h *Handler) extractQueryFromReplyMention(referencedMessage *discordgo.Message, replyAuthor string) string {
	if referencedMessage == nil {
		return ""
	}

	// Use the referenced message content as the query
	queryText := strings.TrimSpace(referencedMessage.Content)

	h.logger.Info("Extracted query from reply mention",
		"query_length", len(queryText),
		"referenced_author", referencedMessage.Author.Username,
		"reply_author", replyAuthor)

	return queryText
}

// processAIQueryWithContext sends the query to the AI service and replies with the response, handling reply mention context
func (h *Handler) processAIQueryWithContext(s *discordgo.Session, m *discordgo.MessageCreate, query string, isInThread bool, isReplyMention bool, referencedMessage *discordgo.Message) {
	// For backward compatibility, delegate to the original function if not a reply mention
	if !isReplyMention {
		h.processAIQuery(s, m, query, isInThread)
		return
	}

	h.logger.Info("Processing AI query with reply mention context",
		"query", query,
		"in_thread", isInThread,
		"reply_mention", isReplyMention,
		"referenced_author", referencedMessage.Author.Username,
		"delete_reply_configured", h.replyMentionConfig.DeleteReplyMessage)

	// Delete the reply message if configured to do so
	if h.replyMentionConfig.DeleteReplyMessage {
		h.deleteReplyMessage(s, m)
	}

	// For reply mentions, process as if it's a main channel query but with attribution
	// This will create a new thread or respond appropriately based on current context
	if isInThread {
		// If we're already in a thread, respond directly with attribution
		h.processReplyMentionInThread(s, m, query, referencedMessage)
	} else {
		// If in main channel, create thread with reply mention context
		h.processReplyMentionInMainChannel(s, m, query, referencedMessage)
	}
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

// processReplyMentionInMainChannel handles reply mentions in main channels by creating threads with attribution
func (h *Handler) processReplyMentionInMainChannel(s *discordgo.Session, m *discordgo.MessageCreate, query string, referencedMessage *discordgo.Message) {
	h.logger.Info("Processing reply mention in main channel, creating thread with attribution",
		"query", query,
		"referenced_author", referencedMessage.Author.Username)

	// Use integrated query with summary to get both response and thread title in one API call
	aiResponse, summary, err := h.aiService.QueryAIWithSummary(query)
	if err != nil {
		h.logger.Error("Failed to get AI response with summary for reply mention", "error", err)
		// Fallback: reply in main channel if AI query fails
		errorMsg := "I'm sorry, I encountered an error while processing your request. Please try again later."
		if _, err := s.ChannelMessageSendReply(m.ChannelID, errorMsg, m.Reference()); err != nil {
			h.logger.Error("Failed to send error reply", "error", err)
		}
		return
	}

	// Create thread title with reply mention context
	var threadTitle string
	if summary != "" {
		threadTitle = fmt.Sprintf("Re: %s - %s", referencedMessage.Author.Username, summary)
		h.logger.Info("Using reply mention summary as thread title", "summary", summary, "referenced_author", referencedMessage.Author.Username)
	} else {
		// Fallback to manual title creation if summary extraction failed
		h.logger.Warn("No summary extracted for reply mention, using fallback title generation")
		threadTitle = fmt.Sprintf("Re: %s - %s", referencedMessage.Author.Username, h.createFallbackTitle(query))
	}

	// Ensure thread title doesn't exceed Discord's limit (100 characters)
	if len(threadTitle) > 100 {
		threadTitle = threadTitle[:97] + "..."
	}

	h.logger.Info("Reply mention thread title determined", "title", threadTitle, "length", len(threadTitle))

	// Create a public thread in the channel
	thread, err := s.ThreadStart(m.ChannelID, threadTitle, discordgo.ChannelTypeGuildPublicThread, 60) // 60 minute auto-archive
	if err != nil {
		h.logger.Error("Failed to create thread for reply mention", "error", err, "channel_id", m.ChannelID)

		// Fallback: reply in main channel if thread creation fails
		attributionText := fmt.Sprintf("*Responding to %s's message:*\n\n", referencedMessage.Author.Username)
		fallbackResponse := attributionText + aiResponse
		if err := h.sendResponseInChunks(s, m.ChannelID, fallbackResponse); err != nil {
			h.logger.Error("Failed to send fallback response for reply mention", "error", err)
		}
		return
	}

	h.logger.Info("Reply mention thread created successfully",
		"thread_id", thread.ID,
		"thread_name", thread.Name,
		"parent_channel", m.ChannelID)

	// Record thread ownership for auto-response functionality
	h.recordThreadOwnership(thread.ID, m.Author.ID, s.State.User.ID)

	// Create attribution message for the reply mention
	attributionText := fmt.Sprintf("*Responding to %s's message: \"%s\"*\n\n",
		referencedMessage.Author.Username,
		h.truncateForAttribution(referencedMessage.Content))

	// Combine attribution with AI response
	responseWithAttribution := attributionText + aiResponse

	// Post the AI response with attribution as the first message in the newly created thread
	if err := h.sendResponseInChunks(s, thread.ID, responseWithAttribution); err != nil {
		h.logger.Error("Failed to send AI response in new reply mention thread", "error", err, "thread_id", thread.ID)

		// If we can't post in the thread, try to reply in main channel as fallback
		fallbackResponse := fmt.Sprintf("I created a thread but couldn't post my response there. Here's my answer:\n\n%s", responseWithAttribution)
		if err := h.sendResponseInChunks(s, m.ChannelID, fallbackResponse); err != nil {
			h.logger.Error("Failed to send fallback response after reply mention thread creation", "error", err)
		}
	} else {
		h.logger.Info("AI response with attribution posted successfully in new reply mention thread",
			"response_length", len(responseWithAttribution),
			"thread_id", thread.ID,
			"message_id", m.ID,
			"thread_owner", m.Author.ID,
			"referenced_author", referencedMessage.Author.Username)
	}
}

// processReplyMentionInThread handles reply mentions within existing threads
func (h *Handler) processReplyMentionInThread(s *discordgo.Session, m *discordgo.MessageCreate, query string, referencedMessage *discordgo.Message) {
	h.logger.Info("Processing reply mention in existing thread",
		"query", query,
		"thread_id", m.ChannelID,
		"referenced_author", referencedMessage.Author.Username)

	var response string
	var err error

	// Fetch thread history for contextual response
	const historyLimit = 50
	includeAllMessages := true
	threadMessages, historyErr := h.fetchThreadHistory(s, m.ChannelID, s.State.User.ID, historyLimit, includeAllMessages)

	if historyErr != nil {
		h.logger.Error("Failed to fetch thread history for reply mention, falling back to regular query",
			"error", historyErr, "channel_id", m.ChannelID)
		// Fallback to regular query if history retrieval fails
		response, err = h.aiService.QueryAI(query)
	} else {
		// Format conversation history for AI context
		conversationHistory := h.formatConversationHistory(threadMessages)
		h.logger.Info("Using contextual query with thread history for reply mention",
			"history_messages", len(threadMessages),
			"history_length", len(conversationHistory))

		// Use contextual query with conversation history
		response, err = h.aiService.QueryWithContext(query, conversationHistory)
	}

	if err != nil {
		h.logger.Error("AI service error for reply mention in thread", "error", err, "query", query)

		// Send user-friendly error message
		errorMsg := "I'm sorry, I encountered an error while processing your request. Please try again later."
		if _, err := s.ChannelMessageSendReply(m.ChannelID, errorMsg, m.Reference()); err != nil {
			h.logger.Error("Failed to send error reply for reply mention", "error", err)
		}
		return
	}

	// Create attribution message for the reply mention in thread
	attributionText := fmt.Sprintf("*Responding to %s's message: \"%s\"*\n\n",
		referencedMessage.Author.Username,
		h.truncateForAttribution(referencedMessage.Content))

	// Combine attribution with AI response
	responseWithAttribution := attributionText + response

	// Send response with attribution in the existing thread
	if err := h.sendResponseInChunks(s, m.ChannelID, responseWithAttribution); err != nil {
		h.logger.Error("Failed to send AI response with attribution in thread", "error", err)
	} else {
		h.logger.Info("AI response with attribution sent successfully in existing thread",
			"response_length", len(responseWithAttribution),
			"message_id", m.ID,
			"referenced_author", referencedMessage.Author.Username)
	}
}

// truncateForAttribution truncates message content for attribution display
func (h *Handler) truncateForAttribution(content string) string {
	const maxLength = 100
	if len(content) <= maxLength {
		return content
	}
	return content[:maxLength-3] + "..."
}

// deleteReplyMessage attempts to delete the reply message that mentioned the bot
func (h *Handler) deleteReplyMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	h.logger.Info("Attempting to delete reply mention message",
		"message_id", m.ID,
		"channel_id", m.ChannelID,
		"author", m.Author.Username)

	err := s.ChannelMessageDelete(m.ChannelID, m.ID)
	if err != nil {
		h.logger.Warn("Failed to delete reply mention message - continuing with normal processing",
			"error", err,
			"message_id", m.ID,
			"channel_id", m.ChannelID,
			"author", m.Author.Username,
			"error_type", fmt.Sprintf("%T", err))

		// Check for specific Discord API errors to provide better context
		if restErr, ok := err.(*discordgo.RESTError); ok {
			h.logger.Warn("Discord API error details",
				"http_code", restErr.Response.StatusCode,
				"discord_code", restErr.Message.Code,
				"discord_message", restErr.Message.Message)

			// Common permission error codes
			switch restErr.Message.Code {
			case 50013: // Missing Permissions
				h.logger.Error("Bot lacks 'Manage Messages' permission to delete reply mention message",
					"channel_id", m.ChannelID,
					"required_permission", "Manage Messages")
			case 10008: // Unknown Message (already deleted)
				h.logger.Info("Reply mention message was already deleted")
			case 50001: // Missing Access
				h.logger.Error("Bot lacks access to delete message in this channel",
					"channel_id", m.ChannelID)
			}
		}
		// Continue processing regardless of deletion failure
		return
	}

	h.logger.Info("Successfully deleted reply mention message",
		"message_id", m.ID,
		"channel_id", m.ChannelID,
		"author", m.Author.Username)
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
	ownership := &storage.ThreadOwnership{
		ThreadID:       threadID,
		OriginalUserID: originalUserID,
		CreatedBy:      botID,
		CreationTime:   time.Now().Unix(),
	}

	// Store in memory for immediate access
	memoryOwnership := &ThreadOwnership{
		OriginalUserID: originalUserID,
		CreatedBy:      botID,
		CreationTime:   ownership.CreationTime,
	}
	h.threadOwnership[threadID] = memoryOwnership

	// Persist to database asynchronously
	if h.storageService != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := h.storageService.UpsertThreadOwnership(ctx, ownership)
			if err != nil {
				h.logger.Error("Failed to persist thread ownership",
					"error", err,
					"thread_id", threadID,
					"original_user", originalUserID,
					"created_by", botID)
			} else {
				h.logger.Info("Thread ownership persisted to database",
					"thread_id", threadID,
					"original_user", originalUserID,
					"created_by", botID)
			}
		}()
	}

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

// GetThreadOwnership is a public method for testing thread ownership retrieval
func (h *Handler) GetThreadOwnership(threadID string) (*ThreadOwnership, bool) {
	return h.getThreadOwnership(threadID)
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

// RecoverThreadOwnership loads thread ownership from database into memory for recovery
func (h *Handler) RecoverThreadOwnership(ctx context.Context) error {
	if h.storageService == nil {
		h.logger.Warn("Storage service not available, skipping thread ownership recovery")
		return nil
	}

	h.logger.Info("Starting thread ownership recovery from database")

	// Get all thread ownerships from database
	ownerships, err := h.storageService.GetAllThreadOwnerships(ctx)
	if err != nil {
		return fmt.Errorf("failed to retrieve thread ownerships from database: %w", err)
	}

	// Load into memory map
	recoveredCount := 0
	for _, ownership := range ownerships {
		memoryOwnership := &ThreadOwnership{
			OriginalUserID: ownership.OriginalUserID,
			CreatedBy:      ownership.CreatedBy,
			CreationTime:   ownership.CreationTime,
		}
		h.threadOwnership[ownership.ThreadID] = memoryOwnership
		recoveredCount++
	}

	h.logger.Info("Thread ownership recovery completed",
		"recovered_threads", recoveredCount,
		"total_in_memory", len(h.threadOwnership))

	return nil
}

// sendResponseInChunks sends a response message, splitting it into chunks if it exceeds Discord's 2000 character limit
func (h *Handler) sendResponseInChunks(s *discordgo.Session, channelID string, response string) error {
	const maxDiscordMessageLength = 2000

	// Ensure proper Discord formatting for line breaks
	formattedResponse := h.formatForDiscord(response)

	// If response fits in one message, send it directly
	if len(formattedResponse) <= maxDiscordMessageLength {
		_, err := s.ChannelMessageSend(channelID, formattedResponse)
		return err
	}

	h.logger.Info("Response exceeds Discord limit, chunking message",
		"response_length", len(formattedResponse),
		"max_length", maxDiscordMessageLength,
		"channel_id", channelID)

	// Split response into chunks at word boundaries to avoid breaking sentences
	chunks := h.splitResponseIntoChunks(formattedResponse, maxDiscordMessageLength)

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

// formatForDiscord ensures proper line break formatting for Discord messages
func (h *Handler) formatForDiscord(response string) string {
	// Discord requires double line breaks for paragraph separation
	// Convert any sequence of single newlines to double newlines for proper paragraph breaks

	// First, normalize line endings to \n
	normalized := strings.ReplaceAll(response, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	// Split into lines and rebuild with proper Discord formatting
	lines := strings.Split(normalized, "\n")
	var formattedLines []string
	var currentParagraph []string

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// If we hit an empty line or line that's clearly a paragraph break
		if trimmedLine == "" {
			// End current paragraph if we have content
			if len(currentParagraph) > 0 {
				formattedLines = append(formattedLines, strings.Join(currentParagraph, " "))
				currentParagraph = nil
			}
			// Add empty line for paragraph break (Discord needs double \n)
			formattedLines = append(formattedLines, "")
		} else {
			// Add to current paragraph
			currentParagraph = append(currentParagraph, trimmedLine)
		}
	}

	// Don't forget the last paragraph
	if len(currentParagraph) > 0 {
		formattedLines = append(formattedLines, strings.Join(currentParagraph, " "))
	}

	// Join with single newlines - Discord will render double newlines as paragraph breaks
	result := strings.Join(formattedLines, "\n")

	// Clean up multiple consecutive empty lines (more than 2 newlines in a row)
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}

	return result
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
	var channelID string

	if isInThread {
		// For threads, use the thread ID as both channel and thread for simplicity
		threadID = &m.ChannelID
		channelID = m.ChannelID
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

// HandleMessageReactionAdd processes Discord message reaction events for reaction-based triggers
func (h *Handler) HandleMessageReactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	// Skip if reaction triggers are disabled
	if !h.reactionTriggerConfig.Enabled {
		return
	}

	// Ignore reactions from the bot itself to prevent loops
	if r.UserID == s.State.User.ID {
		return
	}

	// Check if the reaction matches the configured trigger emoji
	if r.Emoji.Name != h.reactionTriggerConfig.TriggerEmoji {
		h.logger.Debug("Reaction emoji does not match trigger",
			"reaction_emoji", r.Emoji.Name,
			"trigger_emoji", h.reactionTriggerConfig.TriggerEmoji,
			"user_id", r.UserID)
		return
	}

	h.logger.Info("Trigger reaction detected",
		"reaction_emoji", r.Emoji.Name,
		"user_id", r.UserID,
		"message_id", r.MessageID,
		"channel_id", r.ChannelID)

	// Fetch the user to check authorization
	user, err := s.User(r.UserID)
	if err != nil {
		h.logger.Error("Failed to fetch user for reaction trigger",
			"error", err,
			"user_id", r.UserID)
		return
	}

	// Check if user is authorized to use reaction triggers
	if !h.isUserAuthorizedForReactionTrigger(s, user, r.GuildID) {
		h.logger.Info("User not authorized for reaction triggers",
			"user_id", r.UserID,
			"username", user.Username)
		return
	}

	h.logger.Info("Authorized user triggered reaction",
		"user_id", r.UserID,
		"username", user.Username,
		"message_id", r.MessageID)

	// Add confirmation reaction if required
	if h.reactionTriggerConfig.RequireReaction {
		err = s.MessageReactionAdd(r.ChannelID, r.MessageID, "✅")
		if err != nil {
			h.logger.Error("Failed to add confirmation reaction",
				"error", err,
				"message_id", r.MessageID)
		} else {
			h.logger.Info("Added confirmation reaction",
				"message_id", r.MessageID,
				"channel_id", r.ChannelID)
		}
	}

	// Remove the trigger reaction if configured to do so
	if h.reactionTriggerConfig.RemoveTriggerReaction {
		err = s.MessageReactionRemove(r.ChannelID, r.MessageID, r.Emoji.Name, r.UserID)
		if err != nil {
			h.logger.Warn("Failed to remove trigger reaction - continuing with normal processing",
				"error", err,
				"message_id", r.MessageID,
				"channel_id", r.ChannelID,
				"emoji", r.Emoji.Name,
				"user_id", r.UserID)
		} else {
			h.logger.Info("Removed trigger reaction",
				"message_id", r.MessageID,
				"channel_id", r.ChannelID,
				"emoji", r.Emoji.Name,
				"user_id", r.UserID)
		}
	}

	// Fetch the original message to process
	message, err := s.ChannelMessage(r.ChannelID, r.MessageID)
	if err != nil {
		h.logger.Error("Failed to fetch message for reaction trigger",
			"error", err,
			"message_id", r.MessageID,
			"channel_id", r.ChannelID)
		return
	}

	// Skip if message is from a bot to avoid processing bot responses
	if message.Author.Bot {
		h.logger.Info("Reaction trigger on bot message, skipping",
			"message_author", message.Author.Username,
			"message_id", message.ID)
		return
	}

	// Check if message has meaningful content
	if strings.TrimSpace(message.Content) == "" {
		h.logger.Info("Reaction trigger on message with no content, skipping",
			"message_id", message.ID)
		return
	}

	h.logger.Info("Processing reaction trigger",
		"message_id", message.ID,
		"message_author", message.Author.Username,
		"trigger_user", user.Username,
		"content_length", len(message.Content))

	// Create a synthetic MessageCreate event to reuse existing processing logic
	// This allows reaction triggers to integrate seamlessly with existing functionality
	messageCreate := &discordgo.MessageCreate{Message: message}

	// Detect thread context for proper handling
	isInThread := h.isMessageInThread(s, r.ChannelID)

	// Use the original message content as the query
	queryText := strings.TrimSpace(message.Content)

	// Record message state before processing
	h.recordMessageState(messageCreate, isInThread)

	// Process the AI query with reaction trigger attribution
	h.processReactionTriggerQuery(s, messageCreate, queryText, isInThread, user.Username)
}

// isUserAuthorizedForReactionTrigger checks if a user is authorized to use reaction triggers
func (h *Handler) isUserAuthorizedForReactionTrigger(s *discordgo.Session, user *discordgo.User, guildID string) bool {
	// Check if user ID is in approved list
	for _, approvedUserID := range h.reactionTriggerConfig.ApprovedUserIDs {
		if user.ID == approvedUserID {
			h.logger.Info("User authorized by user ID",
				"user_id", user.ID,
				"username", user.Username)
			return true
		}
	}

	// Check if user has approved roles (only for guild channels)
	if guildID != "" && len(h.reactionTriggerConfig.ApprovedRoleNames) > 0 {
		member, err := s.GuildMember(guildID, user.ID)
		if err != nil {
			h.logger.Error("Failed to fetch guild member for role check",
				"error", err,
				"user_id", user.ID,
				"guild_id", guildID)
			return false
		}

		// Get guild roles to map role IDs to names
		roles, err := s.GuildRoles(guildID)
		if err != nil {
			h.logger.Error("Failed to fetch guild roles",
				"error", err,
				"guild_id", guildID)
			return false
		}

		// Create role ID to name mapping
		roleIDToName := make(map[string]string)
		for _, role := range roles {
			roleIDToName[role.ID] = role.Name
		}

		// Check if user has any approved roles
		for _, userRoleID := range member.Roles {
			roleName, exists := roleIDToName[userRoleID]
			if !exists {
				continue
			}

			for _, approvedRoleName := range h.reactionTriggerConfig.ApprovedRoleNames {
				if roleName == approvedRoleName {
					h.logger.Info("User authorized by role",
						"user_id", user.ID,
						"username", user.Username,
						"role_name", roleName)
					return true
				}
			}
		}
	}

	return false
}

// processReactionTriggerQuery processes AI queries triggered by reactions (behaves like direct mention)
func (h *Handler) processReactionTriggerQuery(s *discordgo.Session, m *discordgo.MessageCreate, queryText string, isInThread bool, triggerUser string) {
	// Process reaction triggers exactly like direct mentions - no special attribution needed
	// The reaction itself is the user intent signal, just like a direct mention would be

	if isInThread {
		// Process in existing thread (same as regular mention)
		h.processReactionTriggerInThread(s, m, queryText, triggerUser)
	} else {
		// Create new thread (same as regular mention)
		h.processReactionTriggerInMainChannel(s, m, queryText, triggerUser)
	}
}

// processReactionTriggerInMainChannel handles reaction triggers in main channels by creating a new thread
func (h *Handler) processReactionTriggerInMainChannel(s *discordgo.Session, m *discordgo.MessageCreate, query string, triggerUser string) {
	// Generate thread title using existing logic
	response, title, err := h.aiService.QueryAIWithSummary(query)
	if err != nil {
		h.logger.Error("AI service query failed for reaction trigger",
			"error", err,
			"message_id", m.ID,
			"trigger_user", triggerUser)
		return
	}

	// Use the same thread title as regular mentions (no special indicators needed)
	threadTitle := title
	if len(threadTitle) > 100 {
		threadTitle = threadTitle[:97] + "..."
	}

	// Create thread starting from the original message
	thread, err := s.MessageThreadStartComplex(m.ChannelID, m.ID, &discordgo.ThreadStart{
		Name:                threadTitle,
		AutoArchiveDuration: 60, // Auto-archive after 1 hour of inactivity
		Type:                discordgo.ChannelTypeGuildPublicThread,
	})
	if err != nil {
		h.logger.Error("Failed to create thread for reaction trigger",
			"error", err,
			"message_id", m.ID,
			"trigger_user", triggerUser)
		return
	}

	h.logger.Info("Created thread for reaction trigger",
		"thread_id", thread.ID,
		"thread_name", thread.Name,
		"original_message_id", m.ID,
		"trigger_user", triggerUser)

	// Record thread ownership for the original message author (not the trigger user)
	h.recordThreadOwnership(thread.ID, m.Author.ID, s.State.User.ID)

	// Send response in the new thread (no attribution needed - reaction is the intent signal)
	_, err = s.ChannelMessageSend(thread.ID, response)
	if err != nil {
		h.logger.Error("Failed to send reaction trigger response in thread",
			"error", err,
			"thread_id", thread.ID,
			"trigger_user", triggerUser)
		return
	}

	h.logger.Info("Sent reaction trigger response in new thread",
		"thread_id", thread.ID,
		"response_length", len(response),
		"trigger_user", triggerUser)
}

// processReactionTriggerInThread handles reaction triggers in existing threads
func (h *Handler) processReactionTriggerInThread(s *discordgo.Session, m *discordgo.MessageCreate, query string, triggerUser string) {
	// Fetch thread history for contextual response
	const historyLimit = 50
	includeAllMessages := true
	threadMessages, historyErr := h.fetchThreadHistory(s, m.ChannelID, s.State.User.ID, historyLimit, includeAllMessages)

	var response string
	var err error

	if historyErr != nil {
		h.logger.Error("Failed to fetch thread history for reaction trigger, falling back to regular query",
			"error", historyErr, "thread_id", m.ChannelID)
		// Fallback to regular query if history retrieval fails
		response, err = h.aiService.QueryAI(query)
	} else {
		// Format conversation history for AI context
		conversationHistory := h.formatConversationHistory(threadMessages)
		h.logger.Info("Using contextual query with thread history for reaction trigger",
			"history_messages", len(threadMessages),
			"history_length", len(conversationHistory),
			"trigger_user", triggerUser)
		// Use contextual query with conversation history
		response, err = h.aiService.QueryWithContext(query, conversationHistory)
	}

	if err != nil {
		h.logger.Error("AI service query failed for reaction trigger in thread",
			"error", err,
			"thread_id", m.ChannelID,
			"trigger_user", triggerUser)
		return
	}

	// Send response in the existing thread (no attribution needed - reaction is the intent signal)
	_, err = s.ChannelMessageSend(m.ChannelID, response)
	if err != nil {
		h.logger.Error("Failed to send reaction trigger response in thread",
			"error", err,
			"thread_id", m.ChannelID,
			"trigger_user", triggerUser)
		return
	}

	h.logger.Info("Sent reaction trigger response in existing thread",
		"thread_id", m.ChannelID,
		"response_length", len(response),
		"trigger_user", triggerUser)
}
