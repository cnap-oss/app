package controller

import (
	"context"
	"fmt"

	taskrunner "github.com/cnap-oss/app/internal/runner"
	"github.com/cnap-oss/app/internal/runner/opencode"
	"github.com/cnap-oss/app/internal/storage"
	"go.uber.org/zap"
)

// sendStatusUpdateEvent는 Task 상태 업데이트 이벤트를 전송합니다.
func (c *Controller) sendStatusUpdateEvent(taskID, status string) {
	c.controllerEventChan <- ControllerEvent{
		EventType: EventTypeStatusUpdate,
		TaskID:    taskID,
		Status:    status,
	}
}

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

// OnEvent는 Runner가 SSE 이벤트를 수신할 때 호출됩니다.
// 이를 통해 Connector에 실시간으로 메시지를 전달합니다.
func (c *Controller) OnEvent(taskID string, evt *opencode.Event) error {
	c.logger.Info("OnEvent callback",
		zap.String("task_id", taskID),
		zap.String("event_type", evt.Type),
		zap.Any("properties", evt.Properties),
	)

	event := ControllerEvent{
		TaskID: taskID,
	}

	switch evt.Type {
	case "message.part.updated":
		// 파트 정보 추출
		if props, ok := evt.Properties["part"].(map[string]interface{}); ok {
			partType, _ := props["type"].(string)
			partID, _ := props["id"].(string)
			messageID, _ := props["messageID"].(string)

			event.PartID = partID
			event.MessageID = messageID

			switch partType {
			case "text":
				// delta 필드가 있으면 부분 업데이트
				if delta, hasDelta := evt.Properties["delta"].(string); hasDelta {
					event.EventType = EventTypeStreamDelta
					event.Delta = delta
					event.IsPartial = true
				} else {
					event.EventType = EventTypePartComplete
					if text, ok := props["text"].(string); ok {
						event.Content = text
					}
					event.IsPartial = false

					// PartComplete 시 메시지 role 정보 가져오기
					if messageID != "" {
						c.logger.Debug("Fetching message role",
							zap.String("task_id", taskID),
							zap.String("message_id", messageID),
						)
						c.fetchMessageRole(taskID, messageID, &event)
					}
				}
				event.PartType = PartTypeText
				event.Status = "message" // 하위 호환성

			case "reasoning":
				// delta 필드가 있으면 부분 업데이트
				if delta, hasDelta := evt.Properties["delta"].(string); hasDelta {
					event.EventType = EventTypeStreamDelta
					event.Delta = delta
					event.IsPartial = true
				} else {
					event.EventType = EventTypePartComplete
					if text, ok := props["text"].(string); ok {
						event.Content = text
					}
					event.IsPartial = false
					// PartComplete 시 메시지 role 정보 가져오기
					if messageID != "" {
						c.fetchMessageRole(taskID, messageID, &event)
					}
				}
				event.PartType = PartTypeReasoning
				event.Status = "reasoning"

			case "tool":
				// 도구 상태 확인
				if state, ok := props["state"].(map[string]interface{}); ok {
					status, _ := state["status"].(string)
					callID, _ := props["callID"].(string)
					tool, _ := props["tool"].(string)

					if status == "running" || status == "pending" {
						event.EventType = EventTypeToolStart
						event.PartType = PartTypeTool
						event.Status = "tool_start"
						event.ToolInfo = &ToolEventInfo{
							CallID:   callID,
							ToolName: tool,
						}
						if input, ok := state["input"].(map[string]interface{}); ok {
							event.ToolInfo.Input = input
						}
					} else {
						event.PartType = PartTypeTool
						isError := status == "error"
						if isError {
							event.EventType = EventTypeToolError
							event.Status = "tool_error"
						} else {
							event.EventType = EventTypeToolComplete
							event.Status = "tool_complete"
						}
						event.ToolInfo = &ToolEventInfo{
							CallID:   callID,
							ToolName: tool,
						}
						if output, ok := state["output"].(string); ok {
							if isError {
								event.ToolInfo.Error = output
							} else {
								event.ToolInfo.Output = output
							}
						}
					}
				}
			default:
				// 지원하지 않는 파트 타입은 무시
				return nil
			}
		}

	case "message.completed":
		event.EventType = EventTypeMessageComplete
		event.Status = "message_complete"
		if messageID, ok := evt.Properties["messageID"].(string); ok {
			event.MessageID = messageID
		}

	case "session.status":
		// 세션 상태 변경 이벤트 처리
		// status는 객체이고 type 필드를 가지고 있음
		if statusObj, ok := evt.Properties["status"].(map[string]interface{}); ok {
			if statusType, ok := statusObj["type"].(string); ok {
				c.logger.Info("Session status changed",
					zap.String("task_id", taskID),
					zap.String("status_type", statusType),
				)

				// status.type이 idle이면 Task status를 waiting으로 변경
				if statusType == "idle" {
					// Task 상태를 waiting으로 업데이트
					if err := c.UpdateTaskStatus(context.Background(), taskID, storage.TaskStatusWaiting); err != nil {
						c.logger.Error("Failed to update task status to waiting",
							zap.String("task_id", taskID),
							zap.Error(err),
						)
					} else {
						// 상태 업데이트 이벤트 전송
						c.sendStatusUpdateEvent(taskID, storage.TaskStatusWaiting)
					}
				}
			}
		}
		return nil

	case "session.aborted":
		event.EventType = EventTypeError
		event.Status = "error"
		event.Error = fmt.Errorf("session aborted")

	default:
		// 알 수 없는 이벤트 타입은 무시
		c.logger.Debug("Unknown event type, skipping",
			zap.String("event_type", evt.Type),
		)
		return nil
	}

	// 이벤트 전송 (빈 이벤트는 무시)
	if event.EventType != "" {
		c.controllerEventChan <- event
	}

	return nil
}

