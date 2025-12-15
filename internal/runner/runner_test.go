package taskrunner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// MockStatusCallback은 테스트용 콜백 구현입니다.
type MockStatusCallback struct {
	mu               sync.Mutex
	StartedCalled    bool
	StartedSessionID string
	CompletedCalled  bool
	ErrorCalled      bool
	Messages         []*RunnerMessage
	Result           *RunResult
	Error            error
	Done             chan struct{}
}

func NewMockStatusCallback() *MockStatusCallback {
	return &MockStatusCallback{
		Messages: make([]*RunnerMessage, 0),
		Done:     make(chan struct{}),
	}
}

func (m *MockStatusCallback) OnStarted(taskID string, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StartedCalled = true
	m.StartedSessionID = sessionID
	return nil
}

func (m *MockStatusCallback) OnMessage(taskID string, msg *RunnerMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Messages = append(m.Messages, msg)
	return nil
}

func (m *MockStatusCallback) OnComplete(taskID string, result *RunResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CompletedCalled = true
	m.Result = result
	close(m.Done)
	return nil
}

func (m *MockStatusCallback) OnError(taskID string, err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ErrorCalled = true
	m.Error = err
	close(m.Done)
	return nil
}

func (m *MockStatusCallback) GetTextContent() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var content strings.Builder
	for _, msg := range m.Messages {
		if msg.IsText() {
			content.WriteString(msg.Content)
		}
	}
	return content.String()
}

