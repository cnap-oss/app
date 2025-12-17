package handlers

import (
	"context"
	"fmt"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/cnap-oss/app/internal/controller"
	"go.uber.org/zap"
)

// ControllerHandler는 Controller로부터의 이벤트를 처리합니다.
type ControllerHandler struct {
	logger               *zap.Logger
	session              *discordgo.Session
	toolMessagesMutex    sync.RWMutex
	toolMessages         map[string]string // key: taskID:callID, value: Discord messageID
	threadMainMsgMutex   sync.RWMutex
	threadMainMessages   map[string]string // key: taskID (threadID), value: main message ID
}

// NewControllerHandler는 새로운 ControllerHandler를 생성합니다.
func NewControllerHandler(logger *zap.Logger, session *discordgo.Session) *ControllerHandler {
	return &ControllerHandler{
		logger:             logger.With(zap.String("handler", "controller")),
		session:            session,
		toolMessages:       make(map[string]string),
		threadMainMessages: make(map[string]string),
	}
}

// Start는 Controller 이벤트를 처리하는 goroutine을 시작합니다.
func (h *ControllerHandler) Start(ctx context.Context, eventChan <-chan controller.ControllerEvent) {
	h.logger.Info("Controller event handler started")
	defer h.logger.Info("Controller event handler stopped")

	for {
		select {
		case event := <-eventChan:
			h.handleControllerEvent(event)

		case <-ctx.Done():
			h.logger.Info("Controller event handler shutting down")
			return
		}
	}
}

// handleControllerEvent는 ControllerEvent를 EventType에 따라 분기 처리합니다.
func (h *ControllerHandler) handleControllerEvent(event controller.ControllerEvent) {
	// 새로운 EventType 기반 처리
	switch event.EventType {
	case controller.EventTypeStreamDelta:
		h.handleStreamDelta(event)
	case controller.EventTypePartComplete:
		h.handlePartComplete(event)
	case controller.EventTypeToolStart:
		h.handleToolStart(event)
	case controller.EventTypeToolProgress:
		h.handleToolProgress(event)
	case controller.EventTypeToolComplete:
		h.handleToolComplete(event)
	case controller.EventTypeToolError:
		h.handleToolError(event)
	case controller.EventTypeMessageComplete:
		h.handleMessageComplete(event)
	case controller.EventTypeStatusUpdate:
		h.handleStatusUpdate(event)

	case controller.EventTypeError:
		h.handleError(event)
	case controller.EventTypeLegacy, "":
		// 하위 호환: Status 필드 기반 처리
		h.handleLegacyEvent(event)
	default:
		h.logger.Warn("Unknown EventType",
			zap.String("task_id", event.TaskID),
			zap.String("event_type", string(event.EventType)),
		)
	}
}

// handleStreamDelta는 스트리밍 델타 텍스트를 처리합니다.
func (h *ControllerHandler) handleStreamDelta(event controller.ControllerEvent) {
	h.logger.Debug("[StreamDelta]",
		zap.String("task_id", event.TaskID),
		zap.String("message_id", event.MessageID),
		zap.String("part_id", event.PartID),
		zap.String("delta", truncate(event.Delta, 50)),
	)
	// TODO: Discord 메시지 업데이트 (debounce 메커니즘과 함께 구현 예정)
}

// handlePartComplete는 완료된 Part를 처리합니다.
func (h *ControllerHandler) handlePartComplete(event controller.ControllerEvent) {
	h.logger.Info("[PartComplete]",
		zap.String("task_id", event.TaskID),
		zap.String("message_id", event.MessageID),
		zap.String("part_id", event.PartID),
		zap.String("part_type", string(event.PartType)),
		zap.String("role", event.Role),
		zap.String("content", truncate(event.Content, 100)),
	)
	if event.PartType == controller.PartTypeText && event.Role == "assistant" {
		h.sendMessageToDiscord(event)
	}
	// TODO: Discord 메시지 업데이트
}

