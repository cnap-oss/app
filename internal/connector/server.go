package connector

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// Server는 connector 서버를 나타냅니다.
type Server struct {
	logger *zap.Logger
}

// NewServer는 새로운 connector 서버를 생성합니다.
func NewServer(logger *zap.Logger) *Server {
	return &Server{
		logger: logger,
	}
}

// Start는 connector 서버를 시작합니다.
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("Starting connector server")

	// 더미 프로세스 - 실제 구현 시 여기에 Discord 봇 로직 추가
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Connector server shutting down")
			return ctx.Err()
		case <-ticker.C:
			s.logger.Debug("Connector heartbeat")
		}
	}
}

// Stop은 connector 서버를 정상적으로 종료합니다.
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Stopping connector server")

	// 정리 작업 수행
	select {
	case <-ctx.Done():
		return fmt.Errorf("shutdown timeout exceeded")
	case <-time.After(100 * time.Millisecond):
		s.logger.Info("Connector server stopped")
		return nil
	}
}