// OnComplete는 Task가 완료될 때 호출됩니다.
func (c *Controller) OnComplete(taskID string, result *taskrunner.RunResult) error {
	c.logger.Debug("OnComplete callback",
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
	err := c.UpdateTaskStatus(context.Background(), taskID, storage.TaskStatusCompleted)
	if err == nil {
		// 상태 업데이트 이벤트 전송
		c.sendStatusUpdateEvent(taskID, storage.TaskStatusCompleted)
	}
	return err
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
	updateErr := c.UpdateTaskStatus(context.Background(), taskID, storage.TaskStatusFailed)
	if updateErr == nil {
		// 상태 업데이트 이벤트 전송
		c.sendStatusUpdateEvent(taskID, storage.TaskStatusFailed)
	}
	return updateErr
}

// fetchMessageRole은 메시지 ID로부터 role 정보를 가져와 이벤트에 설정합니다.
func (c *Controller) fetchMessageRole(taskID string, messageID string, event *ControllerEvent) {
	// RunnerManager에서 Runner 가져오기
	runner := c.runnerManager.GetRunner(taskID)
	if runner == nil {
		c.logger.Warn("Runner를 찾을 수 없음",
			zap.String("task_id", taskID),
			zap.String("message_id", messageID),
		)
		return
	}

	// 메시지 정보 조회
	ctx := context.Background()
	msgInfo, err := runner.GetMessage(ctx, messageID)
	if err != nil {
		c.logger.Warn("메시지 정보 조회 실패",
			zap.String("task_id", taskID),
			zap.String("message_id", messageID),
			zap.Error(err),
		)
		return
	}

	// role 설정
	if msgInfo != nil && msgInfo.Info != nil {
		switch msg := msgInfo.Info.(type) {
		case opencode.UserMessage:
			event.Role = msg.Role
		case opencode.AssistantMessage:
			event.Role = msg.Role
		default:
			c.logger.Info("알 수 없는 메시지 타입",
				zap.String("task_id", taskID),
				zap.String("message_id", messageID),
				zap.Any("message", msg),
			)
		}
	}
}

// ensure Controller implements StatusCallback
var _ taskrunner.StatusCallback = (*Controller)(nil)
