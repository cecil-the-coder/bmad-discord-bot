# BMAD Knowledge Bot

A specialized Discord bot built with Go that provides expert knowledge about the BMAD-METHOD framework. The bot answers questions exclusively from the BMAD knowledge base, ensuring accurate and contextual responses about the framework.

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
- Google Gemini CLI (for AI functionality)
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
   DISCORD_TOKEN=your_discord_bot_token
   GEMINI_CLI_PATH=/usr/local/bin/gemini
   BMAD_PROMPT_PATH=internal/knowledge/bmad.md  # Optional, defaults to internal/knowledge/bmad.md
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
export DISCORD_TOKEN=your_discord_bot_token
export GEMINI_CLI_PATH=/usr/local/bin/gemini
export BMAD_PROMPT_PATH=internal/knowledge/bmad.md  # Optional

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
  -e DISCORD_TOKEN=$DISCORD_TOKEN \
  -e GEMINI_CLI_PATH=/usr/local/bin/gemini \
  -e BMAD_PROMPT_PATH=/app/internal/knowledge/bmad.md \
  -v ~/.gemini:/home/botuser/.gemini:rw \
  bmad-knowledge-bot:latest
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
- Volume mounting of `./gemini-config`