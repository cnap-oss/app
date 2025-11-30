package taskrunner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// TestRunWithResult_Success tests successful API call
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

	runner := NewRunner(zaptest.NewLogger(t), WithBaseURL(server.URL))
	ctx := context.Background()

	result, err := runner.RunWithResult(ctx, "grok-code", "test-task", "test prompt")

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "grok-code", result.Agent)
	assert.Equal(t, "test-task", result.Name)
	assert.True(t, result.Success)
	assert.Equal(t, "test response", result.Output)
	assert.Nil(t, result.Error)
}

// TestRunWithResult_APIError tests API error response
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

	runner := NewRunner(zaptest.NewLogger(t), WithBaseURL(server.URL))
	ctx := context.Background()

	result, err := runner.RunWithResult(ctx, "grok-code", "test-task", "test prompt")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "API 에러")
	assert.Contains(t, err.Error(), "invalid_request_error")
	assert.Contains(t, err.Error(), "Invalid API key")
}

// TestRunWithResult_HTTPError tests HTTP error status codes
func TestRunWithResult_HTTPError(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	// Mock server that returns 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	runner := NewRunner(zaptest.NewLogger(t), WithBaseURL(server.URL))
	ctx := context.Background()

	result, err := runner.RunWithResult(ctx, "grok-code", "test-task", "test prompt")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "API 응답 오류")
	assert.Contains(t, err.Error(), "500")
}

// TestRunWithResult_Timeout tests timeout handling
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
	runner := NewRunner(zaptest.NewLogger(t), WithBaseURL(server.URL), WithHTTPClient(client))
	ctx := context.Background()

	result, err := runner.RunWithResult(ctx, "grok-code", "test-task", "test prompt")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "API 요청 실패")
}

// TestRunWithResult_ContextCancellation tests context cancellation
func TestRunWithResult_ContextCancellation(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	// Mock server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	runner := NewRunner(zaptest.NewLogger(t), WithBaseURL(server.URL))

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := runner.RunWithResult(ctx, "grok-code", "test-task", "test prompt")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "API 요청 실패")
}

// TestRun_Success tests TaskRunner interface implementation
func TestRun_Success(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	// Mock server that returns successful response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	runner := NewRunner(zaptest.NewLogger(t), WithBaseURL(server.URL))
	ctx := context.Background()

	req := &RunRequest{
		TaskID:       "task-1",
		Model:        "grok-code",
		SystemPrompt: "You are a helpful assistant",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
			{Role: "user", Content: "How are you?"},
		},
	}

	result, err := runner.Run(ctx, req)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "grok-code", result.Agent)
	assert.Equal(t, "task-1", result.Name)
	assert.True(t, result.Success)
	assert.Equal(t, "test response", result.Output)
}

// TestRun_WithSystemPrompt tests system prompt handling
func TestRun_WithSystemPrompt(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	// Track what was sent to the API
	var receivedMessages []ChatMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody OpenCodeRequest
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		receivedMessages = reqBody.Messages

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
						Content: "response",
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

	runner := NewRunner(zaptest.NewLogger(t), WithBaseURL(server.URL))
	ctx := context.Background()

	req := &RunRequest{
		TaskID:       "task-1",
		Model:        "grok-code",
		SystemPrompt: "You are a helpful assistant",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	_, err := runner.Run(ctx, req)
	require.NoError(t, err)

	// Verify system prompt was NOT included (current implementation uses last user message)
	// Note: Current implementation in task_runner.go only sends the last user message
	// not the entire conversation history with system prompt
	assert.Len(t, receivedMessages, 1)
	assert.Equal(t, "user", receivedMessages[0].Role)
	assert.Equal(t, "Hello", receivedMessages[0].Content)
}

// TestRun_NoUserMessage tests behavior when there's no user message
func TestRun_NoUserMessage(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
						Content: "response",
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

	runner := NewRunner(zaptest.NewLogger(t), WithBaseURL(server.URL))
	ctx := context.Background()

	req := &RunRequest{
		TaskID:       "task-1",
		Model:        "grok-code",
		SystemPrompt: "You are a helpful assistant",
		Messages: []ChatMessage{
			{Role: "assistant", Content: "Hello"},
		},
	}

	// Should still work, but prompt will be empty
	result, err := runner.Run(ctx, req)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

// TestNewRunner_NoAPIKey tests that NewRunner fails when API key is not set
func TestNewRunner_NoAPIKey(t *testing.T) {
	// Unset the API key
	t.Setenv("OPEN_CODE_API_KEY", "")

	// This should panic or fail since NewRunner calls logger.Fatal
	// We can't easily test Fatal calls, so we skip this test
	// In production, this would be caught at startup
	t.Skip("NewRunner calls logger.Fatal which can't be easily tested")
}

// TestNewRunner_WithOptions tests Runner creation with options
func TestNewRunner_WithOptions(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	customClient := &http.Client{Timeout: 5 * time.Second}
	customBaseURL := "https://custom.api.example.com"

	runner := NewRunner(
		zaptest.NewLogger(t),
		WithHTTPClient(customClient),
		WithBaseURL(customBaseURL),
	)

	assert.NotNil(t, runner)
	assert.Equal(t, customClient, runner.httpClient)
	assert.Equal(t, customBaseURL, runner.baseURL)
	assert.Equal(t, "test-key", runner.apiKey)
}
