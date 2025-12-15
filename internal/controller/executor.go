package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	taskrunner "github.com/cnap-oss/app/internal/runner"
	"github.com/cnap-oss/app/internal/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

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
		Callback:     c,
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

		c.controllerEventChan <- ControllerEvent{
			TaskID: taskID,
			Status: status,
			Error:  ctx.Err(),
		}

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
		c.controllerEventChan <- ControllerEvent{
			TaskID: taskID,
			Status: "failed",
			Error:  err,
		}
	} else {
		c.logger.Info("Task execution completed",
			zap.String("task_id", taskID),
			zap.Bool("success", result != nil && result.Success),
		)
	}
}
