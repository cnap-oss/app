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
	Type     string
	TaskID   string
	ThreadID string // Discord thread ID
	Prompt   string // 사용자 메시지 (optional)
}

// ControllerEvent는 Controller에서 Connector로 전송되는 이벤트(결과)를 나타냅니다.
type ControllerEvent struct {
	TaskID   string
	ThreadID string
	// Status는 이벤트 상태를 나타냅니다.
	// 지원 값:
	//   - "message": 중간 응답 (Runner가 생성한 응답, 실시간 전달용)
	//   - "completed": Task 완료
	//   - "failed": Task 실패
	//   - "canceled": Task 취소
	Status  string
	Content string
	Error   error
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