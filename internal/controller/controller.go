package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	taskrunner "github.com/cnap-oss/app/internal/runner"
	"github.com/cnap-oss/app/internal/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
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

// Controller는 에이전트 생성 및 관리를 담당하며, supervisor 기능도 포함합니다.
type Controller struct {
	logger         *zap.Logger
	repo           *storage.Repository
	runnerManager  *taskrunner.RunnerManager
	taskContexts   map[string]*TaskContext
	mu             sync.RWMutex
	taskEventChan  chan TaskEvent
	taskResultChan chan TaskResult
}

// NewController는 새로운 Controller를 생성합니다.
func NewController(logger *zap.Logger, repo *storage.Repository, eventChan chan TaskEvent, resultChan chan TaskResult) *Controller {
	return &Controller{
		logger:         logger,
		repo:           repo,
		runnerManager:  taskrunner.GetRunnerManager(),
		taskContexts:   make(map[string]*TaskContext),
		taskEventChan:  eventChan,
		taskResultChan: resultChan,
	}
}

// Start는 controller 서버를 시작합니다.
func (c *Controller) Start(ctx context.Context) error {
	c.logger.Info("Starting controller server")

	// 이벤트 루프 시작 (별도 goroutine)
	go c.eventLoop(ctx)

	// 하트비트
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Controller server shutting down")
			return ctx.Err()
		case <-ticker.C:
			c.logger.Debug("Controller heartbeat")
		}
	}
}

// Stop은 controller 서버를 정상적으로 종료합니다.
func (c *Controller) Stop(ctx context.Context) error {
	c.logger.Info("Stopping controller server")

	// 정리 작업 수행
	select {
	case <-ctx.Done():
		return fmt.Errorf("shutdown timeout exceeded")
	case <-time.After(100 * time.Millisecond):
		c.logger.Info("Controller server stopped")
		return nil
	}
}

// CreateAgent는 새로운 에이전트를 생성합니다.
func (c *Controller) CreateAgent(ctx context.Context, agentID, description, provider, model, prompt string) error {
	c.logger.Info("Creating agent",
		zap.String("agent_id", agentID),
		zap.String("provider", provider),
		zap.String("model", model),
	)

	if c.repo == nil {
		return fmt.Errorf("controller: repository is not configured")
	}

	payload := &storage.Agent{
		AgentID:     agentID,
		Description: description,
		Provider:    provider,
		Model:       model,
		Prompt:      prompt,
		Status:      storage.AgentStatusActive,
	}

	if err := c.repo.CreateAgent(ctx, payload); err != nil {
		c.logger.Error("Failed to persist agent", zap.Error(err))
		return err
	}

	c.logger.Info("Agent created successfully",
		zap.String("agent", agentID),
		zap.Int64("id", payload.ID),
	)
	return nil
}

// DeleteAgent는 기존 에이전트를 삭제합니다.
func (c *Controller) DeleteAgent(ctx context.Context, agent string) error {
	c.logger.Info("Deleting agent",
		zap.String("agent", agent),
	)

	if c.repo == nil {
		return fmt.Errorf("controller: repository is not configured")
	}

	if err := c.repo.UpsertAgentStatus(ctx, agent, storage.AgentStatusDeleted); err != nil {
		return err
	}

	c.logger.Info("Agent deleted successfully",
		zap.String("agent", agent),
	)
	return nil
}

// ListAgents는 모든 에이전트 목록을 반환합니다.
func (c *Controller) ListAgents(ctx context.Context) ([]string, error) {
	c.logger.Info("Listing agents")

	if c.repo == nil {
		return nil, fmt.Errorf("controller: repository is not configured")
	}

	records, err := c.repo.ListAgents(ctx)
	if err != nil {
		return nil, err
	}

	agents := make([]string, 0, len(records))
	for _, rec := range records {
		agents = append(agents, rec.AgentID)
	}

	c.logger.Info("Listed agents",
		zap.Int("count", len(agents)),
	)
	return agents, nil
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

// GetAgentInfo는 특정 에이전트의 정보를 반환합니다.
func (c *Controller) GetAgentInfo(ctx context.Context, agent string) (*AgentInfo, error) {
	c.logger.Info("Getting agent info",
		zap.String("agent", agent),
	)

	if c.repo == nil {
		return nil, fmt.Errorf("controller: repository is not configured")
	}

	rec, err := c.repo.GetAgent(ctx, agent)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("agent not found: %s", agent)
		}
		return nil, err
	}

	info := &AgentInfo{
		Name:        rec.AgentID,
		Description: rec.Description,
		Provider:    rec.Provider,
		Model:       rec.Model,
		Prompt:      rec.Prompt,
		Status:      rec.Status,
		CreatedAt:   rec.CreatedAt,
		UpdatedAt:   rec.UpdatedAt,
	}

	c.logger.Info("Retrieved agent info",
		zap.String("agent", agent),
		zap.String("status", info.Status),
	)
	return info, nil
}

