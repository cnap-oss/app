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

// TaskEvent는 Task 실행 이벤트를 나타냅니다.
type TaskEvent struct {
	Type     string // "execute", "cancel"
	TaskID   string
	ThreadID string // Discord thread ID
	Prompt   string // 사용자 메시지 (optional)
}

// TaskResult는 Task 실행 결과를 나타냅니다.
type TaskResult struct {
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