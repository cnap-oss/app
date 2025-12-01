package connector

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/cnap-oss/app/internal/controller"
	"github.com/joho/godotenv"
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

// Server는 Discord 봇의 세션, 로거, 에이전트 데이터 등 모든 상태를 관리하는 중앙 구조체입니다.
type Server struct {
	logger         *zap.Logger
	session        *discordgo.Session
	controller     *controller.Controller
	threadsMutex   sync.RWMutex
	activeThreads  map[string]string
	taskEventChan  chan controller.TaskEvent
	taskResultChan <-chan controller.TaskResult
}

// NewServer는 새로운 connector 서버를 생성하고 초기화합니다.
func NewServer(logger *zap.Logger, ctrl *controller.Controller, eventChan chan controller.TaskEvent, resultChan <-chan controller.TaskResult) *Server {
	return &Server{
		logger:         logger,
		controller:     ctrl,
		activeThreads:  make(map[string]string),
		taskEventChan:  eventChan,
		taskResultChan: resultChan,
	}
}

// Start는 Discord 봇을 시작하고 Discord API에 연결합니다.
// 환경 변수 로드, 세션 생성, 이벤트 핸들러 등록, 연결 열기 등의 작업을 수행합니다.
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("Starting connector server (Discord Bot)")

	if err := godotenv.Load(); err != nil {
		s.logger.Warn("Could not load .env file", zap.Error(err))
	}
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		return fmt.Errorf("DISCORD_TOKEN environment variable not set")
	}

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return fmt.Errorf("error creating Discord session: %w", err)
	}
	s.session = dg
	s.session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages

	s.session.AddHandler(s.readyHandler)
	s.session.AddHandler(s.interactionRouter)
	s.session.AddHandler(s.messageCreateHandler)

	if err := s.session.Open(); err != nil {
		return fmt.Errorf("error opening connection: %w", err)
	}

	s.logger.Info("Bot is now running.")

	// 결과 핸들러 goroutine 시작
	go s.resultHandler(ctx)

	// 컨텍스트가 취소될 때까지 대기
	<-ctx.Done()
	s.logger.Info("Connector server shutting down")
	return s.Stop(context.Background()) // 컨텍스트가 이미 완료되었으므로 새 컨텍스트로 Stop 호출
}

// Stop은 Discord 세션을 정상적으로 닫고 봇을 종료합니다.
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Stopping connector server")
	if s.session != nil {
		if err := s.session.Close(); err != nil {
			s.logger.Error("Error closing discord session", zap.Error(err))
			return err
		}
	}
	s.logger.Info("Connector server stopped")
	return nil
}

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
			s.respondEphemeral(i, fmt.Sprintf("오류: 에이전트 '**%s**'의 정보를 가져오는 데 실패했어요. 에러: %v", agentName, err))
			return
		}
		s.showCreateOrEditModal(i, agentName, agent)
	}
}

// handleModal은 모달 제출 상호작용을 처리합니다.
func (s *Server) handleModal(i *discordgo.InteractionCreate) {
	ctx := context.Background() // Create a context
	customID := i.ModalSubmitData().CustomID
	data := i.ModalSubmitData().Components
	name := data[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
	desc := data[1].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
	model := data[2].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
	prompt := data[3].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value

	switch {
	case customID == prefixModalCreate:
		// Assume controller's CreateAgent will handle conflicts (e.g., already exists)
		// Discord 봇에서는 기본 provider로 "opencode" 사용
		if err := s.controller.CreateAgent(ctx, name, desc, "opencode", model, prompt); err != nil {
			s.logger.Error("Failed to create agent via controller", zap.Error(err))
			s.respondEphemeral(i, fmt.Sprintf("오류: 에이전트 '**%s**'을(를) 생성하는 데 실패했어요. 에러: %v", name, err))
			return
		}
		s.respondEphemeral(i, fmt.Sprintf("에이전트 '**%s**'이(가) 성공적으로 생성되었어요!", name))
	case strings.HasPrefix(customID, prefixModalEdit):
		originalName := strings.TrimPrefix(customID, prefixModalEdit)
		// Assumes an UpdateAgent function exists in the controller that can handle renames.
		// Discord 봇에서는 기본 provider로 "opencode" 사용
		if err := s.controller.UpdateAgent(ctx, originalName, desc, "opencode", model, prompt); err != nil {
			s.logger.Error("Failed to update agent via controller", zap.Error(err), zap.String("original_agent_id", originalName))
			s.respondEphemeral(i, fmt.Sprintf("오류: 에이전트 '**%s**'을(를) 수정하는 데 실패했어요. 에러: %v", originalName, err))
			return
		}
		s.respondEphemeral(i, fmt.Sprintf("에이전트 '**%s**'의 정보가 성공적으로 수정되었어요!", name))
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

// respondEphemeral은 사용자에게만 보이는 임시 메시지를 전송합니다.
func (s *Server) respondEphemeral(i *discordgo.InteractionCreate, content string) {
	err := s.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: content, Flags: discordgo.MessageFlagsEphemeral}})
	if err != nil {
		s.logger.Error("Failed to send ephemeral message", zap.Error(err))
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
func (s *Server) callAgentInThread(m *discordgo.Message, agent *controller.AgentInfo) {
	ctx := context.Background()

	// Task ID 생성 (Discord message ID 기반)
	taskID := fmt.Sprintf("task-%s", m.ID)
	threadID := m.ChannelID

	s.logger.Info("Creating task for agent call",
		zap.String("agent", agent.Name),
		zap.String("task_id", taskID),
		zap.String("thread_id", threadID),
		zap.String("user_message", m.Content),
	)

	// 1. Task 생성
	if err := s.controller.CreateTask(ctx, agent.Name, taskID, m.Content); err != nil {
		s.logger.Error("Failed to create task", zap.Error(err))
		_, _ = s.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ Task 생성 실패: %v", err))
		return
	}

	// 2. "처리 중" 메시지 전송
	embed := &discordgo.MessageEmbed{
		Author:      &discordgo.MessageEmbedAuthor{Name: m.Author.Username, IconURL: m.Author.AvatarURL("")},
		Description: m.Content,
		Color:       0x0099ff, // Blue
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("'%s'가 처리 중입니다...", agent.Name)},
	}
	if _, err := s.session.ChannelMessageSendEmbed(m.ChannelID, embed); err != nil {
		s.logger.Error("Failed to send processing message", zap.Error(err))
	}

	// 3. Task 실행 이벤트 전송 (비동기, 논블로킹)
	s.taskEventChan <- controller.TaskEvent{
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

	// This part is tricky. The activeThreads map links a discord thread to an agent name.
	// If an agent is deleted, we should probably also handle the active threads.
	// The controller doesn't know about discord threads. This logic should probably remain here.
	s.threadsMutex.Lock()
	defer s.threadsMutex.Unlock()
	for threadID, agentName := range s.activeThreads {
		if agentName == name {
			delete(s.activeThreads, threadID)
			// Maybe notify the thread that the agent is gone? For now, just deleting the link is fine.
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
