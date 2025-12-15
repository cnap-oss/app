package taskrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// TaskRunner는 Task 실행을 위한 인터페이스입니다.
// 모든 TaskRunner 구현체는 비동기 실행을 지원해야 하며,
// 실행 결과는 생성 시 등록된 콜백을 통해 전달됩니다.
type TaskRunner interface {
	// Run은 주어진 요청으로 비동기 실행을 시작합니다.
	// 이 메서드는 즉시 반환되며, 실행은 별도의 고루틴에서 진행됩니다.
	// 실행 중 발생하는 이벤트와 최종 결과는 StatusCallback을 통해 전달됩니다.
	//
	// Parameters:
	//   - ctx: 실행 컨텍스트 (취소 시그널 전달용)
	//   - req: 실행 요청 (TaskID, Model, Messages 포함)
	//
	// Returns:
	//   - error: 실행 시작 실패 시 에러 (nil이면 성공적으로 시작됨)
	//
	// 주의: 이 메서드는 실행 시작만 담당하며, 실행 완료를 기다리지 않습니다.
	Run(ctx context.Context, req *RunRequest) error
}

// RunRequest는 TaskRunner 실행 요청입니다.
// 콜백은 Runner 생성 시 등록되므로 RunRequest에는 포함되지 않습니다.
type RunRequest struct {
	TaskID       string
	Model        string
	SystemPrompt string
	Messages     []ChatMessage
}

// AgentInfo는 에이전트 실행에 필요한 정보를 담는 구조체입니다.
type AgentInfo struct {
	AgentID       string
	Provider      string
	Model         string
	Prompt        string
	WorkspacePath string // 신규: Agent 작업 공간 경로
}

// StatusCallback은 Task 실행 중 상태 변경을 Controller에 알리기 위한 콜백 인터페이스입니다.
// Runner는 생성 시 이 콜백을 등록받아, 실행 중 발생하는 모든 이벤트를 이 인터페이스를 통해 전달합니다.
//
// 콜백 호출 순서:
//  1. OnStarted - 세션 생성 및 실행 시작
//  2. OnMessage - SSE 이벤트 수신 시 (여러 번 호출 가능)
//  3. OnComplete 또는 OnError - 실행 종료
type StatusCallback interface {
	// OnStarted는 Runner가 시작되고 OpenCode 세션이 생성될 때 호출됩니다.
	//
	// Parameters:
	//   - taskID: Task 식별자
	//   - sessionID: OpenCode 세션 ID (예: "ses_xxx")
	//
	// Returns:
	//   - error: 콜백 처리 실패 시 에러 (로깅용, Runner 실행에는 영향 없음)
	OnStarted(taskID string, sessionID string) error

	// OnMessage는 Runner가 SSE 이벤트를 수신할 때 호출됩니다.
	// 텍스트 스트리밍, 도구 호출, 상태 변경 등 다양한 이벤트를 실시간으로 전달합니다.
	//
	// Parameters:
	//   - taskID: Task 식별자
	//   - msg: RunnerMessage (Type 필드로 이벤트 종류 구분)
	//
	// Returns:
	//   - error: 콜백 처리 실패 시 에러 (로깅용, Runner 실행에는 영향 없음)
	//
	// 주요 메시지 타입:
	//   - MessageTypeText: 스트리밍 텍스트 청크
	//   - MessageTypeToolCall: 도구 호출 시작
	//   - MessageTypeToolResult: 도구 실행 결과
	//   - MessageTypeComplete: 메시지 완료
	OnMessage(taskID string, msg *RunnerMessage) error

	// OnComplete는 Task가 성공적으로 완료될 때 호출됩니다.
	//
	// Parameters:
	//   - taskID: Task 식별자
	//   - result: 실행 결과 (전체 출력 텍스트 포함)
	//
	// Returns:
	//   - error: 콜백 처리 실패 시 에러 (로깅용)
	OnComplete(taskID string, result *RunResult) error

	// OnError는 Task 실행 중 에러가 발생할 때 호출됩니다.
	//
	// Parameters:
	//   - taskID: Task 식별자
	//   - err: 발생한 에러
	//
	// Returns:
	//   - error: 콜백 처리 실패 시 에러 (로깅용)
	OnError(taskID string, err error) error
}

const defaultBaseURL = "https://opencode.ai/zen/v1"

