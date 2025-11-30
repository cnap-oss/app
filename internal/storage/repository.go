package storage

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository는 CNAP 도메인 객체를 위한 영속성 헬퍼를 제공합니다.
type Repository struct {
	db *gorm.DB
}

// NewRepository는 전달된 gorm DB를 이용해 Repository를 생성합니다.
func NewRepository(db *gorm.DB) (*Repository, error) {
	if db == nil {
		return nil, fmt.Errorf("storage: repository requires a non-nil db handle")
	}
	return &Repository{db: db}, nil
}

// DB는 내부 gorm DB 참조를 반환합니다.
func (r *Repository) DB() *gorm.DB {
	return r.db
}

// CreateAgent는 새로운 에이전트 레코드를 저장합니다.
func (r *Repository) CreateAgent(ctx context.Context, agent *Agent) error {
	if agent == nil {
		return fmt.Errorf("storage: nil agent payload")
	}
	return r.db.WithContext(ctx).Create(agent).Error
}

// UpsertAgentStatus는 agentID로 에이전트 상태를 갱신하거나 생성합니다.
func (r *Repository) UpsertAgentStatus(ctx context.Context, agentID, status string) error {
	if agentID == "" {
		return fmt.Errorf("storage: empty agentID")
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "agent_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"status", "updated_at"}),
		}).
		Create(&Agent{
			AgentID: agentID,
			Status:  status,
		}).Error
}