// handleToolStart는 도구 시작을 처리합니다.
func (h *ControllerHandler) handleToolStart(event controller.ControllerEvent) {
	if event.ToolInfo != nil {
		h.logger.Info("[ToolStart]",
			zap.String("task_id", event.TaskID),
			zap.String("tool_name", event.ToolInfo.ToolName),
			zap.String("call_id", event.ToolInfo.CallID),
		)

		// 도구 실행 시작 메시지 생성
		content := formatToolMessage(event.ToolInfo.ToolName, "running", "", event.ToolInfo.Input)

		// 저장된 메시지 ID 확인
		messageKey := event.TaskID + ":" + event.ToolInfo.CallID
		h.toolMessagesMutex.RLock()
		messageID, exists := h.toolMessages[messageKey]
		h.toolMessagesMutex.RUnlock()

		if exists {
			// 기존 메시지가 있으면 업데이트
			_, err := h.session.ChannelMessageEdit(event.TaskID, messageID, content)
			if err != nil {
				h.logger.Error("Failed to update existing tool start message",
					zap.String("task_id", event.TaskID),
					zap.String("tool_name", event.ToolInfo.ToolName),
					zap.String("message_id", messageID),
					zap.Error(err),
				)
			} else {
				h.logger.Debug("Tool start message updated",
					zap.String("task_id", event.TaskID),
					zap.String("message_id", messageID),
				)
			}
		} else {
			// 기존 메시지가 없으면 새로 생성
			msg, err := h.session.ChannelMessageSend(event.TaskID, content)
			if err != nil {
				h.logger.Error("Failed to send tool start message",
					zap.String("task_id", event.TaskID),
					zap.String("tool_name", event.ToolInfo.ToolName),
					zap.Error(err),
				)
				return
			}

			// 메시지 ID 저장 (나중에 업데이트하기 위해)
			h.toolMessagesMutex.Lock()
			h.toolMessages[messageKey] = msg.ID
			h.toolMessagesMutex.Unlock()

			h.logger.Debug("Tool start message created",
				zap.String("task_id", event.TaskID),
				zap.String("message_id", msg.ID),
			)
		}
	}
}

// handleToolProgress는 도구 진행 상태를 처리합니다.
func (h *ControllerHandler) handleToolProgress(event controller.ControllerEvent) {
	if event.ToolInfo != nil {
		h.logger.Debug("[ToolProgress]",
			zap.String("task_id", event.TaskID),
			zap.String("tool_name", event.ToolInfo.ToolName),
			zap.String("call_id", event.ToolInfo.CallID),
		)

		// 저장된 메시지 ID 가져오기
		messageKey := event.TaskID + ":" + event.ToolInfo.CallID
		h.toolMessagesMutex.RLock()
		messageID, exists := h.toolMessages[messageKey]
		h.toolMessagesMutex.RUnlock()

		if !exists {
			h.logger.Warn("Tool message not found for progress update",
				zap.String("task_id", event.TaskID),
				zap.String("call_id", event.ToolInfo.CallID),
			)
			return
		}

		// Progress 상태로 메시지 업데이트
		content := formatToolMessage(event.ToolInfo.ToolName, "running", "", event.ToolInfo.Input)

		_, err := h.session.ChannelMessageEdit(event.TaskID, messageID, content)
		if err != nil {
			h.logger.Error("Failed to update tool progress message",
				zap.String("task_id", event.TaskID),
				zap.String("tool_name", event.ToolInfo.ToolName),
				zap.Error(err),
			)
		}
	}
}

// handleToolComplete는 도구 완료를 처리합니다.
func (h *ControllerHandler) handleToolComplete(event controller.ControllerEvent) {
	if event.ToolInfo != nil {
		h.logger.Info("[ToolComplete]",
			zap.String("task_id", event.TaskID),
			zap.String("tool_name", event.ToolInfo.ToolName),
			zap.String("call_id", event.ToolInfo.CallID),
			zap.String("output", truncate(event.ToolInfo.Output, 100)),
		)

		// 저장된 메시지 ID 가져오기
		messageKey := event.TaskID + ":" + event.ToolInfo.CallID
		h.toolMessagesMutex.RLock()
		messageID, exists := h.toolMessages[messageKey]
		h.toolMessagesMutex.RUnlock()

		if !exists {
			h.logger.Warn("Tool message not found for complete update",
				zap.String("task_id", event.TaskID),
				zap.String("call_id", event.ToolInfo.CallID),
			)
			return
		}

		// Complete 상태로 메시지 업데이트
		content := formatToolMessage(event.ToolInfo.ToolName, "completed", event.ToolInfo.Output, event.ToolInfo.Input)

		_, err := h.session.ChannelMessageEdit(event.TaskID, messageID, content)
		if err != nil {
			h.logger.Error("Failed to update tool complete message",
				zap.String("task_id", event.TaskID),
				zap.String("tool_name", event.ToolInfo.ToolName),
				zap.Error(err),
			)
		}

		// 메시지 ID 정리
		h.toolMessagesMutex.Lock()
		delete(h.toolMessages, messageKey)
		h.toolMessagesMutex.Unlock()
	}
}

