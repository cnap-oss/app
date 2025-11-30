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

// AgentInfo는 에이전트 실행에 필요한 정보를 담는 구조체입니다.
type AgentInfo struct {
	AgentID string
	Model   string
	Prompt  string
}

// StatusCallback은 Task 실행 중 상태 변경을 Controller에 알리기 위한 콜백 인터페이스입니다.
type StatusCallback interface {
	// OnStatusChange는 Task 상태가 변경될 때 호출됩니다.
	OnStatusChange(taskID string, status string) error

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

// RunWithResult는 프롬프트를 OpenCode Zen API의 chat/completions 엔드포인트로 보내고 결과를 반환합니다.
func (r *Runner) RunWithResult(ctx context.Context, model, name, prompt string) (*RunResult, error) {
	promptPreview := prompt
	if len(promptPreview) > 200 {
		promptPreview = promptPreview[:200] + "..."
	}

	// 요청 정보 로그 출력
	r.logger.Info("Sending request to OpenCode Zen API (Chat Completions endpoint)",
		zap.String("model", model),
		zap.String("name", name),
		zap.String("prompt_preview", promptPreview),
	)

	// 요청 본문 구성
	reqBody := OpenCodeRequest{
		Model: model,
		Messages: []ChatMessage{
			{Role: "user", Content: prompt},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("요청 바디 직렬화 실패: %w", err)
	}

	endpoint := strings.TrimRight(r.baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("요청 생성 실패: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	client := r.httpClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
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

	r.logger.Info("OpenCode 응답 수신 완료",
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