// ValidateAgent는 에이전트 이름의 유효성을 검증합니다.
func (c *Controller) ValidateAgent(agent string) error {
	if agent == "" {
		return fmt.Errorf("agent name cannot be empty")
	}

	if len(agent) > 64 {
		return fmt.Errorf("agent name too long (max 64 characters)")
	}

	// 추가 검증 로직
	return nil
}

// CreateTask는 프롬프트와 함께 새로운 작업을 생성합니다.
// 생성 후 SendMessage를 호출하기 전까지 실행되지 않습니다.
func (c *Controller) CreateTask(ctx context.Context, agentID, taskID, prompt string) error {
	c.logger.Info("Creating task",
		zap.String("agent_id", agentID),
		zap.String("task_id", taskID),
	)

	if c.repo == nil {
		return fmt.Errorf("controller: repository is not configured")
	}

	// Agent 존재 여부 확인
	if _, err := c.repo.GetAgent(ctx, agentID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("agent not found: %s", agentID)
		}
		return err
	}

	task := &storage.Task{
		TaskID:  taskID,
		AgentID: agentID,
		Prompt:  prompt,
		Status:  storage.TaskStatusPending,
	}

	if err := c.repo.CreateTask(ctx, task); err != nil {
		c.logger.Error("Failed to create task", zap.Error(err))
		return err
	}

	// Agent 정보 조회
	agent, err := c.repo.GetAgent(ctx, agentID)
	if err != nil {
		c.logger.Error("Failed to get agent info", zap.Error(err))
		return err
	}

	// RunnerManager에 TaskRunner 생성 (logger 주입)
	agentInfo := taskrunner.AgentInfo{
		AgentID:  agentID,
		Provider: agent.Provider,
		Model:    agent.Model,
		Prompt:   agent.Prompt,
	}
	runner := c.runnerManager.CreateRunner(taskID, agentInfo, c.logger)
	if runner == nil {
		return fmt.Errorf("failed to create task runner")
	}

	c.logger.Info("Task created successfully",
		zap.String("task_id", taskID),
		zap.String("agent_id", agentID),
		zap.Int64("id", task.ID),
	)
	return nil
}

// GetTask는 작업 정보를 조회합니다.
func (c *Controller) GetTask(ctx context.Context, taskID string) (*storage.Task, error) {
	c.logger.Info("Getting task",
		zap.String("task_id", taskID),
	)

	if c.repo == nil {
		return nil, fmt.Errorf("controller: repository is not configured")
	}

	task, err := c.repo.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("task not found: %s", taskID)
		}
		return nil, err
	}

	c.logger.Info("Retrieved task",
		zap.String("task_id", taskID),
		zap.String("status", task.Status),
	)
	return task, nil
}

// UpdateTaskStatus는 작업 상태를 업데이트합니다.
func (c *Controller) UpdateTaskStatus(ctx context.Context, taskID, status string) error {
	c.logger.Info("Updating task status",
		zap.String("task_id", taskID),
		zap.String("status", status),
	)

	if c.repo == nil {
		return fmt.Errorf("controller: repository is not configured")
	}

	// 작업 존재 여부 확인
	task, err := c.repo.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("task not found: %s", taskID)
		}
		return err
	}

	// 상태 업데이트
	if err := c.repo.UpsertTaskStatus(ctx, taskID, task.AgentID, status); err != nil {
		c.logger.Error("Failed to update task status", zap.Error(err))
		return err
	}

	c.logger.Info("Task status updated successfully",
		zap.String("task_id", taskID),
		zap.String("old_status", task.Status),
		zap.String("new_status", status),
	)
	return nil
}

// ListTasksByAgent는 에이전트별 작업 목록을 반환합니다.
func (c *Controller) ListTasksByAgent(ctx context.Context, agentID string) ([]storage.Task, error) {
	c.logger.Info("Listing tasks by agent",
		zap.String("agent_id", agentID),
	)

	if c.repo == nil {
		return nil, fmt.Errorf("controller: repository is not configured")
	}

	tasks, err := c.repo.ListTasksByAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}

	c.logger.Info("Listed tasks by agent",
		zap.String("agent_id", agentID),
		zap.Int("count", len(tasks)),
	)
	return tasks, nil
}

