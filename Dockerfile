# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install security updates and build dependencies
RUN apk update && apk upgrade && apk add --no-cache git

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with security flags
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags='-w -s -extldflags "-static"' \
    -o main cmd/bot/main.go

# Final stage - use Node.js slim for Gemini CLI support
FROM node:22-slim

# Add labels for container metadata
LABEL maintainer="BMAD Knowledge Bot Team"
LABEL description="Discord bot for knowledge management"
LABEL version="1.0"
LABEL security.scan="required"

# Install CA certificates and Gemini CLI
RUN apt-get update && apt-get install -y ca-certificates && \
    npm install -g @google/gemini-cli && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -r botuser && useradd -r -g botuser botuser

# Copy the binary with proper ownership and permissions
COPY --from=builder --chown=botuser:botuser /app/main /app/main

# Copy knowledge base files
COPY --chown=botuser:botuser internal/knowledge /app/internal/knowledge

# Create logs directory
RUN mkdir -p /app/logs && chown -R botuser:botuser /app/logs

# Set proper file permissions
USER botuser

# Use non-root working directory
WORKDIR /app

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD ["/app/main", "--health-check"] || exit 1

# Security: Run as non-root user, read-only filesystem
# Note: The main binary will handle the --health-check flag gracefully
CMD ["/app/main"]