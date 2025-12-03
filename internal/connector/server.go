package connector

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/cnap-oss/app/internal/controller"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
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

// respondEphemeral은 사용자에게만 보이는 임시 메시지를 전송합니다.
func (s *Server) respondEphemeral(i *discordgo.InteractionCreate, content string) {
	err := s.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: content, Flags: discordgo.MessageFlagsEphemeral}})
	if err != nil {
		s.logger.Error("Failed to send ephemeral message", zap.Error(err))
	}
}
