package connector

import (
	"context"
	"fmt"

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
		zap.String("role", event.Role),
		zap.String("content", truncate(event.Content, 100)),
	)
	if event.PartType == controller.PartTypeText && event.Role == "assistant" {
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

		// 도구 실행 시작 메시지 생성
		content := formatToolMessage(event.ToolInfo.ToolName, "running", "", event.ToolInfo.Input)

		// Discord 메시지 전송
		msg, err := s.session.ChannelMessageSend(event.TaskID, content)
		if err != nil {
			s.logger.Error("Failed to send tool start message",
				zap.String("task_id", event.TaskID),
				zap.String("tool_name", event.ToolInfo.ToolName),
				zap.Error(err),
			)
			return
		}

		// 메시지 ID 저장 (나중에 업데이트하기 위해)
		messageKey := event.TaskID + ":" + event.ToolInfo.CallID
		s.toolMessagesMutex.Lock()
		s.toolMessages[messageKey] = msg.ID
		s.toolMessagesMutex.Unlock()

		s.logger.Debug("Tool start message sent",
			zap.String("task_id", event.TaskID),
			zap.String("message_id", msg.ID),
		)
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

		// 저장된 메시지 ID 가져오기
		messageKey := event.TaskID + ":" + event.ToolInfo.CallID
		s.toolMessagesMutex.RLock()
		messageID, exists := s.toolMessages[messageKey]
		s.toolMessagesMutex.RUnlock()

		if !exists {
			s.logger.Warn("Tool message not found for progress update",
				zap.String("task_id", event.TaskID),
				zap.String("call_id", event.ToolInfo.CallID),
			)
			return
		}

		// Progress 상태로 메시지 업데이트
		content := formatToolMessage(event.ToolInfo.ToolName, "running", "", event.ToolInfo.Input)

		_, err := s.session.ChannelMessageEdit(event.TaskID, messageID, content)
		if err != nil {
			s.logger.Error("Failed to update tool progress message",
				zap.String("task_id", event.TaskID),
				zap.String("tool_name", event.ToolInfo.ToolName),
				zap.Error(err),
			)
		}
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

		// 저장된 메시지 ID 가져오기
		messageKey := event.TaskID + ":" + event.ToolInfo.CallID
		s.toolMessagesMutex.RLock()
		messageID, exists := s.toolMessages[messageKey]
		s.toolMessagesMutex.RUnlock()

		if !exists {
			s.logger.Warn("Tool message not found for complete update",
				zap.String("task_id", event.TaskID),
				zap.String("call_id", event.ToolInfo.CallID),
			)
			return
		}

		// Complete 상태로 메시지 업데이트
		content := formatToolMessage(event.ToolInfo.ToolName, "completed", event.ToolInfo.Output, event.ToolInfo.Input)

		_, err := s.session.ChannelMessageEdit(event.TaskID, messageID, content)
		if err != nil {
			s.logger.Error("Failed to update tool complete message",
				zap.String("task_id", event.TaskID),
				zap.String("tool_name", event.ToolInfo.ToolName),
				zap.Error(err),
			)
		}

		// 메시지 ID 정리
		s.toolMessagesMutex.Lock()
		delete(s.toolMessages, messageKey)
		s.toolMessagesMutex.Unlock()
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

		// 저장된 메시지 ID 가져오기
		messageKey := event.TaskID + ":" + event.ToolInfo.CallID
		s.toolMessagesMutex.RLock()
		messageID, exists := s.toolMessages[messageKey]
		s.toolMessagesMutex.RUnlock()

		if !exists {
			s.logger.Warn("Tool message not found for error update",
				zap.String("task_id", event.TaskID),
				zap.String("call_id", event.ToolInfo.CallID),
			)
			return
		}

		// Error 상태로 메시지 업데이트
		content := formatToolMessage(event.ToolInfo.ToolName, "error", event.ToolInfo.Error, event.ToolInfo.Input)

		_, err := s.session.ChannelMessageEdit(event.TaskID, messageID, content)
		if err != nil {
			s.logger.Error("Failed to update tool error message",
				zap.String("task_id", event.TaskID),
				zap.String("tool_name", event.ToolInfo.ToolName),
				zap.Error(err),
			)
		}

		// 메시지 ID 정리
		s.toolMessagesMutex.Lock()
		delete(s.toolMessages, messageKey)
		s.toolMessagesMutex.Unlock()
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
	const maxLength = 2000

	// content가 2000자 이하면 그대로 전송
	if len(content) <= maxLength {
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
		return
	}

	// content가 2000자를 초과하면 여러 메시지로 분할 전송
	s.logger.Info("Splitting long message",
		zap.String("task_id", result.TaskID),
		zap.Int("total_length", len(content)),
		zap.Int("chunks", (len(content)+maxLength-1)/maxLength),
	)

	for i := 0; i < len(content); i += maxLength {
		end := i + maxLength
		if end > len(content) {
			end = len(content)
		}

		chunk := content[i:end]
		_, err := s.session.ChannelMessageSend(result.TaskID, chunk)
		if err != nil {
			s.logger.Error("Failed to send message chunk to Discord",
				zap.String("task_id", result.TaskID),
				zap.Int("chunk_index", i/maxLength),
				zap.Error(err),
			)
			return
		}

		s.logger.Debug("Message chunk sent to Discord",
			zap.String("task_id", result.TaskID),
			zap.Int("chunk_index", i/maxLength),
			zap.Int("chunk_length", len(chunk)),
		)
	}

	s.logger.Info("All message chunks sent successfully",
		zap.String("task_id", result.TaskID),
	)
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

// formatToolMessage는 도구 상태에 따라 Discord 메시지를 포맷합니다.
func formatToolMessage(toolName, status, output string, input map[string]any) string {
	var emoji string
	var statusText string

	switch status {
	case "running":
		emoji = "🔧"
		statusText = "실행 중"
	case "completed":
		emoji = "✅"
		statusText = "완료"
	case "error":
		emoji = "❌"
		statusText = "에러"
	default:
		emoji = "🔧"
		statusText = status
	}

	message := fmt.Sprintf("%s **도구 %s**: `%s`", emoji, statusText, toolName)

	// Input 정보 추가 (간단히)
	if len(input) > 0 {
		message += "\n```"
		count := 0
		for key, value := range input {
			if count > 2 { // 최대 3개만 표시
				message += "\n..."
				break
			}
			valueStr := fmt.Sprintf("%v", value)
			if len(valueStr) > 50 {
				valueStr = valueStr[:50] + "..."
			}
			message += fmt.Sprintf("\n%s: %s", key, valueStr)
			count++
		}
		message += "\n```"
	}

	// Output/Error 정보 추가
	if output != "" {
		const maxOutputLen = 300
		if len(output) > maxOutputLen {
			message += fmt.Sprintf("\n```\n%s...\n```", output[:maxOutputLen])
		} else {
			message += fmt.Sprintf("\n```\n%s\n```", output)
		}
	}

	return message
}
