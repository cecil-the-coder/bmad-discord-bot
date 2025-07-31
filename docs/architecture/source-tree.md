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
│   ├── storage/            # Database operations and persistence layer
│   │   ├── interface.go    # Storage interface definition
│   │   └── sqlite.go       # SQLite implementation for message state persistence
│   └── monitor/
│       └── ratelimiter.go  # The rate limit monitoring service
├── data/                   # Runtime data directory (auto-created)
│   └── bot_state.db        # SQLite database for message state persistence
├── .env.example            # Template for environment variables
├── .gitignore
├── go.mod                  # Go module definition
├── go.sum
├── Dockerfile              # Instructions to build the container image
└── README.md