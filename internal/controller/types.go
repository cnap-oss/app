package controller

import (
	"context"
	"time"
)

// TaskContext는 Task별 실행 컨텍스트를 관리합니다.
type TaskContext struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// ConnectorEvent는 Connector에서 Controller로 전송되는 이벤트를 나타냅니다.
type ConnectorEvent struct {
	// Type은 이벤트 유형을 나타냅니다.
	// 지원 타입:
	//   - "execute": Task 실행 시작 (새 Task 또는 처음 실행)
	//   - "cancel": Task 취소
	//   - "continue": 기존 Task에 메시지 추가 후 실행 계속 (멀티턴 대화)
	//   - "complete": Task 명시적 완료
	Type      string
	TaskID    string
	AgentName string
	Prompt    string // 사용자 메시지 (optional)
}

// ControllerEventType은 이벤트의 종류를 구분합니다
type ControllerEventType string

const (
	// EventTypeStreamDelta - 스트리밍 중 부분 텍스트 (delta)
	EventTypeStreamDelta ControllerEventType = "stream_delta"

	// EventTypePartComplete - Part 완료 (전체 내용)
	EventTypePartComplete ControllerEventType = "part_complete"

	// EventTypeToolStart - 도구 호출 시작
	EventTypeToolStart ControllerEventType = "tool_start"

	// EventTypeToolProgress - 도구 실행 중
	EventTypeToolProgress ControllerEventType = "tool_progress"

	// EventTypeToolComplete - 도구 완료
	EventTypeToolComplete ControllerEventType = "tool_complete"

	// EventTypeToolError - 도구 에러
	EventTypeToolError ControllerEventType = "tool_error"

	// EventTypeMessageComplete - 메시지 완료
	EventTypeMessageComplete ControllerEventType = "message_complete"

	// EventTypeError - 일반 에러
	EventTypeError ControllerEventType = "error"

	// EventTypeLegacy - 기존 호환용 (Status 필드 사용)
	EventTypeLegacy ControllerEventType = "legacy"
)

// PartType은 메시지 파트의 종류를 구분합니다
type PartType string

const (
	PartTypeText      PartType = "text"
	PartTypeReasoning PartType = "reasoning"
	PartTypeTool      PartType = "tool"
	PartTypeFile      PartType = "file"
	PartTypeSnapshot  PartType = "snapshot"
)

// ToolEventInfo는 도구 관련 이벤트 정보를 담습니다
type ToolEventInfo struct {
	ToolName string         `json:"tool_name"`
	CallID   string         `json:"call_id"`
	Input    map[string]any `json:"input,omitempty"`
	Output   string         `json:"output,omitempty"`
	Error    string         `json:"error,omitempty"`
}

// ControllerEvent는 Controller에서 Connector로 전송되는 이벤트(결과)를 나타냅니다.
type ControllerEvent struct {
	// 기존 필드 (하위 호환성 유지)
	TaskID string `json:"task_id"`
	// Status는 이벤트 상태를 나타냅니다.
	// 지원 값:
	//   - "message": 중간 응답 (Runner가 생성한 응답, 실시간 전달용)
	//   - "completed": Task 완료
	//   - "failed": Task 실패
	//   - "canceled": Task 취소
	Status  string `json:"status"`   // legacy 호환
	Content string `json:"content"`
	Error   error  `json:"error,omitempty"`

	// 새로 추가되는 필드
	EventType ControllerEventType `json:"event_type"`
	MessageID string              `json:"message_id,omitempty"`  // OpenCode 메시지 ID
	PartID    string              `json:"part_id,omitempty"`     // OpenCode Part ID
	PartType  PartType            `json:"part_type,omitempty"`   // text, tool, reasoning 등
	Delta     string              `json:"delta,omitempty"`       // 부분 업데이트 텍스트
	IsPartial bool                `json:"is_partial,omitempty"`  // 부분 업데이트 여부
	ToolInfo  *ToolEventInfo      `json:"tool_info,omitempty"`   // 도구 관련 정보
}

// IsStreamingEvent는 스트리밍 중인 이벤트인지 확인합니다
func (e ControllerEvent) IsStreamingEvent() bool {
	return e.EventType == EventTypeStreamDelta || e.IsPartial
}

// IsToolEvent는 도구 관련 이벤트인지 확인합니다
func (e ControllerEvent) IsToolEvent() bool {
	return e.EventType == EventTypeToolStart ||
		e.EventType == EventTypeToolProgress ||
		e.EventType == EventTypeToolComplete ||
		e.EventType == EventTypeToolError
}

// IsTerminalEvent는 완료/에러 이벤트인지 확인합니다
func (e ControllerEvent) IsTerminalEvent() bool {
	return e.EventType == EventTypeMessageComplete ||
		e.EventType == EventTypeError
}

// AgentInfo는 에이전트 정보를 나타냅니다.
type AgentInfo struct {
	Name        string
	Description string
	Provider    string
	Model       string
	Prompt      string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TaskInfo는 작업 정보를 나타냅니다.
type TaskInfo struct {
	TaskID    string
	AgentID   string
	Prompt    string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}
