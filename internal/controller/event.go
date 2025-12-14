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
func (c *Controller) handleExecuteEvent(ctx context.Context, event ConnectorEvent) {
	// Task 조회
	task, err := c.repo.GetTask(ctx, event.TaskID)
	if err != nil {
		c.logger.Error("Failed to get task",
			zap.String("task_id", event.TaskID),
			zap.Error(err),
		)
		c.controllerEventChan <- ControllerEvent{
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
		c.controllerEventChan <- ControllerEvent{
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
			TaskID:   event.TaskID,
			ThreadID: event.ThreadID,
			Status:   "failed",
			Error:    fmt.Errorf("task not running"),
		}
		return
	}

	// Context 취소
	taskCtx.cancel()

	c.controllerEventChan <- ControllerEvent{
		TaskID:   event.TaskID,
		ThreadID: event.ThreadID,
		Status:   "canceled",
		Content:  "Task canceled by user",
	}
}