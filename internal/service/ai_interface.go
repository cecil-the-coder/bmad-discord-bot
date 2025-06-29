package service

// AIService defines the interface for AI interaction services
// This interface must be used for all business logic interacting with AI models
type AIService interface {
	// QueryAI sends a query to the AI service and returns the response
	// Following coding standards: all AI business logic must use this interface
	QueryAI(query string) (string, error)

	// SummarizeQuery creates a summarized version of a user query suitable for Discord thread titles
	// Returns a summary limited to 100 characters for Discord thread title requirements
	SummarizeQuery(query string) (string, error)

	// QueryWithContext sends a query with conversation history context to the AI service
	// conversationHistory should be formatted as a string containing previous messages
	QueryWithContext(query string, conversationHistory string) (string, error)

	// SummarizeConversation creates a summary of conversation history for context preservation
	// Returns a summarized version that fits within reasonable token limits
	SummarizeConversation(messages []string) (string, error)

	// GetProviderID returns the unique identifier for this AI provider
	// Used for provider-specific rate limiting and monitoring
	GetProviderID() string
}
