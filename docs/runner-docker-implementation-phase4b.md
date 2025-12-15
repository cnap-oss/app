# Runner Docker 구현 명세서 - Phase 4b: 에러 처리 및 모니터링

본 문서는 Runner Docker 구현의 Phase 4 단계 중 FR-009 에러 처리와 FR-010 로깅/모니터링에 대한 세부 구현 명세서입니다.

---

## 목차

1. [개요](#1-개요)
2. [FR-009: 에러 처리 및 복구](#2-fr-009-에러-처리-및-복구)
3. [FR-010: 로깅 및 모니터링](#3-fr-010-로깅-및-모니터링)
4. [구현 체크리스트](#4-구현-체크리스트)

---

## 1. 개요

### 1.1 Phase 4b 목표

Phase 4b의 목표는 안정적인 운영을 위한 에러 처리와 모니터링을 구현하는 것입니다:

- Container 관련 에러 처리 및 복구
- 구조화된 로깅
- 메트릭 수집 및 모니터링

### 1.2 예상 파일 변경

| 작업 유형 | 파일 경로                     | 설명                |
| --------- | ----------------------------- | ------------------- |
| 신규      | `internal/runner/errors.go`   | 에러 타입 정의      |
| 신규      | `internal/runner/recovery.go` | 복구 메커니즘       |
| 신규      | `internal/runner/metrics.go`  | 메트릭 수집         |
| 수정      | `internal/runner/runner.go`   | 에러 처리 적용      |
| 수정      | `internal/runner/manager.go`  | 로깅 및 메트릭 적용 |

---

## 2. FR-009: 에러 처리 및 복구

### 2.1 요구사항 요약

Container 관련 에러 상황에 대한 처리 및 복구 메커니즘을 구현합니다.

### 2.2 에러 타입 정의

```go
// internal/runner/errors.go

package taskrunner

import (
    "errors"
    "fmt"
)

// 기본 에러 타입
var (
    // Container 관련 에러
    ErrContainerNotFound     = errors.New("container를 찾을 수 없음")
    ErrContainerStartFailed  = errors.New("container 시작 실패")
    ErrContainerStopFailed   = errors.New("container 중지 실패")
    ErrContainerNotRunning   = errors.New("container가 실행 중이 아님")
    ErrContainerUnhealthy    = errors.New("container 상태 비정상")

    // Runner 관련 에러
    ErrRunnerNotReady        = errors.New("runner가 준비되지 않음")
    ErrRunnerAlreadyExists   = errors.New("runner가 이미 존재함")
    ErrRunnerNotFound        = errors.New("runner를 찾을 수 없음")

    // 리소스 관련 에러
    ErrMaxContainersReached  = errors.New("최대 container 수 초과")
    ErrInsufficientResources = errors.New("리소스 부족")

    // 통신 관련 에러
    ErrAPITimeout            = errors.New("API 요청 타임아웃")
    ErrAPIConnectionFailed   = errors.New("API 연결 실패")

    // 작업 공간 관련 에러
    ErrWorkspaceNotFound     = errors.New("작업 공간을 찾을 수 없음")
    ErrWorkspaceCreateFailed = errors.New("작업 공간 생성 실패")
)

// RunnerError는 Runner 관련 에러를 래핑합니다.
type RunnerError struct {
    Op       string // 작업명 (예: "Start", "Stop", "Run")
    RunnerID string // Runner ID
    Err      error  // 원본 에러
}

func (e *RunnerError) Error() string {
    if e.RunnerID != "" {
        return fmt.Sprintf("runner[%s] %s: %v", e.RunnerID, e.Op, e.Err)
    }
    return fmt.Sprintf("runner %s: %v", e.Op, e.Err)
}

func (e *RunnerError) Unwrap() error {
    return e.Err
}

// NewRunnerError는 새 RunnerError를 생성합니다.
func NewRunnerError(op, runnerID string, err error) *RunnerError {
    return &RunnerError{
        Op:       op,
        RunnerID: runnerID,
        Err:      err,
    }
}

// ContainerError는 Container 관련 에러를 래핑합니다.
type ContainerError struct {
    Op          string // 작업명
    ContainerID string // Container ID
    Err         error  // 원본 에러
    Recoverable bool   // 복구 가능 여부
}

func (e *ContainerError) Error() string {
    return fmt.Sprintf("container[%s] %s: %v", e.ContainerID, e.Op, e.Err)
}

func (e *ContainerError) Unwrap() error {
    return e.Err
}

// IsRecoverable는 에러가 복구 가능한지 확인합니다.
func IsRecoverable(err error) bool {
    var containerErr *ContainerError
    if errors.As(err, &containerErr) {
        return containerErr.Recoverable
    }

    // 특정 에러 타입 확인
    switch {
    case errors.Is(err, ErrAPITimeout):
        return true
    case errors.Is(err, ErrAPIConnectionFailed):
        return true
    case errors.Is(err, ErrContainerUnhealthy):
        return true
    default:
        return false
    }
}

// IsRetryable는 재시도 가능한 에러인지 확인합니다.
func IsRetryable(err error) bool {
    switch {
    case errors.Is(err, ErrAPITimeout):
        return true
    case errors.Is(err, ErrAPIConnectionFailed):
        return true
    case errors.Is(err, ErrContainerStartFailed):
        return true
    default:
        return false
    }
}
```

### 2.3 복구 메커니즘

```go
// internal/runner/recovery.go

package taskrunner

import (
    "context"
    "time"

    "go.uber.org/zap"
)

// RecoveryConfig는 복구 설정입니다.
type RecoveryConfig struct {
    MaxRetries     int           // 최대 재시도 횟수
    InitialBackoff time.Duration // 초기 백오프 시간
    MaxBackoff     time.Duration // 최대 백오프 시간
    BackoffFactor  float64       // 백오프 증가 계수
}

// DefaultRecoveryConfig는 기본 복구 설정을 반환합니다.
func DefaultRecoveryConfig() RecoveryConfig {
    return RecoveryConfig{
        MaxRetries:     3,
        InitialBackoff: 1 * time.Second,
        MaxBackoff:     30 * time.Second,
        BackoffFactor:  2.0,
    }
}

// RecoveryManager는 에러 복구를 관리합니다.
type RecoveryManager struct {
    config RecoveryConfig
    logger *zap.Logger
}

// NewRecoveryManager는 새 RecoveryManager를 생성합니다.
func NewRecoveryManager(logger *zap.Logger, config ...RecoveryConfig) *RecoveryManager {
    cfg := DefaultRecoveryConfig()
    if len(config) > 0 {
        cfg = config[0]
    }

    if logger == nil {
        logger = zap.NewNop()
    }

    return &RecoveryManager{
        config: cfg,
        logger: logger,
    }
}

// RetryOperation은 작업을 재시도합니다.
func (rm *RecoveryManager) RetryOperation(ctx context.Context, opName string, op func() error) error {
    var lastErr error
    backoff := rm.config.InitialBackoff

    for attempt := 0; attempt <= rm.config.MaxRetries; attempt++ {
        if attempt > 0 {
            rm.logger.Info("작업 재시도",
                zap.String("operation", opName),
                zap.Int("attempt", attempt),
                zap.Duration("backoff", backoff),
            )

            select {
            case <-time.After(backoff):
            case <-ctx.Done():
                return ctx.Err()
            }

            // 백오프 증가
            backoff = time.Duration(float64(backoff) * rm.config.BackoffFactor)
            if backoff > rm.config.MaxBackoff {
                backoff = rm.config.MaxBackoff
            }
        }

        err := op()
        if err == nil {
            if attempt > 0 {
                rm.logger.Info("작업 재시도 성공",
                    zap.String("operation", opName),
                    zap.Int("attempts", attempt+1),
                )
            }
            return nil
        }

        lastErr = err

        // 재시도 불가능한 에러면 즉시 반환
        if !IsRetryable(err) {
            rm.logger.Warn("재시도 불가능한 에러",
                zap.String("operation", opName),
                zap.Error(err),
            )
            return err
        }

        rm.logger.Warn("작업 실패, 재시도 예정",
            zap.String("operation", opName),
            zap.Int("attempt", attempt),
            zap.Error(err),
        )
    }

    rm.logger.Error("최대 재시도 횟수 초과",
        zap.String("operation", opName),
        zap.Int("max_retries", rm.config.MaxRetries),
        zap.Error(lastErr),
    )

    return lastErr
}

// RecoverContainer는 Container 복구를 시도합니다.
func (rm *RecoveryManager) RecoverContainer(ctx context.Context, runner *Runner) error {
    rm.logger.Info("Container 복구 시작",
        zap.String("runner_id", runner.ID),
        zap.String("status", runner.Status),
    )

    // 1. 기존 Container 정리
    if runner.ContainerID != "" {
        if err := runner.Stop(ctx); err != nil {
            rm.logger.Warn("기존 Container 정리 중 오류",
                zap.String("container_id", runner.ContainerID),
                zap.Error(err),
            )
        }
    }

    // 2. 새 Container 시작 (재시도 적용)
    return rm.RetryOperation(ctx, "RecoverContainer", func() error {
        return runner.Start(ctx)
    })
}
```

### 2.4 Runner에 에러 처리 적용

```go
// internal/runner/runner.go (수정)

// Start에 에러 처리 적용
func (r *Runner) Start(ctx context.Context) error {
    r.logger.Info("Starting runner container",
        zap.String("runner_id", r.ID),
    )

    r.Status = RunnerStatusStarting

    // 작업 공간 디렉토리 생성
    if err := os.MkdirAll(r.WorkspacePath, 0755); err != nil {
        r.Status = RunnerStatusFailed
        return NewRunnerError("Start", r.ID,
            fmt.Errorf("%w: %v", ErrWorkspaceCreateFailed, err))
    }

    // Container 생성
    containerID, err := CreateContainer(ctx, r.dockerClient, /* config */)
    if err != nil {
        r.Status = RunnerStatusFailed
        return &ContainerError{
            Op:          "Create",
            ContainerID: "",
            Err:         err,
            Recoverable: IsRecoverable(err),
        }
    }
    r.ContainerID = containerID

    // Container 시작
    if err := StartContainer(ctx, r.dockerClient, r.ContainerID); err != nil {
        r.Status = RunnerStatusFailed
        _ = RemoveContainer(ctx, r.dockerClient, r.ContainerID, true)
        return &ContainerError{
            Op:          "Start",
            ContainerID: r.ContainerID,
            Err:         err,
            Recoverable: true,
        }
    }

    // Health check
    if err := r.waitForHealthy(ctx); err != nil {
        r.Status = RunnerStatusFailed
        _ = r.Stop(ctx)
        return &ContainerError{
            Op:          "HealthCheck",
            ContainerID: r.ContainerID,
            Err:         fmt.Errorf("%w: %v", ErrContainerUnhealthy, err),
            Recoverable: true,
        }
    }

    r.Status = RunnerStatusReady
    return nil
}

// Run에 에러 처리 적용
func (r *Runner) Run(ctx context.Context, req *RunRequest) (*RunResult, error) {
    if r.Status != RunnerStatusReady {
        return nil, NewRunnerError("Run", r.ID,
            fmt.Errorf("%w (current: %s)", ErrRunnerNotReady, r.Status))
    }

    r.Status = RunnerStatusRunning
    defer func() {
        if r.Status == RunnerStatusRunning {
            r.Status = RunnerStatusReady
        }
    }()

    // API 호출 with 타임아웃
    result, err := r.runWithTimeout(ctx, req)
    if err != nil {
        if errors.Is(err, context.DeadlineExceeded) {
            return nil, NewRunnerError("Run", r.ID, ErrAPITimeout)
        }
        return nil, NewRunnerError("Run", r.ID, err)
    }

    return result, nil
}
```

---

## 3. FR-010: 로깅 및 모니터링

### 3.1 요구사항 요약

Container 실행 상태를 로깅하고 모니터링할 수 있어야 합니다.

### 3.2 메트릭 수집

```go
// internal/runner/metrics.go

package taskrunner

import (
    "sync"
    "sync/atomic"
    "time"
)

// Metrics는 Runner 메트릭을 수집합니다.
type Metrics struct {
    // Container 메트릭
    ContainersCreated   int64
    ContainersStarted   int64
    ContainersStopped   int64
    ContainersFailed    int64

    // Task 메트릭
    TasksExecuted       int64
    TasksSucceeded      int64
    TasksFailed         int64

    // 타이밍 메트릭
    TotalExecutionTime  int64 // 나노초
    ContainerStartTime  int64 // 나노초 (평균 계산용 합계)
    ContainerStartCount int64

    // 에러 메트릭
    ErrorsTotal         int64
    ErrorsRecovered     int64
    RetriesTotal        int64

    mu sync.RWMutex
}

// GlobalMetrics는 전역 메트릭 인스턴스입니다.
var GlobalMetrics = &Metrics{}

// IncrementContainersCreated는 생성된 Container 수를 증가시킵니다.
func (m *Metrics) IncrementContainersCreated() {
    atomic.AddInt64(&m.ContainersCreated, 1)
}

// IncrementContainersStarted는 시작된 Container 수를 증가시킵니다.
func (m *Metrics) IncrementContainersStarted() {
    atomic.AddInt64(&m.ContainersStarted, 1)
}

// IncrementContainersStopped는 중지된 Container 수를 증가시킵니다.
func (m *Metrics) IncrementContainersStopped() {
    atomic.AddInt64(&m.ContainersStopped, 1)
}

// IncrementContainersFailed는 실패한 Container 수를 증가시킵니다.
func (m *Metrics) IncrementContainersFailed() {
    atomic.AddInt64(&m.ContainersFailed, 1)
}

// RecordTaskExecution은 Task 실행을 기록합니다.
func (m *Metrics) RecordTaskExecution(success bool, duration time.Duration) {
    atomic.AddInt64(&m.TasksExecuted, 1)
    atomic.AddInt64(&m.TotalExecutionTime, int64(duration))

    if success {
        atomic.AddInt64(&m.TasksSucceeded, 1)
    } else {
        atomic.AddInt64(&m.TasksFailed, 1)
    }
}

// RecordContainerStartTime은 Container 시작 시간을 기록합니다.
func (m *Metrics) RecordContainerStartTime(duration time.Duration) {
    atomic.AddInt64(&m.ContainerStartTime, int64(duration))
    atomic.AddInt64(&m.ContainerStartCount, 1)
}

// RecordError는 에러를 기록합니다.
func (m *Metrics) RecordError(recovered bool) {
    atomic.AddInt64(&m.ErrorsTotal, 1)
    if recovered {
        atomic.AddInt64(&m.ErrorsRecovered, 1)
    }
}

// RecordRetry는 재시도를 기록합니다.
func (m *Metrics) RecordRetry() {
    atomic.AddInt64(&m.RetriesTotal, 1)
}

// GetSnapshot은 현재 메트릭 스냅샷을 반환합니다.
func (m *Metrics) GetSnapshot() MetricsSnapshot {
    return MetricsSnapshot{
        ContainersCreated:   atomic.LoadInt64(&m.ContainersCreated),
        ContainersStarted:   atomic.LoadInt64(&m.ContainersStarted),
        ContainersStopped:   atomic.LoadInt64(&m.ContainersStopped),
        ContainersFailed:    atomic.LoadInt64(&m.ContainersFailed),
        TasksExecuted:       atomic.LoadInt64(&m.TasksExecuted),
        TasksSucceeded:      atomic.LoadInt64(&m.TasksSucceeded),
        TasksFailed:         atomic.LoadInt64(&m.TasksFailed),
        AvgExecutionTimeMs:  m.calculateAvgExecutionTime(),
        AvgContainerStartMs: m.calculateAvgContainerStart(),
        ErrorsTotal:         atomic.LoadInt64(&m.ErrorsTotal),
        ErrorsRecovered:     atomic.LoadInt64(&m.ErrorsRecovered),
        RetriesTotal:        atomic.LoadInt64(&m.RetriesTotal),
    }
}

func (m *Metrics) calculateAvgExecutionTime() float64 {
    executed := atomic.LoadInt64(&m.TasksExecuted)
    if executed == 0 {
        return 0
    }
    totalNs := atomic.LoadInt64(&m.TotalExecutionTime)
    return float64(totalNs) / float64(executed) / 1e6 // 나노초 -> 밀리초
}

func (m *Metrics) calculateAvgContainerStart() float64 {
    count := atomic.LoadInt64(&m.ContainerStartCount)
    if count == 0 {
        return 0
    }
    totalNs := atomic.LoadInt64(&m.ContainerStartTime)
    return float64(totalNs) / float64(count) / 1e6
}

// MetricsSnapshot은 메트릭 스냅샷입니다.
type MetricsSnapshot struct {
    ContainersCreated   int64   `json:"containers_created"`
    ContainersStarted   int64   `json:"containers_started"`
    ContainersStopped   int64   `json:"containers_stopped"`
    ContainersFailed    int64   `json:"containers_failed"`
    TasksExecuted       int64   `json:"tasks_executed"`
    TasksSucceeded      int64   `json:"tasks_succeeded"`
    TasksFailed         int64   `json:"tasks_failed"`
    AvgExecutionTimeMs  float64 `json:"avg_execution_time_ms"`
    AvgContainerStartMs float64 `json:"avg_container_start_ms"`
    ErrorsTotal         int64   `json:"errors_total"`
    ErrorsRecovered     int64   `json:"errors_recovered"`
    RetriesTotal        int64   `json:"retries_total"`
}
```

### 3.3 구조화된 로깅

```go
// internal/runner/runner.go (로깅 강화)

func (r *Runner) Start(ctx context.Context) error {
    startTime := time.Now()

    r.logger.Info("Container 시작 중",
        zap.String("runner_id", r.ID),
        zap.String("agent_id", r.AgentInfo.AgentID),
        zap.String("image", r.config.ImageName),
    )

    // ... 기존 코드 ...

    duration := time.Since(startTime)
    GlobalMetrics.RecordContainerStartTime(duration)
    GlobalMetrics.IncrementContainersStarted()

    r.logger.Info("Container 시작 완료",
        zap.String("runner_id", r.ID),
        zap.String("container_id", r.ContainerID),
        zap.Int("host_port", r.HostPort),
        zap.Duration("duration", duration),
    )

    return nil
}

func (r *Runner) Run(ctx context.Context, req *RunRequest) (*RunResult, error) {
    startTime := time.Now()

    r.logger.Info("Task 실행 시작",
        zap.String("runner_id", r.ID),
        zap.String("task_id", req.TaskID),
        zap.String("model", req.Model),
        zap.Int("message_count", len(req.Messages)),
    )

    result, err := r.executeTask(ctx, req)

    duration := time.Since(startTime)
    success := err == nil && result != nil && result.Success
    GlobalMetrics.RecordTaskExecution(success, duration)

    if err != nil {
        GlobalMetrics.RecordError(false)
        r.logger.Error("Task 실행 실패",
            zap.String("runner_id", r.ID),
            zap.String("task_id", req.TaskID),
            zap.Duration("duration", duration),
            zap.Error(err),
        )
        return nil, err
    }

    r.logger.Info("Task 실행 완료",
        zap.String("runner_id", r.ID),
        zap.String("task_id", req.TaskID),
        zap.Bool("success", result.Success),
        zap.Duration("duration", duration),
        zap.Int("output_length", len(result.Output)),
    )

    return result, nil
}
```

### 3.4 Container 로그 수집

```go
// internal/runner/logs.go

package taskrunner

import (
    "bufio"
    "context"
    "io"

    "github.com/docker/docker/api/types/container"
    "go.uber.org/zap"
)

// ContainerLogCollector는 Container 로그를 수집합니다.
type ContainerLogCollector struct {
    dockerClient DockerClient
    logger       *zap.Logger
}

// NewContainerLogCollector는 새 로그 수집기를 생성합니다.
func NewContainerLogCollector(client DockerClient, logger *zap.Logger) *ContainerLogCollector {
    return &ContainerLogCollector{
        dockerClient: client,
        logger:       logger,
    }
}

// StreamLogs는 Container 로그를 스트리밍합니다.
func (c *ContainerLogCollector) StreamLogs(ctx context.Context, containerID string, handler func(line string)) error {
    reader, err := c.dockerClient.ContainerLogs(ctx, containerID, container.LogsOptions{
        ShowStdout: true,
        ShowStderr: true,
        Follow:     true,
        Timestamps: true,
    })
    if err != nil {
        return err
    }
    defer reader.Close()

    scanner := bufio.NewScanner(reader)
    for scanner.Scan() {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            handler(scanner.Text())
        }
    }

    return scanner.Err()
}

// GetLogs는 Container 로그를 반환합니다.
func (c *ContainerLogCollector) GetLogs(ctx context.Context, containerID string, tail int) ([]string, error) {
    tailStr := "all"
    if tail > 0 {
        tailStr = fmt.Sprintf("%d", tail)
    }

    reader, err := c.dockerClient.ContainerLogs(ctx, containerID, container.LogsOptions{
        ShowStdout: true,
        ShowStderr: true,
        Tail:       tailStr,
        Timestamps: true,
    })
    if err != nil {
        return nil, err
    }
    defer reader.Close()

    var logs []string
    scanner := bufio.NewScanner(reader)
    for scanner.Scan() {
        logs = append(logs, scanner.Text())
    }

    return logs, scanner.Err()
}
```

### 3.5 커밋 포인트

```
feat(runner): FR-009 에러 처리 및 복구 구현

- 에러 타입 정의 (RunnerError, ContainerError)
- RecoveryManager 재시도 메커니즘
- 복구 가능 에러 분류
- Runner에 에러 처리 적용

Refs: FR-009
```

```
feat(runner): FR-010 로깅 및 모니터링 구현

- Metrics 수집기 구현
- 구조화된 로깅 적용
- Container 로그 수집기
- 메트릭 스냅샷 API

Refs: FR-010
```

---

## 4. 구현 체크리스트

### 4.1 Phase 4b 구현 순서

| 순서 | 작업             | 파일                          | 커밋 메시지                          |
| ---- | ---------------- | ----------------------------- | ------------------------------------ |
| 1    | 에러 타입 정의   | `internal/runner/errors.go`   | `feat(runner): 에러 타입 정의`       |
| 2    | 복구 메커니즘    | `internal/runner/recovery.go` | `feat(runner): FR-009 복구 메커니즘` |
| 3    | Runner 에러 처리 | `internal/runner/runner.go`   | `refactor(runner): 에러 처리 적용`   |
| 4    | 메트릭 수집      | `internal/runner/metrics.go`  | `feat(runner): FR-010 메트릭 수집`   |
| 5    | 로그 수집기      | `internal/runner/logs.go`     | `feat(runner): Container 로그 수집`  |
| 6    | 테스트           | `internal/runner/*_test.go`   | `test(runner): Phase 4b 테스트`      |

### 4.2 테스트 전략

```go
// internal/runner/errors_test.go

func TestIsRecoverable(t *testing.T) {
    tests := []struct {
        name     string
        err      error
        expected bool
    }{
        {"APITimeout", ErrAPITimeout, true},
        {"APIConnectionFailed", ErrAPIConnectionFailed, true},
        {"ContainerNotFound", ErrContainerNotFound, false},
        {"MaxContainersReached", ErrMaxContainersReached, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := IsRecoverable(tt.err)
            assert.Equal(t, tt.expected, result)
        })
    }
}

// internal/runner/metrics_test.go

func TestMetrics_RecordTaskExecution(t *testing.T) {
    m := &Metrics{}

    m.RecordTaskExecution(true, 100*time.Millisecond)
    m.RecordTaskExecution(false, 200*time.Millisecond)

    snapshot := m.GetSnapshot()
    assert.Equal(t, int64(2), snapshot.TasksExecuted)
    assert.Equal(t, int64(1), snapshot.TasksSucceeded)
    assert.Equal(t, int64(1), snapshot.TasksFailed)
}
```

---

## 다음 단계

Phase 4 완료 후 [인덱스 문서](./runner-docker-implementation.md)를 참조하여 전체 구현을 통합합니다.
