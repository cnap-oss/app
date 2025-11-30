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
	AgentID  string
	Provider string
	Model    string
	Prompt   string
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

// getAPIKeyForProvider는 provider에 맞는 API 키를 환경 변수에서 가져옵니다.
func getAPIKeyForProvider(provider string) (string, error) {
	var envKey string
	switch provider {
	case "opencode":
		envKey = "OPEN_CODE_API_KEY"
	case "gemini":
		envKey = "GEMINI_API_KEY"
	case "claude":
		envKey = "ANTHROPIC_API_KEY"
	case "openai":
		envKey = "OPENAI_API_KEY"
	case "xai":
		envKey = "XAI_API_KEY"
	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}

	apiKey := os.Getenv(envKey)
	if apiKey == "" {
		return "", fmt.Errorf("환경 변수 %s가 설정되어 있지 않습니다", envKey)
	}

	return apiKey, nil
}

// getEndpointForProvider는 provider에 맞는 API endpoint를 반환합니다.
func getEndpointForProvider(provider string) (string, error) {
	switch provider {
	case "opencode":
		return "https://opencode.ai/zen/v1/chat/completions", nil
	case "gemini":
		// Gemini API endpoint (Google AI Studio)
		return "https://generativelanguage.googleapis.com/v1/models", nil
	case "claude":
		// Anthropic API endpoint
		return "https://api.anthropic.com/v1/messages", nil
	case "openai":
		// OpenAI API endpoint
		return "https://api.openai.com/v1/chat/completions", nil
	case "xai":
		// xAI API endpoint
		return "https://api.x.ai/v1/chat/completions", nil
	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}
}

// RunWithResult는 프롬프트를 AI API 엔드포인트로 보내고 결과를 반환합니다.
// provider에 따라 다른 API를 호출합니다.
func (r *Runner) RunWithResult(ctx context.Context, model, name, prompt string) (*RunResult, error) {
	// agentInfo에서 provider 가져오기 (없으면 기본값 opencode)
	provider := r.agentInfo.Provider
	if provider == "" {
		provider = "opencode"
	}

	promptPreview := prompt
	if len(promptPreview) > 200 {
		promptPreview = promptPreview[:200] + "..."
	}

	r.logger.Info("Sending request to AI API",
		zap.String("provider", provider),
		zap.String("model", model),
		zap.String("name", name),
		zap.String("prompt_preview", promptPreview),
	)

	// Provider에 맞는 API 키 가져오기
	apiKey, err := getAPIKeyForProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("API 키 조회 실패: %w", err)
	}

	// Provider에 맞는 endpoint 가져오기
	endpoint, err := getEndpointForProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("endpoint 조회 실패: %w", err)
	}

	// Provider별 API 호출
	switch provider {
	case "opencode", "openai", "xai":
		// OpenAI 호환 API (opencode, openai, xai 모두 동일한 포맷)
		return r.callOpenAICompatibleAPI(ctx, endpoint, apiKey, model, name, prompt)
	case "claude":
		// Anthropic Claude API (추후 구현)
		return nil, fmt.Errorf("claude API는 아직 구현되지 않았습니다. opencode provider를 사용하여 claude 모델을 실행할 수 있습니다")
	case "gemini":
		// Google Gemini API (추후 구현)
		return nil, fmt.Errorf("gemini API는 아직 구현되지 않았습니다. opencode provider를 사용하여 gemini 모델을 실행할 수 있습니다")
	default:
		return nil, fmt.Errorf("지원하지 않는 provider: %s", provider)
	}
}

// callOpenAICompatibleAPI는 OpenAI 호환 API를 호출합니다 (opencode, openai, xai).
func (r *Runner) callOpenAICompatibleAPI(ctx context.Context, endpoint, apiKey, model, name, prompt string) (*RunResult, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("요청 생성 실패: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

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
