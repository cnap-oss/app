package handlers

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/cnap-oss/app/internal/controller"
	"go.uber.org/zap"
)

// Discord 명령어 및 UI 요소에 사용될 상수들을 정의합니다.
const (
	cmdAgent          = "agent"
	subCmdCreate      = "create"
	subCmdList        = "list"
	subCmdView        = "view"
	subCmdDelete      = "delete"
	subCmdEdit        = "edit"
	subCmdCall        = "call"
	prefixModalCreate = "modal_agent_create"
	prefixModalEdit   = "modal_agent_edit_"
	prefixButtonEdit  = "edit_agent_"
)

// DiscordHandler는 Discord 이벤트 및 상호작용을 처리합니다.
type DiscordHandler struct {
	logger             *zap.Logger
	session            *discordgo.Session
	controller         *controller.Controller
	connectorEventChan chan controller.ConnectorEvent
	controllerHandler  *ControllerHandler
	threadsMutex       sync.RWMutex
	activeThreads      map[string]string
}

// NewDiscordHandler는 새로운 DiscordHandler를 생성합니다.
func NewDiscordHandler(
	logger *zap.Logger,
	session *discordgo.Session,
	ctrl *controller.Controller,
	eventChan chan controller.ConnectorEvent,
) *DiscordHandler {
	return &DiscordHandler{
		logger:             logger.With(zap.String("handler", "discord")),
		session:            session,
		controller:         ctrl,
		connectorEventChan: eventChan,
		activeThreads:      make(map[string]string),
	}
}

// SetControllerHandler는 ControllerHandler를 설정합니다.
func (h *DiscordHandler) SetControllerHandler(handler *ControllerHandler) {
	h.controllerHandler = handler
}

// RegisterHandlers는 Discord 세션에 이벤트 핸들러를 등록합니다.
func (h *DiscordHandler) RegisterHandlers() {
	h.session.AddHandler(h.readyHandler)
	h.session.AddHandler(h.interactionRouter)
	h.session.AddHandler(h.messageCreateHandler)
}

// readyHandler는 봇이 Discord에 성공적으로 연결되었을 때 호출됩니다.
// 여기서 전역 애플리케이션 명령어를 등록합니다.
func (h *DiscordHandler) readyHandler(_ *discordgo.Session, r *discordgo.Ready) {
	h.logger.Info("Bot is ready! Registering commands...", zap.String("username", r.User.Username))

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

	_, err := h.session.ApplicationCommandBulkOverwrite(h.session.State.User.ID, "", commands)
	if err != nil {
		h.logger.Error("Could not register commands", zap.Error(err))
	} else {
		h.logger.Info("Successfully registered commands.")
	}
}

// interactionRouter는 Discord 상호작용을 적절한 핸들러로 라우팅합니다.
func (h *DiscordHandler) interactionRouter(_ *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		h.handleSlashCommand(i)
	case discordgo.InteractionMessageComponent:
		h.handleButton(i)
	case discordgo.InteractionModalSubmit:
		h.handleModal(i)
	case discordgo.InteractionApplicationCommandAutocomplete:
		h.handleAutocomplete(i)
	}
}

// handleButton은 버튼 클릭 상호작용을 처리합니다.
func (h *DiscordHandler) handleButton(i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	if strings.HasPrefix(customID, prefixButtonEdit) {
		agentName := strings.TrimPrefix(customID, prefixButtonEdit)
		ctx := context.Background()
		agent, err := h.controller.GetAgentInfo(ctx, agentName)
		if err != nil {
			h.logger.Error("Failed to get agent info from controller for edit button", zap.Error(err), zap.String("agent_id", agentName))
			h.respondEphemeral(i, "오류: 에이전트의 정보를 가져오는 데 실패했어요.")
			return
		}
		h.showCreateOrEditModal(i, agentName, agent)
	}
}

