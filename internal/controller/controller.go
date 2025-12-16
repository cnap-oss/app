package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	taskrunner "github.com/cnap-oss/app/internal/runner"
	"github.com/cnap-oss/app/internal/storage"
	"go.uber.org/zap"
)

// Controller는 에이전트 생성 및 관리를 담당하며, supervisor 기능도 포함합니다.
type Controller struct {
	logger              *zap.Logger
	repo                *storage.Repository
	runnerManager       *taskrunner.RunnerManager
	taskContexts        map[string]*TaskContext
	mu                  sync.RWMutex
	connectorEventChan  chan ConnectorEvent
	controllerEventChan chan ControllerEvent
}

// NewController는 새로운 Controller를 생성합니다.
func NewController(logger *zap.Logger, repo *storage.Repository, eventChan chan ConnectorEvent, resultChan chan ControllerEvent) *Controller {
	return &Controller{
		logger:              logger,
		repo:                repo,
		runnerManager:       taskrunner.GetRunnerManager(taskrunner.WithLogger(logger)),
		taskContexts:        make(map[string]*TaskContext),
		connectorEventChan:  eventChan,
		controllerEventChan: resultChan,
	}
}

// Start는 controller 서버를 시작합니다.
func (c *Controller) Start(ctx context.Context) error {
	c.logger.Info("Starting controller server")

	// RunnerManager 시작
	if err := c.runnerManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start runner manager: %w", err)
	}

	// 이벤트 루프 시작 (별도 goroutine)
	go c.eventLoop(ctx)

	// 하트비트
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Controller server shutting down")
			return ctx.Err()
		case <-ticker.C:
			c.logger.Debug("Controller heartbeat")
		}
	}
}

// Stop은 controller 서버를 정상적으로 종료합니다.
func (c *Controller) Stop(ctx context.Context) error {
	c.logger.Info("Stopping controller server")

	// RunnerManager 종료 (모든 컨테이너 정리)
	if err := c.runnerManager.Stop(ctx); err != nil {
		c.logger.Error("Failed to stop runner manager", zap.Error(err))
		return fmt.Errorf("failed to stop runner manager: %w", err)
	}

	c.logger.Info("Controller server stopped")
	return nil
}
