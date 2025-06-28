# BMAD Knowledge Bot

A Discord bot built with Go that provides AI-powered knowledge assistance.

## Setup

### Prerequisites

- Go 1.24.x
- Discord Bot Token
- Google Gemini CLI (for AI functionality)
- Docker and Docker Compose (optional, for containerized deployment)

### Installation

1. Clone the repository
2. Copy the environment template:
   ```bash
   cp .env.example .env
   ```
3. Edit `.env` and set your configuration:
   ```
   BOT_TOKEN=your_discord_bot_token_here
   GEMINI_CLI_PATH=/path/to/gemini-cli
   ```

### Getting a Discord Bot Token

1. Go to the [Discord Developer Portal](https://discord.com/developers/applications)
2. Create a new application
3. Go to the "Bot" section
4. Click "Add Bot"
5. Copy the token and add it to your `.env` file

### Running the Bot

#### Local Development
```bash
# Set environment variable
export BOT_TOKEN=your_discord_bot_token_here

# Run the bot
go run cmd/bot/main.go
```

#### Using Docker
```bash
# Build the container
docker build -t bmad-knowledge-bot .

# Run the container
docker run -e BOT_TOKEN=your_discord_bot_token_here bmad-knowledge-bot
```

#### Using Docker Compose (Recommended)
```bash
# Create required directories
mkdir -p gemini-config logs

# Start the bot with Docker Compose
docker-compose up -d

# View logs
docker-compose logs -f

# Stop the bot
docker-compose down

# Rebuild and restart
docker-compose up --build -d
```

The Docker Compose setup includes:
- Automatic loading of environment variables from `.env` file
- Volume mounting of `./gemini-config` to `/home/botuser/.gemini` for Gemini CLI configuration
- Persistent logging to `./logs` directory
- Health checks and resource limits
- Automatic restart on failure

### Project Structure

```
bmad-knowledge-bot/
├── cmd/bot/main.go         # Main application entry point
├── internal/
│   ├── bot/                # Core bot logic, event handlers
│   ├── service/            # AI service interface and Gemini CLI
│   └── monitor/            # Rate limit monitoring
├── .env.example            # Environment template
├── Dockerfile              # Container build instructions
├── docker-compose.yml      # Docker Compose configuration
├── gemini-config/          # Gemini CLI configuration (mounted volume)
├── logs/                   # Application logs (mounted volume)
└── README.md               # This file
```

## Development

The bot follows Go best practices and includes structured logging. All secrets are read from environment variables for security.

## License

This project is part of the BMAD (Bot Management and Development) framework.