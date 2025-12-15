# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

CNAP (Cloud Native AI Platform) is a Discord bot-based AI agent management system that orchestrates task execution through a controller-connector architecture. The system uses PostgreSQL for persistence and is containerized using Docker.

## Build and Test Commands

### Building

```bash
# Build the binary
make build

# Or directly with go
go build -o bin/cnap ./cmd/cnap

# Build with version info
make build VERSION=v1.0.0
```

### Testing

```bash
# Run all tests
go test ./...

# Run tests with coverage
make test

# Run specific package tests
go test ./internal/controller/...
go test ./internal/storage/...

# Run a single test
go test -v ./internal/controller -run TestControllerCreateTask

# Generate coverage report
make test-coverage
```

### Linting and Formatting

```bash
# Format code
make fmt

# Run linter
make lint

# Run all checks (fmt, lint, test)
make check
```

### Docker

```bash
# Build Docker image
make docker-build

# Run with Docker Compose (from project root)
docker compose -f docker/docker-compose.yml up -d

# View logs
docker logs cnap-unified

# Stop containers
docker compose -f docker/docker-compose.yml down
```

## Architecture

### Three-Layer Architecture

1. **Connector Layer** (`internal/connector`)

   - Discord bot interface (not yet fully implemented)
   - Receives commands from Discord users
   - Translates Discord messages into task requests

2. **Controller Layer** (`internal/controller`)

   - Central orchestration layer
   - Manages Agent and Task lifecycle
   - Handles state transitions and persistence
   - Methods: `CreateAgent`, `CreateTask`, `UpdateTaskStatus`, `ListTasksByAgent`, etc.

3. **Storage Layer** (`internal/storage`)
   - GORM-based PostgreSQL persistence
   - Auto-migration on startup
   - Repository pattern implementation

### Data Model Relationships

```
Agent (1) ──→ (N) Task ──→ (N) MessageIndex
                    │
                    ├──→ (N) RunStep
                    └──→ (N) Checkpoint
```

**Key Entities:**

- `Agent`: Multi-tenant logical units (status: active, idle, busy, deleted)
- `Task`: Execution units tied to agents (status: pending, running, completed, failed, canceled)
- `MessageIndex`: File path references to JSON message bodies (not stored in DB)
- `RunStep`: Step-by-step execution tracking (types: system, tool, model, checkpoint)
- `Checkpoint`: Git snapshot (hash) references for task state

### Application Entry Points

The `cnap` CLI has multiple commands:

- `cnap start`: Starts both controller and connector servers in goroutines
- `cnap health`: Health check endpoint for Docker
- `cnap agent create <name>`: Create a new agent
- `cnap agent run <agent> <name> <prompt>`: Execute short-lived agent task

### Concurrent Server Execution

The `start` command in `cmd/cnap/main.go` runs two servers concurrently:

1. **Controller Server**: Heartbeat-based monitoring (actual task execution logic pending)
2. **Connector Server**: Discord bot server (placeholder implementation)

Both use context-based cancellation and graceful shutdown with 30s timeout.

## Important Patterns

### Repository Pattern

All storage operations go through `storage.Repository`. Never use `db.Create()` directly in controller logic. Always use repository methods like `CreateAgent()`, `CreateTask()`, etc.

### Status Constants

Status values are defined in `internal/storage/constants.go`. Always use these constants:

- Agent statuses: `AgentStatusActive`, `AgentStatusIdle`, `AgentStatusBusy`, `AgentStatusDeleted`
- Task statuses: `TaskStatusPending`, `TaskStatusRunning`, `TaskStatusCompleted`, `TaskStatusFailed`, `TaskStatusCanceled`

### Testing Strategy

- Use in-memory SQLite for unit tests (see `controller_test.go`)
- Each test gets isolated database via `newTestController(t)` helper
- Test both success and error paths (e.g., `TestControllerCreateTaskWithoutAgent`)

## Environment Variables

### Database Configuration

- `DATABASE_URL`: PostgreSQL DSN (required, e.g., `postgres://user:pass@localhost:5432/cnap?sslmode=disable`)
- `DB_LOG_LEVEL`: GORM log level (silent, error, warn, info) - default: warn
- `DB_MAX_IDLE`: Connection pool idle count - default: 5
- `DB_MAX_OPEN`: Connection pool max count - default: 20
- `DB_CONN_LIFETIME`: Connection max lifetime - default: 30m
- `DB_SKIP_DEFAULT_TXN`: Skip default transaction - default: true
- `DB_PREPARE_STMT`: Enable prepared statement cache - default: false

### Application Configuration

- `ENV`: Environment (development, production)
- `LOG_LEVEL`: Application log level (debug, info, warn, error)

### Docker Compose Variables

- `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_PORT`
- `APP_ENV`, `APP_LOG_LEVEL`

## Docker Architecture

**Unified Container**: PostgreSQL and CNAP application run in a single container for simplified deployment.

The startup script (`docker/start.sh`) performs:

1. Initialize PostgreSQL (if first run)
2. Configure PostgreSQL for remote access
3. Start PostgreSQL in background
4. Wait for PostgreSQL readiness
5. Create database and user
6. Start CNAP application in foreground

## Git Workflow

This project uses:

- **Conventional Commits**: Prefix commits with `feat:`, `fix:`, `refactor:`, `test:`, `chore:`, `docs:`
- **Issue-based branches**: `<user>/<issue-number>` (e.g., `hyun/8`)
- **Korean commit messages**: Commit body and PR descriptions are in Korean

Example commit:

```
feat(controller): Task 관리 메서드 구현

디스코드 명령어로 작업을 시작하고 관리할 수 있도록
Controller에 Task 관리 메서드를 추가합니다.

Closes #8
```

## Current Implementation Status

### ✅ Implemented

- Agent CRUD operations
- Task CRUD operations
- Storage layer with GORM
- Docker unified container
- Health check endpoint
- Basic CLI structure

### 🚧 Pending Implementation

- Discord bot integration (connector is placeholder)
- Actual task execution in controller
- Message processing and storage
- RunStep tracking during execution
- Checkpoint creation for Git snapshots
- Connector ↔ Controller communication mechanism

## Next Development Steps

To implement Discord bot functionality:

1. Add `github.com/bwmarrin/discordgo` dependency
2. Implement Discord event handlers in `internal/connector/server.go`
3. Create communication channel between Connector and Controller
4. Implement actual task execution logic in Controller
5. Add message persistence to local JSON files with MessageIndex tracking
