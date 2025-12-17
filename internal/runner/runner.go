package taskrunner

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cnap-oss/app/internal/runner/docker"
	"github.com/cnap-oss/app/internal/runner/opencode"
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
	Messages     []opencode.ChatMessage
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

	// OnEvent는 Runner가 SSE 이벤트를 수신할 때 호출됩니다.
	// 텍스트 스트리밍, 도구 호출, 상태 변경 등 다양한 이벤트를 실시간으로 전달합니다.
	//
	// Parameters:
	//   - taskID: Task 식별자
	//   - event: Event (원시 SSE 이벤트)
	//
	// Returns:
	//   - error: 콜백 처리 실패 시 에러 (로깅용, Runner 실행에는 영향 없음)
	OnEvent(taskID string, event *opencode.Event) error

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

	// 세션 관리 (Runner 생명 주기 동안 유지)
	apiClient   *opencode.OpenCodeClient // OpenCode API 클라이언트
	session     *opencode.Session        // OpenCode 세션
	sessionID   string                   // 세션 ID
	eventCtx    context.Context          // 이벤트 스트림 컨텍스트
	eventCancel context.CancelFunc       // 이벤트 스트림 취소 함수
	eventDone   chan error               // 이벤트 스트림 완료 채널
	fullContent *strings.Builder         // 누적 컨텐츠

	// 콜백 핸들러 (생성 시 등록)
	callback StatusCallback

	// 내부 의존성
	dockerClient docker.DockerClient
	httpClient   *http.Client
	logger       *zap.Logger

	// 레거시 필드 (Phase 2 이후 제거 예정)
	apiKey  string
	baseURL string
}

// RunnerOption은 Runner 초기화 옵션을 설정하기 위한 함수 타입입니다.
type RunnerOption func(*Runner)

