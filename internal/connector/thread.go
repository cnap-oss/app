package connector

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/cnap-oss/app/internal/controller"
	"go.uber.org/zap"
)

// messageCreateHandler는 새로운 메시지가 생성될 때 호출됩니다.
func (s *Server) messageCreateHandler(_ *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.session.State.User.ID {
		return
	}

	s.threadsMutex.RLock()
	agentName, ok := s.activeThreads[m.ChannelID]
	s.threadsMutex.RUnlock()

	if ok {
		ctx := context.Background()
		agent, err := s.controller.GetAgentInfo(ctx, agentName)
		if err != nil {
			s.logger.Error("Failed to get agent info from controller for message handler", zap.Error(err), zap.String("agent_id", agentName))
			if _, sendErr := s.session.ChannelMessageSend(m.ChannelID, "오류: 이 스레드에 연결된 에이전트를 찾을 수 없습니다."); sendErr != nil {
				s.logger.Error("Failed to send error message to channel", zap.Error(sendErr), zap.String("channel_id", m.ChannelID))
			}
			return
		}
		s.callAgentInThread(m.Message, agent)
	}
}

// startAgentThread는 지정된 에이전트와의 새로운 대화 스레드를 시작합니다.
func (s *Server) startAgentThread(i *discordgo.InteractionCreate, agentName string) {
	ctx := context.Background()
	agent, err := s.controller.GetAgentInfo(ctx, agentName)
	if err != nil {
		s.logger.Error("Failed to get agent info from controller", zap.Error(err), zap.String("agent_id", agentName))
		s.respondEphemeral(i, fmt.Sprintf("오류: 에이전트 '**%s**'의 정보를 가져오는 데 실패했어요. 에러: %v", agentName, err))
		return
	}

	s.respondEphemeral(i, fmt.Sprintf("'**%s**'와의 대화 스레드를 생성 중...", agentName))

	thread, err := s.session.ThreadStart(i.ChannelID, fmt.Sprintf("[%s] 대화방", agent.Name), discordgo.ChannelTypeGuildPublicThread, 60)
	if err != nil {
		s.logger.Error("Failed to create thread", zap.Error(err), zap.String("agent", agentName))
		return
	}

	s.threadsMutex.Lock()
	s.activeThreads[thread.ID] = agent.Name
	s.threadsMutex.Unlock()

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("'%s'와의 대화 시작", agent.Name),
		Description: "이 스레드에 메시지를 입력하여 대화를 시작하세요.",
		Color:       0x33cc33, // Green
		Fields: []*discordgo.MessageEmbedField{
			{Name: "에이전트 모델", Value: agent.Model, Inline: true},
			{Name: "역할 정의 (프롬프트)", Value: fmt.Sprintf("```\n%s\n```", agent.Prompt), Inline: false},
		},
	}
	if _, err := s.session.ChannelMessageSendEmbed(thread.ID, embed); err != nil {
		s.logger.Error("Failed to send initial thread message", zap.Error(err), zap.String("thread_id", thread.ID))
	}
}

// callAgentInThread는 활성화된 에이전트 스레드 내에서 메시지를 처리합니다.
// Thread ID를 Task ID로 사용하여 하나의 Thread 내 모든 대화가 동일한 Task에서 처리됩니다.
func (s *Server) callAgentInThread(m *discordgo.Message, agent *controller.AgentInfo) {
	ctx := context.Background()

	// Thread ID를 Task ID로 사용 (Thread-Task 1:1 매핑)
	taskID := m.ChannelID
	threadID := m.ChannelID

	s.logger.Info("Processing message in thread",
		zap.String("agent", agent.Name),
		zap.String("task_id", taskID),
		zap.String("thread_id", threadID),
		zap.String("user_message", m.Content),
	)

	// Task 존재 여부 확인
	existingTask, err := s.controller.GetTask(ctx, taskID)
	if err != nil {
		// Task가 없으면 새로 생성 (Thread 첫 메시지)
		s.logger.Info("Creating new task for thread",
			zap.String("task_id", taskID),
			zap.String("agent", agent.Name),
		)

		if err := s.controller.CreateTask(ctx, agent.Name, taskID, m.Content); err != nil {
			s.logger.Error("Failed to create task", zap.Error(err))
			_, _ = s.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ Task 생성 실패: %v", err))
			return
		}
	} else {
		// Task가 있으면 메시지만 추가 (Thread 후속 메시지)
		s.logger.Info("Adding message to existing task",
			zap.String("task_id", taskID),
			zap.String("existing_status", existingTask.Status),
		)

		if err := s.controller.AddMessage(ctx, taskID, "user", m.Content); err != nil {
			s.logger.Error("Failed to add message to task", zap.Error(err))
			_, _ = s.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ 메시지 추가 실패: %v", err))
			return
		}
	}

	// "처리 중" 메시지 전송
	embed := &discordgo.MessageEmbed{
		Author:      &discordgo.MessageEmbedAuthor{Name: m.Author.Username, IconURL: m.Author.AvatarURL("")},
		Description: m.Content,
		Color:       0x0099ff, // Blue
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("'%s'가 처리 중입니다...", agent.Name)},
	}
	if _, err := s.session.ChannelMessageSendEmbed(m.ChannelID, embed); err != nil {
		s.logger.Error("Failed to send processing message", zap.Error(err))
	}

	// Task 실행 이벤트 전송 (비동기, 논블로킹)
	s.connectorEventChan <- controller.ConnectorEvent{
		Type:     "execute",
		TaskID:   taskID,
		ThreadID: threadID,
		Prompt:   m.Content,
	}

	s.logger.Info("Task execution event sent",
		zap.String("task_id", taskID),
		zap.String("agent", agent.Name),
	)
}