package connector

import (
	"context"
	"strings"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

// readyHandler는 봇이 Discord에 성공적으로 연결되었을 때 호출됩니다.
// 여기서 전역 애플리케이션 명령어를 등록합니다.
func (s *Server) readyHandler(_ *discordgo.Session, r *discordgo.Ready) {
	s.logger.Info("Bot is ready! Registering commands...", zap.String("username", r.User.Username))

	commands := []*discordgo.ApplicationCommand{
		{
			Name:        cmdAgent,
			Description: "에이전트 관리 및 호출 명령어",
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionSubCommand, Name: subCmdCreate, Description: "새로운 에이전트를 생성합니다."},
				{Type: discordgo.ApplicationCommandOptionSubCommand, Name: subCmdList, Description: "생성된 에이전트 목록을 봅니다."},
				{Type: discordgo.ApplicationCommandOptionSubCommand, Name: subCmdView, Description: "특정 에이전트의 상세 정보를 봅니다.", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "정보를 볼 에이전트의 이름", Required: true, Autocomplete: true}}},
				{Type: discordgo.ApplicationCommandOptionSubCommand, Name: subCmdDelete, Description: "특정 에이전트를 삭제합니다.", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "삭제할 에이전트의 이름", Required: true, Autocomplete: true}}},
				{Type: discordgo.ApplicationCommandOptionSubCommand, Name: subCmdEdit, Description: "특정 에이전트의 정보를 수정합니다.", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "수정할 에이전트의 이름", Required: true, Autocomplete: true}}},
				{Type: discordgo.ApplicationCommandOptionSubCommand, Name: subCmdCall, Description: "에이전트와의 대화 스레드를 시작합니다.", Options: []*discordgo.ApplicationCommandOption{{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "호출할 에이전트의 이름", Required: true, Autocomplete: true}}},
			},
		},
	}

	_, err := s.session.ApplicationCommandBulkOverwrite(s.session.State.User.ID, "", commands)
	if err != nil {
		s.logger.Error("Could not register commands", zap.Error(err))
	} else {
		s.logger.Info("Successfully registered commands.")
	}
}

// interactionRouter는 Discord 상호작용을 적절한 핸들러로 라우팅합니다.
func (s *Server) interactionRouter(_ *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		s.handleSlashCommand(i)
	case discordgo.InteractionMessageComponent:
		s.handleButton(i)
	case discordgo.InteractionModalSubmit:
		s.handleModal(i)
	case discordgo.InteractionApplicationCommandAutocomplete:
		s.handleAutocomplete(i)
	}
}

// handleSlashCommand는 '/agent' 슬래시 명령어를 처리합니다.
func (s *Server) handleSlashCommand(i *discordgo.InteractionCreate) {
	if i.ApplicationCommandData().Name != cmdAgent {
		return
	}
	subCommand := i.ApplicationCommandData().Options[0]
	switch subCommand.Name {
	case subCmdCreate:
		s.showCreateOrEditModal(i, "", nil)
	case subCmdList:
		s.showAgentList(i)
	case subCmdView:
		s.showAgentDetails(i, subCommand.Options[0].StringValue())
	case subCmdDelete:
		s.deleteAgent(i, subCommand.Options[0].StringValue())
	case subCmdEdit:
		s.showEditUI(i, subCommand.Options[0].StringValue())
	case subCmdCall:
		s.startAgentThread(i, subCommand.Options[0].StringValue())
	}
}

// handleButton은 버튼 클릭 상호작용을 처리합니다.
func (s *Server) handleButton(i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	if strings.HasPrefix(customID, prefixButtonEdit) {
		agentName := strings.TrimPrefix(customID, prefixButtonEdit)
		ctx := context.Background()
		agent, err := s.controller.GetAgentInfo(ctx, agentName)
		if err != nil {
			s.logger.Error("Failed to get agent info from controller for edit button", zap.Error(err), zap.String("agent_id", agentName))
			s.respondEphemeral(i, "오류: 에이전트의 정보를 가져오는 데 실패했어요.")
			return
		}
		s.showCreateOrEditModal(i, agentName, agent)
	}
}

// handleAutocomplete는 자동 완성 상호작용을 처리합니다.
func (s *Server) handleAutocomplete(i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options[0].Options[0]
	if options.Focused {
		ctx := context.Background()
		agents, err := s.controller.ListAgentsWithInfo(ctx)
		if err != nil {
			s.logger.Error("Failed to list agents from controller for autocomplete", zap.Error(err))
			// Can't respond with an ephemeral message here, so we just log and return empty choices
			_ = s.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionApplicationCommandAutocompleteResult, Data: &discordgo.InteractionResponseData{Choices: []*discordgo.ApplicationCommandOptionChoice{}}})
			return
		}

		var choices []*discordgo.ApplicationCommandOptionChoice
		for _, agent := range agents {
			if strings.HasPrefix(strings.ToLower(agent.Name), strings.ToLower(options.StringValue())) {
				choices = append(choices, &discordgo.ApplicationCommandOptionChoice{Name: agent.Name, Value: agent.Name})
			}
		}

		// Discord has a limit of 25 choices for autocomplete
		if len(choices) > 25 {
			choices = choices[:25]
		}

		if err := s.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionApplicationCommandAutocompleteResult, Data: &discordgo.InteractionResponseData{Choices: choices}}); err != nil {
			s.logger.Error("Failed to send autocomplete response", zap.Error(err))
		}
	}
}