// DeleteTask는 작업을 삭제합니다 (hard delete).
func (c *Controller) DeleteTask(ctx context.Context, taskID string) error {
	c.logger.Info("Deleting task",
		zap.String("task_id", taskID),
	)

	if c.repo == nil {
		return fmt.Errorf("controller: repository is not configured")
	}

	// Task 존재 여부 확인
	_, err := c.repo.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("task not found: %s", taskID)
		}
		return err
	}

	// Soft delete (status를 deleted로 변경)
	if err := c.repo.DeleteTask(ctx, taskID); err != nil {
		c.logger.Error("Failed to delete task", zap.Error(err))
		return err
	}

	// Runner도 삭제
	c.runnerManager.DeleteRunner(taskID)

	c.logger.Info("Task deleted successfully",
		zap.String("task_id", taskID),
	)
	return nil
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

// GetTaskInfo는 작업의 상세 정보를 반환합니다.
func (c *Controller) GetTaskInfo(ctx context.Context, taskID string) (*TaskInfo, error) {
	c.logger.Info("Getting task info",
		zap.String("task_id", taskID),
	)

	if c.repo == nil {
		return nil, fmt.Errorf("controller: repository is not configured")
	}

	task, err := c.repo.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("task not found: %s", taskID)
		}
		return nil, err
	}

	info := &TaskInfo{
		TaskID:    task.TaskID,
		AgentID:   task.AgentID,
		Prompt:    task.Prompt,
		Status:    task.Status,
		CreatedAt: task.CreatedAt,
		UpdatedAt: task.UpdatedAt,
	}

	c.logger.Info("Retrieved task info",
		zap.String("task_id", taskID),
		zap.String("status", info.Status),
	)
	return info, nil
}

// ValidateTask는 작업 ID의 유효성을 검증합니다.
func (c *Controller) ValidateTask(taskID string) error {
	if taskID == "" {
		return fmt.Errorf("task ID cannot be empty")
	}

	if len(taskID) > 64 {
		return fmt.Errorf("task ID too long (max 64 characters)")
	}

	return nil
}

// UpdateAgent는 에이전트 정보를 수정합니다.
func (c *Controller) UpdateAgent(ctx context.Context, agentID, description, provider, model, prompt string) error {
	c.logger.Info("Updating agent",
		zap.String("agent_id", agentID),
	)

	if c.repo == nil {
		return fmt.Errorf("controller: repository is not configured")
	}

	// Agent 존재 여부 확인
	if _, err := c.repo.GetAgent(ctx, agentID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("agent not found: %s", agentID)
		}
		return err
	}

	agent := &storage.Agent{
		AgentID:     agentID,
		Description: description,
		Provider:    provider,
		Model:       model,
		Prompt:      prompt,
	}

	if err := c.repo.UpdateAgent(ctx, agent); err != nil {
		c.logger.Error("Failed to update agent", zap.Error(err))
		return err
	}

	c.logger.Info("Agent updated successfully", zap.String("agent", agentID))
	return nil
}

// ListAgentsWithInfo는 상세 정보를 포함한 에이전트 목록을 반환합니다.
func (c *Controller) ListAgentsWithInfo(ctx context.Context) ([]*AgentInfo, error) {
	c.logger.Info("Listing agents with info")

	if c.repo == nil {
		return nil, fmt.Errorf("controller: repository is not configured")
	}

	records, err := c.repo.ListAgents(ctx)
	if err != nil {
		return nil, err
	}

	agents := make([]*AgentInfo, 0, len(records))
	for _, rec := range records {
		agents = append(agents, &AgentInfo{
			Name:        rec.AgentID,
			Description: rec.Description,
			Model:       rec.Model,
			Prompt:      rec.Prompt,
			Status:      rec.Status,
			CreatedAt:   rec.CreatedAt,
			UpdatedAt:   rec.UpdatedAt,
		})
	}

	c.logger.Info("Listed agents with info",
		zap.Int("count", len(agents)),
	)
	return agents, nil
}

// AddMessage adds a message to an existing task without executing it.
// The message will be stored and can be sent later using SendMessage.
func (c *Controller) AddMessage(ctx context.Context, taskID, role, content string) error {
	c.logger.Info("Adding message to task",
		zap.String("task_id", taskID),
		zap.String("role", role),
	)

	if c.repo == nil {
		return fmt.Errorf("controller: repository is not configured")
	}

	// Task 존재 여부 확인
	if _, err := c.repo.GetTask(ctx, taskID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("task not found: %s", taskID)
		}
		return err
	}

	// 메시지를 파일로 저장하고 인덱스 생성
	filePath, err := c.saveMessageToFile(ctx, taskID, role, content)
	if err != nil {
		c.logger.Error("Failed to save message to file", zap.Error(err))
		return err
	}

	if _, err := c.repo.AppendMessageIndex(ctx, taskID, role, filePath); err != nil {
		c.logger.Error("Failed to add message", zap.Error(err))
		return err
	}

	c.logger.Info("Message added successfully",
		zap.String("task_id", taskID),
		zap.String("role", role),
	)
	return nil
}