// handleAutocomplete는 자동 완성 상호작용을 처리합니다.
func (h *DiscordHandler) handleAutocomplete(i *discordgo.InteractionCreate) {
	options := i.ApplicationCommandData().Options[0].Options[0]
	if options.Focused {
		ctx := context.Background()
		agents, err := h.controller.ListAgentsWithInfo(ctx)
		if err != nil {
			h.logger.Error("Failed to list agents from controller for autocomplete", zap.Error(err))
			// Can't respond with an ephemeral message here, so we just log and return empty choices
			_ = h.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionApplicationCommandAutocompleteResult, Data: &discordgo.InteractionResponseData{Choices: []*discordgo.ApplicationCommandOptionChoice{}}})
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

		if err := h.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionApplicationCommandAutocompleteResult, Data: &discordgo.InteractionResponseData{Choices: choices}}); err != nil {
			h.logger.Error("Failed to send autocomplete response", zap.Error(err))
		}
	}
}

// showAgentList는 현재 등록된 모든 에이전트의 목록을 Discord에 표시합니다.
func (h *DiscordHandler) showAgentList(i *discordgo.InteractionCreate) {
	ctx := context.Background()
	agents, err := h.controller.ListAgentsWithInfo(ctx)
	if err != nil {
		h.logger.Error("Failed to list agents from controller", zap.Error(err))
		h.respondEphemeral(i, fmt.Sprintf("오류: 에이전트 목록을 불러오는 데 실패했어요. 에러: %v", err))
		return
	}

	if len(agents) == 0 {
		h.respondEphemeral(i, "생성된 에이전트가 아직 없어요. `/agent create`로 먼저 생성해주세요!")
		return
	}
	fields := []*discordgo.MessageEmbedField{}
	for _, agent := range agents {
		fields = append(fields, &discordgo.MessageEmbedField{Name: agent.Name, Value: agent.Description, Inline: false})
	}
	err = h.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
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
		h.logger.Error("Failed to show agent list", zap.Error(err))
	}
}

// showAgentDetails는 특정 에이전트의 상세 정보를 Discord에 표시합니다.
func (h *DiscordHandler) showAgentDetails(i *discordgo.InteractionCreate, name string) {
	ctx := context.Background()
	agent, err := h.controller.GetAgentInfo(ctx, name)
	if err != nil {
		h.logger.Error("Failed to get agent info from controller", zap.Error(err), zap.String("agent_id", name))
		h.respondEphemeral(i, fmt.Sprintf("오류: 에이전트 '**%s**'의 정보를 가져오는 데 실패했어요. 에러: %v", name, err))
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
	err = h.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}}})
	if err != nil {
		h.logger.Error("Failed to show agent details", zap.Error(err), zap.String("agent", name))
	}
}

// deleteAgent는 지정된 이름의 에이전트를 삭제합니다.
func (h *DiscordHandler) deleteAgent(i *discordgo.InteractionCreate, name string) {
	ctx := context.Background()
	if err := h.controller.DeleteAgent(ctx, name); err != nil {
		h.logger.Error("Failed to delete agent from controller", zap.Error(err), zap.String("agent_id", name))
		h.respondEphemeral(i, fmt.Sprintf("오류: 에이전트 '**%s**'을(를) 삭제하는 데 실패했어요. 에러: %v", name, err))
		return
	}

	// activeThreads 맵에서 삭제된 에이전트와 연결된 스레드 정리
	h.threadsMutex.Lock()
	defer h.threadsMutex.Unlock()
	for threadID, agentName := range h.activeThreads {
		if agentName == name {
			delete(h.activeThreads, threadID)
		}
	}
	h.respondEphemeral(i, fmt.Sprintf("에이전트 '**%s**'이(가) 성공적으로 삭제되었어요.", name))
}

