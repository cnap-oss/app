package controller

import (
	"context"

	taskrunner "github.com/cnap-oss/app/internal/runner"
	"github.com/cnap-oss/app/internal/storage"
	"go.uber.org/zap"
)

// StatusCallback 인터페이스 구현 (Phase 1)

// OnStarted는 Runner가 시작되고 세션이 생성될 때 호출됩니다.
func (c *Controller) OnStarted(taskID string, sessionID string) error {
	c.logger.Info("OnStarted callback",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
	)

	// 세션 ID를 Task 메타데이터에 저장할 수 있음 (추후 Phase에서 구현)
	return nil
}

// OnMessage는 Runner가 중간 응답을 생성할 때 호출됩니다.
// 이를 통해 Connector에 실시간으로 메시지를 전달합니다.
func (c *Controller) OnMessage(taskID string, msg *taskrunner.RunnerMessage) error {
	c.logger.Debug("OnMessage callback",
		zap.String("task_id", taskID),
		zap.String("message_type", string(msg.Type)),
		zap.Int("content_length", len(msg.Content)),
	)

	// 텍스트 메시지인 경우에만 ControllerEvent로 전달
	if msg.IsText() && msg.Content != "" {
		c.controllerEventChan <- ControllerEvent{
			TaskID:  taskID,
			Status:  "message",
			Content: msg.Content,
		}
	}

	return c.UpdateTaskStatus(context.Background(), taskID, storage.TaskStatusWaiting)
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

// ensure Controller implements StatusCallback
var _ taskrunner.StatusCallback = (*Controller)(nil)
