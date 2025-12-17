package connector

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/cnap-oss/app/internal/common"
	"github.com/cnap-oss/app/internal/connector/handlers"
	"github.com/cnap-oss/app/internal/controller"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

// Connector는 Discord 봇의 세션, 로거, 에이전트 데이터 등 모든 상태를 관리하는 중앙 구조체입니다.
type Connector struct {
	logger              *zap.Logger
	session             *discordgo.Session
	controller          *controller.Controller
	connectorEventChan  chan controller.ConnectorEvent
	controllerEventChan <-chan controller.ControllerEvent
	discordHandler      *handlers.DiscordHandler
	controllerHandler   *handlers.ControllerHandler
	config              *common.Config
}

// NewServer는 새로운 connector 서버를 생성하고 초기화합니다.
func NewServer(logger *zap.Logger, ctrl *controller.Controller, eventChan chan controller.ConnectorEvent, resultChan <-chan controller.ControllerEvent) *Connector {
	return &Connector{
		logger:              logger.Named("connector"),
		controller:          ctrl,
		connectorEventChan:  eventChan,
		controllerEventChan: resultChan,
	}
}

// Start는 Discord 봇을 시작하고 Discord API에 연결합니다.
// 환경 변수 로드, 세션 생성, 이벤트 핸들러 등록, 연결 열기 등의 작업을 수행합니다.
func (s *Connector) Start(ctx context.Context) error {
	s.logger.Info("Starting connector server (Discord Bot)")

	if err := godotenv.Load(); err != nil {
		s.logger.Warn("Could not load .env file", zap.Error(err))
	}

	// Load centralized config
	cfg, err := common.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	s.config = cfg

	if cfg.Discord.Token == "" {
		return fmt.Errorf("CNAP_DISCORD_TOKEN environment variable not set")
	}

	dg, err := discordgo.New("Bot " + cfg.Discord.Token)
	if err != nil {
		return fmt.Errorf("error creating Discord session: %w", err)
	}
	s.session = dg
	s.session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages

	// 핸들러 초기화
	s.discordHandler = handlers.NewDiscordHandler(s.logger, s.session, s.controller, s.connectorEventChan)
	s.controllerHandler = handlers.NewControllerHandler(s.logger, s.session)

	// Discord 이벤트 핸들러 등록
	s.discordHandler.RegisterHandlers()

	if err := s.session.Open(); err != nil {
		return fmt.Errorf("error opening connection: %w", err)
	}

	s.logger.Info("Bot is now running.")

	// Controller 이벤트 핸들러 goroutine 시작
	go s.controllerHandler.Start(ctx, s.controllerEventChan)

	// 컨텍스트가 취소될 때까지 대기
	<-ctx.Done()
	s.logger.Info("Connector server shutting down")
	return s.Stop(context.Background()) // 컨텍스트가 이미 완료되었으므로 새 컨텍스트로 Stop 호출
}

// Stop은 Discord 세션을 정상적으로 닫고 봇을 종료합니다.
func (s *Connector) Stop(ctx context.Context) error {
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
