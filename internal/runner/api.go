package taskrunner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// OpenCodeClient는 OpenCode Server REST API 클라이언트입니다.
type OpenCodeClient struct {
	baseURL    string
	directory  string // OpenCode 작업 디렉토리
	httpClient *http.Client
	logger     *zap.Logger
}

// OpenCodeClientOption은 OpenCodeClient 옵션입니다.
type OpenCodeClientOption func(*OpenCodeClient)

// WithOpenCodeHTTPClient는 HTTP 클라이언트를 설정합니다.
func WithOpenCodeHTTPClient(client *http.Client) OpenCodeClientOption {
	return func(c *OpenCodeClient) {
		c.httpClient = client
	}
}

// WithOpenCodeLogger는 로거를 설정합니다.
func WithOpenCodeLogger(logger *zap.Logger) OpenCodeClientOption {
	return func(c *OpenCodeClient) {
		c.logger = logger
	}
}

// WithOpenCodeDirectory는 OpenCode 작업 디렉토리를 설정합니다.
func WithOpenCodeDirectory(directory string) OpenCodeClientOption {
	return func(c *OpenCodeClient) {
		c.directory = directory
	}
}

// NewOpenCodeClient는 새 OpenCode API 클라이언트를 생성합니다.
func NewOpenCodeClient(baseURL string, opts ...OpenCodeClientOption) *OpenCodeClient {
	c := &OpenCodeClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 300 * time.Second, // OpenCode는 긴 처리 시간이 필요할 수 있음
		},
		logger: zap.NewNop(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// ======================================
// Session Management
// ======================================

// CreateSession은 새 세션을 생성합니다.
func (c *OpenCodeClient) CreateSession(ctx context.Context, req *CreateSessionRequest) (*Session, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/session", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result Session
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	c.logger.Info("세션 생성됨",
		zap.String("session_id", result.ID),
		zap.String("title", result.Title),
	)

	return &result, nil
}

// GetSession은 세션 정보를 조회합니다.
func (c *OpenCodeClient) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/session/"+sessionID, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result Session
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	return &result, nil
}

// UpdateSession은 세션을 업데이트합니다.
func (c *OpenCodeClient) UpdateSession(ctx context.Context, sessionID string, req *UpdateSessionRequest) (*Session, error) {
	resp, err := c.doRequest(ctx, http.MethodPatch, "/session/"+sessionID, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result Session
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	return &result, nil
}

// DeleteSession은 세션을 종료합니다.
func (c *OpenCodeClient) DeleteSession(ctx context.Context, sessionID string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, "/session/"+sessionID, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	c.logger.Info("세션 삭제됨",
		zap.String("session_id", sessionID),
	)

	return nil
}

// ListSessions는 모든 세션 목록을 조회합니다.
func (c *OpenCodeClient) ListSessions(ctx context.Context) ([]Session, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/session", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result []Session
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	return result, nil
}

// ======================================
// Message API
// ======================================

// Message는 메시지를 전송하고 응답을 수신합니다.
func (c *OpenCodeClient) Message(ctx context.Context, sessionID string, req *PromptRequest) (*PromptResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/session/%s/message", sessionID), req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 응답 body 읽기
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("응답 읽기 실패: %w", err)
	}

	// 빈 응답 처리 - OpenCode Server는 비동기 처리 시 빈 응답을 반환할 수 있음
	if len(bodyBytes) == 0 {
		c.logger.Warn("빈 응답 수신 - 비동기 처리 중일 수 있음",
			zap.String("session_id", sessionID),
		)
		// 빈 응답이지만 정상 처리로 간주 (이벤트 스트림으로 실제 응답 수신 예정)
		return &PromptResponse{
			Info: AssistantMessage{
				ID:   "", // 빈 ID - 이벤트에서 실제 메시지 확인 필요
				Role: "assistant",
			},
			Parts: []Part{},
		}, nil
	}

	var result PromptResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		// body preview (최대 200자)
		preview := string(bodyBytes)
		if len(preview) > 200 {
			preview = preview[:200]
		}
		c.logger.Error("응답 파싱 실패",
			zap.String("session_id", sessionID),
			zap.Int("body_length", len(bodyBytes)),
			zap.String("body_preview", preview),
			zap.Error(err),
		)
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	c.logger.Info("메시지 전송 완료",
		zap.String("session_id", sessionID),
		zap.String("message_id", result.Info.ID),
		zap.Int("parts_count", len(result.Parts)),
	)

	return &result, nil
}

