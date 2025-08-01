# Epic 2: BMAD Knowledge Bot Specialization

**Goal**: Transform the general-purpose conversational bot into a specialized BMAD-METHOD expert that provides accurate, contextual answers exclusively from the BMAD knowledge base, with robust error handling and optimized performance.

## Story 2.1: Transform Bot to BMAD Knowledge Expert

As a user, I want the bot to answer questions exclusively using the BMAD-METHOD knowledge base, so that I get accurate, contextual answers about the BMAD framework without hallucinations or off-topic responses.

* **Acceptance Criteria**:
    * 2.1.1: The bot's Gemini CLI integration is updated to include the BMAD prompt knowledge base (`docs/bmadprompt.md`) as the primary context for all queries.
    * 2.1.2: The bot's system prompt instructs it to answer ONLY based on the BMAD knowledge base, refusing to answer questions outside this scope.
    * 2.1.3: When asked about topics not covered in the BMAD knowledge base, the bot politely indicates that the information is not available in its knowledge base.
    * 2.1.4: The bot maintains citation capability, referencing specific sections of the BMAD documentation when providing answers.
    * 2.1.5: The bot's responses are focused, contextual, and directly relevant to the BMAD-METHOD framework.
    * 2.1.6: The conversational context in threads (from Story 1.4) continues to work but is constrained to BMAD-related discussions.

## Story 2.2: Implement Daily Quota Handling

As a system administrator, I want the bot to gracefully handle daily API quota exhaustion with user-friendly messages and automatic service restoration, so that users understand service availability and know when functionality will be restored.

* **Acceptance Criteria**:
    * 2.2.1: The system detects daily quota exhaustion from Gemini API 429 errors with specific daily quota patterns.
    * 2.2.2: The quota state is managed independently from regular rate limiting with proper reset time calculation.
    * 2.2.3: User-facing methods return informative messages about quota exhaustion and restoration times.
    * 2.2.4: The system automatically clears quota exhaustion flags at UTC midnight reset time.
    * 2.2.5: Integration with existing rate limiting system maintains backward compatibility.

## Story 2.3: Implement Gemini Model Fallback Support

As a system administrator or bot user, I want the bot to automatically fallback to a lighter Gemini model when the primary model hits its rate limit, so that the bot maintains functionality even when the primary model quota is exhausted, providing continuous service availability.

* **Acceptance Criteria**:
    * 2.3.1: The system supports configurable primary and fallback Gemini models via environment variables.
    * 2.3.2: The system detects model-specific rate limiting and quota exhaustion independently.
    * 2.3.3: Automatic fallback to lighter model when primary model becomes unavailable.
    * 2.3.4: Independent state management for each model's availability and reset times.
    * 2.3.5: Graceful restoration of primary model usage after rate limit reset.
    * 2.3.6: Enhanced error handling when all models are unavailable.
    * 2.3.7: Integration with existing rate limiting and quota systems.

## Story 2.4: Integrate Query Summarization into Main Response

As a system administrator, I want query summarization to be included in the main AI response instead of requiring a separate API call, so that I can reduce API usage by 50% while maintaining thread title generation functionality.

* **Acceptance Criteria**:
    * 2.4.1: The `QueryAI()` method includes summary generation instructions directly in the BMAD-constrained prompt.
    * 2.4.2: The AI response contains both the main answer and a concise summary marked with `[SUMMARY]:` delimiter.
    * 2.4.3: Response parsing logic extracts the summary from the main response for Discord thread titles.
    * 2.4.4: The separate `SummarizeQuery()` method calls are eliminated when creating thread titles.
    * 2.4.5: Summary generation maintains the existing 8-word limit and BMAD topic focus.
    * 2.4.6: The integration preserves all existing BMAD knowledge base constraints and error handling.
    * 2.4.7: Rate limiting and quota management continue to work with the integrated approach.
    * 2.4.8: Thread title generation uses the extracted summary instead of separate API calls.

## Story 2.5: Implement Bot State Persistence

As a system administrator, I want the bot to persist its operational state and message tracking information across restarts, so that conversational context and system health monitoring continue seamlessly after deployments or system failures.

* **Acceptance Criteria**:
    * 2.5.1: SQLite database integration for bot state persistence with proper initialization and migration handling.
    * 2.5.2: Message tracking across bot restarts preserving thread relationships and user context.
    * 2.5.3: Thread context preservation including message history and conversation state.
    * 2.5.4: Rate limiting state recovery maintaining API usage counters and status across restarts.
    * 2.5.5: Graceful database initialization, schema creation, and error handling for database operations.

## Story 2.6: Implement Periodic BMAD Knowledge Base Refresh

