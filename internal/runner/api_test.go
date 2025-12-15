package taskrunner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeClient_Health(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		resp := HealthResponse{
			Status:  "ok",
			Version: "1.0.0",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	resp, err := client.Health(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "1.0.0", resp.Version)
}

func TestOpenCodeClient_CreateSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/sessions", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		var req CreateSessionRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "gpt-4", req.Model)

		resp := CreateSessionResponse{
			Session: Session{
				ID:     "session-123",
				Model:  "gpt-4",
				Status: "active",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	session, err := client.CreateSession(context.Background(), &CreateSessionRequest{
		Model: "gpt-4",
	})

	require.NoError(t, err)
	assert.Equal(t, "session-123", session.ID)
	assert.Equal(t, "gpt-4", session.Model)
	assert.Equal(t, "active", session.Status)
}

func TestOpenCodeClient_GetSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/sessions/session-123", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		resp := Session{
			ID:     "session-123",
			Model:  "gpt-4",
			Status: "active",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	session, err := client.GetSession(context.Background(), "session-123")

	require.NoError(t, err)
	assert.Equal(t, "session-123", session.ID)
	assert.Equal(t, "gpt-4", session.Model)
}

func TestOpenCodeClient_DeleteSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/sessions/session-123", r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	err := client.DeleteSession(context.Background(), "session-123")

	require.NoError(t, err)
}

func TestOpenCodeClient_Chat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/chat", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		var req ChatRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "gpt-4", req.Model)
		assert.Len(t, req.Messages, 1)
		assert.False(t, req.Stream)

		resp := ChatResponse{
			ID:    "resp-123",
			Model: "gpt-4",
		}
		resp.Response.Role = "assistant"
		resp.Response.Content = "Hello! How can I help you?"
		resp.FinishReason = "stop"
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	resp, err := client.Chat(context.Background(), &ChatRequest{
		Model: "gpt-4",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "Hello! How can I help you?", resp.Response.Content)
	assert.Equal(t, "assistant", resp.Response.Role)
	assert.Equal(t, "stop", resp.FinishReason)
}

func TestOpenCodeClient_ChatStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/chat/stream", r.URL.Path)
		assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			`data: {"event":"message","data":{"content":"Hello"}}`,
			`data: {"event":"message","data":{"content":" World"}}`,
			`data: {"event":"done","data":{"finish_reason":"stop"}}`,
		}

		for _, event := range events {
			_, _ = w.Write([]byte(event + "\n\n"))
			w.(http.Flusher).Flush()
		}
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	var collected []string

	err := client.ChatStream(context.Background(), &ChatRequest{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "Hello"}},
	}, func(event *ChatStreamEvent) error {
		if event.Event == "message" {
			collected = append(collected, event.Data.Content)
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"Hello", " World"}, collected)
}

func TestOpenCodeClient_ErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(APIError{
			Error: struct {
				Type    string `json:"type"`
				Message string `json:"message"`
				Code    string `json:"code"`
			}{
				Type:    "invalid_request",
				Message: "Model not found",
				Code:    "model_not_found",
			},
		})
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	_, err := client.Chat(context.Background(), &ChatRequest{
		Model:    "invalid-model",
		Messages: []ChatMessage{{Role: "user", Content: "Hello"}},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Model not found")
}

func TestOpenCodeClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate long-running request
		select {
		case <-r.Context().Done():
			return
		case <-make(chan struct{}):
			// Never completes
		}
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.Chat(ctx, &ChatRequest{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "Hello"}},
	})

	require.Error(t, err)
}

func TestOpenCodeClient_StreamContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Send one event
		_, _ = w.Write([]byte(`data: {"event":"message","data":{"content":"Hello"}}` + "\n\n"))
		w.(http.Flusher).Flush()

		// Wait indefinitely
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	ctx, cancel := context.WithCancel(context.Background())

	eventCount := 0
	err := client.ChatStream(ctx, &ChatRequest{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "Hello"}},
	}, func(event *ChatStreamEvent) error {
		eventCount++
		if eventCount == 1 {
			cancel() // Cancel after first event
		}
		return nil
	})

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, 1, eventCount)
}
