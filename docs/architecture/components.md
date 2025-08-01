# Components

The monolithic service will be internally structured into these logical components:

1.  **Discord Session Manager**: Handles the primary connection to the Discord Gateway, manages the bot's lifecycle, and registers event handlers.
2.  **Interaction Handler**: A function or set of functions that trigger on Discord message events. It will parse the message content, identify mentions, and route the request to the appropriate service.
3.  **AIService Interface**: A Go interface defining the contract for getting answers from an AI model. This enforces the decoupled design.
4.  **OllamaAIService**: The concrete implementation of the `AIService` interface. This component will be responsible for constructing the correct prompt, making HTTP requests to the Ollama API, and parsing the responses. The AIService interface remains extensible for future AI provider integrations.
5.  **RateLimitMonitor**: A background service running in a separate goroutine. It periodically checks the `RateLimitState` and calls the Discord Session Manager to update the bot's public presence (status).
6.  **ThreadManager**: A component responsible for creating new threads or finding existing ones to post replies in.