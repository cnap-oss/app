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

func TestOpenCodeClient_CreateSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/session", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		var req CreateSessionRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "Test Session", req.Title)

		resp := Session{
			ID:        "ses_123",
			ProjectID: "proj_1",
			Directory: "/workspace",
			Title:     "Test Session",
			Version:   "1.0.0",
			Time: SessionTime{
				Created: 1234567890,
				Updated: 1234567890,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	session, err := client.CreateSession(context.Background(), &CreateSessionRequest{
		Title: "Test Session",
	})

	require.NoError(t, err)
	assert.Equal(t, "ses_123", session.ID)
	assert.Equal(t, "Test Session", session.Title)
}

func TestOpenCodeClient_GetSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/session/ses_123", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		resp := Session{
			ID:        "ses_123",
			ProjectID: "proj_1",
			Directory: "/workspace",
			Title:     "Test Session",
			Version:   "1.0.0",
			Time: SessionTime{
				Created: 1234567890,
				Updated: 1234567890,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	session, err := client.GetSession(context.Background(), "ses_123")

	require.NoError(t, err)
	assert.Equal(t, "ses_123", session.ID)
	assert.Equal(t, "Test Session", session.Title)
}

func TestOpenCodeClient_DeleteSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/session/ses_123", r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(true)
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	err := client.DeleteSession(context.Background(), "ses_123")

	require.NoError(t, err)
}

func TestOpenCodeClient_Prompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/session/ses_123/message", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		// PromptPart가 인터페이스라서 직접 역직렬화 불가
		// 대신 원시 데이터로 확인
		var rawReq map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&rawReq)
		require.NoError(t, err)

		model, ok := rawReq["model"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "anthropic", model["providerID"])
		assert.Equal(t, "claude-3-5-sonnet-20241022", model["modelID"])

		parts, ok := rawReq["parts"].([]interface{})
		require.True(t, ok)
		assert.Len(t, parts, 1)

		resp := PromptResponse{
			Info: AssistantMessage{
				ID:         "msg_123",
				SessionID:  "ses_123",
				Role:       "assistant",
				ParentID:   "msg_000",
				ModelID:    "claude-3-5-sonnet-20241022",
				ProviderID: "anthropic",
				Mode:       "code",
				Path: MessagePath{
					Cwd:  "/workspace",
					Root: "/workspace",
				},
				Time: MessageTime{
					Created: 1234567890,
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
					SessionID: "ses_123",
					MessageID: "msg_123",
					Type:      "text",
					Text:      "Hello! How can I help you?",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	resp, err := client.Message(context.Background(), "ses_123", &PromptRequest{
		Model: &PromptModel{
			ProviderID: "anthropic",
			ModelID:    "claude-3-5-sonnet-20241022",
		},
		Parts: []PromptPart{
			TextPartInput{
				Type: "text",
				Text: "Hello",
			},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "msg_123", resp.Info.ID)
	assert.Len(t, resp.Parts, 1)
	assert.Equal(t, "Hello! How can I help you?", resp.Parts[0].Text)
}

func TestOpenCodeClient_GetMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/session/ses_123/message", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		resp := []map[string]interface{}{
			{
				"info": map[string]interface{}{
					"id":        "msg_001",
					"sessionID": "ses_123",
					"role":      "user",
					"time": map[string]interface{}{
						"created": float64(1234567890),
					},
					"agent": "code",
					"model": map[string]interface{}{
						"providerID": "anthropic",
						"modelID":    "claude-3-5-sonnet-20241022",
					},
				},
				"parts": []interface{}{
					map[string]interface{}{
						"id":        "prt_001",
						"sessionID": "ses_123",
						"messageID": "msg_001",
						"type":      "text",
						"text":      "Hello",
					},
				},
			},
			{
				"info": map[string]interface{}{
					"id":         "msg_002",
					"sessionID":  "ses_123",
					"role":       "assistant",
					"parentID":   "msg_001",
					"modelID":    "claude-3-5-sonnet-20241022",
					"providerID": "anthropic",
					"mode":       "code",
					"path": map[string]interface{}{
						"cwd":  "/workspace",
						"root": "/workspace",
					},
					"time": map[string]interface{}{
						"created": float64(1234567891),
					},
					"cost": 0.01,
					"tokens": map[string]interface{}{
						"input":     float64(100),
						"output":    float64(200),
						"reasoning": float64(0),
						"cache": map[string]interface{}{
							"read":  float64(0),
							"write": float64(0),
						},
					},
				},
				"parts": []interface{}{
					map[string]interface{}{
						"id":        "prt_002",
						"sessionID": "ses_123",
						"messageID": "msg_002",
						"type":      "text",
						"text":      "Hi there!",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	messages, err := client.GetMessages(context.Background(), "ses_123", nil)

	require.NoError(t, err)
	assert.Len(t, messages, 2)

	// 첫 번째 메시지는 UserMessage
	_, ok := messages[0].Info.(UserMessage)
	assert.True(t, ok, "첫 번째 메시지는 UserMessage여야 함")

	// 두 번째 메시지는 AssistantMessage
	_, ok = messages[1].Info.(AssistantMessage)
	assert.True(t, ok, "두 번째 메시지는 AssistantMessage여야 함")
}

func TestOpenCodeClient_GetPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/path", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		resp := PathInfo{
			Home:      "/home/user",
			State:     "/home/user/.opencode",
			Config:    "/home/user/.config/opencode",
			Worktree:  "/workspace",
			Directory: "/workspace",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	path, err := client.GetPath(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "/workspace", path.Worktree)
	assert.Equal(t, "/workspace", path.Directory)
}

func TestOpenCodeClient_ErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(BadRequestError{
			Success: false,
			Errors: []map[string]interface{}{
				{"message": "Invalid request"},
			},
		})
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	_, err := client.CreateSession(context.Background(), &CreateSessionRequest{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "잘못된 요청")
}

func TestOpenCodeClient_NotFoundError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(NotFoundError{
			Name: "NotFoundError",
			Data: map[string]interface{}{
				"message": "Session not found",
			},
		})
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	_, err := client.GetSession(context.Background(), "ses_nonexistent")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Session not found")
}

func TestOpenCodeClient_WithDirectory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// directory 쿼리 파라미터 확인
		assert.Equal(t, "/workspace/project", r.URL.Query().Get("directory"))

		resp := Session{
			ID:        "ses_123",
			ProjectID: "proj_1",
			Directory: "/workspace/project",
			Title:     "Test Session",
			Version:   "1.0.0",
			Time: SessionTime{
				Created: 1234567890,
				Updated: 1234567890,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL, WithOpenCodeDirectory("/workspace/project"))
	session, err := client.CreateSession(context.Background(), &CreateSessionRequest{
		Title: "Test Session",
	})

	require.NoError(t, err)
	assert.Equal(t, "ses_123", session.ID)
}

func TestOpenCodeClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 요청이 취소될 때까지 대기
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewOpenCodeClient(server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 즉시 취소

	_, err := client.CreateSession(ctx, &CreateSessionRequest{
		Title: "Test",
	})

	require.Error(t, err)
}