// GetAgent는 식별자로 에이전트를 조회합니다.
func (r *Repository) GetAgent(ctx context.Context, agentID string) (*Agent, error) {
	if agentID == "" {
		return nil, fmt.Errorf("storage: empty agentID")
	}
	var agent Agent
	if err := r.db.WithContext(ctx).
		Where("agent_id = ?", agentID).
		First(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

// ListAgents는 상태 필터를 적용해 에이전트 목록을 반환합니다.
func (r *Repository) ListAgents(ctx context.Context, statuses ...string) ([]Agent, error) {
	q := r.db.WithContext(ctx).Model(&Agent{})
	if len(statuses) > 0 {
		q = q.Where("status IN ?", statuses)
	}
	var agents []Agent
	if err := q.Order("created_at ASC").Find(&agents).Error; err != nil {
		return nil, err
	}
	return agents, nil
}

// UpdateAgent는 에이전트 정보를 업데이트합니다.
func (r *Repository) UpdateAgent(ctx context.Context, agent *Agent) error {
	if agent == nil {
		return fmt.Errorf("storage: nil agent payload")
	}
	if agent.AgentID == "" {
		return fmt.Errorf("storage: empty agentID")
	}
	return r.db.WithContext(ctx).
		Model(&Agent{}).
		Where("agent_id = ?", agent.AgentID).
		Updates(map[string]interface{}{
			"description": agent.Description,
			"provider":    agent.Provider,
			"model":       agent.Model,
			"prompt":      agent.Prompt,
			"updated_at":  time.Now(),
		}).Error
}

// CreateTask는 새로운 작업 레코드를 추가합니다.
func (r *Repository) CreateTask(ctx context.Context, task *Task) error {
	if task == nil {
		return fmt.Errorf("storage: nil task payload")
	}
	return r.db.WithContext(ctx).Create(task).Error
}

// UpsertTaskStatus는 작업 레코드를 만들거나 상태를 갱신합니다.
func (r *Repository) UpsertTaskStatus(ctx context.Context, taskID, agentID, status string) error {
	if taskID == "" {
		return fmt.Errorf("storage: empty taskID")
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "task_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"status", "updated_at"}),
		}).
		Create(&Task{
			TaskID:  taskID,
			AgentID: agentID,
			Status:  status,
		}).Error
}

// GetTask는 작업 식별자로 레코드를 조회합니다.
func (r *Repository) GetTask(ctx context.Context, taskID string) (*Task, error) {
	var task Task
	if err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

// ListTasksByAgent는 에이전트별 작업 목록을 반환합니다.
func (r *Repository) ListTasksByAgent(ctx context.Context, agentID string) ([]Task, error) {
	var tasks []Task
	if err := r.db.WithContext(ctx).
		Where("agent_id = ?", agentID).
		Order("created_at ASC").
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

// GetNextConversationIndex는 해당 Task의 다음 ConversationIndex를 반환합니다.
func (r *Repository) GetNextConversationIndex(ctx context.Context, taskID string) (int, error) {
	if taskID == "" {
		return 0, fmt.Errorf("storage: empty taskID")
	}
	var maxIndex struct {
		MaxIndex *int
	}
	if err := r.db.WithContext(ctx).
		Model(&MessageIndex{}).
		Select("MAX(conversation_index) as max_index").
		Where("task_id = ?", taskID).
		Scan(&maxIndex).Error; err != nil {
		return 0, err
	}
	if maxIndex.MaxIndex == nil {
		return 0, nil
	}
	return *maxIndex.MaxIndex + 1, nil
}

// AppendMessageIndex는 새로운 메시지를 대화에 추가합니다 (ConversationIndex 자동 증가).
func (r *Repository) AppendMessageIndex(ctx context.Context, taskID, role, filePath string) (*MessageIndex, error) {
	if taskID == "" {
		return nil, fmt.Errorf("storage: empty taskID")
	}
	if role == "" {
		return nil, fmt.Errorf("storage: empty role")
	}
	if filePath == "" {
		return nil, fmt.Errorf("storage: empty filePath")
	}

	nextIndex, err := r.GetNextConversationIndex(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("storage: failed to get next conversation index: %w", err)
	}

	payload := &MessageIndex{
		TaskID:            taskID,
		ConversationIndex: nextIndex,
		Role:              role,
		FilePath:          filePath,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}

	if err := r.db.WithContext(ctx).Create(payload).Error; err != nil {
		return nil, err
	}

	return payload, nil
}

// ListMessageIndexByTask는 작업에 연결된 메시지 참조 목록을 순서대로 반환합니다.
func (r *Repository) ListMessageIndexByTask(ctx context.Context, taskID string) ([]MessageIndex, error) {
	var rows []MessageIndex
	if err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("conversation_index ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// UpsertRunStep은 실행 단계를 생성하거나 갱신합니다.
func (r *Repository) UpsertRunStep(ctx context.Context, step *RunStep) error {
	if step == nil {
		return fmt.Errorf("storage: nil run step payload")
	}
	if step.CreatedAt.IsZero() {
		step.CreatedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "task_id"}, {Name: "step_no"}},
			DoUpdates: clause.AssignmentColumns([]string{"type", "status"}),
		}).
		Create(step).Error
}

// ListRunSteps는 작업별 실행 단계 목록을 번호 순으로 반환합니다.
func (r *Repository) ListRunSteps(ctx context.Context, taskID string) ([]RunStep, error) {
	var steps []RunStep
	if err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("step_no ASC").
		Find(&steps).Error; err != nil {
		return nil, err
	}
	return steps, nil
}

// CreateCheckpoint는 작업에 대한 체크포인트를 기록합니다.
func (r *Repository) CreateCheckpoint(ctx context.Context, checkpoint *Checkpoint) error {
	if checkpoint == nil {
		return fmt.Errorf("storage: nil checkpoint payload")
	}
	if checkpoint.CreatedAt.IsZero() {
		checkpoint.CreatedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "task_id"}, {Name: "git_hash"}},
			DoNothing: true,
		}).
		Create(checkpoint).Error
}

// ListCheckpoints는 작업별 체크포인트를 생성 시간 순으로 반환합니다.
func (r *Repository) ListCheckpoints(ctx context.Context, taskID string) ([]Checkpoint, error) {
	var checkpoints []Checkpoint
	if err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("created_at ASC").
		Find(&checkpoints).Error; err != nil {
		return nil, err
	}
	return checkpoints, nil
}
