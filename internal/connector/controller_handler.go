package connector

import (
	"context"

	"github.com/bwmarrin/discordgo"
	"github.com/cnap-oss/app/internal/controller"
	"go.uber.org/zap"
)

// controllerEventHandler는 Task 실행 결과를 처리하는 goroutine입니다.
func (s *Server) controllerEventHandler(ctx context.Context) {
	s.logger.Info("Result handler started")
	defer s.logger.Info("Result handler stopped")

	for {
		select {
		case event := <-s.controllerEventChan:
			s.handleControllerEvent(event)

		case <-ctx.Done():
			s.logger.Info("Result handler shutting down")
			return
		}
	}
}

// handleControllerEvent는 ControllerEvent를 EventType에 따라 분기 처리합니다.
func (s *Server) handleControllerEvent(event controller.ControllerEvent) {
	// 새로운 EventType 기반 처리
	switch event.EventType {
	case controller.EventTypeStreamDelta:
		s.handleStreamDelta(event)
	case controller.EventTypePartComplete:
		s.handlePartComplete(event)
	case controller.EventTypeToolStart:
		s.handleToolStart(event)
	case controller.EventTypeToolProgress:
		s.handleToolProgress(event)
	case controller.EventTypeToolComplete:
		s.handleToolComplete(event)
	case controller.EventTypeToolError:
		s.handleToolError(event)
	case controller.EventTypeMessageComplete:
		s.handleMessageComplete(event)
	case controller.EventTypeError:
		s.handleError(event)
	case controller.EventTypeLegacy, "":
		// 하위 호환: Status 필드 기반 처리
		s.handleLegacyEvent(event)
	default:
		s.logger.Warn("Unknown EventType",
			zap.String("task_id", event.TaskID),
			zap.String("event_type", string(event.EventType)),
		)
	}
}

// handleStreamDelta는 스트리밍 델타 텍스트를 처리합니다.
func (s *Server) handleStreamDelta(event controller.ControllerEvent) {
	s.logger.Debug("[StreamDelta]",
		zap.String("task_id", event.TaskID),
		zap.String("message_id", event.MessageID),
		zap.String("part_id", event.PartID),
		zap.String("delta", truncate(event.Delta, 50)),
	)
	// TODO: Discord 메시지 업데이트 (debounce 메커니즘과 함께 구현 예정)
}

// handlePartComplete는 완료된 Part를 처리합니다.
func (s *Server) handlePartComplete(event controller.ControllerEvent) {
	s.logger.Info("[PartComplete]",
		zap.String("task_id", event.TaskID),
		zap.String("message_id", event.MessageID),
		zap.String("part_id", event.PartID),
		zap.String("part_type", string(event.PartType)),
		zap.String("content", truncate(event.Content, 100)),
	)
	if event.PartType == controller.PartTypeText {
		s.sendMessageToDiscord(event)
	}
	// TODO: Discord 메시지 업데이트
}

// handleToolStart는 도구 시작을 처리합니다.
func (s *Server) handleToolStart(event controller.ControllerEvent) {
	if event.ToolInfo != nil {
		s.logger.Info("[ToolStart]",
			zap.String("task_id", event.TaskID),
			zap.String("tool_name", event.ToolInfo.ToolName),
			zap.String("call_id", event.ToolInfo.CallID),
		)
		// TODO: Discord 메시지에 도구 시작 상태 표시
	}
}

// handleToolProgress는 도구 진행 상태를 처리합니다.
func (s *Server) handleToolProgress(event controller.ControllerEvent) {
	if event.ToolInfo != nil {
		s.logger.Debug("[ToolProgress]",
			zap.String("task_id", event.TaskID),
			zap.String("tool_name", event.ToolInfo.ToolName),
			zap.String("call_id", event.ToolInfo.CallID),
		)
		// TODO: Discord 메시지에 도구 진행 상태 업데이트
	}
}

// handleToolComplete는 도구 완료를 처리합니다.
func (s *Server) handleToolComplete(event controller.ControllerEvent) {
	if event.ToolInfo != nil {
		s.logger.Info("[ToolComplete]",
			zap.String("task_id", event.TaskID),
			zap.String("tool_name", event.ToolInfo.ToolName),
			zap.String("call_id", event.ToolInfo.CallID),
			zap.String("output", truncate(event.ToolInfo.Output, 100)),
		)
		// TODO: Discord 메시지에 도구 완료 상태 표시
	}
}

// handleToolError는 도구 에러를 처리합니다.
func (s *Server) handleToolError(event controller.ControllerEvent) {
	if event.ToolInfo != nil {
		s.logger.Error("[ToolError]",
			zap.String("task_id", event.TaskID),
			zap.String("tool_name", event.ToolInfo.ToolName),
			zap.String("call_id", event.ToolInfo.CallID),
			zap.String("error", event.ToolInfo.Error),
		)
		// TODO: Discord 메시지에 도구 에러 표시
	}
}

