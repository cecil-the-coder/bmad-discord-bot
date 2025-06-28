# Test Strategy

  * **Unit Tests**: All core business logic within the services (e.g., text summarization for prompts, rate limit calculation) and helper functions will be unit tested using Go's standard testing package. Mocks will be used for external dependencies like the Discord API.
  * **Integration Tests**: A small suite of integration tests will validate the bot's ability to handle a simulated Discord message event, call the `AIService`, and prepare a reply. These tests will run against a mocked Discord Gateway and a stubbed version of the Gemini CLI.