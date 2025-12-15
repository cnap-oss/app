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
		case result := <-s.controllerEventChan:
			s.logger.Info("Received controller event",
				zap.String("task_id", result.TaskID),
				zap.String("status", result.Status),
				zap.String("content", result.Content),
			)
			switch result.Status {
			case "message":
				s.sendMessageToDiscord(result)
			case "completed", "failed", "canceled":
				s.sendResultToDiscord(result)
			default:
				s.logger.Warn("Unknown controller event status",
					zap.String("task_id", result.TaskID),
					zap.String("status", result.Status),
				)
			}

		case <-ctx.Done():
			s.logger.Info("Result handler shutting down")
			return
		}
	}
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