// SendMessage triggers the execution of a task.
// This method should be called after creating a task and optionally adding messages.
// The actual execution will be handled by the RunnerManager (to be implemented).
func (c *Controller) SendMessage(ctx context.Context, taskID string) error {
	c.logger.Info("Sending message for task",
		zap.String("task_id", taskID),
	)

	if c.repo == nil {
		return fmt.Errorf("controller: repository is not configured")
	}

	// Task 조회
	task, err := c.repo.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("task not found: %s", taskID)
		}
		return err
	}

	// 이미 실행 중인 경우 에러
	if task.Status == storage.TaskStatusRunning {
		return fmt.Errorf("task is already running: %s", taskID)
	}

	// 완료된 작업은 재실행 불가
	if task.Status == storage.TaskStatusCompleted || task.Status == storage.TaskStatusFailed {
		return fmt.Errorf("task is already finished: %s (status: %s)", taskID, task.Status)
	}

	// 메시지 목록 조회
	messages, err := c.repo.ListMessageIndexByTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to list messages: %w", err)
	}

	// 프롬프트나 메시지가 없으면 에러
	if task.Prompt == "" && len(messages) == 0 {
		return fmt.Errorf("no prompt or messages to send for task: %s", taskID)
	}

	// 상태를 running으로 변경
	if err := c.repo.UpsertTaskStatus(ctx, taskID, task.AgentID, storage.TaskStatusRunning); err != nil {
		c.logger.Error("Failed to update task status", zap.Error(err))
		return err
	}

	c.logger.Info("Task execution triggered",
		zap.String("task_id", taskID),
		zap.String("agent_id", task.AgentID),
		zap.Int("message_count", len(messages)),
	)

	// Task 실행을 위한 context 생성 (5분 타임아웃)
	taskCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	// TaskContext 저장
	c.mu.Lock()
	c.taskContexts[taskID] = &TaskContext{
		ctx:    taskCtx,
		cancel: cancel,
	}
	c.mu.Unlock()

	// RunnerManager를 통해 실제 실행 트리거
	go c.executeTask(taskCtx, taskID, task)

	return nil
}

// ExecuteTask는 생성된 Task를 실행합니다.
func (c *Controller) ExecuteTask(ctx context.Context, taskID string) error {
	c.logger.Info("Executing task", zap.String("task_id", taskID))

	if c.repo == nil {
		return fmt.Errorf("controller: repository is not configured")
	}

	// Task 조회
	task, err := c.repo.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("task not found: %s", taskID)
		}
		return err
	}

	// 상태 확인 (이미 실행 중이거나 완료된 경우 방지)
	if task.Status != storage.TaskStatusPending {
		return fmt.Errorf("task is not in pending state (current: %s)", task.Status)
	}

	// Runner 확인
	runner := c.runnerManager.GetRunner(taskID)
	if runner == nil {
		return fmt.Errorf("runner not found for task: %s", taskID)
	}

	// context with timeout
	taskCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	c.mu.Lock()
	c.taskContexts[taskID] = &TaskContext{ctx: taskCtx, cancel: cancel}
	c.mu.Unlock()

	// 비동기 실행
	go c.executeTask(taskCtx, taskID, task)

	c.logger.Info("Task execution started",
		zap.String("task_id", taskID),
	)
	return nil
}

// CancelTask는 실행 중인 Task를 취소합니다.
func (c *Controller) CancelTask(ctx context.Context, taskID string) error {
	c.logger.Info("Canceling task",
		zap.String("task_id", taskID),
	)

	if c.repo == nil {
		return fmt.Errorf("controller: repository is not configured")
	}

	// Task 조회
	task, err := c.repo.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("task not found: %s", taskID)
		}
		return err
	}

	// running 상태가 아니면 취소 불가
	if task.Status != storage.TaskStatusRunning {
		return fmt.Errorf("task is not running: %s (status: %s)", taskID, task.Status)
	}

	// TaskContext에서 cancel 호출
	c.mu.RLock()
	taskCtx, ok := c.taskContexts[taskID]
	c.mu.RUnlock()

	if !ok {
		// Context가 없으면 이미 완료되었거나 존재하지 않음
		return fmt.Errorf("task context not found: %s", taskID)
	}

	// Context 취소
	taskCtx.cancel()

	c.logger.Info("Task canceled",
		zap.String("task_id", taskID),
	)

	return nil
}

