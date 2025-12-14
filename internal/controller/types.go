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
	Type     string // "execute", "cancel"
	TaskID   string
	ThreadID string // Discord thread ID
	Prompt   string // 사용자 메시지 (optional)
}

// ControllerEvent는 Controller에서 Connector로 전송되는 이벤트(결과)를 나타냅니다.
type ControllerEvent struct {
	TaskID   string
	ThreadID string
	Status   string // "completed", "failed"
	Content  string
	Error    error
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