package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/cnap-oss/app/internal/connector"
	"github.com/cnap-oss/app/internal/controller"
	"github.com/cnap-oss/app/internal/storage"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	// Logger 초기화
	logger, err := initLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	rootCmd := &cobra.Command{
		Use:     "cnap",
		Short:   "CNAP - AI Agent Supervisor CLI",
		Long:    `CNAP is a command-line interface for managing AI agent supervisor and connector servers.`,
		Version: fmt.Sprintf("%s (built at %s)", Version, BuildTime),
	}

	// start 명령어
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start controller and connector server processes",
		Long:  `Start the server processes for internal/controller and internal/connector.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(logger)
		},
	}

	// health 명령어
	healthCmd := &cobra.Command{
		Use:   "health",
		Short: "Check application health status",
		Long:  `Check if the application is running and healthy.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("OK")
			return nil
		},
	}

	// 명령어 구성
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(healthCmd)
	rootCmd.AddCommand(buildAgentCommands(logger))
	rootCmd.AddCommand(buildTaskCommands(logger))

	if err := rootCmd.Execute(); err != nil {
		logger.Error("Command execution failed", zap.Error(err))
		os.Exit(1)
	}
}

// initLogger는 zap logger를 초기화합니다.
func initLogger() (*zap.Logger, error) {
	env := os.Getenv("ENV")
	logLevel := os.Getenv("LOG_LEVEL")

	var config zap.Config
	if env == "production" {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
	}

	// LOG_LEVEL 환경변수가 설정되어 있으면 적용
	if logLevel != "" {
		level, err := zap.ParseAtomicLevel(logLevel)
		if err == nil {
			config.Level = level
		}
	}

	return config.Build()
}

// runStart는 controller와 connector 서버를 시작합니다.
func runStart(logger *zap.Logger) error {
	logger.Info("Starting CNAP servers",
		zap.String("version", Version),
		zap.String("build_time", BuildTime),
	)

	repo, cleanup, err := initStorage(logger)
	if err != nil {
		logger.Error("Failed to initialize storage", zap.Error(err))
		return err
	}
	defer cleanup()

	// Context 생성
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown을 위한 signal 처리
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 이벤트 채널 생성 (버퍼 크기: 100)
	connectorEventChan := make(chan controller.ConnectorEvent, 100)
	controllerEventChan := make(chan controller.ControllerEvent, 100)

	// 서버 인스턴스 생성
	controllerServer := controller.NewController(logger.Named("controller"), repo, connectorEventChan, controllerEventChan)
	connectorServer := connector.NewServer(logger.Named("connector"), controllerServer, connectorEventChan, controllerEventChan)

	// 에러 채널
	errChan := make(chan error, 2)
	var wg sync.WaitGroup

	// Controller 서버 시작
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := controllerServer.Start(ctx); err != nil && err != context.Canceled {
			errChan <- fmt.Errorf("controller error: %w", err)
		}
	}()

	// Connector 서버 시작
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := connectorServer.Start(ctx); err != nil && err != context.Canceled {
			errChan <- fmt.Errorf("connector error: %w", err)
		}
	}()

	// 종료 대기
	select {
	case <-sigChan:
		logger.Info("Shutdown signal received")
		cancel()
	case err := <-errChan:
		logger.Error("Server error", zap.Error(err))
		cancel()
		return err
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	shutdownErrChan := make(chan error, 2)

	go func() {
		shutdownErrChan <- controllerServer.Stop(shutdownCtx)
	}()

	go func() {
		shutdownErrChan <- connectorServer.Stop(shutdownCtx)
	}()

	// 모든 고루틴이 종료될 때까지 대기
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// Shutdown 에러 확인
	for i := 0; i < 2; i++ {
		if err := <-shutdownErrChan; err != nil {
			logger.Error("Shutdown error", zap.Error(err))
		}
	}

	logger.Info("Servers stopped gracefully")
	return nil
}

func initStorage(logger *zap.Logger) (*storage.Repository, func(), error) {
	cfg, err := storage.ConfigFromEnv()
	if err != nil {
		return nil, func() {}, err
	}

	db, err := storage.Open(cfg)
	if err != nil {
		return nil, func() {}, err
	}

	if err := storage.AutoMigrate(db); err != nil {
		_ = storage.Close(db)
		return nil, func() {}, err
	}

	repo, err := storage.NewRepository(db)
	if err != nil {
		_ = storage.Close(db)
		return nil, func() {}, err
	}

	cleanup := func() {
		if err := storage.Close(db); err != nil {
			logger.Warn("Failed to close storage", zap.Error(err))
		}
	}

	return repo, cleanup, nil
}

func newController(logger *zap.Logger) (*controller.Controller, func(), error) {
	repo, cleanup, err := initStorage(logger)
	if err != nil {
		return nil, func() {}, err
	}

	// CLI 단일 실행용으로 채널 생성 (버퍼 크기: 10)
	connectorEventChan := make(chan controller.ConnectorEvent, 10)
	controllerEventChan := make(chan controller.ControllerEvent, 10)

	ctrl := controller.NewController(logger.Named("controller"), repo, connectorEventChan, controllerEventChan)
	return ctrl, cleanup, nil
}