// PromptAsync는 메시지를 비동기적으로 전송합니다.
func (c *OpenCodeClient) PromptAsync(ctx context.Context, sessionID string, req *PromptRequest) error {
	resp, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/session/%s/prompt_async", sessionID), req)
	if err != nil {
		// 204 No Content는 성공으로 간주
		if apiErr, ok := err.(*APIError); ok && apiErr.StatusCode == http.StatusNoContent {
			c.logger.Info("비동기 프롬프트 전송 성공 (204 No Content)",
				zap.String("session_id", sessionID),
			)
			return nil
		}
		return err
	}
	defer resp.Body.Close()

	c.logger.Info("비동기 프롬프트 전송 성공",
		zap.String("session_id", sessionID),
	)

	return nil
}

// GetMessages는 세션의 모든 메시지를 조회합니다.
func (c *OpenCodeClient) GetMessages(ctx context.Context, sessionID string, limit *int) ([]struct {
	Info  Message `json:"info"`
	Parts []Part  `json:"parts"`
}, error) {
	path := fmt.Sprintf("/session/%s/message", sessionID)
	if limit != nil {
		path = fmt.Sprintf("%s?limit=%d", path, *limit)
	}

	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result []struct {
		Info  json.RawMessage `json:"info"`
		Parts []Part          `json:"parts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	// Message는 UserMessage 또는 AssistantMessage일 수 있으므로 변환
	messages := make([]struct {
		Info  Message `json:"info"`
		Parts []Part  `json:"parts"`
	}, len(result))

	for i, msg := range result {
		// role 필드로 타입 구분
		var roleCheck struct {
			Role string `json:"role"`
		}
		if err := json.Unmarshal(msg.Info, &roleCheck); err != nil {
			return nil, fmt.Errorf("role 파싱 실패: %w", err)
		}

		if roleCheck.Role == "user" {
			var userMsg UserMessage
			if err := json.Unmarshal(msg.Info, &userMsg); err != nil {
				return nil, fmt.Errorf("user message 파싱 실패: %w", err)
			}
			messages[i].Info = userMsg
		} else {
			var assistantMsg AssistantMessage
			if err := json.Unmarshal(msg.Info, &assistantMsg); err != nil {
				return nil, fmt.Errorf("assistant message 파싱 실패: %w", err)
			}
			messages[i].Info = assistantMsg
		}
		messages[i].Parts = msg.Parts
	}

	return messages, nil
}

// GetMessage는 특정 메시지를 조회합니다.
func (c *OpenCodeClient) GetMessage(ctx context.Context, sessionID, messageID string) (*struct {
	Info  Message `json:"info"`
	Parts []Part  `json:"parts"`
}, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/session/%s/message/%s", sessionID, messageID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rawResult struct {
		Info  json.RawMessage `json:"info"`
		Parts []Part          `json:"parts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rawResult); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	result := &struct {
		Info  Message `json:"info"`
		Parts []Part  `json:"parts"`
	}{
		Parts: rawResult.Parts,
	}

	// role 필드로 타입 구분
	var roleCheck struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(rawResult.Info, &roleCheck); err != nil {
		return nil, fmt.Errorf("role 파싱 실패: %w", err)
	}

	if roleCheck.Role == "user" {
		var userMsg UserMessage
		if err := json.Unmarshal(rawResult.Info, &userMsg); err != nil {
			return nil, fmt.Errorf("user message 파싱 실패: %w", err)
		}
		result.Info = userMsg
	} else {
		var assistantMsg AssistantMessage
		if err := json.Unmarshal(rawResult.Info, &assistantMsg); err != nil {
			return nil, fmt.Errorf("assistant message 파싱 실패: %w", err)
		}
		result.Info = assistantMsg
	}

	return result, nil
}

