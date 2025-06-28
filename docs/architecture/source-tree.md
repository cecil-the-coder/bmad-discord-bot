# Source Tree

The project will follow a standard Golang service layout.

```plaintext
bmad-knowledge-bot/
├── cmd/
│   └── bot/
│       └── main.go         # Main application entry point
├── internal/
│   ├── bot/                # Core bot logic, event handlers
│   │   ├── handler.go
│   │   └── session.go
│   ├── service/
│   │   ├── ai_interface.go # The AIService interface definition
│   │   └── gemini_cli.go   # The CLI implementation of the service
│   └── monitor/
│       └── ratelimiter.go  # The rate limit monitoring service
├── .env.example            # Template for environment variables
├── .gitignore
├── go.mod                  # Go module definition
├── go.sum
├── Dockerfile              # Instructions to build the container image
└── README.md