// showEditUI는 특정 에이전트의 현재 정보를 임베드 메시지로 표시하고, 수정 모달을 열기 위한 버튼을 제공합니다.
func (h *DiscordHandler) showEditUI(i *discordgo.InteractionCreate, name string) {
	ctx := context.Background()
	agent, err := h.controller.GetAgentInfo(ctx, name)
	if err != nil {
		h.logger.Error("Failed to get agent info from controller", zap.Error(err), zap.String("agent_id", name))
		h.respondEphemeral(i, fmt.Sprintf("오류: 에이전트 '**%s**'의 정보를 가져오는 데 실패했어요. 에러: %v", name, err))
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
	err = h.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{
		Embeds: []*discordgo.MessageEmbed{embed},
		Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "정보 수정하기", Style: discordgo.PrimaryButton, CustomID: prefixButtonEdit + name},
		}}},
	}})
	if err != nil {
		h.logger.Error("Failed to show edit UI", zap.Error(err), zap.String("agent", name))
	}
}

// messageCreateHandler는 새로운 메시지가 생성될 때 호출됩니다.
func (h *DiscordHandler) messageCreateHandler(_ *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == h.session.State.User.ID {
		return
	}

	h.threadsMutex.RLock()
	agentName, ok := h.activeThreads[m.ChannelID]
	h.threadsMutex.RUnlock()

	if ok {
		ctx := context.Background()
		agent, err := h.controller.GetAgentInfo(ctx, agentName)
		if err != nil {
			h.logger.Error("Failed to get agent info from controller for message handler", zap.Error(err), zap.String("agent_id", agentName))
			if _, sendErr := h.session.ChannelMessageSend(m.ChannelID, "오류: 이 스레드에 연결된 에이전트를 찾을 수 없습니다."); sendErr != nil {
				h.logger.Error("Failed to send error message to channel", zap.Error(sendErr), zap.String("channel_id", m.ChannelID))
			}
			return
		}
		h.callAgentInThread(m.Message, agent)
	} else {
		task, err := h.controller.GetTask(context.Background(), m.ChannelID)
		if err == nil && task.AgentID != "" {
			h.threadsMutex.Lock()
			h.activeThreads[m.ChannelID] = task.AgentID
			h.threadsMutex.Unlock()
			ctx := context.Background()
			agent, err := h.controller.GetAgentInfo(ctx, task.AgentID)
			if err != nil {
				h.logger.Error("Failed to get agent info from controller for existing task", zap.Error(err), zap.String("agent_id", task.AgentID))
				return
			}
			h.callAgentInThread(m.Message, agent)
		}
	}
}

// startAgentThread는 지정된 에이전트와의 새로운 대화 스레드를 시작합니다.
func (h *DiscordHandler) startAgentThread(i *discordgo.InteractionCreate, agentName string) {
	ctx := context.Background()
	agent, err := h.controller.GetAgentInfo(ctx, agentName)
	if err != nil {
		h.logger.Error("Failed to get agent info from controller", zap.Error(err), zap.String("agent_id", agentName))
		h.respondEphemeral(i, fmt.Sprintf("오류: 에이전트 '**%s**'의 정보를 가져오는 데 실패했어요. 에러: %v", agentName, err))
		return
	}

	h.respondEphemeral(i, fmt.Sprintf("'**%s**'와의 대화 스레드를 생성 중...", agentName))

	thread, err := h.session.ThreadStart(i.ChannelID, fmt.Sprintf("[%s] 대화방", agent.Name), discordgo.ChannelTypeGuildPublicThread, 60)
	if err != nil {
		h.logger.Error("Failed to create thread", zap.Error(err), zap.String("agent", agentName))
		return
	}

	h.threadsMutex.Lock()
	h.activeThreads[thread.ID] = agent.Name
	h.threadsMutex.Unlock()

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("'%s'와의 대화 시작", agent.Name),
		Description: "이 스레드에 메시지를 입력하여 대화를 시작하세요.",
		Color:       0x0099FF, // Blue
		Fields: []*discordgo.MessageEmbedField{
			{Name: "에이전트 모델", Value: agent.Model, Inline: true},
			{Name: "상태", Value: "⏳ 대기 중", Inline: true},
			{Name: "역할 정의 (프롬프트)", Value: fmt.Sprintf("```\n%s\n```", agent.Prompt), Inline: false},
		},
	}
	msg, err := h.session.ChannelMessageSendEmbed(thread.ID, embed)
	if err != nil {
		h.logger.Error("Failed to send initial thread message", zap.Error(err), zap.String("thread_id", thread.ID))
	} else if h.controllerHandler != nil {
		// Thread 메인 메시지 ID를 ControllerHandler에 등록
		h.controllerHandler.RegisterThreadMainMessage(thread.ID, msg.ID)
	}
}