// AbortSession은 활성 세션을 중단합니다.
func (c *OpenCodeClient) AbortSession(ctx context.Context, sessionID string) error {
	resp, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/session/%s/abort", sessionID), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	c.logger.Info("세션 중단됨",
		zap.String("session_id", sessionID),
	)

	return nil
}

// ======================================
// Path API
// ======================================

// GetPath는 경로 정보를 조회합니다.
func (c *OpenCodeClient) GetPath(ctx context.Context) (*PathInfo, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/path", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result PathInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	return &result, nil
}

// ======================================
// Event Streaming
// ======================================

// EventHandler는 SSE 이벤트 핸들러입니다.
type EventHandler func(event *Event) error

// SubscribeEvents는 이벤트 스트림을 구독합니다.
func (c *OpenCodeClient) SubscribeEvents(ctx context.Context, handler EventHandler) error {
	httpReq, err := c.buildRequest(ctx, http.MethodGet, "/event", nil)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("요청 실패: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.handleErrorResponse(resp)
	}

	c.logger.Info("이벤트 스트림 구독 시작")

	// SSE 파싱
	reader := bufio.NewReader(resp.Body)
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("이벤트 스트림 구독 중단")
			return ctx.Err()
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				c.logger.Info("이벤트 스트림 종료")
				return nil
			}
			return fmt.Errorf("스트림 읽기 실패: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ":") {
			continue // 빈 줄 또는 주석
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			var event Event
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				c.logger.Warn("이벤트 파싱 실패",
					zap.String("data", data),
					zap.Error(err),
				)
				continue
			}

			if err := handler(&event); err != nil {
				return err
			}
		}
	}
}

// ======================================
// Internal Methods
// ======================================

// doRequest는 HTTP 요청을 수행합니다.
func (c *OpenCodeClient) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	req, err := c.buildRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("요청 실패: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, c.handleErrorResponse(resp)
	}

	return resp, nil
}

// buildRequest는 HTTP 요청을 구성합니다.
func (c *OpenCodeClient) buildRequest(ctx context.Context, method, path string, body interface{}) (*http.Request, error) {
	url := c.baseURL + path

	// directory 쿼리 파라미터 추가
	if c.directory != "" {
		if strings.Contains(url, "?") {
			url += "&directory=" + c.directory
		} else {
			url += "?directory=" + c.directory
		}
	}

	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("요청 바디 직렬화 실패: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("요청 생성 실패: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

// handleErrorResponse는 에러 응답을 처리합니다.
func (c *OpenCodeClient) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	// BadRequestError (400) 시도
	if resp.StatusCode == http.StatusBadRequest {
		var badReqErr BadRequestError
		if err := json.Unmarshal(body, &badReqErr); err == nil && len(badReqErr.Errors) > 0 {
			return &APIError{
				StatusCode: resp.StatusCode,
				Message:    fmt.Sprintf("잘못된 요청: %v", badReqErr.Errors),
				Body:       string(body),
			}
		}
	}

	// NotFoundError (404) 시도
	if resp.StatusCode == http.StatusNotFound {
		var notFoundErr NotFoundError
		if err := json.Unmarshal(body, &notFoundErr); err == nil {
			if msg, ok := notFoundErr.Data["message"].(string); ok {
				return &APIError{
					StatusCode: resp.StatusCode,
					Message:    msg,
					Body:       string(body),
				}
			}
		}
	}

	// 일반 에러
	return &APIError{
		StatusCode: resp.StatusCode,
		Message:    fmt.Sprintf("HTTP 에러 [%d]", resp.StatusCode),
		Body:       string(body),
	}
}
