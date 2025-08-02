# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install security updates and build dependencies
RUN apk update && apk upgrade && apk add --no-cache git gcc musl-dev

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application (CGO disabled for MySQL-only operation)
RUN CGO_ENABLED=0 GOOS=linux go build \
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

# Install CA certificates
RUN apk update && apk add --no-cache ca-certificates

# Copy the binary with proper ownership and permissions
COPY --from=builder --chown=node:node /app/main /app/main

# Knowledge base files removed in Story 2.12 - now fetched from remote URL with ephemeral caching

# Create logs directory with proper permissions (data directory removed in Story 2.12)
RUN mkdir -p /app/logs && \
    chown -R node:node /app/logs && \
    chmod -R 775 /app/logs

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