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
}