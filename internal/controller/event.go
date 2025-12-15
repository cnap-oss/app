package controller

import (
	"context"
	"fmt"

	"github.com/cnap-oss/app/internal/storage"
	"go.uber.org/zap"
)

// eventLoop는 Task 이벤트를 처리하는 메인 루프입니다.
func (c *Controller) eventLoop(ctx context.Context) {
	c.logger.Info("Event loop started")
	defer c.logger.Info("Event loop stopped")

	for {
		select {
		case event := <-c.connectorEventChan:
			// 각 이벤트를 별도 goroutine으로 처리 (병렬 처리)
			go c.handleConnectorEvent(ctx, event)

		case <-ctx.Done():
			c.logger.Info("Event loop shutting down")
			return
		}
	}
}

// handleConnectorEvent는 단일 Connector 이벤트를 처리합니다.
func (c *Controller) handleConnectorEvent(ctx context.Context, event ConnectorEvent) {
	c.logger.Info("Handling connector event",
		zap.String("type", event.Type),
		zap.String("task_id", event.TaskID),
		zap.String("agent_name", event.AgentName),
	)

	switch event.Type {
	case "execute":
		c.handleExecuteEvent(ctx, event)
	case "continue":
		c.handleContinueEvent(ctx, event)
	case "cancel":
		c.handleCancelEvent(ctx, event)
	case "complete":
		c.handleCompleteEvent(ctx, event)
	default:
		c.logger.Warn("Unknown event type",
			zap.String("type", event.Type),
			zap.String("task_id", event.TaskID),
		)
	}
}

// handleExecuteEvent는 Task 실행 이벤트를 처리합니다.
func (c *Controller) handleExecuteEvent(ctx context.Context, event ConnectorEvent) {
	c.logger.Info("Creating new task for thread",
		zap.String("task_id", event.TaskID),
		zap.String("agent", event.AgentName),
	)

	if err := c.CreateTask(ctx, event.AgentName, event.TaskID, event.Prompt); err != nil {
		c.logger.Error("Failed to create task", zap.Error(err))
		c.controllerEventChan <- ControllerEvent{
			TaskID: event.TaskID,
			Status: "failed",
			Error:  fmt.Errorf("failed to create task: %w", err),
		}
		return
	}

	task, err := c.repo.GetTask(ctx, event.TaskID)
	if err != nil {
		c.logger.Error("Failed to get newly created task", zap.Error(err))
		c.controllerEventChan <- ControllerEvent{
			TaskID: event.TaskID,
			Status: "failed",
			Error:  fmt.Errorf("task not found after creation: %w", err),
		}
		return
	}

	go c.executeTask(ctx, event.TaskID, task)
}

// handleContinueEvent는 기존 Task에 메시지 추가 후 실행 계속 이벤트를 처리합니다.
// Thread 후속 메시지로 인해 기존 Task를 계속 실행해야 할 때 사용됩니다.
func (c *Controller) handleContinueEvent(ctx context.Context, event ConnectorEvent) {
	taskID := event.TaskID

	c.logger.Info("Handling continue event",
		zap.String("task_id", taskID),
	)

	if err := c.AddMessage(ctx, taskID, "user", event.Prompt); err != nil {
		c.logger.Error("Failed to add message to task", zap.Error(err))
		c.controllerEventChan <- ControllerEvent{
			TaskID: taskID,
			Status: "failed",
			Error:  fmt.Errorf("failed to add message: %w", err),
		}
		return
	}

	// 4. Task 실행 (메시지 히스토리 포함) - SendMessage 사용
	if err := c.SendMessage(ctx, taskID); err != nil {
		c.logger.Error("Failed to send message",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		c.controllerEventChan <- ControllerEvent{
			TaskID: taskID,
			Status: "failed",
			Error:  fmt.Errorf("failed to send message: %w", err),
		}
	}
}

// handleCancelEvent는 Task 취소 이벤트를 처리합니다.
func (c *Controller) handleCancelEvent(ctx context.Context, event ConnectorEvent) {
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
		c.controllerEventChan <- ControllerEvent{
			TaskID: event.TaskID,
			Status: "failed",
			Error:  fmt.Errorf("task not running"),
		}
		return
	}

	// Context 취소
	taskCtx.cancel()

	c.controllerEventChan <- ControllerEvent{
		TaskID:  event.TaskID,
		Status:  "canceled",
		Content: "Task canceled by user",
	}
}

// handleCompleteEvent handles the explicit task completion event.
func (c *Controller) handleCompleteEvent(ctx context.Context, event ConnectorEvent) {
	taskID := event.TaskID

	c.logger.Info("Handling complete event",
		zap.String("task_id", taskID),
	)

	// 1. Task 조회
	task, err := c.repo.GetTask(ctx, taskID)
	if err != nil {
		c.logger.Error("Failed to get task for complete event",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		c.controllerEventChan <- ControllerEvent{
			TaskID: taskID,
			Status: "failed",
			Error:  fmt.Errorf("task not found: %w", err),
		}
		return
	}

	// 2. Task 상태를 completed로 변경
	if err := c.repo.UpsertTaskStatus(ctx, taskID, task.AgentID, storage.TaskStatusCompleted); err != nil {
		c.logger.Error("Failed to update task status to completed",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
		c.controllerEventChan <- ControllerEvent{
			TaskID: taskID,
			Status: "failed",
			Error:  fmt.Errorf("failed to update status: %w", err),
		}
		return
	}

	// 3. Runner 삭제 (명시적 완료 시에만)
	c.runnerManager.DeleteRunner(taskID)

	// 4. completed 이벤트 전송
	c.controllerEventChan <- ControllerEvent{
		TaskID:  taskID,
		Status:  "completed",
		Content: "Task completed successfully",
	}

	c.logger.Info("Task completed explicitly",
		zap.String("task_id", taskID),
	)
}