// handleMessageComplete는 메시지 완료를 처리합니다.
func (s *Server) handleMessageComplete(event controller.ControllerEvent) {
	s.logger.Info("[MessageComplete]",
		zap.String("task_id", event.TaskID),
		zap.String("message_id", event.MessageID),
		zap.String("content", truncate(event.Content, 200)),
	)
	// 기존 메시지 전송 로직 재사용
	// s.sendMessageToDiscord(event)
}

// handleError는 에러 이벤트를 처리합니다.
func (s *Server) handleError(event controller.ControllerEvent) {
	s.logger.Error("[Error]",
		zap.String("task_id", event.TaskID),
		zap.Error(event.Error),
	)
	// 기존 결과 전송 로직 재사용
	s.sendResultToDiscord(event)
}

// handleLegacyEvent는 기존 Status 필드 기반 이벤트를 처리합니다 (하위 호환).
func (s *Server) handleLegacyEvent(event controller.ControllerEvent) {
	s.logger.Info("Received controller event (legacy)",
		zap.String("task_id", event.TaskID),
		zap.String("status", event.Status),
		zap.String("content", truncate(event.Content, 100)),
	)

	switch event.Status {
	case "completed", "failed", "canceled":
		s.sendResultToDiscord(event)
	default:
		s.logger.Warn("Unknown controller event status",
			zap.String("task_id", event.TaskID),
			zap.String("status", event.Status),
		)
	}
}

// truncate는 문자열을 최대 길이로 자르고 "..."을 추가합니다.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (s *Server) sendMessageToDiscord(result controller.ControllerEvent) {
	if result.TaskID == "" {
		s.logger.Warn("Task ID is empty, cannot send result",
			zap.String("task_id", result.TaskID),
		)
		return
	}

	content := result.Content

	// 일반 메시지로 전송 (Embed 없이)
	_, err := s.session.ChannelMessageSend(result.TaskID, content)
	if err != nil {
		s.logger.Error("Failed to send message to Discord",
			zap.String("task_id", result.TaskID),
			zap.Error(err),
		)
	} else {
		s.logger.Debug("Message sent to Discord",
			zap.String("task_id", result.TaskID),
		)
	}
}

// sendResultToDiscord는 Task 실행 결과를 Discord Thread에 전송합니다.
func (s *Server) sendResultToDiscord(result controller.ControllerEvent) {
	if result.TaskID == "" {
		s.logger.Warn("Task ID is empty, cannot send result",
			zap.String("task_id", result.TaskID),
		)
		return
	}

	// Embed 메시지 생성
	var embed *discordgo.MessageEmbed

	if result.Error != nil || result.Status == "failed" {
		// 실패 시 빨간색
		embed = &discordgo.MessageEmbed{
			Title: "❌ Task 실행 실패",
			Color: 0xff0000, // 빨간색
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Task ID", Value: result.TaskID, Inline: true},
				{Name: "Status", Value: result.Status, Inline: true},
			},
		}

		if result.Error != nil {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:  "오류",
				Value: result.Error.Error(),
			})
		}
	} else if result.Status == "canceled" {
		// 취소 시 노란색
		embed = &discordgo.MessageEmbed{
			Title: "⚠️ Task 취소됨",
			Color: 0xffff00, // 노란색
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Task ID", Value: result.TaskID, Inline: true},
				{Name: "Status", Value: result.Status, Inline: true},
			},
		}

		if result.Content != "" {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:  "메시지",
				Value: result.Content,
			})
		}
	} else if result.Status == "completed" {
		// 최종 완료 시 초록색
		embed = &discordgo.MessageEmbed{
			Title: "✅ Task 실행 완료",
			Color: 0x00ff00, // 초록색
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Task ID", Value: result.TaskID, Inline: true},
				{Name: "Status", Value: result.Status, Inline: true},
			},
		}

		// 결과 내용 추가 (너무 길면 잘라내기)
		content := result.Content

		if content != "" {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:  "결과",
				Value: content,
			})
		}
	} else {
		// 알 수 없는 상태 - 기본 메시지로 처리
		s.logger.Warn("Unknown status received",
			zap.String("task_id", result.TaskID),
			zap.String("status", result.Status),
		)
		_, err := s.session.ChannelMessageSend(result.TaskID, result.Content)
		if err != nil {
			s.logger.Error("Failed to send message to Discord",
				zap.String("task_id", result.TaskID),
				zap.Error(err),
			)
		}
		return
	}

	// Discord에 메시지 전송
	_, err := s.session.ChannelMessageSendEmbed(result.TaskID, embed)
	if err != nil {
		s.logger.Error("Failed to send result to Discord",
			zap.String("task_id", result.TaskID),
			zap.Error(err),
		)
	} else {
		s.logger.Info("Result sent to Discord",
			zap.String("task_id", result.TaskID),
		)
	}
}
