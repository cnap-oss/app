package connector

import (
	"context"

	"github.com/bwmarrin/discordgo"
	"github.com/cnap-oss/app/internal/controller"
	"go.uber.org/zap"
)

// resultHandler는 Task 실행 결과를 처리하는 goroutine입니다.
func (s *Server) resultHandler(ctx context.Context) {
	s.logger.Info("Result handler started")
	defer s.logger.Info("Result handler stopped")

	for {
		select {
		case result := <-s.taskResultChan:
			s.logger.Info("Received task result",
				zap.String("task_id", result.TaskID),
				zap.String("thread_id", result.ThreadID),
				zap.String("status", result.Status),
			)
			s.sendResultToDiscord(result)

		case <-ctx.Done():
			s.logger.Info("Result handler shutting down")
			return
		}
	}
}

// sendResultToDiscord는 Task 실행 결과를 Discord Thread에 전송합니다.
func (s *Server) sendResultToDiscord(result controller.TaskResult) {
	if result.ThreadID == "" {
		s.logger.Warn("Thread ID is empty, cannot send result",
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
	} else {
		// 성공 시 초록색
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
		if len(content) > 1000 {
			content = content[:1000] + "...\n(결과가 너무 길어 잘렸습니다)"
		}

		if content != "" {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:  "결과",
				Value: content,
			})
		}
	}

	// Discord에 메시지 전송
	_, err := s.session.ChannelMessageSendEmbed(result.ThreadID, embed)
	if err != nil {
		s.logger.Error("Failed to send result to Discord",
			zap.String("task_id", result.TaskID),
			zap.String("thread_id", result.ThreadID),
			zap.Error(err),
		)
	} else {
		s.logger.Info("Result sent to Discord",
			zap.String("task_id", result.TaskID),
			zap.String("thread_id", result.ThreadID),
		)
	}
}