// Runner 상태 상수
const (
	RunnerStatusPending  = "pending"
	RunnerStatusStarting = "starting"
	RunnerStatusReady    = "ready"
	RunnerStatusRunning  = "running"
	RunnerStatusStopping = "stopping"
	RunnerStatusStopped  = "stopped"
	RunnerStatusFailed   = "failed"
)

// Runner는 Docker Container 기반 TaskRunner 구현체입니다.
type Runner struct {
	// 식별 정보
	ID            string // Task ID (Runner 식별자)
	ContainerID   string // Docker Container ID
	ContainerName string // Docker Container 이름

	// 상태 정보
	Status string // Runner 상태

	// Agent 정보
	agentInfo AgentInfo

	// 네트워크 정보
	HostPort      int    // 호스트에 매핑된 포트
	ContainerPort int    // Container 내부 포트
	BaseURL       string // OpenCode Server URL (http://localhost:{HostPort})

	// 작업 공간
	WorkspacePath string // 마운트된 작업 공간 경로

	// 콜백 핸들러 (생성 시 등록)
	callback StatusCallback

	// 내부 의존성
	dockerClient DockerClient
	httpClient   *http.Client
	logger       *zap.Logger

	// 레거시 필드 (Phase 2 이후 제거 예정)
	apiKey  string
	baseURL string
}

// RunnerOption은 Runner 초기화 옵션을 설정하기 위한 함수 타입입니다.
type RunnerOption func(*Runner)

// WithDockerClient는 Runner가 사용할 DockerClient를 주입합니다(테스트용).
func WithDockerClient(client DockerClient) RunnerOption {
	return func(r *Runner) {
		r.dockerClient = client
	}
}

// WithHTTPClient는 Runner가 사용할 http.Client를 주입합니다(테스트용).
func WithHTTPClient(client *http.Client) RunnerOption {
	return func(r *Runner) {
		r.httpClient = client
	}
}

// WithBaseURL은 Runner가 요청할 기본 URL을 지정합니다(테스트용).
func WithBaseURL(url string) RunnerOption {
	return func(r *Runner) {
		r.baseURL = url
	}
}

// WithContainerPort는 Container 내부 포트를 지정합니다(테스트용).
func WithContainerPort(port int) RunnerOption {
	return func(r *Runner) {
		r.ContainerPort = port
	}
}

// WithWorkspacePath는 작업 공간 경로를 지정합니다(테스트용).
func WithWorkspacePath(path string) RunnerOption {
	return func(r *Runner) {
		r.WorkspacePath = path
	}
}

// OpenCodeRequest는 OpenCode Zen API 요청 바디입니다 (레거시).
type OpenCodeRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
}

