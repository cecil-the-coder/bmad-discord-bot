# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install security updates and build dependencies including SQLite
RUN apk update && apk upgrade && apk add --no-cache git gcc musl-dev sqlite-dev

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with CGO enabled for SQLite support
RUN CGO_ENABLED=1 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags='-w -s' \
    -o main cmd/bot/main.go

# Final stage - use Node.js Alpine for musl compatibility
FROM node:22-alpine

# Add labels for container metadata
LABEL maintainer="BMAD Knowledge Bot Team"
LABEL description="Discord bot for knowledge management"
LABEL version="1.0"
LABEL security.scan="required"

# Install CA certificates, SQLite runtime, and Gemini CLI
RUN apk update && apk add --no-cache ca-certificates sqlite && \
    npm install -g @google/gemini-cli

# Copy the binary with proper ownership and permissions
COPY --from=builder --chown=node:node /app/main /app/main

# Copy knowledge base files  
COPY --chown=node:node internal/knowledge /app/internal/knowledge

# Create logs and data directories with proper permissions
RUN mkdir -p /app/logs /app/data && \
    chown -R node:node /app/logs /app/data && \
    chmod -R 775 /app/logs /app/data

# Set proper file permissions
USER node

# Use non-root working directory
WORKDIR /app

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD ["/app/main", "--health-check"] || exit 1

# Security: Run as non-root user, read-only filesystem
# Note: The main binary will handle the --health-check flag gracefully
CMD ["/app/main"]