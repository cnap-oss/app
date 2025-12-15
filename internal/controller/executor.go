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

			c.controllerEventChan <- ControllerEvent{
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

		c.controllerEventChan <- ControllerEvent{
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

		c.controllerEventChan <- ControllerEvent{
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

	// RunRequest 구성 - 콜백 포함
	req := &taskrunner.RunRequest{
		TaskID:       taskID,
		Model:        agent.Model,
		SystemPrompt: agent.Prompt,
		Messages:     chatMessages,
		Callback:     c, // Controller가 StatusCallback 구현
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

		c.controllerEventChan <- ControllerEvent{
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

		c.controllerEventChan <- ControllerEvent{
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

		// 성공 시 - waiting 상태로 유지 (자동 완료하지 않음)
		_ = c.repo.UpsertTaskStatus(context.Background(), taskID, task.AgentID, storage.TaskStatusWaiting)

		// message 이벤트로 중간 응답 전송 (completed 대신)
		c.controllerEventChan <- ControllerEvent{
			TaskID:   taskID,
			ThreadID: threadID,
			Status:   "message",
			Content:  result.Output,
		}
	}

	// Runner 정리
	c.runnerManager.DeleteRunner(taskID)
}