// OpenCodeResponse는 OpenCode Zen API 응답 바디입니다.
type OpenCodeResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// NewRunner는 새로운 Container 기반 Runner를 생성합니다.
// callback은 생성자에서만 등록되며, nil이면 에러를 반환합니다.
// 이 함수는 Container를 생성하지 않고 Runner 구조체만 초기화합니다.
// Container 시작은 Start() 메서드로 별도로 수행해야 합니다.
func NewRunner(taskID string, agentInfo AgentInfo, callback StatusCallback, logger *zap.Logger, opts ...RunnerOption) (*Runner, error) {
	if callback == nil {
		return nil, fmt.Errorf("callback is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	// 기본 설정
	workspaceBaseDir := os.Getenv("RUNNER_WORKSPACE_DIR")
	if workspaceBaseDir == "" {
		workspaceBaseDir = "./data/workspace"
	}

	workspacePath := agentInfo.WorkspacePath
	if workspacePath == "" {
		workspacePath = fmt.Sprintf("%s/%s", workspaceBaseDir, agentInfo.AgentID)
	}

	// 상대 경로를 절대 경로로 변환 (Docker 볼륨 마운트 요구사항)
	absPath, err := filepath.Abs(workspacePath)
	if err != nil {
		return nil, fmt.Errorf("작업 공간 절대 경로 변환 실패: %w", err)
	}
	workspacePath = absPath

	r := &Runner{
		ID:            taskID,
		Status:        RunnerStatusPending,
		agentInfo:     agentInfo,
		callback:      callback,
		logger:        logger,
		httpClient:    &http.Client{Timeout: 120 * time.Second},
		ContainerPort: 3000,
		WorkspacePath: workspacePath,
		ContainerName: fmt.Sprintf("cnap-runner-%s", taskID),
		// 레거시 필드 (Phase 2 이후 제거)
		apiKey:  os.Getenv("OPEN_CODE_API_KEY"),
		baseURL: defaultBaseURL,
	}

	// 옵션 적용
	for _, opt := range opts {
		opt(r)
	}

	// DockerClient가 주입되지 않았으면 새로 생성
	if r.dockerClient == nil {
		client, err := NewDockerClient()
		if err != nil {
			return nil, fmt.Errorf("docker client 생성 실패: %w", err)
		}
		r.dockerClient = client
	}

	return r, nil
}

// Start는 Runner Container를 시작합니다.
func (r *Runner) Start(ctx context.Context) error {
	r.logger.Info("Starting runner container",
		zap.String("runner_id", r.ID),
		zap.String("container_name", r.ContainerName),
	)

	r.Status = RunnerStatusStarting

	// 작업 공간 디렉토리 생성
	if err := os.MkdirAll(r.WorkspacePath, 0755); err != nil {
		r.Status = RunnerStatusFailed
		return fmt.Errorf("작업 공간 생성 실패: %w", err)
	}

	// 환경 변수 구성
	env := r.buildEnvironmentVariables()

	// Docker 이미지 이름
	imageName := os.Getenv("RUNNER_IMAGE")
	if imageName == "" {
		imageName = "cnap-runner:latest"
	}

	// Container 생성
	containerID, err := r.dockerClient.CreateContainer(ctx, ContainerConfig{
		Image: imageName,
		Name:  r.ContainerName,
		Env:   env,
		Mounts: []MountConfig{
			{
				Source: r.WorkspacePath,
				Target: "/workspace",
			},
		},
		PortBinding: &PortConfig{
			HostPort:      "0", // 동적 포트 할당
			ContainerPort: fmt.Sprintf("%d", r.ContainerPort),
		},
		Labels: map[string]string{
			"cnap.runner.id":      r.ID,
			"cnap.agent.id":       r.agentInfo.AgentID,
			"cnap.runner.managed": "true",
		},
	})
	if err != nil {
		r.Status = RunnerStatusFailed
		return fmt.Errorf("container 생성 실패: %w", err)
	}
	r.ContainerID = containerID

	// Container 시작
	if err := r.dockerClient.StartContainer(ctx, r.ContainerID); err != nil {
		r.Status = RunnerStatusFailed
		// 생성된 Container 정리
		_ = r.dockerClient.RemoveContainer(ctx, r.ContainerID)
		return fmt.Errorf("container 시작 실패: %w", err)
	}

	// Container 정보 조회하여 포트 매핑 확인
	info, err := r.dockerClient.ContainerInspect(ctx, r.ContainerID)
	if err != nil {
		r.Status = RunnerStatusFailed
		_ = r.Stop(ctx)
		return fmt.Errorf("container 조회 실패: %w", err)
	}

	// 포트 매핑 확인
	portKey := fmt.Sprintf("%d/tcp", r.ContainerPort)
	hostPort, ok := info.Ports[portKey]
	if !ok {
		r.Status = RunnerStatusFailed
		_ = r.Stop(ctx)
		return fmt.Errorf("포트 매핑을 찾을 수 없음: %d", r.ContainerPort)
	}

	var port int
	_, err = fmt.Sscanf(hostPort, "%d", &port)
	if err != nil {
		r.Status = RunnerStatusFailed
		_ = r.Stop(ctx)
		return fmt.Errorf("포트 파싱 실패: %w", err)
	}
	r.HostPort = port
	r.BaseURL = fmt.Sprintf("http://localhost:%d", port)

	// Health check 대기
	if err := r.waitForHealthy(ctx); err != nil {
		r.Status = RunnerStatusFailed
		_ = r.Stop(ctx)
		return fmt.Errorf("health check 실패: %w", err)
	}

	r.Status = RunnerStatusReady
	r.logger.Info("Runner container started successfully",
		zap.String("runner_id", r.ID),
		zap.String("container_id", r.ContainerID),
		zap.Int("host_port", r.HostPort),
	)

	return nil
}

// Stop은 Runner Container를 중지하고 제거합니다.
func (r *Runner) Stop(ctx context.Context) error {
	r.logger.Info("Stopping runner container",
		zap.String("runner_id", r.ID),
		zap.String("container_id", r.ContainerID),
	)

	r.Status = RunnerStatusStopping

	if r.ContainerID == "" {
		r.Status = RunnerStatusStopped
		return nil
	}

	// Container 중지
	if err := r.dockerClient.StopContainer(ctx, r.ContainerID, 10); err != nil {
		r.logger.Warn("Container 중지 중 오류",
			zap.String("container_id", r.ContainerID),
			zap.Error(err),
		)
	}

	// Container 삭제
	if err := r.dockerClient.RemoveContainer(ctx, r.ContainerID); err != nil {
		r.logger.Warn("Container 삭제 중 오류",
			zap.String("container_id", r.ContainerID),
			zap.Error(err),
		)
	}

	r.Status = RunnerStatusStopped
	r.ContainerID = ""

	return nil
}

// buildEnvironmentVariables는 Container에 전달할 환경 변수를 구성합니다.
func (r *Runner) buildEnvironmentVariables() []string {
	env := []string{
		fmt.Sprintf("OPENCODE_MODEL=%s", r.agentInfo.Model),
	}

	// API 키 전달 (환경 변수에서 읽기)
	if apiKey := os.Getenv("OPENCODE_API_KEY"); apiKey != "" {
		env = append(env, fmt.Sprintf("OPENCODE_API_KEY=%s", apiKey))
	}
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		env = append(env, fmt.Sprintf("ANTHROPIC_API_KEY=%s", apiKey))
	}
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		env = append(env, fmt.Sprintf("OPENAI_API_KEY=%s", apiKey))
	}
	// 레거시 지원
	if apiKey := os.Getenv("OPEN_CODE_API_KEY"); apiKey != "" {
		env = append(env, fmt.Sprintf("OPENCODE_API_KEY=%s", apiKey))
	}

	return env
}

