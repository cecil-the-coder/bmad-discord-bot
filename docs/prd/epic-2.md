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