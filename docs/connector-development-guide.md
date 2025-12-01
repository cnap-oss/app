# Connector 개발 가이드

CNAP에 새로운 Connector를 개발하여 연결하는 방법을 설명합니다.

## 목차

- [개요](#개요)
- [Connector란?](#connector란)
- [아키텍처](#아키텍처)
- [개발 준비](#개발-준비)
- [Connector 구현](#connector-구현)
- [이벤트 채널 패턴](#이벤트-채널-패턴)
- [예제: Slack Connector](#예제-slack-connector)
- [테스트](#테스트)
- [배포](#배포)
- [FAQ](#faq)

## 개요

CNAP은 다양한 플랫폼에서 AI Agent를 실행할 수 있도록 Connector 아키텍처를 제공합니다. 현재 Discord Connector가 구현되어 있으며, 이 가이드를 따라 Slack, Telegram, Web 등 다양한 플랫폼용 Connector를 추가할 수 있습니다.

## Connector란?

Connector는 **외부 플랫폼과 CNAP Controller를 연결하는 인터페이스**입니다.

### Connector의 역할

1. **사용자 인터페이스 제공**
   - 플랫폼별 사용자 인터페이스 구현 (채팅, 버튼, 명령어 등)
   - 사용자 입력 수신 및 검증

2. **Controller와 통신**
   - 사용자 요청을 Controller로 전달
   - Task 실행 결과를 사용자에게 회신

3. **이벤트 채널 관리**
   - TaskEvent 채널로 Task 실행 요청 전송
   - TaskResult 채널에서 결과 수신 및 처리

## 아키텍처

### 전체 구조

```
┌─────────────────────────────────────────────────────────────┐
│                   외부 플랫폼 (Discord, Slack, ...)           │
│                   사용자 인터페이스                            │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                   Connector (신규 개발 대상)                   │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ 1. 플랫폼 이벤트 핸들러                                 │   │
│  │    - 메시지 수신                                      │   │
│  │    - 명령어 처리                                      │   │
│  │    - UI 상호작용                                     │   │
│  └──────────────────────────────────────────────────────┘   │
│                         │                                    │
│                         ▼                                    │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ 2. Task 생성 및 이벤트 전송                            │   │
│  │    - controller.CreateTask()                        │   │
│  │    - taskEventChan <- TaskEvent                     │   │
│  └──────────────────────────────────────────────────────┘   │
│                         │                                    │
│                         ▼                                    │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ 3. 결과 수신 및 회신                                   │   │
│  │    - resultHandler() goroutine                      │   │
│  │    - TaskResult 채널에서 읽기                          │   │
│  │    - 플랫폼에 결과 전송                                │   │
│  └──────────────────────────────────────────────────────┘   │
└────────────────────────▲────────────────────────────────────┘
                         │
                         │ TaskEvent, TaskResult 채널
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                   Controller (기존 구현)                       │
│  - eventLoop(): 이벤트 처리                                    │
│  - executeTaskWithResult(): Task 실행                        │
│  - Agent/Task 관리                                           │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                   Storage (PostgreSQL)                       │
└─────────────────────────────────────────────────────────────┘
```

### 데이터 흐름

```
사용자 입력
    ↓
Connector.handleUserInput()
    ↓
1. controller.CreateTask(agentName, taskID, userMessage)
2. 플랫폼에 "처리 중..." 메시지 전송
3. taskEventChan <- TaskEvent{Type: "execute", TaskID, ThreadID}
    ↓
Controller.eventLoop()
    ↓
executeTaskWithResult()
    ↓
taskResultChan <- TaskResult{TaskID, ThreadID, Status, Content}
    ↓
Connector.resultHandler()
    ↓
sendResultToUser(result)
    ↓
사용자에게 결과 전송
```

## 개발 준비

### 1. 프로젝트 구조

새로운 Connector는 `internal/connector/` 디렉토리 내에 별도 패키지로 구성합니다.

```
internal/connector/
├── discord/           # Discord Connector (참고용)
│   └── server.go
├── slack/             # 신규 Slack Connector (예시)
│   ├── server.go
│   ├── handlers.go
│   └── client.go
├── telegram/          # 신규 Telegram Connector (예시)
│   └── server.go
└── connector.go       # 공통 인터페이스 정의 (선택)
```

### 2. 필수 의존성

```go
import (
    "context"
    "github.com/cnap-oss/app/internal/controller"
    "go.uber.org/zap"
)
```

### 3. 플랫폼별 SDK 추가

```bash
# 예: Slack SDK
go get github.com/slack-go/slack

# 예: Telegram SDK
go get github.com/go-telegram-bot-api/telegram-bot-api/v5
```

## Connector 구현

### 1. Server 구조체 정의

```go
package slack

import (
    "context"
    "sync"

    "github.com/cnap-oss/app/internal/controller"
    "github.com/slack-go/slack"
    "go.uber.org/zap"
)

type Server struct {
    logger         *zap.Logger
    client         *slack.Client
    controller     *controller.Controller
    taskEventChan  chan controller.TaskEvent
    taskResultChan <-chan controller.TaskResult

    // 플랫폼별 상태 관리
    activeThreads  map[string]string // threadID → agentName
    threadsMutex   sync.RWMutex
}
```

### 2. NewServer 생성자

```go
func NewServer(
    logger *zap.Logger,
    ctrl *controller.Controller,
    eventChan chan controller.TaskEvent,
    resultChan <-chan controller.TaskResult,
) *Server {
    return &Server{
        logger:         logger,
        controller:     ctrl,
        taskEventChan:  eventChan,
        taskResultChan: resultChan,
        activeThreads:  make(map[string]string),
    }
}
```

### 3. Start/Stop 메서드

```go
func (s *Server) Start(ctx context.Context) error {
    s.logger.Info("Starting Slack connector server")

    // 1. 플랫폼 API 초기화
    token := os.Getenv("SLACK_TOKEN")
    if token == "" {
        return fmt.Errorf("SLACK_TOKEN environment variable not set")
    }
    s.client = slack.New(token)

    // 2. 결과 핸들러 goroutine 시작
    go s.resultHandler(ctx)

    // 3. 플랫폼 이벤트 수신 시작
    rtm := s.client.NewRTM()
    go rtm.ManageConnection()

    // 4. 이벤트 루프
    for {
        select {
        case msg := <-rtm.IncomingEvents:
            s.handleEvent(msg)
        case <-ctx.Done():
            s.logger.Info("Slack connector shutting down")
            return ctx.Err()
        }
    }
}

func (s *Server) Stop(ctx context.Context) error {
    s.logger.Info("Stopping Slack connector server")
    // 리소스 정리
    return nil
}
```

### 4. 이벤트 핸들러 구현

```go
func (s *Server) handleEvent(event slack.RTMEvent) {
    switch ev := event.Data.(type) {
    case *slack.MessageEvent:
        s.handleMessage(ev)
    case *slack.ConnectedEvent:
        s.logger.Info("Slack bot connected")
    // 기타 이벤트 처리...
    }
}

func (s *Server) handleMessage(msg *slack.MessageEvent) {
    // 봇 자신의 메시지는 무시
    if msg.User == "" || msg.BotID != "" {
        return
    }

    ctx := context.Background()

    // Agent 스레드 확인
    s.threadsMutex.RLock()
    agentName, isActiveThread := s.activeThreads[msg.Channel]
    s.threadsMutex.RUnlock()

    if !isActiveThread {
        // 일반 메시지 또는 명령어 처리
        if msg.Text == "/agent list" {
            s.showAgentList(msg.Channel)
        }
        return
    }

    // Agent 대화 처리
    s.callAgentInThread(msg, agentName)
}
```

### 5. Task 생성 및 실행

```go
func (s *Server) callAgentInThread(msg *slack.MessageEvent, agentName string) {
    ctx := context.Background()

    // Task ID 생성 (플랫폼의 고유 ID 활용)
    taskID := fmt.Sprintf("task-%s", msg.Timestamp)
    threadID := msg.Channel

    s.logger.Info("Creating task for agent call",
        zap.String("agent", agentName),
        zap.String("task_id", taskID),
        zap.String("thread_id", threadID),
        zap.String("user_message", msg.Text),
    )

    // 1. Task 생성
    if err := s.controller.CreateTask(ctx, agentName, taskID, msg.Text); err != nil {
        s.logger.Error("Failed to create task", zap.Error(err))
        s.client.PostMessage(msg.Channel, slack.MsgOptionText(
            fmt.Sprintf("❌ Task 생성 실패: %v", err),
            false,
        ))
        return
    }

    // 2. "처리 중" 메시지 전송
    s.client.PostMessage(msg.Channel, slack.MsgOptionText(
        fmt.Sprintf("⏳ '%s'가 처리 중입니다...", agentName),
        false,
    ))

    // 3. Task 실행 이벤트 전송 (비동기, 논블로킹)
    s.taskEventChan <- controller.TaskEvent{
        Type:     "execute",
        TaskID:   taskID,
        ThreadID: threadID,
        Prompt:   msg.Text,
    }

    s.logger.Info("Task execution event sent",
        zap.String("task_id", taskID),
        zap.String("agent", agentName),
    )
}
```

## 이벤트 채널 패턴

### TaskEvent 채널 (Connector → Controller)

**용도:** Task 실행 요청을 Controller로 전송

```go
type TaskEvent struct {
    Type     string // "execute", "cancel"
    TaskID   string
    ThreadID string // 플랫폼의 대화 스레드 ID
    Prompt   string // 선택 사항
}

// 사용 예시
s.taskEventChan <- controller.TaskEvent{
    Type:     "execute",
    TaskID:   "task-12345",
    ThreadID: "slack-thread-abc",
    Prompt:   "사용자 메시지",
}
```

**주의사항:**
- 채널 전송은 **논블로킹**입니다 (버퍼가 가득 차지 않은 한)
- TaskID는 **고유해야** 합니다
- ThreadID는 결과 회신에 사용되므로 **정확해야** 합니다

### TaskResult 채널 (Controller → Connector)

**용도:** Task 실행 결과를 Controller로부터 수신

```go
type TaskResult struct {
    TaskID   string
    ThreadID string
    Status   string // "completed", "failed", "canceled"
    Content  string
    Error    error
}
```

**resultHandler 구현:**

```go
func (s *Server) resultHandler(ctx context.Context) {
    s.logger.Info("Result handler started")
    defer s.logger.Info("Result handler stopped")

    for {
        select {
        case result := <-s.taskResultChan:
            s.logger.Info("Received task result",
                zap.String("task_id", result.TaskID),
                zap.String("thread_id", result.ThreadID),
                zap.String("status", result.Status),
            )
            s.sendResultToUser(result)

        case <-ctx.Done():
            s.logger.Info("Result handler shutting down")
            return
        }
    }
}

func (s *Server) sendResultToUser(result controller.TaskResult) {
    if result.ThreadID == "" {
        s.logger.Warn("Thread ID is empty, cannot send result")
        return
    }

    // 상태별 메시지 구성
    var message string
    if result.Error != nil || result.Status == "failed" {
        message = fmt.Sprintf("❌ Task 실행 실패\nTask ID: %s\n오류: %v",
            result.TaskID, result.Error)
    } else if result.Status == "canceled" {
        message = fmt.Sprintf("⚠️ Task 취소됨\nTask ID: %s", result.TaskID)
    } else {
        message = fmt.Sprintf("✅ Task 실행 완료\nTask ID: %s\n\n결과:\n%s",
            result.TaskID, result.Content)
    }

    // 플랫폼에 전송
    _, _, err := s.client.PostMessage(
        result.ThreadID,
        slack.MsgOptionText(message, false),
    )
    if err != nil {
        s.logger.Error("Failed to send result to Slack",
            zap.String("task_id", result.TaskID),
            zap.Error(err),
        )
    }
}
```

## 예제: Slack Connector

### 완전한 구현 예시

```go
package slack

import (
    "context"
    "fmt"
    "os"
    "sync"

    "github.com/cnap-oss/app/internal/controller"
    slackapi "github.com/slack-go/slack"
    "go.uber.org/zap"
)

type Server struct {
    logger         *zap.Logger
    client         *slackapi.Client
    rtm            *slackapi.RTM
    controller     *controller.Controller
    taskEventChan  chan controller.TaskEvent
    taskResultChan <-chan controller.TaskResult
    activeThreads  map[string]string
    threadsMutex   sync.RWMutex
}

func NewServer(
    logger *zap.Logger,
    ctrl *controller.Controller,
    eventChan chan controller.TaskEvent,
    resultChan <-chan controller.TaskResult,
) *Server {
    return &Server{
        logger:         logger,
        controller:     ctrl,
        taskEventChan:  eventChan,
        taskResultChan: resultChan,
        activeThreads:  make(map[string]string),
    }
}

func (s *Server) Start(ctx context.Context) error {
    s.logger.Info("Starting Slack connector server")

    token := os.Getenv("SLACK_BOT_TOKEN")
    if token == "" {
        return fmt.Errorf("SLACK_BOT_TOKEN environment variable not set")
    }

    s.client = slackapi.New(token)
    s.rtm = s.client.NewRTM()

    // 결과 핸들러 시작
    go s.resultHandler(ctx)

    // RTM 연결 관리
    go s.rtm.ManageConnection()

    // 이벤트 루프
    for {
        select {
        case msg := <-s.rtm.IncomingEvents:
            s.handleEvent(msg)
        case <-ctx.Done():
            s.logger.Info("Slack connector shutting down")
            return s.Stop(context.Background())
        }
    }
}

func (s *Server) Stop(ctx context.Context) error {
    s.logger.Info("Stopping Slack connector server")
    if s.rtm != nil {
        if err := s.rtm.Disconnect(); err != nil {
            return err
        }
    }
    return nil
}

func (s *Server) handleEvent(event slackapi.RTMEvent) {
    switch ev := event.Data.(type) {
    case *slackapi.MessageEvent:
        s.handleMessage(ev)
    case *slackapi.ConnectedEvent:
        s.logger.Info("Slack bot connected",
            zap.Int("connection_count", ev.ConnectionCount),
        )
    }
}

func (s *Server) handleMessage(msg *slackapi.MessageEvent) {
    if msg.User == "" || msg.BotID != "" {
        return
    }

    // 명령어 처리
    if msg.Text == "/agent start" {
        s.startAgentThread(msg)
        return
    }

    // Agent 스레드에서 메시지 처리
    s.threadsMutex.RLock()
    agentName, ok := s.activeThreads[msg.Channel]
    s.threadsMutex.RUnlock()

    if ok {
        s.callAgentInThread(msg, agentName)
    }
}

func (s *Server) startAgentThread(msg *slackapi.MessageEvent) {
    // Agent 선택 UI 표시 (간소화 버전)
    agentName := "default-agent"

    s.threadsMutex.Lock()
    s.activeThreads[msg.Channel] = agentName
    s.threadsMutex.Unlock()

    s.client.PostMessage(msg.Channel, slackapi.MsgOptionText(
        fmt.Sprintf("✅ Agent '%s'와 대화를 시작합니다.", agentName),
        false,
    ))
}

func (s *Server) callAgentInThread(msg *slackapi.MessageEvent, agentName string) {
    ctx := context.Background()
    taskID := fmt.Sprintf("task-%s", msg.Timestamp)

    if err := s.controller.CreateTask(ctx, agentName, taskID, msg.Text); err != nil {
        s.client.PostMessage(msg.Channel, slackapi.MsgOptionText(
            fmt.Sprintf("❌ Task 생성 실패: %v", err),
            false,
        ))
        return
    }

    s.client.PostMessage(msg.Channel, slackapi.MsgOptionText(
        "⏳ 처리 중입니다...",
        false,
    ))

    s.taskEventChan <- controller.TaskEvent{
        Type:     "execute",
        TaskID:   taskID,
        ThreadID: msg.Channel,
        Prompt:   msg.Text,
    }
}

func (s *Server) resultHandler(ctx context.Context) {
    for {
        select {
        case result := <-s.taskResultChan:
            s.sendResultToUser(result)
        case <-ctx.Done():
            return
        }
    }
}

func (s *Server) sendResultToUser(result controller.TaskResult) {
    var message string
    if result.Error != nil {
        message = fmt.Sprintf("❌ 실패: %v", result.Error)
    } else {
        message = fmt.Sprintf("✅ 완료\n\n%s", result.Content)
    }

    s.client.PostMessage(result.ThreadID, slackapi.MsgOptionText(message, false))
}
```

## 테스트

### 1. 단위 테스트

```go
package slack_test

import (
    "context"
    "testing"

    "github.com/cnap-oss/app/internal/connector/slack"
    "github.com/cnap-oss/app/internal/controller"
    "github.com/stretchr/testify/assert"
    "go.uber.org/zap/zaptest"
)

func TestSlackServerCreation(t *testing.T) {
    logger := zaptest.NewLogger(t)

    eventChan := make(chan controller.TaskEvent, 10)
    resultChan := make(chan controller.TaskResult, 10)

    server := slack.NewServer(logger, nil, eventChan, resultChan)

    assert.NotNil(t, server)
}

func TestResultHandler(t *testing.T) {
    logger := zaptest.NewLogger(t)

    eventChan := make(chan controller.TaskEvent, 10)
    resultChan := make(chan controller.TaskResult, 10)

    server := slack.NewServer(logger, nil, eventChan, resultChan)

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    go server.resultHandler(ctx)

    // 테스트 결과 전송
    result := controller.TaskResult{
        TaskID:   "test-task",
        ThreadID: "test-thread",
        Status:   "completed",
        Content:  "Test result",
    }

    resultChan <- result

    // 결과 처리 확인 (실제로는 mock client 필요)
}
```

### 2. 통합 테스트

```bash
# 환경 변수 설정
export SLACK_BOT_TOKEN=xoxb-your-token
export DATABASE_URL=postgres://cnap:cnap@localhost:5432/cnap

# Connector 포함하여 전체 시스템 시작
./bin/cnap start

# Slack에서 테스트
# 1. /agent start 입력
# 2. 메시지 전송
# 3. 결과 확인
```

## 배포

### 1. main.go 통합

```go
// cmd/cnap/main.go
func runStart(logger *zap.Logger) error {
    // ... 기존 코드 ...

    // 채널 생성
    taskEventChan := make(chan controller.TaskEvent, 100)
    taskResultChan := make(chan controller.TaskResult, 100)

    // Controller 생성
    controllerServer := controller.NewController(logger, repo, taskEventChan, taskResultChan)

    // 여러 Connector 동시 실행
    discordServer := discord.NewServer(logger, controllerServer, taskEventChan, taskResultChan)
    slackServer := slack.NewServer(logger, controllerServer, taskEventChan, taskResultChan)

    // 병렬 실행
    go controllerServer.Start(ctx)
    go discordServer.Start(ctx)
    go slackServer.Start(ctx)

    // ...
}
```

### 2. 환경 변수 설정

```bash
# .env
DISCORD_TOKEN=your_discord_token
SLACK_BOT_TOKEN=xoxb-your-slack-token
TELEGRAM_BOT_TOKEN=your_telegram_token
DATABASE_URL=postgres://cnap:cnap@localhost:5432/cnap
```

### 3. Docker 구성

```yaml
# docker-compose.yml
services:
  cnap-app:
    environment:
      - DISCORD_TOKEN=${DISCORD_TOKEN}
      - SLACK_BOT_TOKEN=${SLACK_BOT_TOKEN}
      - TELEGRAM_BOT_TOKEN=${TELEGRAM_BOT_TOKEN}
```

## FAQ

### Q1: 여러 Connector를 동시에 실행할 수 있나요?

**A:** 네, 가능합니다. 모든 Connector는 동일한 `taskEventChan`과 `taskResultChan`을 공유하므로 여러 플랫폼에서 동시에 Task를 실행할 수 있습니다.

```go
go discordServer.Start(ctx)
go slackServer.Start(ctx)
go telegramServer.Start(ctx)
```

### Q2: 채널 버퍼 크기는 어떻게 결정하나요?

**A:** 동시 사용자 수와 예상 부하에 따라 결정합니다.
- 소규모 (10-100명): 버퍼 크기 10-50
- 중규모 (100-1000명): 버퍼 크기 100-500
- 대규모 (1000명+): 버퍼 크기 500-1000 또는 DB 기반 큐 사용

### Q3: TaskID 중복을 어떻게 방지하나요?

**A:** 플랫폼의 고유 ID를 활용하거나 UUID를 사용하세요.

```go
// 방법 1: 플랫폼 고유 ID 활용
taskID := fmt.Sprintf("task-%s-%s", platformName, messageID)

// 방법 2: UUID 사용
import "github.com/google/uuid"
taskID := uuid.New().String()

// 방법 3: Timestamp + Random
taskID := fmt.Sprintf("task-%d-%d", time.Now().UnixNano(), rand.Int())
```

### Q4: 긴 실행 시간 Task는 어떻게 처리하나요?

**A:** Controller의 `executeTaskWithResult()`에 이미 5분 타임아웃이 설정되어 있습니다. 필요 시 플랫폼에 진행 상황을 업데이트할 수 있습니다.

```go
// 진행 상황 채널 추가 (선택 사항)
type TaskProgress struct {
    TaskID   string
    ThreadID string
    Message  string
}

progressChan := make(chan TaskProgress, 100)

// Connector에서 진행 상황 수신
go func() {
    for progress := range progressChan {
        s.client.PostMessage(progress.ThreadID,
            slack.MsgOptionText(progress.Message, false))
    }
}()
```

### Q5: 에러 처리는 어떻게 하나요?

**A:** TaskResult의 Error 필드를 확인하여 사용자에게 친절한 메시지를 전송하세요.

```go
func (s *Server) sendResultToUser(result controller.TaskResult) {
    if result.Error != nil {
        // 에러 타입별 메시지 커스터마이징
        var userMessage string
        switch {
        case strings.Contains(result.Error.Error(), "timeout"):
            userMessage = "⏱️ 처리 시간이 초과되었습니다. 더 간단한 요청을 시도해주세요."
        case strings.Contains(result.Error.Error(), "API key"):
            userMessage = "🔑 API 키 오류가 발생했습니다. 관리자에게 문의하세요."
        default:
            userMessage = fmt.Sprintf("❌ 오류가 발생했습니다: %v", result.Error)
        }

        s.client.PostMessage(result.ThreadID,
            slack.MsgOptionText(userMessage, false))
    }
}
```

### Q6: 플랫폼별 인증은 어떻게 처리하나요?

**A:** 환경 변수로 토큰을 관리하고, Start() 메서드에서 초기화하세요.

```go
func (s *Server) Start(ctx context.Context) error {
    token := os.Getenv("SLACK_BOT_TOKEN")
    if token == "" {
        return fmt.Errorf("SLACK_BOT_TOKEN not set")
    }

    s.client = slack.New(token)

    // 토큰 유효성 검증
    auth, err := s.client.AuthTest()
    if err != nil {
        return fmt.Errorf("invalid token: %w", err)
    }

    s.logger.Info("Authenticated as",
        zap.String("user", auth.User),
        zap.String("team", auth.Team))

    // ...
}
```

## 참고 자료

- **Discord Connector 구현:** `internal/connector/server.go`
- **Controller 이벤트 처리:** `internal/controller/controller.go` (eventLoop, handleTaskEvent)
- **이벤트 채널 패턴 이슈:** #63
- **Discord Bot 배포 가이드:** `docs/linux-deployment-guide.md`

## 기여

새로운 Connector를 개발하셨다면 Pull Request를 환영합니다!

1. 브랜치 생성: `git checkout -b feature/telegram-connector`
2. 구현 및 테스트
3. 문서 업데이트 (이 가이드에 예제 추가)
4. PR 생성

---

**다음 단계:**
- Slack Connector 구현 예제를 참고하여 원하는 플랫폼용 Connector 개발
- 테스트 후 배포
- 사용자 피드백 수집 및 개선