// waitForHealthy는 Container가 준비될 때까지 대기합니다.
func (r *Runner) waitForHealthy(ctx context.Context) error {
	healthURL := fmt.Sprintf("%s/health", r.BaseURL)
	timeout := 60 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := r.httpClient.Get(healthURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("health check 타임아웃")
}

// ensure Runner implements TaskRunner interface
var _ TaskRunner = (*Runner)(nil)

// Run은 TaskRunner 인터페이스를 구현하며, 비동기로 Task 실행을 시작합니다.
//
// 이 메서드는 즉시 반환되며, 실제 실행은 별도의 고루틴에서 진행됩니다.
// 실행 중 발생하는 모든 이벤트는 생성 시 등록된 StatusCallback을 통해 전달됩니다.
//
// 실행 흐름:
//  1. Runner 상태 및 요청 검증
//  2. 고루틴 시작 (즉시 반환)
//  3. [고루틴 내부] OpenCode API 세션 생성
//  4. [고루틴 내부] SSE 이벤트 구독 및 프롬프트 전송
//  5. [고루틴 내부] 이벤트 수신 및 콜백 호출
//  6. [고루틴 내부] 완료 시 OnComplete 또는 OnError 호출
//
// Parameters:
//   - ctx: 실행 컨텍스트 (고루틴에 전달되어 취소 시그널로 사용)
//   - req: 실행 요청 (TaskID, Model, SystemPrompt, Messages 포함)
//
// Returns:
//   - error: 실행 시작 실패 시 에러 (Runner 미준비, nil 요청 등)
//
// 주의사항:
//   - 콜백은 반드시 NewRunner() 생성자에서 등록되어야 함
//   - Runner 상태가 RunnerStatusReady가 아니면 실패
//   - 에러 반환은 실행 시작 실패를 의미하며, 실행 중 에러는 OnError 콜백으로 전달됨
func (r *Runner) Run(ctx context.Context, req *RunRequest) error {
	if r.Status != RunnerStatusReady {
		return fmt.Errorf("runner가 준비되지 않음 (status: %s)", r.Status)
	}

	// 요청 검증
	if req == nil {
		return fmt.Errorf("request is nil")
	}

	r.logger.Info("Runner starting async execution",
		zap.String("runner_id", r.ID),
		zap.String("task_id", req.TaskID),
		zap.Int("message_count", len(req.Messages)),
	)

	// 비동기 실행 시작
	go func() {
		result, err := r.runInternal(ctx, req)
		if err != nil {
			_ = r.callback.OnError(req.TaskID, err)
			return
		}
		_ = r.callback.OnComplete(req.TaskID, result)
	}()

	return nil
}

// runInternal은 실제 실행 로직을 담당합니다 (기존 runWithStreaming 로직 기반).
func (r *Runner) runInternal(ctx context.Context, req *RunRequest) (*RunResult, error) {
	r.Status = RunnerStatusRunning
	defer func() {
		r.Status = RunnerStatusReady
	}()

	r.logger.Info("Runner executing task",
		zap.String("runner_id", r.ID),
		zap.String("task_id", req.TaskID),
		zap.Int("message_count", len(req.Messages)),
	)

	// OpenCode API 클라이언트 생성
	apiClient := NewOpenCodeClient(
		r.BaseURL,
		WithOpenCodeHTTPClient(r.httpClient),
		WithOpenCodeLogger(r.logger),
	)

	// 시스템 프롬프트와 메시지 결합
	messages := r.buildMessages(req)

	// 스트리밍 모드로 실행 (콜백 사용)
	return r.executeWithStreaming(ctx, apiClient, req, messages)
}

// executeWithStreaming는 스트리밍 모드로 실행합니다.
func (r *Runner) executeWithStreaming(ctx context.Context, client *OpenCodeClient, req *RunRequest, messages []ChatMessage) (*RunResult, error) {
	// 1. 세션 생성
	session, err := client.CreateSession(ctx, &CreateSessionRequest{
		Title: req.TaskID,
	})
	if err != nil {
		return nil, fmt.Errorf("세션 생성 실패: %w", err)
	}

	defer func() {
		if err := client.DeleteSession(context.Background(), session.ID); err != nil {
			r.logger.Warn("세션 삭제 실패",
				zap.String("session_id", session.ID),
				zap.Error(err),
			)
		}
	}()

	// 세션 생성 알림
	if r.callback != nil {
		if err := r.callback.OnStarted(req.TaskID, session.ID); err != nil {
			r.logger.Warn("OnStarted 콜백 실패", zap.Error(err))
		}
	}

	// 2. 이벤트 구독 시작 (비동기)
	var fullContent strings.Builder
	eventCtx, cancelEvent := context.WithCancel(ctx)
	defer cancelEvent()

	messageCompleted := make(chan bool, 1)
	eventDone := make(chan error, 1)
	go func() {
		err := client.SubscribeEvents(eventCtx, func(event *Event) error {
			r.logger.Debug("이벤트 수신",
				zap.String("task_id", req.TaskID),
				zap.String("event_type", event.Type),
			)

			// SSE 이벤트를 RunnerMessage로 변환
			msg := r.convertEventToMessage(event, session.ID)
			if msg == nil {
				return nil // 지원하지 않는 이벤트 타입
			}

			// 텍스트 이벤트인 경우 전체 컨텐츠에 추가
			if msg.IsText() && msg.Content != "" {
				fullContent.WriteString(msg.Content)
			}

			// 콜백으로 메시지 전달
			if r.callback != nil {
				if err := r.callback.OnMessage(req.TaskID, msg); err != nil {
					r.logger.Warn("콜백 전달 실패",
						zap.String("task_id", req.TaskID),
						zap.String("msg_type", string(msg.Type)),
						zap.Error(err),
					)
				}
			}

			// 완료 이벤트인 경우 채널로 알림
			if msg.Type == MessageTypeComplete {
				select {
				case messageCompleted <- true:
				default:
				}
			}

			return nil
		})
		eventDone <- err
	}()

	// 잠시 대기 (이벤트 구독 준비)
	time.Sleep(500 * time.Millisecond)

	// 3. 모델 정보 파싱
	providerID, modelID := parseModel(req.Model)

	// 4. 메시지를 프롬프트 파트로 변환
	parts := make([]PromptPart, 0, len(messages))
	for _, msg := range messages {
		parts = append(parts, TextPartInput{
			Type: "text",
			Text: msg.Content,
		})
	}

	// 5. 프롬프트 전송
	promptReq := &PromptRequest{
		Model: &PromptModel{
			ProviderID: providerID,
			ModelID:    modelID,
		},
		Parts: parts,
	}

	_, err = client.Prompt(ctx, session.ID, promptReq)
	if err != nil {
		cancelEvent()
		return nil, fmt.Errorf("프롬프트 전송 실패: %w", err)
	}

	// 6. 메시지 완료 또는 타임아웃 대기
	eventWaitTimeout := 30 * time.Second
	select {
	case <-messageCompleted:
		r.logger.Info("메시지 완료 이벤트 수신",
			zap.String("task_id", req.TaskID),
			zap.Int("content_length", fullContent.Len()),
		)
		cancelEvent()
	case err := <-eventDone:
		if err != nil && err != context.Canceled {
			r.logger.Warn("이벤트 스트림 에러",
				zap.String("task_id", req.TaskID),
				zap.Error(err),
			)
			if fullContent.Len() == 0 {
				return nil, fmt.Errorf("이벤트 스트림 에러: %w", err)
			}
		}
		cancelEvent()
	case <-time.After(eventWaitTimeout):
		r.logger.Info("이벤트 대기 타임아웃, 받은 컨텐츠로 진행",
			zap.String("task_id", req.TaskID),
			zap.Int("content_length", fullContent.Len()),
		)
		cancelEvent()
	case <-ctx.Done():
		r.logger.Warn("컨텍스트 취소됨",
			zap.String("task_id", req.TaskID),
			zap.Error(ctx.Err()),
		)
		cancelEvent()
		if fullContent.Len() == 0 {
			return nil, ctx.Err()
		}
	}

	output := fullContent.String()

	if output == "" {
		r.logger.Warn("빈 응답 수신", zap.String("task_id", req.TaskID))
	}

	result := &RunResult{
		Agent:   req.Model,
		Name:    req.TaskID,
		Success: true,
		Output:  output,
		Error:   nil,
	}

	return result, nil
}

// convertEventToMessage는 OpenCode SSE Event를 RunnerMessage로 변환합니다.
//
// 이 메서드는 OpenCode API에서 수신한 원시 SSE 이벤트를 타입 안전한 RunnerMessage 구조체로 변환합니다.
// 지원하지 않는 이벤트 타입이나 파트 타입은 nil을 반환하여 무시됩니다.
//
// 지원하는 SSE 이벤트:
//   - "message.part.updated": 메시지 파트 업데이트 (text, reasoning, tool)
//   - "message.completed": 메시지 완료
//   - "session.aborted": 세션 중단
//
// 변환 규칙:
//   - text 파트 → MessageTypeText (Content 필드에 텍스트)
//   - reasoning 파트 → MessageTypeReasoning (Content 필드에 텍스트)
//   - tool 파트 (pending/running) → MessageTypeToolCall (ToolCall 필드)
//   - tool 파트 (completed/error) → MessageTypeToolResult (ToolResult 필드)
//   - message.completed → MessageTypeComplete
//   - session.aborted → MessageTypeSessionAborted
//
// Parameters:
//   - event: OpenCode SSE 이벤트 (Type과 Properties 포함)
//   - sessionID: 현재 세션 ID
//
// Returns:
//   - *RunnerMessage: 변환된 메시지 (nil이면 지원하지 않는 이벤트)
func (r *Runner) convertEventToMessage(event *Event, sessionID string) *RunnerMessage {
	msg := &RunnerMessage{
		SessionID: sessionID,
		Timestamp: time.Now(),
		RawEvent:  event,
	}

	r.logger.Info("opencode message received",
		zap.String("event_type", event.Type),
		zap.Any("properties", event.Properties),
	)

	switch event.Type {
	case "message.part.updated":
		// 파트 정보 추출
		if props, ok := event.Properties["part"].(map[string]interface{}); ok {
			partType, _ := props["type"].(string)
			partID, _ := props["id"].(string)
			messageID, _ := event.Properties["messageID"].(string)

			msg.PartID = partID
			msg.MessageID = messageID

			switch partType {
			case "text":
				msg.Type = MessageTypeText
				msg.Content, _ = props["text"].(string)
			case "reasoning":
				msg.Type = MessageTypeReasoning
				msg.Content, _ = props["text"].(string)
			case "tool":
				// 도구 상태 확인
				if state, ok := props["state"].(map[string]interface{}); ok {
					status, _ := state["status"].(string)
					callID, _ := props["callID"].(string)
					tool, _ := props["tool"].(string)

					if status == "running" || status == "pending" {
						msg.Type = MessageTypeToolCall
						msg.ToolCall = &ToolCallInfo{
							ToolID:   callID,
							ToolName: tool,
						}
						if input, ok := state["input"].(map[string]interface{}); ok {
							msg.ToolCall.Arguments = input
						}
					} else {
						msg.Type = MessageTypeToolResult
						msg.ToolResult = &ToolResultInfo{
							ToolID:   callID,
							ToolName: tool,
							Result:   "",
							IsError:  status == "error",
						}
						if output, ok := state["output"].(string); ok {
							msg.ToolResult.Result = output
						}
					}
				}
			default:
				return nil // 지원하지 않는 파트 타입
			}
		}

	case "message.completed":
		msg.Type = MessageTypeComplete
		if messageID, ok := event.Properties["messageID"].(string); ok {
			msg.MessageID = messageID
		}

	case "session.aborted":
		msg.Type = MessageTypeSessionAborted

	default:
		return nil // 지원하지 않는 이벤트 타입
	}

	return msg
}

// buildMessages는 요청 메시지를 구성합니다.
func (r *Runner) buildMessages(req *RunRequest) []ChatMessage {
	messages := make([]ChatMessage, 0, len(req.Messages)+1)

	// 시스템 프롬프트 추가
	if req.SystemPrompt != "" {
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}

	// 기존 메시지 추가
	messages = append(messages, req.Messages...)

	return messages
}

// parseModel은 "provider/model" 형식의 모델 문자열을 파싱합니다.
func parseModel(model string) (providerID, modelID string) {
	parts := strings.SplitN(model, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	// 기본값
	return "anthropic", model
}

// Request는 메시지 배열을 OpenCode AI API 엔드포인트로 보내고 결과를 반환합니다.
func (r *Runner) Request(ctx context.Context, model, name string, messages []ChatMessage) (*RunResult, error) {
	// 마지막 user 메시지를 로깅용 preview로 사용
	var messagePreview string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			messagePreview = messages[i].Content
			break
		}
	}
	if len(messagePreview) > 200 {
		messagePreview = messagePreview[:200] + "..."
	}

	r.logger.Info("Sending request to OpenCode AI API",
		zap.String("model", model),
		zap.String("name", name),
		zap.String("message_preview", messagePreview),
		zap.Int("message_count", len(messages)),
	)

	// OpenCode endpoint - baseURL 사용 (테스트 시 mock server 사용 가능)
	endpoint := r.baseURL + "/chat/completions"

	// OpenCode API 호출 (OpenAI 호환 포맷)
	return r.callOpenCodeAPI(ctx, endpoint, r.apiKey, model, name, messages)
}

