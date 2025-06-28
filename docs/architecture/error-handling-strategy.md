# Error Handling Strategy

  * Structured logging using Go's `slog` package will be implemented globally.
  * All errors returned from services will be logged with context (e.g., the Discord channel/user that triggered the request).
  * The `AIService` will have specific, typed errors for scenarios like "CLI command failed" or "no output from model" to allow the bot to respond with a helpful message (e.g., "Sorry, I'm having trouble thinking right now.") instead of crashing.
  * The application will be configured to perform a graceful shutdown, ensuring it disconnects cleanly from the Discord Gateway on a fatal error.