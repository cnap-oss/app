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

4. **Runner Layer** (`internal/runner`) - 추가됨 (2024-12-15)
   - Docker Container 기반 Task 실행 환경
   - OpenCode API 통합 및 SSE 이벤트 스트리밍
   - 비동기 실행 및 콜백 기반 결과 전달
   - RunnerManager를 통한 생명주기 관리

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

## Runner 비동기 콜백 아키텍처

### 개요

Runner는 Docker Container 기반으로 Task를 실행하는 컴포넌트입니다. 완전한 비동기 실행 모델을 사용하며, 실행 결과는 콜백을 통해 Controller에 전달됩니다.

### 주요 컴포넌트

1. **TaskRunner 인터페이스** (`internal/runner/runner.go`)

   - `Run(ctx, req)`: 비동기 실행 시작 (즉시 반환)
   - 구현체: `Runner` (Docker Container 기반)

2. **StatusCallback 인터페이스** (`internal/runner/runner.go`)

   - `OnStarted(taskID, sessionID)`: 실행 시작 및 세션 생성
   - `OnMessage(taskID, *RunnerMessage)`: SSE 이벤트 수신
   - `OnComplete(taskID, *RunResult)`: 성공 완료
   - `OnError(taskID, error)`: 에러 발생

3. **RunnerMessage 타입** (`internal/runner/api_types.go`)

   - SSE 이벤트를 타입 안전하게 추상화
   - 타입: Text, Reasoning, ToolCall, ToolResult, Complete, Error 등
   - 헬퍼 메서드: `IsText()`, `IsToolRelated()`, `IsTerminal()`

4. **RunnerManager** (`internal/runner/manager.go`)
   - Runner 생명주기 관리 (생성, 시작, 중지, 삭제)
   - `CreateRunner(ctx, taskID, agentInfo, callback, opts...)`: 콜백과 함께 Runner 생성
   - `StartRunner(ctx, taskID)`: Container 시작
   - `StopRunner(ctx, taskID)`: Container 중지 및 제거

### 실행 흐름

```
1. Controller.CreateTask()
   └─> RunnerManager.CreateRunner(callback) - 콜백 등록
       └─> Runner 생성 (Container는 아직 시작 안됨)

2. RunnerManager.StartRunner()
   └─> Docker Container 시작
   └─> Health check 대기

3. Controller.executeTask() (goroutine)
   └─> Runner.Run(ctx, req) - 즉시 반환
       └─> [별도 goroutine] runInternal()
           ├─> OpenCode 세션 생성
           ├─> callback.OnStarted(taskID, sessionID)
           ├─> SSE 이벤트 구독 시작
           ├─> 프롬프트 전송
           └─> 이벤트 수신 루프
               ├─> convertEventToMessage() - SSE → RunnerMessage
               ├─> callback.OnMessage(taskID, msg)
               └─> 완료 시 callback.OnComplete() 또는 OnError()
```

### 콜백 생명주기

```
NewRunner(taskID, agentInfo, callback, ...)  # 콜백 등록 (단 한 번)
  │
  ├─> StartRunner()                           # Container 시작
  │
  └─> Run()                                   # 비동기 실행
       │
       ├─> OnStarted(taskID, sessionID)       # 세션 생성
       │
       ├─> OnMessage(taskID, msg) ────┐       # SSE 이벤트 (여러 번)
       ├─> OnMessage(taskID, msg)     │
       ├─> OnMessage(taskID, msg)     │ 반복
       ├─> ...                        │
       │                              │
       └─> OnComplete(taskID, result) ┘       # 성공 완료
           또는
           OnError(taskID, err)               # 에러 발생
```

### RunnerMessage 타입 시스템

Controller는 `msg.Type`을 통해 이벤트 종류를 식별하고 처리합니다:

```go
switch msg.Type {
case MessageTypeText:
    // msg.Content에 스트리밍 텍스트
    connector.SendStreamingText(msg.Content)

case MessageTypeToolCall:
    // msg.ToolCall에 도구 호출 정보
    connector.SendToolStatus(msg.ToolCall.ToolName, "running")

case MessageTypeToolResult:
    // msg.ToolResult에 도구 실행 결과
    connector.SendToolResult(msg.ToolResult.Result)

case MessageTypeComplete:
    // 메시지 완료 (OnComplete 직전 호출됨)
    // 전체 출력은 OnComplete에서 전달됨
}
```

### 레거시 제거

Phase 5에서 다음 항목들이 제거되었습니다:

- `Runner.runSync()` - 동기 폴링 방식
- `Runner.runWithStreaming()` - 로직은 executeWithStreaming으로 통합
- `RunRequest.Callback` 필드 - 콜백은 생성자에서만 등록

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

- `CNAP_DB_DSN`: PostgreSQL DSN (required, e.g., `postgres://user:pass@localhost:5432/cnap?sslmode=disable`)
- `CNAP_DB_LOG_LEVEL`: GORM log level (silent, error, warn, info) - default: warn
- `CNAP_DB_MAX_IDLE`: Connection pool idle count - default: 5
- `CNAP_DB_MAX_OPEN`: Connection pool max count - default: 20
- `CNAP_DB_CONN_LIFETIME`: Connection max lifetime - default: 30m
- `CNAP_DB_SKIP_DEFAULT_TXN`: Skip default transaction - default: true
- `CNAP_DB_PREPARE_STMT`: Enable prepared statement cache - default: false

### Application Configuration

- `CNAP_ENV`: Environment (development, production)
- `CNAP_LOG_LEVEL`: Application log level (debug, info, warn, error)

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
- **Runner 비동기 콜백 아키텍처** (2024-12-15)
  - Docker Container 기반 TaskRunner 구현
  - OpenCode API 통합 (SSE 이벤트 스트리밍)
  - 비동기 실행 및 콜백 기반 결과 전달
  - RunnerMessage 타입 시스템으로 타입 안전성 확보
  - RunnerManager를 통한 Runner 생명주기 관리

### 🚧 Pending Implementation

- Discord bot integration (connector is placeholder)
- Message processing and storage
- RunStep tracking during execution
- Checkpoint creation for Git snapshots
- Connector ↔ Controller communication mechanism
- Runner 통합 테스트 확장

## Next Development Steps

To implement Discord bot functionality:

2. Implement Discord event handlers in `internal/connector/server.go`
3. Create communication channel between Connector and Controller
4. Implement actual task execution logic in Controller
5. Add message persistence to local JSON files with MessageIndex tracking
