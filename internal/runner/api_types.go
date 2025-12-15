package taskrunner

import "time"

// ======================================
// Runner Message Types (Phase 1)
// ======================================

// RunnerMessageType SSE 이벤트의 타입을 나타내는 상수
type RunnerMessageType string

const (
	// 텍스트 관련
	MessageTypeText      RunnerMessageType = "text"
	MessageTypeReasoning RunnerMessageType = "reasoning"

	// 도구 관련
	MessageTypeToolCall   RunnerMessageType = "tool_call"
	MessageTypeToolResult RunnerMessageType = "tool_result"

	// 상태 관련
	MessageTypeStatus   RunnerMessageType = "status"
	MessageTypeProgress RunnerMessageType = "progress"

	// 완료 관련
	MessageTypeComplete RunnerMessageType = "complete"
	MessageTypeError    RunnerMessageType = "error"

	// 세션 관련
	MessageTypeSessionCreated RunnerMessageType = "session_created"
	MessageTypeSessionAborted RunnerMessageType = "session_aborted"
)

// RunnerMessage SSE 이벤트를 추상화한 메시지 구조체
type RunnerMessage struct {
	Type       RunnerMessageType `json:"type"`
	SessionID  string            `json:"session_id,omitempty"`
	MessageID  string            `json:"message_id,omitempty"`
	PartID     string            `json:"part_id,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
	Content    string            `json:"content,omitempty"`
	ToolCall   *ToolCallInfo     `json:"tool_call,omitempty"`
	ToolResult *ToolResultInfo   `json:"tool_result,omitempty"`
	Status     string            `json:"status,omitempty"`
	Progress   float64           `json:"progress,omitempty"`
	Error      *MessageErrorInfo `json:"error,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
	RawEvent   *Event            `json:"raw_event,omitempty"`
}

