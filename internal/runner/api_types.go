package taskrunner

import "time"

// ======================================
// Health Check
// ======================================

// HealthResponse는 /health 응답입니다.
type HealthResponse struct {
	Status  string `json:"status"`  // "ok" 또는 "error"
	Version string `json:"version"` // OpenCode 버전
}

// ======================================
// Session Management
// ======================================

// CreateSessionRequest는 /api/sessions POST 요청입니다.
type CreateSessionRequest struct {
	Model        string `json:"model,omitempty"`         // AI 모델 (선택)
	SystemPrompt string `json:"system_prompt,omitempty"` // 시스템 프롬프트 (선택)
}

// Session은 OpenCode 세션 정보입니다.
type Session struct {
	ID        string    `json:"id"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"` // "active", "idle", "closed"
}

// CreateSessionResponse는 /api/sessions POST 응답입니다.
type CreateSessionResponse struct {
	Session Session `json:"session"`
}

// ======================================
// Chat API
// ======================================

// ChatRequest는 /api/chat POST 요청입니다.
type ChatRequest struct {
	SessionID string        `json:"session_id,omitempty"` // 세션 ID (선택, 없으면 임시 세션)
	Model     string        `json:"model,omitempty"`      // AI 모델 (선택)
	Messages  []ChatMessage `json:"messages"`             // 대화 메시지
	Stream    bool          `json:"stream,omitempty"`     // 스트리밍 여부
}

// ChatResponse는 /api/chat POST 응답입니다.
type ChatResponse struct {
	ID       string `json:"id"`    // 응답 ID
	Model    string `json:"model"` // 사용된 모델
	Response struct {
		Role    string `json:"role"`    // "assistant"
		Content string `json:"content"` // 응답 내용
	} `json:"response"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	FinishReason string `json:"finish_reason"` // "stop", "length", "tool_use"
}

// ChatStreamEvent는 스트리밍 응답의 개별 이벤트입니다.
type ChatStreamEvent struct {
	Event string `json:"event"` // "message", "done", "error"
	Data  struct {
		Content      string `json:"content,omitempty"`       // 부분 응답
		FinishReason string `json:"finish_reason,omitempty"` // 완료 시
		Error        string `json:"error,omitempty"`         // 에러 시
	} `json:"data"`
}

// ======================================
// Error Response
// ======================================

// APIError는 API 에러 응답입니다.
type APIError struct {
	Error struct {
		Type    string `json:"type"`    // 에러 타입
		Message string `json:"message"` // 에러 메시지
		Code    string `json:"code"`    // 에러 코드
	} `json:"error"`
}

func (e *APIError) String() string {
	return e.Error.Type + ": " + e.Error.Message
}