// handleToolError는 도구 에러를 처리합니다.
func (h *ControllerHandler) handleToolError(event controller.ControllerEvent) {
	if event.ToolInfo != nil {
		h.logger.Error("[ToolError]",
			zap.String("task_id", event.TaskID),
			zap.String("tool_name", event.ToolInfo.ToolName),
			zap.String("call_id", event.ToolInfo.CallID),
			zap.String("error", event.ToolInfo.Error),
		)

		// 저장된 메시지 ID 가져오기
		messageKey := event.TaskID + ":" + event.ToolInfo.CallID
		h.toolMessagesMutex.RLock()
		messageID, exists := h.toolMessages[messageKey]
		h.toolMessagesMutex.RUnlock()

		if !exists {
			h.logger.Warn("Tool message not found for error update",
				zap.String("task_id", event.TaskID),
				zap.String("call_id", event.ToolInfo.CallID),
			)
			return
		}

		// Error 상태로 메시지 업데이트
		content := formatToolMessage(event.ToolInfo.ToolName, "error", event.ToolInfo.Error, event.ToolInfo.Input)

		_, err := h.session.ChannelMessageEdit(event.TaskID, messageID, content)
		if err != nil {
			h.logger.Error("Failed to update tool error message",
				zap.String("task_id", event.TaskID),
				zap.String("tool_name", event.ToolInfo.ToolName),
				zap.Error(err),
			)
		}

		// 메시지 ID 정리
		h.toolMessagesMutex.Lock()
		delete(h.toolMessages, messageKey)
		h.toolMessagesMutex.Unlock()
	}
}

// handleStatusUpdate는 Task 상태 업데이트를 처리합니다.
func (h *ControllerHandler) handleStatusUpdate(event controller.ControllerEvent) {
	h.logger.Info("[StatusUpdate]",
		zap.String("task_id", event.TaskID),
		zap.String("status", event.Status),
	)
	h.updateThreadMainMessage(event.TaskID, event.Status)
}

// handleMessageComplete는 메시지 완료를 처리합니다.
func (h *ControllerHandler) handleMessageComplete(event controller.ControllerEvent) {
	h.logger.Info("[MessageComplete]",
		zap.String("task_id", event.TaskID),
		zap.String("message_id", event.MessageID),
		zap.String("content", truncate(event.Content, 200)),
	)
	// 기존 메시지 전송 로직 재사용
	// h.sendMessageToDiscord(event)
}

// handleError는 에러 이벤트를 처리합니다.
func (h *ControllerHandler) handleError(event controller.ControllerEvent) {
	h.logger.Error("[Error]",
		zap.String("task_id", event.TaskID),
		zap.Error(event.Error),
	)
	// 기존 결과 전송 로직 재사용
	h.sendResultToDiscord(event)
}