As a system administrator, I want the bot to periodically check and refresh the internal BMAD knowledge base from the remote source, so that the bot always provides answers based on the latest BMAD-METHOD documentation without requiring manual updates or redeployment.

* **Acceptance Criteria**:
    * 2.6.1: The system includes a configurable HTTP client service that can fetch the remote BMAD knowledge base from `https://github.com/bmadcode/BMAD-METHOD/raw/refs/heads/main/bmad-core/data/bmad-kb.md`.
    * 2.6.2: A background service checks for updates to the remote knowledge base on a configurable interval (default: every 6 hours).
    * 2.6.3: The system compares the remote content with the local `internal/knowledge/bmad.md` file to detect changes (using content hash or last-modified headers).
    * 2.6.4: When changes are detected, the system downloads the remote content and merges it with the local file, preserving the first line (system prompt) of the local `internal/knowledge/bmad.md` file.
    * 2.6.5: The refresh process includes proper error handling for network failures, invalid responses, and file system errors.
    * 2.6.6: The refresh interval, remote URL, and feature enable/disable are configurable via environment variables.
    * 2.6.7: The system logs refresh attempts, successes, failures, and change detections with appropriate severity levels.
    * 2.6.8: The bot continues to function normally if the refresh service fails, using the existing local knowledge base.

## Story 2.7: Implement Ollama API Integration with Devstral Model

As a system administrator, I want to integrate the Ollama API with the "devstral" model as an alternative AI service, so that I can evaluate local AI model capabilities for the BMAD knowledge bot while maintaining compatibility with the existing AIService interface, including proper testing and response format validation.

* **Acceptance Criteria**:
    * 2.7.1: The system includes a new OllamaAIService that implements the AIService interface and connects to a local Ollama server instance.
    * 2.7.2: The Ollama service uses the "devstral" model specifically for all AI operations and validates model availability during initialization.
    * 2.7.3: All existing AIService interface methods (QueryAI, QueryAIWithSummary, SummarizeQuery, QueryWithContext, SummarizeConversation, GetProviderID) are fully implemented with BMAD knowledge base integration.
    * 2.7.4: The Ollama service includes proper error handling for network failures, model unavailability, and API response validation.
    * 2.7.5: The service supports configurable Ollama server connection settings (host, port, timeout) via environment variables.
    * 2.7.6: Response format validation ensures outputs match expected patterns and length constraints for Discord integration.
    * 2.7.7: The implementation includes comprehensive unit and integration tests with mock Ollama responses.
    * 2.7.8: The service can be selected as an AI provider through configuration without modifying existing bot logic.

## Story 2.8: Support Bot Inclusion via Reply Mentions

As a Discord user, I want to include the BMAD knowledge bot in a conversation by replying to a user's question with just the bot mention, so that the bot can provide BMAD-related answers in the context of the existing conversation thread without requiring the original user to directly mention the bot.

* **Acceptance Criteria**:
    * 2.8.1: The bot detects when it is mentioned in a reply to another user's message and processes the original message content as the query.
    * 2.8.2: The bot creates appropriate threading behavior, either responding in the existing thread or creating a new thread based on the conversation context.
    * 2.8.3: The bot maintains all existing BMAD knowledge base constraints and response formatting when processing reply-mentioned queries.
    * 2.8.4: The system preserves existing mention detection logic and thread management for direct mentions and auto-response scenarios.
    * 2.8.5: The reply mention feature works in both main channels and existing threads.
    * 2.8.6: The bot provides clear attribution when responding to reply mentions, indicating which message it is addressing.

## Story 2.9: Migrate from SQLite to MySQL for Cloud-Native Deployment

As a system administrator, I want to migrate the bot's data persistence from SQLite to MySQL, so that the application is decoupled from the local filesystem and suitable for Kubernetes/cloud-native deployment with external database services.

* **Acceptance Criteria**:
    * 2.9.1: The system supports MySQL database connection with configurable host, port, database name, username, and password via environment variables.
    * 2.9.2: All existing SQLite schema and data models are migrated to MySQL with equivalent functionality and data integrity.
    * 2.9.3: Database initialization automatically creates required tables and indexes if they don't exist, with proper error handling for connection failures.
    * 2.9.4: All existing storage operations (message tracking, rate limiting state, thread context) work identically with MySQL backend.
    * 2.9.5: The system includes database migration scripts or automated migration from existing SQLite data to MySQL.
    * 2.9.6: Connection pooling and proper connection management are implemented for production reliability and performance.
    * 2.9.7: The MySQL integration maintains backward compatibility with existing storage interface contracts.
    * 2.9.8: Comprehensive error handling for database connectivity issues, including graceful degradation when database is unavailable.