// callAgentInThread는 활성화된 에이전트 스레드 내에서 메시지를 처리합니다.
// Thread ID를 Task ID로 사용하여 하나의 Thread 내 모든 대화가 동일한 Task에서 처리됩니다.
func (h *DiscordHandler) callAgentInThread(m *discordgo.Message, agent *controller.AgentInfo) {
	ctx := context.Background()

	// Thread ID를 Task ID로 사용 (Thread-Task 1:1 매핑)
	taskID := m.ChannelID
	threadID := m.ChannelID

	h.logger.Info("Processing message in thread",
		zap.String("agent", agent.Name),
		zap.String("task_id", taskID),
		zap.String("thread_id", threadID),
		zap.String("user_message", m.Content),
	)

	_, err := h.controller.GetTask(ctx, taskID)
	if err != nil {
		// Task 실행 이벤트 전송 (새 Task, 비동기, 논블로킹)
		h.connectorEventChan <- controller.ConnectorEvent{
			Type:      "execute",
			TaskID:    taskID,
			AgentName: agent.Name,
			Prompt:    m.Content,
		}
	} else {
		// "처리 중" 메시지 전송
		embed := &discordgo.MessageEmbed{
			Author:      &discordgo.MessageEmbedAuthor{Name: m.Author.Username, IconURL: m.Author.AvatarURL("")},
			Description: m.Content,
			Color:       0x0099ff, // Blue
			Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("'%s'가 처리 중입니다...", agent.Name)},
		}
		if _, err := h.session.ChannelMessageSendEmbed(m.ChannelID, embed); err != nil {
			h.logger.Error("Failed to send processing message", zap.Error(err))
		}

		// continue 이벤트 전송 (기존 Task 실행 계속)
		h.connectorEventChan <- controller.ConnectorEvent{
			Type:      "continue",
			TaskID:    taskID,
			AgentName: agent.Name,
			Prompt:    m.Content,
		}

		h.logger.Info("Task continue event sent",
			zap.String("task_id", taskID),
			zap.String("agent", agent.Name),
		)
		return
	}

	// "처리 중" 메시지 전송 (새 Task 생성 시)
	embed := &discordgo.MessageEmbed{
		Author:      &discordgo.MessageEmbedAuthor{Name: m.Author.Username, IconURL: m.Author.AvatarURL("")},
		Description: m.Content,
		Color:       0x0099ff, // Blue
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("'%s'가 처리 중입니다...", agent.Name)},
	}
	if _, err := h.session.ChannelMessageSendEmbed(m.ChannelID, embed); err != nil {
		h.logger.Error("Failed to send processing message", zap.Error(err))
	}

	h.logger.Info("Task execution event sent",
		zap.String("task_id", taskID),
		zap.String("agent", agent.Name),
	)
}

// respondEphemeral은 사용자에게만 보이는 임시 메시지를 전송합니다.
func (h *DiscordHandler) respondEphemeral(i *discordgo.InteractionCreate, content string) {
	err := h.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: content, Flags: discordgo.MessageFlagsEphemeral}})
	if err != nil {
		h.logger.Error("Failed to send ephemeral message", zap.Error(err))
	}
}