// WithDockerClient는 Runner가 사용할 DockerClient를 주입합니다(테스트용).
func WithDockerClient(client docker.DockerClient) RunnerOption {
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
	Model    string                 `json:"model"`
	Messages []opencode.ChatMessage `json:"messages"`
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
	workspaceBaseDir := os.Getenv("CNAP_RUNNER_WORKSPACE_DIR")
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
		client, err := docker.NewClient()
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

	// Docker 이미지 이름 - CNAP_ENV에 따라 분기
	imageName := os.Getenv("CNAP_RUNNER_IMAGE")
	if imageName == "" {
		// 환경별 기본 이미지 설정 (기본값: production)
		env := os.Getenv("CNAP_ENV")
		if env == "development" {
			imageName = "cnap-runner:latest"
		} else {
			// production 또는 CNAP_ENV 미설정 시 ghcr.io 이미지 사용
			imageName = "ghcr.io/cnap-oss/cnap-runner:latest"
		}
	}

	// Container 생성
	containerID, err := r.dockerClient.CreateContainer(ctx, docker.ContainerConfig{
		Image: imageName,
		Name:  r.ContainerName,
		Env:   env,
		Mounts: []docker.MountConfig{
			{
				Source: r.WorkspacePath,
				Target: "/workspace",
			},
		},
		PortBinding: &docker.PortConfig{
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

	// OpenCode API 클라이언트 생성
	r.apiClient = opencode.NewClient(
		r.BaseURL,
		opencode.WithHTTPClient(r.httpClient),
		opencode.WithLogger(r.logger),
	)

	// 세션 생성
	session, err := r.apiClient.CreateSession(ctx, &opencode.CreateSessionRequest{
		Title: r.ID,
	})
	if err != nil {
		r.Status = RunnerStatusFailed
		_ = r.Stop(ctx)
		return fmt.Errorf("세션 생성 실패: %w", err)
	}
	r.session = session
	r.sessionID = session.ID

	r.logger.Info("OpenCode 세션 생성됨",
		zap.String("runner_id", r.ID),
		zap.String("session_id", r.sessionID),
	)

	// 세션 생성 콜백 호출
	if r.callback != nil {
		if err := r.callback.OnStarted(r.ID, r.sessionID); err != nil {
			r.logger.Warn("OnStarted 콜백 실패", zap.Error(err))
		}
	}

	// 누적 컨텐츠 초기화
	r.fullContent = &strings.Builder{}

	// SSE 이벤트 구독 시작 (백그라운드에서 실행)
	r.eventCtx, r.eventCancel = context.WithCancel(context.Background())
	r.eventDone = make(chan error, 1)

	go func() {
		err := r.apiClient.SubscribeEvents(r.eventCtx, r.handleEvent)
		select {
		case r.eventDone <- err:
		default:
		}
	}()

	// 이벤트 구독이 준비될 때까지 잠시 대기
	time.Sleep(500 * time.Millisecond)

	r.Status = RunnerStatusReady
	r.logger.Info("Runner container started successfully",
		zap.String("runner_id", r.ID),
		zap.String("container_id", r.ContainerID),
		zap.String("session_id", r.sessionID),
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

	// 정상 종료 상태 확인 (Ready 또는 Running 상태에서 Stop 호출 시)
	shouldComplete := r.Status == RunnerStatusReady || r.Status == RunnerStatusRunning

	r.Status = RunnerStatusStopping

	// 정상 종료 시 OnComplete 콜백 호출
	if shouldComplete && r.callback != nil {
		r.logger.Info("Runner stopping normally, calling OnComplete",
			zap.String("runner_id", r.ID),
		)

		output := ""
		if r.fullContent != nil {
			output = r.fullContent.String()
		}

		result := &RunResult{
			Success: true,
			Output:  output,
		}

		if err := r.callback.OnComplete(r.ID, result); err != nil {
			r.logger.Warn("OnComplete 콜백 실패", zap.Error(err))
		}
	}

	// 이벤트 스트림 중지
	if r.eventCancel != nil {
		r.eventCancel()
		r.eventCancel = nil
	}

	// 세션 삭제
	if r.sessionID != "" && r.apiClient != nil {
		if err := r.apiClient.DeleteSession(context.Background(), r.sessionID); err != nil {
			r.logger.Warn("세션 삭제 실패",
				zap.String("session_id", r.sessionID),
				zap.Error(err),
			)
		}
		r.sessionID = ""
		r.session = nil
	}

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

// handleEvent는 SSE 이벤트를 처리하는 핸들러입니다.
// 이 메서드는 백그라운드 고루틴에서 실행되며, 모든 이벤트를 수신하여 처리합니다.
func (r *Runner) handleEvent(event *opencode.Event) error {
	r.logger.Debug("이벤트 수신",
		zap.String("runner_id", r.ID),
		zap.String("event_type", event.Type),
		zap.Any("properties", event.Properties),
	)

	// 콜백으로 이벤트 전달
	if r.callback != nil {
		if err := r.callback.OnEvent(r.ID, event); err != nil {
			r.logger.Warn("콜백 전달 실패",
				zap.String("runner_id", r.ID),
				zap.String("event_type", event.Type),
				zap.Error(err),
			)
		}
	}

	return nil
}

// buildEnvironmentVariables는 Container에 전달할 환경 변수를 구성합니다.
func (r *Runner) buildEnvironmentVariables() []string {
	env := []string{
		fmt.Sprintf("OPENCODE_MODEL=%s", r.agentInfo.Model),
	}

	// API 키 전달 (환경 변수에서 읽기)
	if apiKey := os.Getenv("CNAP_OPENCODE_API_KEY"); apiKey != "" {
		env = append(env, fmt.Sprintf("OPENCODE_API_KEY=%s", apiKey))
	}
	if apiKey := os.Getenv("CNAP_ANTHROPIC_API_KEY"); apiKey != "" {
		env = append(env, fmt.Sprintf("ANTHROPIC_API_KEY=%s", apiKey))
	}
	if apiKey := os.Getenv("CNAP_OPENAI_API_KEY"); apiKey != "" {
		env = append(env, fmt.Sprintf("OPENAI_API_KEY=%s", apiKey))
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
// Runner는 Start() 시점에 이미 OpenCode 세션을 생성하고 SSE 이벤트 구독을 시작했으므로,
// Run() 호출 시에는 기존 세션을 사용하여 프롬프트만 전송합니다.
// 이를 통해 하나의 Runner 인스턴스에서 여러 Run을 호출해도 동일한 세션을 유지합니다.
//
// 실행 흐름:
//  1. Runner 상태 및 요청 검증
//  2. 고루틴 시작 (즉시 반환)
//  3. [고루틴 내부] 기존 세션을 사용하여 프롬프트 전송
//  4. [고루틴 내부] 이벤트 수신 대기 (백그라운드 handleEvent가 처리 중)
//  5. [고루틴 내부] 완료 시 OnComplete 또는 OnError 호출
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
//   - 세션은 Start()에서 이미 생성되었어야 함
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
		err := r.runInternal(ctx, req)
		if err != nil {
			_ = r.callback.OnError(req.TaskID, err)
			return
		}
	}()

	return nil
}

// runInternal은 실제 실행 로직을 담당합니다.
// Start()에서 이미 세션이 생성되고 이벤트 구독이 시작되었으므로,
// 여기서는 프롬프트만 전송하고 결과를 기다립니다.
func (r *Runner) runInternal(ctx context.Context, req *RunRequest) error {
	r.Status = RunnerStatusRunning
	defer func() {
		r.Status = RunnerStatusReady
	}()

	r.logger.Info("Runner executing task",
		zap.String("runner_id", r.ID),
		zap.String("task_id", req.TaskID),
		zap.Int("message_count", len(req.Messages)),
	)

	// 세션이 준비되었는지 확인
	if r.sessionID == "" || r.apiClient == nil {
		return fmt.Errorf("세션이 준비되지 않음")
	}

	// 누적 컨텐츠 초기화 (새 실행 시작)
	r.fullContent.Reset()

	// 시스템 프롬프트와 메시지 결합
	messages := r.buildMessages(req)

	// 모델 정보 파싱
	providerID, modelID := parseModel(req.Model)

	// 메시지를 프롬프트 파트로 변환
	parts := make([]opencode.PromptPart, 0, len(messages))
	for _, msg := range messages {
		parts = append(parts, opencode.TextPartInput{
			Type: "text",
			Text: msg.Content,
		})
	}

	// 프롬프트 전송
	promptReq := &opencode.PromptRequest{
		Model: &opencode.PromptModel{
			ProviderID: providerID,
			ModelID:    modelID,
		},
		Parts: parts,
	}

	_, err := r.apiClient.Message(ctx, r.sessionID, promptReq)
	if err != nil {
		return fmt.Errorf("메시지 전송 실패: %w", err)
	}

	return nil
}

// buildMessages는 요청 메시지를 구성합니다.
// 마지막 사용자 메시지만 반환합니다.
func (r *Runner) buildMessages(req *RunRequest) []opencode.ChatMessage {
	// 마지막 사용자 메시지 찾기
	var lastUserMessage *opencode.ChatMessage
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			lastUserMessage = &req.Messages[i]
			break
		}
	}

	// 마지막 사용자 메시지가 없으면 빈 배열 반환
	if lastUserMessage == nil {
		return []opencode.ChatMessage{}
	}

	// 마지막 사용자 메시지만 반환
	return []opencode.ChatMessage{*lastUserMessage}
}

// GetMessage는 특정 메시지의 정보를 조회합니다.
func (r *Runner) GetMessage(ctx context.Context, messageID string) (*struct {
	Info  opencode.Message `json:"info"`
	Parts []opencode.Part  `json:"parts"`
}, error) {
	if r.apiClient == nil || r.sessionID == "" {
		return nil, fmt.Errorf("runner가 초기화되지 않음")
	}

	return r.apiClient.GetMessage(ctx, r.sessionID, messageID)
}

// parseModel은 "provider/model" 형식의 모델 문자열을 파싱합니다.
func parseModel(model string) (providerID, modelID string) {
	parts := strings.SplitN(model, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	// 기본값
	return "opencode", model
}

// RunResult는 에이전트 실행 결과를 나타냅니다.
type RunResult struct {
	Agent   string
	Name    string
	Success bool
	Output  string
	Error   error
}