// eventLoop는 Task 이벤트를 처리하는 메인 루프입니다.
func (c *Controller) eventLoop(ctx context.Context) {
	c.logger.Info("Event loop started")
	defer c.logger.Info("Event loop stopped")

	for {
		select {
		case event := <-c.taskEventChan:
			// 각 이벤트를 별도 goroutine으로 처리 (병렬 처리)
			go c.handleTaskEvent(ctx, event)

		case <-ctx.Done():
			c.logger.Info("Event loop shutting down")
			return
		}
	}
}

// handleTaskEvent는 단일 Task 이벤트를 처리합니다.
func (c *Controller) handleTaskEvent(ctx context.Context, event TaskEvent) {
	c.logger.Info("Handling task event",
		zap.String("type", event.Type),
		zap.String("task_id", event.TaskID),
		zap.String("thread_id", event.ThreadID),
	)

	switch event.Type {
	case "execute":
		c.handleExecuteEvent(ctx, event)
	case "cancel":
		c.handleCancelEvent(ctx, event)
	default:
		c.logger.Warn("Unknown event type",
			zap.String("type", event.Type),
			zap.String("task_id", event.TaskID),
		)
	}
}

// handleExecuteEvent는 Task 실행 이벤트를 처리합니다.
func (c *Controller) handleExecuteEvent(ctx context.Context, event TaskEvent) {
	// Task 조회
	task, err := c.repo.GetTask(ctx, event.TaskID)
	if err != nil {
		c.logger.Error("Failed to get task",
			zap.String("task_id", event.TaskID),
			zap.Error(err),
		)
		c.taskResultChan <- TaskResult{
			TaskID:   event.TaskID,
			ThreadID: event.ThreadID,
			Status:   "failed",
			Error:    fmt.Errorf("task not found: %w", err),
		}
		return
	}

	// 상태를 running으로 변경
	if err := c.repo.UpsertTaskStatus(ctx, event.TaskID, task.AgentID, storage.TaskStatusRunning); err != nil {
		c.logger.Error("Failed to update task status",
			zap.String("task_id", event.TaskID),
			zap.Error(err),
		)
		c.taskResultChan <- TaskResult{
			TaskID:   event.TaskID,
			ThreadID: event.ThreadID,
			Status:   "failed",
			Error:    fmt.Errorf("failed to update status: %w", err),
		}
		return
	}

	// Task 실행 (별도 함수로 분리하여 결과 처리)
	c.executeTaskWithResult(ctx, event.TaskID, event.ThreadID, task)
}

// handleCancelEvent는 Task 취소 이벤트를 처리합니다.
func (c *Controller) handleCancelEvent(ctx context.Context, event TaskEvent) {
	c.logger.Info("Canceling task",
		zap.String("task_id", event.TaskID),
	)

	// TaskContext에서 cancel 호출
	c.mu.RLock()
	taskCtx, ok := c.taskContexts[event.TaskID]
	c.mu.RUnlock()

	if !ok {
		c.logger.Warn("Task context not found for cancellation",
			zap.String("task_id", event.TaskID),
		)
		c.taskResultChan <- TaskResult{
			TaskID:   event.TaskID,
			ThreadID: event.ThreadID,
			Status:   "failed",
			Error:    fmt.Errorf("task not running"),
		}
		return
	}

	// Context 취소
	taskCtx.cancel()

	c.taskResultChan <- TaskResult{
		TaskID:   event.TaskID,
		ThreadID: event.ThreadID,
		Status:   "canceled",
		Content:  "Task canceled by user",
	}
}