// handleLegacyEvent는 기존 Status 필드 기반 이벤트를 처리합니다 (하위 호환).
func (h *ControllerHandler) handleLegacyEvent(event controller.ControllerEvent) {
	h.logger.Info("Received controller event (legacy)",
		zap.String("task_id", event.TaskID),
		zap.String("status", event.Status),
		zap.String("content", truncate(event.Content, 100)),
	)

	switch event.Status {
	case "completed", "failed", "canceled":
		h.sendResultToDiscord(event)
	default:
		h.logger.Warn("Unknown controller event status",
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

func (h *ControllerHandler) sendMessageToDiscord(result controller.ControllerEvent) {
	if result.TaskID == "" {
		h.logger.Warn("Task ID is empty, cannot send result",
			zap.String("task_id", result.TaskID),
		)
		return
	}

	content := result.Content
	const maxLength = 2000

	// content가 2000자 이하면 그대로 전송
	if len(content) <= maxLength {
		_, err := h.session.ChannelMessageSend(result.TaskID, content)
		if err != nil {
			h.logger.Error("Failed to send message to Discord",
				zap.String("task_id", result.TaskID),
				zap.Error(err),
			)
		} else {
			h.logger.Debug("Message sent to Discord",
				zap.String("task_id", result.TaskID),
			)
		}
		return
	}

	// content가 2000자를 초과하면 여러 메시지로 분할 전송
	h.logger.Info("Splitting long message",
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
		_, err := h.session.ChannelMessageSend(result.TaskID, chunk)
		if err != nil {
			h.logger.Error("Failed to send message chunk to Discord",
				zap.String("task_id", result.TaskID),
				zap.Int("chunk_index", i/maxLength),
				zap.Error(err),
			)
			return
		}

		h.logger.Debug("Message chunk sent to Discord",
			zap.String("task_id", result.TaskID),
			zap.Int("chunk_index", i/maxLength),
			zap.Int("chunk_length", len(chunk)),
		)
	}

	h.logger.Info("All message chunks sent successfully",
		zap.String("task_id", result.TaskID),
	)
}

// sendResultToDiscord는 Task 실행 결과를 Discord Thread에 전송합니다.
func (h *ControllerHandler) sendResultToDiscord(result controller.ControllerEvent) {
	if result.TaskID == "" {
		h.logger.Warn("Task ID is empty, cannot send result",
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
		h.logger.Warn("Unknown status received",
			zap.String("task_id", result.TaskID),
			zap.String("status", result.Status),
		)
		_, err := h.session.ChannelMessageSend(result.TaskID, result.Content)
		if err != nil {
			h.logger.Error("Failed to send message to Discord",
				zap.String("task_id", result.TaskID),
				zap.Error(err),
			)
		}
		return
	}

	// Discord에 메시지 전송
	_, err := h.session.ChannelMessageSendEmbed(result.TaskID, embed)
	if err != nil {
		h.logger.Error("Failed to send result to Discord",
			zap.String("task_id", result.TaskID),
			zap.Error(err),
		)
	} else {
		h.logger.Info("Result sent to Discord",
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

// RegisterThreadMainMessage는 Thread의 메인 메시지 ID를 등록합니다.
func (h *ControllerHandler) RegisterThreadMainMessage(taskID, messageID string) {
	h.threadMainMsgMutex.Lock()
	defer h.threadMainMsgMutex.Unlock()
	h.threadMainMessages[taskID] = messageID
	
	h.logger.Debug("Thread main message registered",
		zap.String("task_id", taskID),
		zap.String("message_id", messageID),
	)
}

// updateThreadMainMessage는 Thread 메인 메시지를 Task 상태에 따라 업데이트합니다.
func (h *ControllerHandler) updateThreadMainMessage(taskID, status string) {
	h.threadMainMsgMutex.RLock()
	messageID, exists := h.threadMainMessages[taskID]
	h.threadMainMsgMutex.RUnlock()

	if !exists {
		h.logger.Warn("Thread main message not found",
			zap.String("task_id", taskID),
		)
		return
	}

	// 상태에 따라 Embed 생성
	var embed *discordgo.MessageEmbed
	
	switch status {
	case "pending":
		embed = &discordgo.MessageEmbed{
			Title: "⏳ 대기 중",
			Color: 0xFFFF00, // 노란색
			Description: "작업이 시작을 기다리고 있습니다.",
		}
	case "running":
		embed = &discordgo.MessageEmbed{
			Title: "🔄 실행 중",
			Color: 0x0099FF, // 파란색
			Description: "작업을 실행하고 있습니다...",
		}
	case "waiting":
		embed = &discordgo.MessageEmbed{
			Title: "⏸️ 입력 대기 중",
			Color: 0xFFA500, // 주황색
			Description: "사용자 입력을 기다리고 있습니다.",
		}
	case "completed":
		embed = &discordgo.MessageEmbed{
			Title: "✅ 완료",
			Color: 0x00FF00, // 초록색
			Description: "작업이 성공적으로 완료되었습니다.",
		}
	case "failed":
		embed = &discordgo.MessageEmbed{
			Title: "❌ 실패",
			Color: 0xFF0000, // 빨간색
			Description: "작업 실행에 실패했습니다.",
		}
	case "canceled":
		embed = &discordgo.MessageEmbed{
			Title: "🚫 취소됨",
			Color: 0x808080, // 회색
			Description: "작업이 취소되었습니다.",
		}
	default:
		h.logger.Warn("Unknown task status",
			zap.String("task_id", taskID),
			zap.String("status", status),
		)
		return
	}

	// 메시지 업데이트
	_, err := h.session.ChannelMessageEditEmbed(taskID, messageID, embed)
	if err != nil {
		h.logger.Error("Failed to update thread main message",
			zap.String("task_id", taskID),
			zap.String("message_id", messageID),
			zap.String("status", status),
			zap.Error(err),
		)
	} else {
		h.logger.Info("Thread main message updated",
			zap.String("task_id", taskID),
			zap.String("status", status),
		)
	}
}
