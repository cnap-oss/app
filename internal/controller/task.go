package controller

import (
	"context"
	"errors"
	"fmt"

	taskrunner "github.com/cnap-oss/app/internal/runner"
	"github.com/cnap-oss/app/internal/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

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

// DeleteTask는 작업을 삭제합니다 (soft delete).
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