// ToolCallInfo 도구 호출 정보
type ToolCallInfo struct {
	ToolID    string         `json:"tool_id"`
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// ToolResultInfo 도구 결과 정보
type ToolResultInfo struct {
	ToolID   string `json:"tool_id"`
	ToolName string `json:"tool_name"`
	Result   string `json:"result"`
	IsError  bool   `json:"is_error"`
}

// MessageErrorInfo 메시지 에러 정보
type MessageErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// IsText는 메시지가 텍스트 유형인지 확인합니다.
func (m *RunnerMessage) IsText() bool {
	return m.Type == MessageTypeText || m.Type == MessageTypeReasoning
}

// IsToolRelated는 메시지가 도구 관련 유형인지 확인합니다.
func (m *RunnerMessage) IsToolRelated() bool {
	return m.Type == MessageTypeToolCall || m.Type == MessageTypeToolResult
}

// IsTerminal은 메시지가 종료 유형인지 확인합니다.
func (m *RunnerMessage) IsTerminal() bool {
	return m.Type == MessageTypeComplete || m.Type == MessageTypeError || m.Type == MessageTypeSessionAborted
}

// ======================================
// Session Management
// ======================================

// CreateSessionRequest는 /session POST 요청입니다.
type CreateSessionRequest struct {
	ParentID string `json:"parentID,omitempty"` // 부모 세션 ID (패턴: ^ses.*)
	Title    string `json:"title,omitempty"`    // 세션 제목
}

// Session은 OpenCode 세션 정보입니다.
type Session struct {
	ID        string          `json:"id"`                 // 세션 ID (패턴: ^ses.*)
	ProjectID string          `json:"projectID"`          // 프로젝트 ID
	Directory string          `json:"directory"`          // 디렉토리 경로
	ParentID  string          `json:"parentID,omitempty"` // 부모 세션 ID (패턴: ^ses.*)
	Title     string          `json:"title"`              // 세션 제목
	Version   string          `json:"version"`            // 버전
	Time      SessionTime     `json:"time"`               // 시간 정보
	Summary   *SessionSummary `json:"summary,omitempty"`  // 세션 요약
	Share     *SessionShare   `json:"share,omitempty"`    // 공유 정보
	Revert    *SessionRevert  `json:"revert,omitempty"`   // 되돌리기 정보
}

// SessionTime은 세션 시간 정보입니다.
type SessionTime struct {
	Created    int64  `json:"created"`              // 생성 시간 (Unix timestamp)
	Updated    int64  `json:"updated"`              // 업데이트 시간 (Unix timestamp)
	Compacting *int64 `json:"compacting,omitempty"` // 압축 중 시간
	Archived   *int64 `json:"archived,omitempty"`   // 아카이브 시간
}

// SessionSummary는 세션 요약 정보입니다.
type SessionSummary struct {
	Additions int        `json:"additions"` // 추가된 라인 수
	Deletions int        `json:"deletions"` // 삭제된 라인 수
	Files     int        `json:"files"`     // 변경된 파일 수
	Diffs     []FileDiff `json:"diffs"`     // 파일 변경 내역
}

// SessionShare는 세션 공유 정보입니다.
type SessionShare struct {
	URL string `json:"url"` // 공유 URL
}

// SessionRevert는 세션 되돌리기 정보입니다.
type SessionRevert struct {
	MessageID string  `json:"messageID"`          // 메시지 ID
	PartID    *string `json:"partID,omitempty"`   // 파트 ID (패턴: ^prt.*)
	Snapshot  *string `json:"snapshot,omitempty"` // 스냅샷
	Diff      *string `json:"diff,omitempty"`     // 차이
}

// FileDiff는 파일 변경 정보입니다.
type FileDiff struct {
	File      string `json:"file"`      // 파일 경로
	Before    string `json:"before"`    // 변경 전 내용
	After     string `json:"after"`     // 변경 후 내용
	Additions int    `json:"additions"` // 추가된 라인 수
	Deletions int    `json:"deletions"` // 삭제된 라인 수
}

// UpdateSessionRequest는 /session/{sessionID} PATCH 요청입니다.
type UpdateSessionRequest struct {
	Title *string            `json:"title,omitempty"` // 세션 제목
	Time  *UpdateSessionTime `json:"time,omitempty"`  // 시간 업데이트
}

// UpdateSessionTime은 세션 시간 업데이트 정보입니다.
type UpdateSessionTime struct {
	Archived *int64 `json:"archived,omitempty"` // 아카이브 시간
}

// ======================================
// Message API
// ======================================

// PromptRequest는 /session/{sessionID}/message POST 요청입니다.
type PromptRequest struct {
	MessageID string          `json:"messageID,omitempty"` // 메시지 ID (패턴: ^msg.*)
	Model     *PromptModel    `json:"model,omitempty"`     // 모델 정보
	Agent     string          `json:"agent,omitempty"`     // 에이전트 이름
	NoReply   bool            `json:"noReply,omitempty"`   // 응답 없이 전송
	System    string          `json:"system,omitempty"`    // 시스템 프롬프트
	Tools     map[string]bool `json:"tools,omitempty"`     // 도구 활성화 여부
	Parts     []PromptPart    `json:"parts"`               // 메시지 파트들 (필수)
}

// PromptModel은 AI 모델 정보입니다.
type PromptModel struct {
	ProviderID string `json:"providerID"` // 프로바이더 ID (필수)
	ModelID    string `json:"modelID"`    // 모델 ID (필수)
}

// PromptPart는 메시지 파트 인터페이스입니다.
type PromptPart interface {
	isPromptPart()
}

// TextPartInput은 텍스트 파트입니다.
type TextPartInput struct {
	ID        string                 `json:"id,omitempty"`        // 파트 ID
	Type      string                 `json:"type"`                // "text" (필수)
	Text      string                 `json:"text"`                // 텍스트 내용 (필수)
	Synthetic bool                   `json:"synthetic,omitempty"` // 합성 여부
	Ignored   bool                   `json:"ignored,omitempty"`   // 무시 여부
	Time      *PartTime              `json:"time,omitempty"`      // 시간 정보
	Metadata  map[string]interface{} `json:"metadata,omitempty"`  // 메타데이터
}

func (TextPartInput) isPromptPart() {}

// FilePartInput은 파일 파트입니다.
type FilePartInput struct {
	ID       string          `json:"id,omitempty"`       // 파트 ID
	Type     string          `json:"type"`               // "file" (필수)
	Mime     string          `json:"mime"`               // MIME 타입 (필수)
	Filename string          `json:"filename,omitempty"` // 파일명
	URL      string          `json:"url"`                // 파일 URL (필수)
	Source   *FilePartSource `json:"source,omitempty"`   // 소스 정보
}

func (FilePartInput) isPromptPart() {}

// FilePartSource는 파일 소스 정보입니다.
type FilePartSource struct {
	Text  *FilePartSourceText `json:"text"`            // 텍스트 정보
	Type  string              `json:"type"`            // "file" 또는 "symbol"
	Path  string              `json:"path"`            // 파일 경로
	Range *Range              `json:"range,omitempty"` // 범위 (심볼인 경우)
	Name  string              `json:"name,omitempty"`  // 이름 (심볼인 경우)
	Kind  *int                `json:"kind,omitempty"`  // 종류 (심볼인 경우)
}

// FilePartSourceText는 파일 소스 텍스트 정보입니다.
type FilePartSourceText struct {
	Value string `json:"value"` // 텍스트 값
	Start int    `json:"start"` // 시작 위치
	End   int    `json:"end"`   // 끝 위치
}

// Range는 범위 정보입니다.
type Range struct {
	Start Position `json:"start"` // 시작 위치
	End   Position `json:"end"`   // 끝 위치
}

// Position은 위치 정보입니다.
type Position struct {
	Line      int `json:"line"`      // 라인 번호
	Character int `json:"character"` // 문자 위치
}

// AgentPartInput은 에이전트 파트입니다.
type AgentPartInput struct {
	ID     string              `json:"id,omitempty"`     // 파트 ID
	Type   string              `json:"type"`             // "agent" (필수)
	Name   string              `json:"name"`             // 에이전트 이름 (필수)
	Source *FilePartSourceText `json:"source,omitempty"` // 소스 정보
}

func (AgentPartInput) isPromptPart() {}

// SubtaskPartInput은 서브태스크 파트입니다.
type SubtaskPartInput struct {
	ID          string `json:"id,omitempty"` // 파트 ID
	Type        string `json:"type"`         // "subtask" (필수)
	Prompt      string `json:"prompt"`       // 프롬프트 (필수)
	Description string `json:"description"`  // 설명 (필수)
	Agent       string `json:"agent"`        // 에이전트 (필수)
}

func (SubtaskPartInput) isPromptPart() {}

// PartTime은 파트 시간 정보입니다.
type PartTime struct {
	Start int64  `json:"start"`         // 시작 시간
	End   *int64 `json:"end,omitempty"` // 끝 시간
}

// PromptResponse는 /session/{sessionID}/message POST 응답입니다.
type PromptResponse struct {
	Info  AssistantMessage `json:"info"`  // 메시지 정보
	Parts []Part           `json:"parts"` // 파트들
}

// ======================================
// Message Types
// ======================================

// Message는 메시지 인터페이스입니다.
type Message interface {
	isMessage()
}

// UserMessage는 사용자 메시지입니다.
type UserMessage struct {
	ID        string          `json:"id"`                // 메시지 ID
	SessionID string          `json:"sessionID"`         // 세션 ID
	Role      string          `json:"role"`              // "user"
	Time      MessageTime     `json:"time"`              // 시간 정보
	Summary   *MessageSummary `json:"summary,omitempty"` // 요약 정보
	Agent     string          `json:"agent"`             // 에이전트
	Model     PromptModel     `json:"model"`             // 모델
	System    string          `json:"system,omitempty"`  // 시스템 프롬프트
	Tools     map[string]bool `json:"tools,omitempty"`   // 도구
}

func (UserMessage) isMessage() {}

// AssistantMessage는 어시스턴트 메시지입니다.
type AssistantMessage struct {
	ID         string        `json:"id"`                // 메시지 ID
	SessionID  string        `json:"sessionID"`         // 세션 ID
	Role       string        `json:"role"`              // "assistant"
	Time       MessageTime   `json:"time"`              // 시간 정보
	Error      *MessageError `json:"error,omitempty"`   // 에러 정보
	ParentID   string        `json:"parentID"`          // 부모 메시지 ID
	ModelID    string        `json:"modelID"`           // 모델 ID
	ProviderID string        `json:"providerID"`        // 프로바이더 ID
	Mode       string        `json:"mode"`              // 모드
	Path       MessagePath   `json:"path"`              // 경로 정보
	Summary    *bool         `json:"summary,omitempty"` // 요약 여부
	Cost       float64       `json:"cost"`              // 비용
	Tokens     MessageTokens `json:"tokens"`            // 토큰 정보
	Finish     string        `json:"finish,omitempty"`  // 완료 이유
}

func (AssistantMessage) isMessage() {}

// MessageTime은 메시지 시간 정보입니다.
type MessageTime struct {
	Created   int64  `json:"created"`             // 생성 시간
	Completed *int64 `json:"completed,omitempty"` // 완료 시간
}

// MessageSummary는 메시지 요약 정보입니다.
type MessageSummary struct {
	Title string     `json:"title"` // 제목
	Body  string     `json:"body"`  // 본문
	Diffs []FileDiff `json:"diffs"` // 파일 변경 내역
}

// MessageError는 메시지 에러 정보입니다.
type MessageError struct {
	Name string                 `json:"name"` // 에러 이름
	Data map[string]interface{} `json:"data"` // 에러 데이터
}

// MessagePath는 메시지 경로 정보입니다.
type MessagePath struct {
	Cwd  string `json:"cwd"`  // 현재 작업 디렉토리
	Root string `json:"root"` // 루트 디렉토리
}

// MessageTokens는 토큰 사용량 정보입니다.
type MessageTokens struct {
	Input     int               `json:"input"`     // 입력 토큰
	Output    int               `json:"output"`    // 출력 토큰
	Reasoning int               `json:"reasoning"` // 추론 토큰
	Cache     MessageTokenCache `json:"cache"`     // 캐시 토큰
}

// MessageTokenCache는 캐시 토큰 정보입니다.
type MessageTokenCache struct {
	Read  int `json:"read"`  // 읽기 토큰
	Write int `json:"write"` // 쓰기 토큰
}

// Part는 메시지 파트입니다.
type Part struct {
	ID        string                 `json:"id"`                  // 파트 ID
	SessionID string                 `json:"sessionID"`           // 세션 ID
	MessageID string                 `json:"messageID"`           // 메시지 ID
	Type      string                 `json:"type"`                // 파트 타입
	Text      string                 `json:"text,omitempty"`      // 텍스트 (text, reasoning 타입)
	Synthetic bool                   `json:"synthetic,omitempty"` // 합성 여부
	Ignored   bool                   `json:"ignored,omitempty"`   // 무시 여부
	Time      *PartTime              `json:"time,omitempty"`      // 시간 정보
	Metadata  map[string]interface{} `json:"metadata,omitempty"`  // 메타데이터

	// Tool 파트 전용
	CallID string     `json:"callID,omitempty"` // 호출 ID
	Tool   string     `json:"tool,omitempty"`   // 도구 이름
	State  *ToolState `json:"state,omitempty"`  // 도구 상태

	// File 파트 전용
	Mime     string          `json:"mime,omitempty"`     // MIME 타입
	Filename string          `json:"filename,omitempty"` // 파일명
	URL      string          `json:"url,omitempty"`      // URL
	Source   *FilePartSource `json:"source,omitempty"`   // 소스

	// Subtask 파트 전용
	Prompt      string `json:"prompt,omitempty"`      // 프롬프트
	Description string `json:"description,omitempty"` // 설명
	Agent       string `json:"agent,omitempty"`       // 에이전트

	// Agent 파트 전용
	Name string `json:"name,omitempty"` // 이름

	// Step 파트 전용
	Snapshot string         `json:"snapshot,omitempty"` // 스냅샷
	Reason   string         `json:"reason,omitempty"`   // 이유
	Cost     float64        `json:"cost,omitempty"`     // 비용
	Tokens   *MessageTokens `json:"tokens,omitempty"`   // 토큰

	// Patch 파트 전용
	Hash  string   `json:"hash,omitempty"`  // 해시
	Files []string `json:"files,omitempty"` // 파일들

	// Retry 파트 전용
	Attempt int           `json:"attempt,omitempty"` // 시도 횟수
	Error   *MessageError `json:"error,omitempty"`   // 에러

	// Compaction 파트 전용
	Auto bool `json:"auto,omitempty"` // 자동 여부
}

// ToolState는 도구 상태입니다.
type ToolState struct {
	Status      string                 `json:"status"`                // "pending", "running", "completed", "error"
	Input       map[string]interface{} `json:"input"`                 // 입력
	Raw         string                 `json:"raw,omitempty"`         // 원시 데이터
	Title       string                 `json:"title,omitempty"`       // 제목
	Output      string                 `json:"output,omitempty"`      // 출력
	Error       string                 `json:"error,omitempty"`       // 에러
	Metadata    map[string]interface{} `json:"metadata,omitempty"`    // 메타데이터
	Time        *ToolStateTime         `json:"time,omitempty"`        // 시간
	Attachments []Part                 `json:"attachments,omitempty"` // 첨부파일
}

// ToolStateTime은 도구 상태 시간 정보입니다.
type ToolStateTime struct {
	Start     int64  `json:"start"`               // 시작 시간
	End       *int64 `json:"end,omitempty"`       // 종료 시간
	Compacted *int64 `json:"compacted,omitempty"` // 압축 시간
}

// ======================================
// Event Stream
// ======================================

// Event는 SSE 이벤트입니다.
type Event struct {
	Type       string                 `json:"type"`       // 이벤트 타입
	Properties map[string]interface{} `json:"properties"` // 이벤트 속성
}

// ======================================
// Path API
// ======================================

// PathInfo는 /path GET 응답입니다.
type PathInfo struct {
	Home      string `json:"home"`      // 홈 디렉토리
	State     string `json:"state"`     // 상태 디렉토리
	Config    string `json:"config"`    // 설정 디렉토리
	Worktree  string `json:"worktree"`  // 작업 트리 디렉토리
	Directory string `json:"directory"` // 현재 디렉토리
}

// ======================================
// Error Response
// ======================================

// BadRequestError는 400 에러 응답입니다.
type BadRequestError struct {
	Data    interface{}              `json:"data"`    // 데이터
	Errors  []map[string]interface{} `json:"errors"`  // 에러 목록
	Success bool                     `json:"success"` // 성공 여부 (항상 false)
}

// NotFoundError는 404 에러 응답입니다.
type NotFoundError struct {
	Name string                 `json:"name"` // "NotFoundError"
	Data map[string]interface{} `json:"data"` // 에러 데이터
}

// APIError는 일반 API 에러입니다.
type APIError struct {
	StatusCode int    `json:"status_code"` // HTTP 상태 코드
	Message    string `json:"message"`     // 에러 메시지
	Body       string `json:"body"`        // 응답 본문
}

func (e *APIError) Error() string {
	return e.Message
}

// ======================================
// Helper Types
// ======================================

// ChatMessage는 간단한 채팅 메시지입니다 (호환성용).
type ChatMessage struct {
	Role    string `json:"role"`    // "user" 또는 "assistant"
	Content string `json:"content"` // 메시지 내용
}
