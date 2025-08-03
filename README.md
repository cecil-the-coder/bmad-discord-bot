# BMAD Knowledge Bot

A comprehensive Discord bot built with Go that serves as an intelligent assistant for the BMAD-METHOD framework community. This production-ready bot features advanced user rate limiting, channel restrictions, threaded conversations, and hot-reloadable database configuration management. It integrates seamlessly with AI providers (Ollama, Gemini) to deliver contextual responses while maintaining strict focus on BMAD knowledge base content, complete with citation support and abuse prevention mechanisms.

Developed using the **BMad-Method** itself, this project demonstrates the framework's effectiveness in creating enterprise-grade applications with clean architecture, comprehensive testing, and maintainable code. The bot includes sophisticated features like admin command interfaces for runtime configuration, persistent rate limiting across restarts, emergency bypass controls, and multi-provider AI integration with intelligent fallback handling.

## Features

- **BMAD-Focused Responses**: Answers questions exclusively from the BMAD-METHOD knowledge base
- **Citation Support**: Maintains citation markers from the source documentation
- **Contextual Conversations**: Supports threaded conversations while maintaining BMAD context
- **Knowledge Constraints**: Politely refuses to answer questions outside the BMAD scope
- **Rate Limiting**: Visual Discord status indicators for API health
- **Thread Management**: Automatically creates threads for organized conversations

## Setup

### Prerequisites

- Go 1.24.x
- Discord Bot Token
- Ollama AI service (for AI functionality)
- BMAD Knowledge Base file (`docs/bmadprompt.md`)
- Docker and Docker Compose (optional, for containerized deployment)

### Installation

1. Clone the repository
2. Copy the environment template:
   ```bash
   cp .env.example .env
   ```
3. Edit `.env` and set your configuration:
   ```
   BOT_TOKEN=your_discord_bot_token
   AI_PROVIDER=ollama
   MYSQL_HOST=localhost
   MYSQL_DATABASE=bmad_bot
   MYSQL_USERNAME=bmad_user
   MYSQL_PASSWORD=your_mysql_password
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
# Set environment variables
export BOT_TOKEN=your_discord_bot_token
export AI_PROVIDER=ollama
export MYSQL_HOST=localhost
export MYSQL_DATABASE=bmad_bot
export MYSQL_USERNAME=bmad_user
export MYSQL_PASSWORD=your_mysql_password

# Run the bot
go run cmd/bot/main.go
```

#### Using Docker
```bash
# Build the container
docker build -t bmad-knowledge-bot .

# Run the container
docker run -d \
  --name bmad-discord-bot \
  -e BOT_TOKEN=$BOT_TOKEN \
  -e AI_PROVIDER=ollama \
  -e MYSQL_HOST=$MYSQL_HOST \
  -e MYSQL_DATABASE=$MYSQL_DATABASE \
  -e MYSQL_USERNAME=$MYSQL_USERNAME \
  -e MYSQL_PASSWORD=$MYSQL_PASSWORD \
  bmad-knowledge-bot:latest
```

#### Using Docker Compose (Recommended)
```bash
# Create required directories
mkdir -p logs

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
- Volume mounting of `./gemini-config`