// executeTaskWithResult는 Task를 실행하고 결과를 resultChan으로 전송합니다.
func (c *Controller) executeTaskWithResult(ctx context.Context, taskID, threadID string, task *storage.Task) {
	defer func() {
		// TaskContext 정리
		c.mu.Lock()
		if taskCtx, ok := c.taskContexts[taskID]; ok {
			taskCtx.cancel()
			delete(c.taskContexts, taskID)
		}
		c.mu.Unlock()

		if r := recover(); r != nil {
			c.logger.Error("Task execution panicked",
				zap.String("task_id", taskID),
				zap.Any("panic", r),
			)
			_ = c.repo.UpsertTaskStatus(context.Background(), taskID, task.AgentID, storage.TaskStatusFailed)

			c.taskResultChan <- TaskResult{
				TaskID:   taskID,
				ThreadID: threadID,
				Status:   "failed",
				Error:    fmt.Errorf("panic: %v", r),
			}
		}
	}()

	// Task 실행을 위한 context 생성 (5분 타임아웃)
	taskCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	// TaskContext 저장
	c.mu.Lock()
	c.taskContexts[taskID] = &TaskContext{
		ctx:    taskCtx,
		cancel: cancel,
	}
	c.mu.Unlock()

	// Agent 정보 조회
	agent, err := c.repo.GetAgent(ctx, task.AgentID)
	if err != nil {
		c.logger.Error("Failed to get agent info", zap.Error(err))
		_ = c.repo.UpsertTaskStatus(ctx, taskID, task.AgentID, storage.TaskStatusFailed)

		c.taskResultChan <- TaskResult{
			TaskID:   taskID,
			ThreadID: threadID,
			Status:   "failed",
			Error:    fmt.Errorf("agent not found: %w", err),
		}
		return
	}

	// RunnerManager에서 TaskRunner 조회 또는 생성
	runner := c.runnerManager.GetRunner(taskID)
	if runner == nil {
		c.logger.Info("Runner not found, creating...",
			zap.String("task_id", taskID),
		)

		agentInfo := taskrunner.AgentInfo{
			AgentID:  agent.AgentID,
			Provider: agent.Provider,
			Model:    agent.Model,
			Prompt:   agent.Prompt,
		}
		runner = c.runnerManager.CreateRunner(taskID, agentInfo, c.logger)
	}

	// 메시지 목록 조회
	messages, err := c.repo.ListMessageIndexByTask(ctx, taskID)
	if err != nil {
		c.logger.Error("Failed to list messages", zap.Error(err))
		_ = c.repo.UpsertTaskStatus(ctx, taskID, task.AgentID, storage.TaskStatusFailed)

		c.taskResultChan <- TaskResult{
			TaskID:   taskID,
			ThreadID: threadID,
			Status:   "failed",
			Error:    fmt.Errorf("failed to list messages: %w", err),
		}
		return
	}

	// ChatMessage로 변환
	chatMessages := make([]taskrunner.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		content, err := c.readMessageFromFile(msg.FilePath)
		if err != nil {
			c.logger.Warn("Failed to read message file, skipping",
				zap.String("file_path", msg.FilePath),
				zap.Error(err),
			)
			continue
		}

		chatMessages = append(chatMessages, taskrunner.ChatMessage{
			Role:    msg.Role,
			Content: content,
		})
	}

	// Prompt가 있으면 추가
	if task.Prompt != "" {
		chatMessages = append(chatMessages, taskrunner.ChatMessage{
			Role:    "user",
			Content: task.Prompt,
		})
	}

	// RunRequest 구성
	req := &taskrunner.RunRequest{
		TaskID:       taskID,
		Model:        agent.Model,
		SystemPrompt: agent.Prompt,
		Messages:     chatMessages,
	}

	// TaskRunner 실행
	result, err := runner.Run(taskCtx, req)

	// Context 취소/타임아웃 확인
	if taskCtx.Err() != nil {
		c.logger.Warn("Task execution canceled or timed out",
			zap.String("task_id", taskID),
			zap.Error(taskCtx.Err()),
		)

		status := storage.TaskStatusFailed
		if errors.Is(taskCtx.Err(), context.Canceled) {
			status = storage.TaskStatusCanceled
		}
		_ = c.repo.UpsertTaskStatus(context.Background(), taskID, task.AgentID, status)

		c.taskResultChan <- TaskResult{
			TaskID:   taskID,
			ThreadID: threadID,
			Status:   status,
			Error:    taskCtx.Err(),
		}

		c.runnerManager.DeleteRunner(taskID)
		return
	}

	// 실행 결과 처리
	if err != nil {
		c.logger.Error("Task execution failed",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		_ = c.repo.UpsertTaskStatus(context.Background(), taskID, task.AgentID, storage.TaskStatusFailed)

		c.taskResultChan <- TaskResult{
			TaskID:   taskID,
			ThreadID: threadID,
			Status:   "failed",
			Error:    err,
		}
	} else {
		c.logger.Info("Task execution completed",
			zap.String("task_id", taskID),
			zap.Bool("success", result != nil && result.Success),
		)

		// 결과를 파일로 저장
		if result.Success {
			filePath, err := c.saveMessageToFile(context.Background(), taskID, "assistant", result.Output)
			if err != nil {
				c.logger.Error("Failed to save result to file", zap.Error(err))
			} else {
				// MessageIndex에 추가
				if _, err := c.repo.AppendMessageIndex(context.Background(), taskID, "assistant", filePath); err != nil {
					c.logger.Error("Failed to append message index", zap.Error(err))
				}
			}
		}

		_ = c.repo.UpsertTaskStatus(context.Background(), taskID, task.AgentID, storage.TaskStatusCompleted)

		c.taskResultChan <- TaskResult{
			TaskID:   taskID,
			ThreadID: threadID,
			Status:   "completed",
			Content:  result.Output,
		}
	}

	// Runner 정리
	c.runnerManager.DeleteRunner(taskID)
}

// executeTask는 Task를 비동기로 실행합니다.
func (c *Controller) executeTask(ctx context.Context, taskID string, task *storage.Task) {
	defer func() {
		// TaskContext 정리
		c.mu.Lock()
		if taskCtx, ok := c.taskContexts[taskID]; ok {
			taskCtx.cancel()
			delete(c.taskContexts, taskID)
		}
		c.mu.Unlock()

		if r := recover(); r != nil {
			c.logger.Error("Task execution panicked",
				zap.String("task_id", taskID),
				zap.Any("panic", r),
			)
			// 상태를 failed로 변경
			_ = c.repo.UpsertTaskStatus(context.Background(), taskID, task.AgentID, storage.TaskStatusFailed)
		}
	}()

	// Agent 정보 조회 (Runner 생성에 필요)
	agent, err := c.repo.GetAgent(ctx, task.AgentID)
	if err != nil {
		c.logger.Error("Failed to get agent info", zap.Error(err))
		_ = c.repo.UpsertTaskStatus(ctx, taskID, task.AgentID, storage.TaskStatusFailed)
		return
	}

	// RunnerManager에서 TaskRunner 조회
	runner := c.runnerManager.GetRunner(taskID)
	if runner == nil {
		// Runner가 없으면 자동으로 재생성 (CLI 단일 실행 프로세스 지원)
		c.logger.Info("Runner not found, recreating...",
			zap.String("task_id", taskID),
			zap.String("agent_id", task.AgentID),
		)

		agentInfo := taskrunner.AgentInfo{
			AgentID:  agent.AgentID,
			Provider: agent.Provider,
			Model:    agent.Model,
			Prompt:   agent.Prompt,
		}
		runner = c.runnerManager.CreateRunner(taskID, agentInfo, c.logger)

		c.logger.Info("Runner recreated successfully",
			zap.String("task_id", taskID),
		)
	}

	// 메시지 목록 조회 및 변환
	messages, err := c.repo.ListMessageIndexByTask(ctx, taskID)
	if err != nil {
		c.logger.Error("Failed to list messages", zap.Error(err))
		_ = c.repo.UpsertTaskStatus(ctx, taskID, task.AgentID, storage.TaskStatusFailed)
		return
	}

	// ChatMessage로 변환 - 파일에서 실제 내용 읽기
	chatMessages := make([]taskrunner.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		// 파일에서 메시지 내용 읽기
		content, err := c.readMessageFromFile(msg.FilePath)
		if err != nil {
			c.logger.Warn("Failed to read message file, skipping",
				zap.String("task_id", taskID),
				zap.String("file_path", msg.FilePath),
				zap.Error(err),
			)
			// 파일 읽기 실패 시 건너뛰기
			continue
		}

		chatMessages = append(chatMessages, taskrunner.ChatMessage{
			Role:    msg.Role,
			Content: content,
		})
	}

	// Prompt가 있으면 추가
	if task.Prompt != "" {
		chatMessages = append(chatMessages, taskrunner.ChatMessage{
			Role:    "user",
			Content: task.Prompt,
		})
	}

	// RunRequest 구성
	req := &taskrunner.RunRequest{
		TaskID:       taskID,
		Model:        agent.Model,
		SystemPrompt: agent.Prompt,
		Messages:     chatMessages,
	}

	// TaskRunner 실행 (callback이 상태 변경과 결과 저장 처리)
	result, err := runner.Run(ctx, req)

	// Context 취소/타임아웃 확인
	if ctx.Err() != nil {
		c.logger.Warn("Task execution canceled or timed out",
			zap.String("task_id", taskID),
			zap.Error(ctx.Err()),
		)
		// 상태를 canceled 또는 failed로 변경
		status := storage.TaskStatusFailed
		if errors.Is(ctx.Err(), context.Canceled) {
			status = storage.TaskStatusCanceled
		}
		_ = c.repo.UpsertTaskStatus(context.Background(), taskID, task.AgentID, status)

		// 실행 완료 후 TaskRunner 정리
		c.runnerManager.DeleteRunner(taskID)
		return
	}

	// 로그 출력
	if err != nil {
		c.logger.Error("TaskRunner execution failed",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
	} else {
		c.logger.Info("Task execution completed",
			zap.String("task_id", taskID),
			zap.Bool("success", result != nil && result.Success),
		)
	}

	// 실행 완료 후 TaskRunner 정리
	c.runnerManager.DeleteRunner(taskID)
}

// ListMessages returns all messages for a task in conversation order.
func (c *Controller) ListMessages(ctx context.Context, taskID string) ([]storage.MessageIndex, error) {
	c.logger.Info("Listing messages for task",
		zap.String("task_id", taskID),
	)

	if c.repo == nil {
		return nil, fmt.Errorf("controller: repository is not configured")
	}

	messages, err := c.repo.ListMessageIndexByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	c.logger.Info("Listed messages",
		zap.String("task_id", taskID),
		zap.Int("count", len(messages)),
	)
	return messages, nil
}

// saveMessageToFile saves message content to a file and returns the file path.
// Messages are stored in data/messages/{taskID}/{conversationIndex}.json
func (c *Controller) saveMessageToFile(ctx context.Context, taskID, role, content string) (string, error) {
	// 1. 디렉토리 생성
	dir := filepath.Join("data", "messages", taskID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.logger.Error("Failed to create message directory",
			zap.String("dir", dir),
			zap.Error(err),
		)
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// 2. Conversation index 조회
	messages, err := c.repo.ListMessageIndexByTask(ctx, taskID)
	if err != nil {
		c.logger.Error("Failed to list messages", zap.Error(err))
		return "", fmt.Errorf("failed to list messages: %w", err)
	}
	index := len(messages)

	// 3. JSON 파일 저장
	filename := fmt.Sprintf("%04d.json", index)
	filePath := filepath.Join(dir, filename)

	msg := map[string]interface{}{
		"role":      role,
		"content":   content,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		c.logger.Error("Failed to marshal message", zap.Error(err))
		return "", fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		c.logger.Error("Failed to write file",
			zap.String("path", filePath),
			zap.Error(err),
		)
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	c.logger.Debug("Message saved to file",
		zap.String("task_id", taskID),
		zap.String("role", role),
		zap.String("path", filePath),
	)

	return filePath, nil
}

// StatusCallback 인터페이스 구현

// OnStatusChange는 Task 상태가 변경될 때 호출됩니다.
func (c *Controller) OnStatusChange(taskID string, status string) error {
	c.logger.Debug("OnStatusChange callback",
		zap.String("task_id", taskID),
		zap.String("status", status),
	)

	return c.UpdateTaskStatus(context.Background(), taskID, status)
}

// OnComplete는 Task가 완료될 때 호출됩니다.
func (c *Controller) OnComplete(taskID string, result *taskrunner.RunResult) error {
	c.logger.Info("OnComplete callback",
		zap.String("task_id", taskID),
		zap.Bool("success", result.Success),
	)

	// 결과를 파일로 저장
	if result.Success {
		filePath, err := c.saveMessageToFile(context.Background(), taskID, "assistant", result.Output)
		if err != nil {
			c.logger.Error("Failed to save result to file", zap.Error(err))
			return err
		}

		// MessageIndex에 추가
		if _, err := c.repo.AppendMessageIndex(context.Background(), taskID, "assistant", filePath); err != nil {
			c.logger.Error("Failed to append message index", zap.Error(err))
			return err
		}
	}

	// 상태를 completed로 변경
	return c.UpdateTaskStatus(context.Background(), taskID, storage.TaskStatusCompleted)
}

// OnError는 Task 실행 중 에러가 발생할 때 호출됩니다.
func (c *Controller) OnError(taskID string, err error) error {
	c.logger.Error("OnError callback",
		zap.String("task_id", taskID),
		zap.Error(err),
	)

	// 상태를 failed로 변경
	return c.UpdateTaskStatus(context.Background(), taskID, storage.TaskStatusFailed)
}

// readMessageFromFile reads message content from a JSON file.
func (c *Controller) readMessageFromFile(filePath string) (string, error) {
	// 파일 읽기
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// JSON 파싱
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %w", err)
	}

	// content 필드 추출
	content, ok := msg["content"].(string)
	if !ok {
		return "", fmt.Errorf("content field not found or not a string")
	}

	return content, nil
}

// ensure Controller implements StatusCallback
var _ taskrunner.StatusCallback = (*Controller)(nil)

