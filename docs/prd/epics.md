# Epics

This file provides an overview of all project epics. For detailed epic content, see individual epic files:

## Epic 1: Core Conversational Bot

**Goal**: Establish a production-ready Discord bot that can answer user questions with conversational context within threads and proactively report on its API health.

### Story 1.1: Basic Bot Setup and Connection

As a server administrator, I want to set up the bot project and see it connect to Discord, so that I can confirm the basic infrastructure is working.

* **Acceptance Criteria**:
    * 1.1.1: A new Go project/module is initialized.
    * 1.1.2: The project includes a dependency for a Discord API library (e.g., `discordgo`).
    * 1.1.3: The application can read a bot token from an environment variable.
    * 1.1.4: When the application is run, the bot successfully connects to the Discord Gateway and appears as "Online" in the server.

### Story 1.2: Simple Mention-and-Reply Functionality

As a user, I want to mention the bot with a question and get a simple answer, so that I can validate the core question-answering workflow.

* **Acceptance Criteria**:
    * 1.2.1: When the bot is @-mentioned with a text query, the content of the query is captured by the backend.
    * 1.2.2: The backend service has a wrapper function that executes the Gemini CLI with the user's query.
    * 1.2.3: The text output from the Gemini CLI is captured by the backend service.
    * 1.2.4: The bot replies directly to the user's message with the complete, unformatted text from the Gemini CLI.
    * 1.2.5: This interaction does not yet use or create threads.

### Story 1.3: Threaded Conversation Creation

As a user, when I ask the bot a question, I want it to create a new thread for the answer, so that our conversation is neatly organized and doesn't clutter the main channel.

* **Acceptance Criteria**:
    * 1.3.1: When the bot replies to a user's initial @-mention (that is not already in a thread), it must create a new public Discord Thread.
    * 1.3.2: The thread title should be a summarized version of the user's initial question.
    * 1.3.3: The bot's answer (from Story 1.2 functionality) is posted as the first message within the newly created thread.

### Story 1.4: Implement Conversational Context in Threads

As a user, I want to ask follow-up questions within a thread and have the bot understand the context of our conversation, so that I can have a more natural and helpful interaction.

* **Acceptance Criteria**:
    * 1.4.1: When the bot is @-mentioned within a thread it created, it must fetch the message history of that thread.
    * 1.4.2: The backend service must have a function to summarize the conversation history.
    * 1.4.3: The prompt sent to the Gemini CLI must include both the summarized history and the user's new question.
    * 1.4.4: The bot's new answer is posted as a reply within the same thread.
    * 1.4.5: When the original user sends a message in a thread that was created by the bot in response to their question, the bot must automatically process and respond to the message without requiring an @mention.

### Story 1.5: API Usage Monitoring

As the bot operator, I want the application to internally track its usage of the Gemini API, so that it can operate reliably without being rate-limited.

* **Acceptance Criteria**:
    * 1.5.1: The backend service must maintain an internal counter for Gemini API calls.
    * 1.5.2: The counter should track usage over a configurable time window (e.g., requests per minute).
    * 1.5.3: The system exposes an internal state representing the current usage level (e.g., Normal, Warning, Throttled).

### Story 1.6: Dynamic Bot Status for API Health

As a user, I want to see the bot's Discord status change color, so that I have a quick visual indicator of its current API capacity and health.

* **Acceptance Criteria**:
    * 1.6.1: The bot's presence/status on Discord is updated based on the internal API usage monitor from Story 1.5.
    * 1.6.2: When API usage is low, the status is set to "Online" (Green).
    * 1.6.3: When API usage is approaching the rate limit (e.g., >75% capacity), the status is set to "Idle" (Yellow).
    * 1.6.4: If the rate limit has been exceeded, the status is set to "Do Not Disturb" (Red).
    * 1.6.5: The status returns to normal once the usage level drops.

## Epic 2: BMAD Knowledge Bot Specialization

**Goal**: Transform the general-purpose conversational bot into a specialized BMAD-METHOD expert that provides accurate, contextual answers exclusively from the BMAD knowledge base, with robust error handling and optimized performance.

**📋 Detailed Epic**: See [epic-2.md](epic-2.md) for complete story definitions and acceptance criteria.

**Stories Overview**:
- **Story 2.1**: Transform Bot to BMAD Knowledge Expert
- **Story 2.2**: Implement Daily Quota Handling  
- **Story 2.3**: Implement Gemini Model Fallback Support
- **Story 2.4**: Integrate Query Summarization into Main Response
- **Story 2.5**: Implement Bot State Persistence
- **Story 2.6**: Implement Periodic BMAD Knowledge Base Refresh