package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/cnap-oss/app/internal/controller"
	"go.uber.org/zap"
)

// handleModal은 모달 제출 상호작용을 처리합니다.
func (s *Server) handleModal(i *discordgo.InteractionCreate) {
	ctx := context.Background()
	customID := i.ModalSubmitData().CustomID
	data := i.ModalSubmitData().Components
	name := data[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
	desc := data[1].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
	model := data[2].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
	prompt := data[3].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value

	switch {
	case customID == prefixModalCreate:
		// Discord 봇에서는 기본 provider로 "opencode" 사용
		if err := s.controller.CreateAgent(ctx, name, desc, "opencode", model, prompt); err != nil {
			s.logger.Error("Failed to create agent via controller", zap.Error(err))
			s.respondEphemeral(i, fmt.Sprintf("오류: 에이전트 '**%s**'을(를) 생성하는 데 실패했어요. 에러: %v", name, err))
			return
		}
		s.respondEphemeral(i, fmt.Sprintf("에이전트 '**%s**'이(가) 성공적으로 생성되었어요!", name))
	case strings.HasPrefix(customID, prefixModalEdit):
		originalName := strings.TrimPrefix(customID, prefixModalEdit)
		// Discord 봇에서는 기본 provider로 "opencode" 사용
		if err := s.controller.UpdateAgent(ctx, originalName, desc, "opencode", model, prompt); err != nil {
			s.logger.Error("Failed to update agent via controller", zap.Error(err), zap.String("original_agent_id", originalName))
			s.respondEphemeral(i, fmt.Sprintf("오류: 에이전트 '**%s**'을(를) 수정하는 데 실패했어요. 에러: %v", originalName, err))
			return
		}
		s.respondEphemeral(i, fmt.Sprintf("에이전트 '**%s**'의 정보가 성공적으로 수정되었어요!", name))
	}
}

// showCreateOrEditModal은 에이전트 생성/수정 모달을 표시합니다.
func (s *Server) showCreateOrEditModal(i *discordgo.InteractionCreate, originalName string, agent *controller.AgentInfo) {
	modalTitle := "새로운 에이전트 생성"
	customID := prefixModalCreate
	name, desc, model, prompt := "", "", "", ""

	if agent != nil { // 수정 모드
		modalTitle = "에이전트 정보 수정"
		customID = prefixModalEdit + originalName
		name, desc, model, prompt = agent.Name, agent.Description, agent.Model, agent.Prompt
	}

	modal := &discordgo.InteractionResponseData{
		CustomID: customID, Title: modalTitle,
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "name", Label: "이름", Style: discordgo.TextInputShort, Required: true, Value: name}}},
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "desc", Label: "설명", Style: discordgo.TextInputParagraph, Required: true, Value: desc}}},
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "model", Label: "모델", Style: discordgo.TextInputShort, Required: true, Value: model}}},
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: "prompt", Label: "역할 정의 (프롬프트)", Style: discordgo.TextInputParagraph, Required: true, Value: prompt}}},
		},
	}
	err := s.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseModal, Data: modal})
	if err != nil {
		s.logger.Error("Failed to show create/edit modal", zap.Error(err))
	}
}
