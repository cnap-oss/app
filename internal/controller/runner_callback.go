package controller

import (
	"context"
	"fmt"

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
	c.logger.Info("OnMessage callback",
		zap.String("task_id", taskID),
		zap.String("message_type", string(msg.Type)),
		zap.Int("content_length", len(msg.Content)),
	)

	event := ControllerEvent{
		TaskID:    taskID,
		MessageID: msg.MessageID,
		PartID:    msg.PartID,
		IsPartial: msg.IsPartial,
	}

	switch msg.Type {
	case taskrunner.MessageTypeText:
		if msg.IsPartial {
			event.EventType = EventTypeStreamDelta
			event.Delta = msg.Delta
		} else {
			event.EventType = EventTypePartComplete
			event.Content = msg.Content
		}
		event.PartType = PartTypeText
		event.Status = "message" // 하위 호환성

	case taskrunner.MessageTypeReasoning:
		if msg.IsPartial {
			event.EventType = EventTypeStreamDelta
			event.Delta = msg.Delta
		} else {
			event.EventType = EventTypePartComplete
			event.Content = msg.Content
		}
		event.PartType = PartTypeReasoning
		event.Status = "reasoning"

	case taskrunner.MessageTypeToolCall:
		event.EventType = EventTypeToolStart
		event.PartType = PartTypeTool
		event.Status = "tool_start"
		if msg.ToolCall != nil {
			event.ToolInfo = &ToolEventInfo{
				ToolName: msg.ToolCall.ToolName,
				CallID:   msg.ToolCall.ToolID,
				Input:    msg.ToolCall.Arguments,
			}
		}

	case taskrunner.MessageTypeToolResult:
		event.EventType = EventTypeToolComplete
		event.PartType = PartTypeTool
		event.Status = "tool_complete"
		if msg.ToolResult != nil {
			event.ToolInfo = &ToolEventInfo{
				ToolName: msg.ToolResult.ToolName,
				CallID:   msg.ToolResult.ToolID,
				Output:   msg.ToolResult.Result,
			}
			if msg.ToolResult.IsError {
				event.EventType = EventTypeToolError
				event.ToolInfo.Error = msg.ToolResult.Result
				event.Status = "tool_error"
			}
		}

	case taskrunner.MessageTypeComplete:
		event.EventType = EventTypeMessageComplete
		event.Content = msg.Content
		event.Status = "message_complete"

	case taskrunner.MessageTypeError:
		event.EventType = EventTypeError
		event.Status = "error"
		if msg.Error != nil {
			event.Error = fmt.Errorf("%s: %s", msg.Error.Code, msg.Error.Message)
			event.Content = msg.Error.Message
		}

	default:
		// 알 수 없는 타입은 legacy 방식으로 처리
		event.EventType = EventTypeLegacy
		event.Status = "message"
		event.Content = msg.Content
	}

	// 이벤트 전송 (빈 이벤트는 무시)
	if event.EventType != "" {
		c.controllerEventChan <- event
	}

	return nil // UpdateTaskStatus는 상위에서 처리
}

// OnComplete는 Task가 완료될 때 호출됩니다.
func (c *Controller) OnComplete(taskID string, result *taskrunner.RunResult) error {
	c.logger.Info("OnComplete callback",
		zap.String("task_id", taskID),
		zap.Bool("success", result.Success),
		zap.String("output", result.Output),
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

	c.controllerEventChan <- ControllerEvent{
		TaskID:  taskID,
		Status:  "completed",
		Content: result.Output,
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

	c.controllerEventChan <- ControllerEvent{
		TaskID: taskID,
		Status: "failed",
		Error:  err,
	}

	// 상태를 failed로 변경
	return c.UpdateTaskStatus(context.Background(), taskID, storage.TaskStatusFailed)
}

// ensure Controller implements StatusCallback
var _ taskrunner.StatusCallback = (*Controller)(nil)
