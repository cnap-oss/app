package connector

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

// showAgentList는 현재 등록된 모든 에이전트의 목록을 Discord에 표시합니다.
func (s *Server) showAgentList(i *discordgo.InteractionCreate) {
	ctx := context.Background()
	agents, err := s.controller.ListAgentsWithInfo(ctx)
	if err != nil {
		s.logger.Error("Failed to list agents from controller", zap.Error(err))
		s.respondEphemeral(i, fmt.Sprintf("오류: 에이전트 목록을 불러오는 데 실패했어요. 에러: %v", err))
		return
	}

	if len(agents) == 0 {
		s.respondEphemeral(i, "생성된 에이전트가 아직 없어요. `/agent create`로 먼저 생성해주세요!")
		return
	}
	fields := []*discordgo.MessageEmbedField{}
	for _, agent := range agents {
		fields = append(fields, &discordgo.MessageEmbedField{Name: agent.Name, Value: agent.Description, Inline: false})
	}
	err = s.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{{
				Title:  "생성된 에이전트 목록",
				Fields: fields,
				Color:  0x0099ff,
			}},
		},
	})
	if err != nil {
		s.logger.Error("Failed to show agent list", zap.Error(err))
	}
}

// showAgentDetails는 특정 에이전트의 상세 정보를 Discord에 표시합니다.
func (s *Server) showAgentDetails(i *discordgo.InteractionCreate, name string) {
	ctx := context.Background()
	agent, err := s.controller.GetAgentInfo(ctx, name)
	if err != nil {
		s.logger.Error("Failed to get agent info from controller", zap.Error(err), zap.String("agent_id", name))
		s.respondEphemeral(i, fmt.Sprintf("오류: 에이전트 '**%s**'의 정보를 가져오는 데 실패했어요. 에러: %v", name, err))
		return
	}

	embed := &discordgo.MessageEmbed{
		Title: "에이전트 상세 정보: " + agent.Name, Color: 0x0099ff,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "설명", Value: agent.Description},
			{Name: "모델", Value: agent.Model, Inline: true},
			{Name: "역할 정의 (프롬프트)", Value: fmt.Sprintf("```\n%s\n```", agent.Prompt)},
			{Name: "실행한 작업 목록", Value: "(아직 구현되지 않은 기능이에요)"},
		},
	}
	err = s.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}}})
	if err != nil {
		s.logger.Error("Failed to show agent details", zap.Error(err), zap.String("agent", name))
	}
}

// deleteAgent는 지정된 이름의 에이전트를 삭제합니다.
func (s *Server) deleteAgent(i *discordgo.InteractionCreate, name string) {
	ctx := context.Background()
	if err := s.controller.DeleteAgent(ctx, name); err != nil {
		s.logger.Error("Failed to delete agent from controller", zap.Error(err), zap.String("agent_id", name))
		s.respondEphemeral(i, fmt.Sprintf("오류: 에이전트 '**%s**'을(를) 삭제하는 데 실패했어요. 에러: %v", name, err))
		return
	}

	// activeThreads 맵에서 삭제된 에이전트와 연결된 스레드 정리
	s.threadsMutex.Lock()
	defer s.threadsMutex.Unlock()
	for threadID, agentName := range s.activeThreads {
		if agentName == name {
			delete(s.activeThreads, threadID)
		}
	}
	s.respondEphemeral(i, fmt.Sprintf("에이전트 '**%s**'이(가) 성공적으로 삭제되었어요.", name))
}

// showEditUI는 특정 에이전트의 현재 정보를 임베드 메시지로 표시하고, 수정 모달을 열기 위한 버튼을 제공합니다.
func (s *Server) showEditUI(i *discordgo.InteractionCreate, name string) {
	ctx := context.Background()
	agent, err := s.controller.GetAgentInfo(ctx, name)
	if err != nil {
		s.logger.Error("Failed to get agent info from controller", zap.Error(err), zap.String("agent_id", name))
		s.respondEphemeral(i, fmt.Sprintf("오류: 에이전트 '**%s**'의 정보를 가져오는 데 실패했어요. 에러: %v", name, err))
		return
	}

	embed := &discordgo.MessageEmbed{
		Title: "에이전트 수정: " + agent.Name, Description: "아래는 현재 정보예요. 수정하려면 버튼을 눌러주세요.", Color: 0xffaa00,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "설명", Value: agent.Description},
			{Name: "모델", Value: agent.Model, Inline: true},
			{Name: "역할 정의 (프롬프트)", Value: fmt.Sprintf("```\n%s\n```", agent.Prompt)},
		},
	}
	err = s.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{
		Embeds: []*discordgo.MessageEmbed{embed},
		Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "정보 수정하기", Style: discordgo.PrimaryButton, CustomID: prefixButtonEdit + name},
		}}},
	}})
	if err != nil {
		s.logger.Error("Failed to show edit UI", zap.Error(err), zap.String("agent", name))
	}
}