// TestRunWithResult_Success tests successful API call (레거시 OpenCode Zen API)
func TestRunWithResult_Success(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	// Mock server that returns successful response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		// Verify request body
		var reqBody OpenCodeRequest
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)
		assert.Equal(t, "grok-code", reqBody.Model)
		// 전체 메시지 배열이 전달됨
		assert.Len(t, reqBody.Messages, 1)
		assert.Equal(t, "user", reqBody.Messages[0].Role)
		assert.Equal(t, "test prompt", reqBody.Messages[0].Content)

		// Return successful response
		resp := OpenCodeResponse{
			ID:      "test-id",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "grok-code",
			Choices: []struct {
				Index   int `json:"index"`
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{
				{
					Index: 0,
					Message: struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					}{
						Role:    "assistant",
						Content: "test response",
					},
					FinishReason: "stop",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	callback := NewMockStatusCallback()
	runner, err := NewRunner("test-task", AgentInfo{AgentID: "test-agent", Model: "grok-code"}, callback, zaptest.NewLogger(t), WithBaseURL(server.URL))
	require.NoError(t, err)
	ctx := context.Background()

	result, err := runner.Request(ctx, "grok-code", "test-task", []ChatMessage{
		{Role: "user", Content: "test prompt"},
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "grok-code", result.Agent)
	assert.Equal(t, "test-task", result.Name)
	assert.True(t, result.Success)
	assert.Equal(t, "test response", result.Output)
	assert.Nil(t, result.Error)
}

// TestRunWithResult_APIError tests API error response (레거시)
func TestRunWithResult_APIError(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	// Mock server that returns error response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := OpenCodeResponse{
			Error: &struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			}{
				Type:    "invalid_request_error",
				Message: "Invalid API key",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	callback := NewMockStatusCallback()
	runner, err := NewRunner("test-task", AgentInfo{AgentID: "test-agent", Model: "grok-code"}, callback, zaptest.NewLogger(t), WithBaseURL(server.URL))
	require.NoError(t, err)
	ctx := context.Background()

	result, err := runner.Request(ctx, "grok-code", "test-task", []ChatMessage{
		{Role: "user", Content: "test prompt"},
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "API 에러")
	assert.Contains(t, err.Error(), "invalid_request_error")
	assert.Contains(t, err.Error(), "Invalid API key")
}

// TestRunWithResult_HTTPError tests HTTP error status codes (레거시)
func TestRunWithResult_HTTPError(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	// Mock server that returns 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	callback := NewMockStatusCallback()
	runner, err := NewRunner("test-task", AgentInfo{AgentID: "test-agent", Model: "grok-code"}, callback, zaptest.NewLogger(t), WithBaseURL(server.URL))
	require.NoError(t, err)
	ctx := context.Background()

	result, err := runner.Request(ctx, "grok-code", "test-task", []ChatMessage{
		{Role: "user", Content: "test prompt"},
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "API 응답 오류")
	assert.Contains(t, err.Error(), "500")
}

// TestRunWithResult_Timeout tests timeout handling (레거시)
func TestRunWithResult_Timeout(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	// Mock server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create client with very short timeout
	client := &http.Client{Timeout: 100 * time.Millisecond}
	callback := NewMockStatusCallback()
	runner, err := NewRunner("test-task", AgentInfo{AgentID: "test-agent", Model: "grok-code"}, callback, zaptest.NewLogger(t), WithBaseURL(server.URL), WithHTTPClient(client))
	require.NoError(t, err)
	ctx := context.Background()

	result, err := runner.Request(ctx, "grok-code", "test-task", []ChatMessage{
		{Role: "user", Content: "test prompt"},
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "API 요청 실패")
}

// TestRunWithResult_ContextCancellation tests context cancellation (레거시)
func TestRunWithResult_ContextCancellation(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	// Mock server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	callback := NewMockStatusCallback()
	runner, err := NewRunner("test-task", AgentInfo{AgentID: "test-agent", Model: "grok-code"}, callback, zaptest.NewLogger(t), WithBaseURL(server.URL))
	require.NoError(t, err)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := runner.Request(ctx, "grok-code", "test-task", []ChatMessage{
		{Role: "user", Content: "test prompt"},
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "API 요청 실패")
}

// TestRun_Async_Success tests async Run implementation with new OpenCode API
func TestRun_Async_Success(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	sessionID := "ses_test123"

	// Mock server for new OpenCode API with SSE support
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == "POST" && r.URL.Path == "/session":
			// 세션 생성
			resp := Session{
				ID:        sessionID,
				ProjectID: "proj_1",
				Directory: "/workspace",
				Title:     "task-1",
				Version:   "1.0.0",
				Time: SessionTime{
					Created: 1234567890,
					Updated: 1234567890,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.Method == "GET" && r.URL.Path == "/events":
			// SSE 이벤트 스트림 (간단한 버전)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			// 텍스트 이벤트 전송
			event1 := Event{
				Type: "message.part.updated",
				Properties: map[string]interface{}{
					"messageID": "msg_123",
					"part": map[string]interface{}{
						"id":   "prt_123",
						"type": "text",
						"text": "test response",
					},
				},
			}
			data1, _ := json.Marshal(event1)
			_, _ = w.Write([]byte("data: " + string(data1) + "\n\n"))

			// 완료 이벤트 전송
			event2 := Event{
				Type: "message.completed",
				Properties: map[string]interface{}{
					"messageID": "msg_123",
				},
			}
			data2, _ := json.Marshal(event2)
			_, _ = w.Write([]byte("data: " + string(data2) + "\n\n"))

			w.(http.Flusher).Flush()

		case r.Method == "POST" && r.URL.Path == "/session/"+sessionID+"/message":
			// 프롬프트 전송
			completed := int64(1234567900)
			resp := PromptResponse{
				Info: AssistantMessage{
					ID:         "msg_123",
					SessionID:  sessionID,
					Role:       "assistant",
					ParentID:   "msg_000",
					ModelID:    "grok-code",
					ProviderID: "anthropic",
					Mode:       "code",
					Path: MessagePath{
						Cwd:  "/workspace",
						Root: "/workspace",
					},
					Time: MessageTime{
						Created:   1234567890,
						Completed: &completed,
					},
					Cost: 0.01,
					Tokens: MessageTokens{
						Input:     100,
						Output:    200,
						Reasoning: 0,
						Cache: MessageTokenCache{
							Read:  0,
							Write: 0,
						},
					},
				},
				Parts: []Part{
					{
						ID:        "prt_123",
						SessionID: sessionID,
						MessageID: "msg_123",
						Type:      "text",
						Text:      "test response",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.Method == "DELETE" && r.URL.Path == "/session/"+sessionID:
			// 세션 삭제
			_ = json.NewEncoder(w).Encode(true)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Mock callback 생성
	callback := NewMockStatusCallback()

	runner, err := NewRunner("task-1", AgentInfo{AgentID: "test-agent", Model: "anthropic/grok-code"}, callback, zaptest.NewLogger(t))
	require.NoError(t, err)
	runner.Status = RunnerStatusReady
	runner.BaseURL = server.URL
	ctx := context.Background()

	req := &RunRequest{
		TaskID:       "task-1",
		Model:        "anthropic/grok-code",
		SystemPrompt: "You are a helpful assistant",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	// Run은 비동기로 즉시 반환
	err = runner.Run(ctx, req)
	require.NoError(t, err)

	// 완료 대기 (콜백으로 결과 수신)
	select {
	case <-callback.Done:
		assert.True(t, callback.StartedCalled)
		assert.Equal(t, sessionID, callback.StartedSessionID)
		assert.True(t, callback.CompletedCalled)
		assert.NotNil(t, callback.Result)
		assert.Equal(t, "anthropic/grok-code", callback.Result.Agent)
		assert.Equal(t, "task-1", callback.Result.Name)
		assert.True(t, callback.Result.Success)
		assert.Equal(t, "test response", callback.Result.Output)
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for callback")
	}
}

// TestNewRunner_WithOptions tests Runner creation with options
func TestNewRunner_WithOptions(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	customClient := &http.Client{Timeout: 5 * time.Second}
	customBaseURL := "https://custom.api.example.com"

	callback := NewMockStatusCallback()
	runner, err := NewRunner(
		"test-task",
		AgentInfo{AgentID: "test-agent", Model: "grok-code"},
		callback,
		zaptest.NewLogger(t),
		WithHTTPClient(customClient),
		WithBaseURL(customBaseURL),
	)

	require.NoError(t, err)
	assert.NotNil(t, runner)
	assert.Equal(t, customClient, runner.httpClient)
	assert.Equal(t, customBaseURL, runner.baseURL)
	assert.Equal(t, "test-key", runner.apiKey)
}
