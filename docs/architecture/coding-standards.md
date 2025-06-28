# Coding Standards

  * All code must be formatted with `gofmt`.
  * Standard Go linting rules will be applied.
  * **Critical Rule**: All business logic for interacting with the Gemini model **MUST** be implemented via the `AIService` interface. No direct calls to the `gemini-cli` command should exist outside of the `GeminiCLIService` implementation.
  * Secrets (Bot Token, API Keys) must only be read from environment variables at startup. Do not hardcode them.