// callOpenCodeAPI는 OpenCode API를 호출합니다 (OpenAI 호환 포맷).
func (r *Runner) callOpenCodeAPI(ctx context.Context, endpoint, apiKey, model, name string, messages []ChatMessage) (*RunResult, error) {
	// 요청 본문 구성 - 전달받은 전체 메시지 배열 사용
	reqBody := OpenCodeRequest{
		Model:    model,
		Messages: messages,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("요청 바디 직렬화 실패: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("요청 생성 실패: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := r.httpClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API 요청 실패: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("응답 읽기 실패: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	r.logger.Debug("Response received",
		zap.String("content_type", contentType),
		zap.String("body_preview", summarizeBody(bodyBytes)),
	)

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("API 응답 오류: %s - %s", resp.Status, summarizeBody(bodyBytes))
	}

	var apiResp OpenCodeResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w\n\n[응답 원문]\n%s", err, string(bodyBytes))
	}

	// 에러 필드 처리
	if apiResp.Error != nil {
		return nil, fmt.Errorf("API 에러: %s - %s", apiResp.Error.Type, apiResp.Error.Message)
	}

	output := "(empty result)"
	if len(apiResp.Choices) > 0 {
		output = apiResp.Choices[0].Message.Content
	}

	r.logger.Info("AI API 응답 수신 완료",
		zap.String("output_preview", summarizeBody([]byte(output))),
	)

	return &RunResult{
		Agent:   model,
		Name:    name,
		Success: true,
		Output:  output,
		Error:   nil,
	}, nil
}

// RunResult는 에이전트 실행 결과를 나타냅니다.
type RunResult struct {
	Agent   string
	Name    string
	Success bool
	Output  string
	Error   error
}

func summarizeBody(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "<empty>"
	}
	if len(trimmed) > 200 {
		return trimmed[:200] + "..."
	}
	return trimmed
}
