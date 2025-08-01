# Coding Standards

  * All code must be formatted with `gofmt`.
  * Standard Go linting rules will be applied.
  * **Critical Rule**: All business logic for interacting with AI models **MUST** be implemented via the `AIService` interface. No direct calls to AI APIs should exist outside of the concrete AIService implementations (e.g., OllamaAIService). This maintains clean separation of concerns and enables future AI provider integrations.
  * Secrets (Bot Token, API Keys) must only be read from environment variables at startup. Do not hardcode them.