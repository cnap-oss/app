package taskrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
)

// TaskRunner는 Task 실행을 위한 인터페이스입니다.
type TaskRunner interface {
	// Run은 주어진 메시지들로 AI API를 호출하고 결과를 반환합니다.
	Run(ctx context.Context, req *RunRequest) (*RunResult, error)
}

// RunRequest는 TaskRunner 실행 요청입니다.
type RunRequest struct {
	TaskID       string
	Model        string
	SystemPrompt string
	Messages     []ChatMessage
	Callback     StatusCallback // 중간 응답 콜백 (optional)
}

// AgentInfo는 에이전트 실행에 필요한 정보를 담는 구조체입니다.
type AgentInfo struct {
	AgentID  string
	Provider string
	Model    string
	Prompt   string
}

// StatusCallback은 Task 실행 중 상태 변경을 Controller에 알리기 위한 콜백 인터페이스입니다.
type StatusCallback interface {
	// OnStatusChange는 Task 상태가 변경될 때 호출됩니다.
	OnStatusChange(taskID string, status string) error

	// OnMessage는 Runner가 중간 응답을 생성할 때 호출됩니다.
	// 이를 통해 Controller가 Connector에 실시간으로 메시지를 전달할 수 있습니다.
	OnMessage(taskID string, message string) error

	// OnComplete는 Task가 완료될 때 호출됩니다.
	OnComplete(taskID string, result *RunResult) error

	// OnError는 Task 실행 중 에러가 발생할 때 호출됩니다.
	OnError(taskID string, err error) error
}

const defaultBaseURL = "https://opencode.ai/zen/v1"

// Runner는 short-living 에이전트 실행을 담당하는 TaskRunner 구현체입니다.
type Runner struct {
	ID         string
	Status     string
	agentInfo  AgentInfo
	logger     *zap.Logger
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// RunnerOption은 Runner 초기화 옵션을 설정하기 위한 함수 타입입니다.
type RunnerOption func(*Runner)

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

// OpenCodeRequest는 OpenCode Zen API 요청 바디입니다.
type OpenCodeRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
}

// ChatMessage는 OpenCode Zen API 요청 바디의 messages 필드입니다.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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

// NewRunner는 새로운 Runner를 생성합니다.
func NewRunner(logger *zap.Logger, opts ...RunnerOption) *Runner {
	if logger == nil {
		logger = zap.NewNop()
	}

	apiKey := os.Getenv("OPEN_CODE_API_KEY")
	if apiKey == "" {
		logger.Fatal("환경 변수 OPEN_CODE_API_KEY가 설정되어 있지 않습니다")
	}

	r := &Runner{
		logger:     logger,
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// ensure Runner implements TaskRunner interface
var _ TaskRunner = (*Runner)(nil)

// Run implements TaskRunner interface.
func (r *Runner) Run(ctx context.Context, req *RunRequest) (*RunResult, error) {
	r.logger.Info("Runner starting task", zap.String("messages", fmt.Sprintf("%+v", req.Messages)))
	// 시스템 프롬프트와 메시지를 결합
	messages := make([]ChatMessage, 0, len(req.Messages)+1)
	if req.SystemPrompt != "" {
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}
	messages = append(messages, req.Messages...)

	// API 호출 - 전체 메시지 배열 전달
	result, err := r.Request(ctx, req.Model, req.TaskID, messages)
	if err != nil {
		// 콜백이 있으면 에러 알림
		if req.Callback != nil {
			_ = req.Callback.OnError(req.TaskID, err)
		}
		return nil, err
	}

	// AI 응답 수신 완료 - 콜백으로 중간 응답 전달
	if req.Callback != nil && result.Success && result.Output != "" {
		if err := req.Callback.OnMessage(req.TaskID, result.Output); err != nil {
			r.logger.Warn("Failed to send message callback",
				zap.String("task_id", req.TaskID),
				zap.Error(err),
			)
		}
	}

	return result, nil
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
