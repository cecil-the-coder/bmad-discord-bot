package bot

import (
	"log/slog"
	"strings"

	"github.com/bwmarrin/discordgo"
	"bmad-knowledge-bot/internal/service"
)

// Handler manages Discord event handling
type Handler struct {
	logger    *slog.Logger
	aiService service.AIService
}

// NewHandler creates a new bot event handler
func NewHandler(logger *slog.Logger, aiService service.AIService) *Handler {
	return &Handler{
		logger:    logger,
		aiService: aiService,
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
	
	// Log incoming message for debugging including thread context
	h.logger.Info("Received message",
		"author", m.Author.Username,
		"channel", m.ChannelID,
		"content_length", len(m.Content),
		"content", m.Content,
		"mentions_count", len(m.Mentions),
		"in_thread", isInThread)

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
			"mentions_count", len(m.Mentions))
	}

	// If bot is mentioned, extract the query text
	if botMentioned {
		queryText = h.extractQueryFromMention(m.Content, s.State.User.ID)
		if strings.TrimSpace(queryText) == "" {
			h.logger.Info("Bot mentioned but no query text found", "message_id", m.ID)
			return
		}

		h.logger.Info("Bot mentioned with query",
			"query_length", len(queryText),
			"message_id", m.ID)

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

	// Call the AI service
	response, err := h.aiService.QueryAI(query)
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
		// If already in a thread, reply directly (maintain Story 1.2 behavior)
		if _, err := s.ChannelMessageSendReply(m.ChannelID, response, m.Reference()); err != nil {
			h.logger.Error("Failed to send AI response in thread", "error", err)
		} else {
			h.logger.Info("AI response sent successfully in existing thread",
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
			"message_id", m.ID)
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