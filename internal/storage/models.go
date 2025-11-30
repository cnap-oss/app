package storage

import "time"

// Agent는 agents 테이블 레코드를 나타냅니다.
type Agent struct {
	ID          int64     `gorm:"column:id;type:bigserial;primaryKey"`
	AgentID     string    `gorm:"column:agent_id;type:varchar(64);not null;uniqueIndex:idx_agents_agent_id"`
	Description string    `gorm:"column:description;type:text"`
	Provider    string    `gorm:"column:provider;type:varchar(32);not null;default:'opencode'"`
	Model       string    `gorm:"column:model;type:varchar(64)"`
	Prompt      string    `gorm:"column:prompt;type:text"`
	Status      string    `gorm:"column:status;type:varchar(32);not null;default:'active'"`
	CreatedAt   time.Time `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null;autoUpdateTime"`
}

// TableName은 gorm Tabler 인터페이스를 구현합니다.
func (Agent) TableName() string {
	return "agents"
}

// Task는 tasks 테이블 레코드를 나타냅니다.
type Task struct {
	ID        int64     `gorm:"column:id;type:bigserial;primaryKey"`
	TaskID    string    `gorm:"column:task_id;type:varchar(64);not null;uniqueIndex:idx_tasks_task_id"`
	AgentID   string    `gorm:"column:agent_id;type:varchar(64);not null;index:idx_tasks_agent_id"`
	Prompt    string    `gorm:"column:prompt;type:text"`
	Status    string    `gorm:"column:status;type:varchar(32);not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;autoUpdateTime"`
}

// TableName은 gorm Tabler 인터페이스를 구현합니다.
func (Task) TableName() string {
	return "tasks"
}

// MessageIndex는 작업별 메시지 파일 경로를 추적합니다.
type MessageIndex struct {
	ID                int64     `gorm:"column:id;type:bigserial;primaryKey"`
	TaskID            string    `gorm:"column:task_id;type:varchar(64);not null;index:idx_msg_index_task;uniqueIndex:idx_msg_idx_task_conv,priority:1"`
	ConversationIndex int       `gorm:"column:conversation_index;type:int;not null;uniqueIndex:idx_msg_idx_task_conv,priority:2"`
	Role              string    `gorm:"column:role;type:varchar(32);not null"`
	FilePath          string    `gorm:"column:file_path;type:text;not null"`
	CreatedAt         time.Time `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt         time.Time `gorm:"column:updated_at;not null;autoUpdateTime"`
}

// TableName은 gorm Tabler 인터페이스를 구현합니다.
func (MessageIndex) TableName() string {
	return "msg_index"
}

// RunStep은 작업 실행 단계를 기록합니다.
type RunStep struct {
	ID        int64     `gorm:"column:id;type:bigserial;primaryKey"`
	TaskID    string    `gorm:"column:task_id;type:varchar(64);not null;index:idx_run_steps_task;uniqueIndex:idx_run_steps_task_step,priority:1"`
	StepNo    int       `gorm:"column:step_no;type:int;not null;uniqueIndex:idx_run_steps_task_step,priority:2"`
	Type      string    `gorm:"column:type;type:varchar(32);not null"`
	Status    string    `gorm:"column:status;type:varchar(32);not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null;autoCreateTime"`
}

// TableName은 gorm Tabler 인터페이스를 구현합니다.
func (RunStep) TableName() string {
	return "run_steps"
}

// Checkpoint는 작업의 Git 스냅샷 참조를 저장합니다.
type Checkpoint struct {
	ID        int64     `gorm:"column:id;type:bigserial;primaryKey"`
	TaskID    string    `gorm:"column:task_id;type:varchar(64);not null;index:idx_checkpoints_task;uniqueIndex:idx_checkpoints_task_git,priority:1"`
	GitHash   string    `gorm:"column:git_hash;type:varchar(64);not null;uniqueIndex:idx_checkpoints_task_git,priority:2"`
	CreatedAt time.Time `gorm:"column:created_at;not null;autoCreateTime"`
}

// TableName implements gorm's tabler interface.
func (Checkpoint) TableName() string {
	return "checkpoints"
}
