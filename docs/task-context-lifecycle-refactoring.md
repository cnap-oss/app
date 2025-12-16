# TaskContext 생명주기 리팩토링 계획

## 배경

### 현재 문제점

1. **TaskContext가 `ExecuteTask()` 호출마다 새로 생성**됨 (`executor.go:44-47`)
2. **5분 타임아웃**이 설정되어 있음 (`context.WithTimeout(context.Background(), 5*time.Minute)`)
3. **Runner의 Container는 `Start()` 시 생성되어 `Stop()` 시까지 유지**됨 (멀티턴 재사용 가능)
4. 이 **생명주기 불일치**로 인해 멀티턴 대화에서 `context canceled` 에러 발생

### 생명주기 비교

| 구분          | Runner                         | TaskContext (현재)       |
| ------------- | ------------------------------ | ------------------------ |
| **생성 시점** | CreateRunner/StartRunner       | ExecuteTask 호출마다     |
| **생명주기**  | Task 단위 (멀티턴 재사용 가능) | 각 턴마다 새로 생성      |
| **타임아웃**  | 없음 (SSE는 Background)        | 5분                      |
| **정리 시점** | DeleteRunner 명시 호출         | executeTask 완료 시 자동 |

## 목표

- TaskContext의 생명주기를 Runner의 Container lifecycle과 일치시킴
- 5분 타임아웃을 제거하여 멀티턴 대화에서 context canceled 에러 방지
- ExecuteTask에서 기존 TaskContext를 재사용하도록 수정

## 변경 사항

### 1. TaskContext 생성 위치 변경

**변경 전 (`executor.go:44-47`):**

```go
// ExecuteTask 메서드 내부에서 매번 생성
taskCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
c.taskContexts[taskID] = &TaskContext{ctx: taskCtx, cancel: cancel}
```

**변경 후:**

- Runner 생성 시점에 TaskContext도 함께 생성
- `context.WithCancel(context.Background())` 사용 (타임아웃 없음)
- 동일 taskID로 `ExecuteTask` 호출 시 기존 TaskContext 재사용

### 2. 5분 타임아웃 제거

**변경 전:**

```go
context.WithTimeout(context.Background(), 5*time.Minute)
```

**변경 후:**

```go
context.WithCancel(context.Background())
```

- Container가 살아있는 동안 TaskContext도 유지
- 타임아웃으로 인한 강제 취소 방지

### 3. ExecuteTask 로직 수정

**변경 전:**

```go
func (c *Controller) ExecuteTask(taskID string, prompt string, requestID string) error {
    taskCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    c.taskContexts[taskID] = &TaskContext{ctx: taskCtx, cancel: cancel}
    // ...
}
```

**변경 후:**

```go
func (c *Controller) ExecuteTask(taskID string, prompt string, requestID string) error {
    // 기존 TaskContext 조회 (Runner 생성 시 함께 생성됨)
    c.mu.RLock()
    taskCtx, exists := c.taskContexts[taskID]
    c.mu.RUnlock()

    if !exists {
        // TaskContext가 없으면 Runner 생성과 함께 생성
        // ...
    }
    // 기존 context 재사용
    // ...
}
```

### 4. TaskContext 정리 시점 변경

**변경 전 (`executor.go:104-111`):**

- `executeTask()` 완료 시 defer에서 정리

**변경 후:**

- Runner 삭제(`DeleteRunner`) 시에만 TaskContext 정리
- `executeTask()` 완료 시에는 TaskContext 유지

## 변경 파일 목록

1. `internal/controller/executor.go` - TaskContext 생성/정리 로직 수정
2. `internal/controller/types.go` - TaskContext 구조체 확인 (필요시)
3. `internal/controller/controller.go` - 필요시 수정
4. `internal/controller/multiturn_test.go` - 테스트 수정

## 작업 순서

1. [x] 계획 수립 및 사용자 확인
2. [ ] TaskContext 생성 위치를 Runner 생성 시점으로 이동
3. [ ] 5분 타임아웃 제거 (context.Background() 기반으로 변경)
4. [ ] ExecuteTask에서 기존 TaskContext 재사용하도록 수정
5. [ ] Runner 삭제 시 TaskContext도 함께 정리하도록 수정
6. [ ] 관련 테스트 코드 수정
7. [ ] 멀티턴 테스트로 검증

## 예상 효과

- 멀티턴 대화에서 `prompt_async` 호출 시 context canceled 에러 해결
- TaskContext와 Runner의 생명주기 일치
- Container가 살아있는 동안 안정적인 멀티턴 대화 지원

## 리스크 및 고려사항

1. **무한 대기 가능성**: 타임아웃 제거로 인해 무한히 대기하는 상황이 발생할 수 있음
   - 대응: Runner의 Container가 종료되면 자동으로 TaskContext도 정리됨
2. **리소스 누수**: TaskContext가 정리되지 않는 경우
   - 대응: DeleteRunner 호출 시 반드시 TaskContext 정리
3. **기존 테스트 실패 가능성**: 타임아웃 기반 테스트가 있을 수 있음
   - 대응: 테스트 코드 수정
