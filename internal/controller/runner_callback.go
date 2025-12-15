package controller

import (
	"context"

	taskrunner "github.com/cnap-oss/app/internal/runner"
	"github.com/cnap-oss/app/internal/storage"
	"go.uber.org/zap"
)

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

// OnMessage는 Runner가 중간 응답을 생성할 때 호출됩니다.
// 이를 통해 Connector에 실시간으로 메시지를 전달합니다.
func (c *Controller) OnMessage(taskID string, message string) error {
	c.logger.Debug("OnMessage callback",
		zap.String("task_id", taskID),
		zap.Int("message_length", len(message)),
	)

	// ControllerEvent로 메시지 전달
	// 현재는 TaskID를 ThreadID로도 사용 (추후 별도 매핑 필요 시 수정)
	c.controllerEventChan <- ControllerEvent{
		TaskID:  taskID,
		Status:  "message",
		Content: message,
	}

	return c.UpdateTaskStatus(context.Background(), taskID, storage.TaskStatusWaiting)
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

// ensure Controller implements StatusCallback
var _ taskrunner.StatusCallback = (*Controller)(nil)
