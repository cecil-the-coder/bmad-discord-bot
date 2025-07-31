# Tech Stack

This table represents the definitive technology selection for the project.

| Category | Technology | Version | Purpose | Rationale |
| :--- | :--- | :--- | :--- | :--- |
| **Language** | Golang | 1.24.x | Backend Service Development | Fulfills PRD requirement (NFR2); excellent for concurrent, performant services. |
| **Discord Library** | discordgo | v0.28.x | Discord Gateway API Interaction | A popular and well-maintained library for building Discord bots in Go. |
| **Cache** | go-cache | v2.1.x | In-memory API Rate Counter | Provides a simple, thread-safe in-memory cache perfect for the rate limit monitor. |
| **Testing** | Go Test | 1.24.x | Unit & Integration Testing | Built-in to the Go toolchain; provides a robust testing framework. |
| **Build Tool** | Go Toolchain | 1.24.x | Compiling the application | The standard, built-in build system for Go. |
| **IaC / Runtime** | Docker | 26.x | Containerization | Fulfills PRD requirement (NFR3) for portable and scalable deployment. |
| **Logging** | slog | 1.24.x | Structured Logging | The official structured logging package in Go's standard library. |
| **Database** | SQLite3 | v3.x | Message State Persistence | Local file-based database for storing bot state and message tracking information. |
| **Database Driver** | go-sqlite3 | v1.14.x | SQLite Go Driver | CGO-based SQLite driver for Go applications. |