package taskrunner

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// RecoveryConfig는 복구 설정입니다.
type RecoveryConfig struct {
	MaxRetries     int           // 최대 재시도 횟수
	InitialBackoff time.Duration // 초기 백오프 시간
	MaxBackoff     time.Duration // 최대 백오프 시간
	BackoffFactor  float64       // 백오프 증가 계수
}

// DefaultRecoveryConfig는 기본 복구 설정을 반환합니다.
func DefaultRecoveryConfig() RecoveryConfig {
	return RecoveryConfig{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		BackoffFactor:  2.0,
	}
}

// RecoveryManager는 에러 복구를 관리합니다.
type RecoveryManager struct {
	config RecoveryConfig
	logger *zap.Logger
}

// NewRecoveryManager는 새 RecoveryManager를 생성합니다.
func NewRecoveryManager(logger *zap.Logger, config ...RecoveryConfig) *RecoveryManager {
	cfg := DefaultRecoveryConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &RecoveryManager{
		config: cfg,
		logger: logger,
	}
}

// RetryOperation은 작업을 재시도합니다.
func (rm *RecoveryManager) RetryOperation(ctx context.Context, opName string, op func() error) error {
	var lastErr error
	backoff := rm.config.InitialBackoff

	for attempt := 0; attempt <= rm.config.MaxRetries; attempt++ {
		if attempt > 0 {
			rm.logger.Info("작업 재시도",
				zap.String("operation", opName),
				zap.Int("attempt", attempt),
				zap.Duration("backoff", backoff),
			)

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}

			// 백오프 증가
			backoff = time.Duration(float64(backoff) * rm.config.BackoffFactor)
			if backoff > rm.config.MaxBackoff {
				backoff = rm.config.MaxBackoff
			}
		}

		err := op()
		if err == nil {
			if attempt > 0 {
				rm.logger.Info("작업 재시도 성공",
					zap.String("operation", opName),
					zap.Int("attempts", attempt+1),
				)
			}
			return nil
		}

		lastErr = err

		// 재시도 불가능한 에러면 즉시 반환
		if !IsRetryable(err) {
			rm.logger.Warn("재시도 불가능한 에러",
				zap.String("operation", opName),
				zap.Error(err),
			)
			return err
		}

		rm.logger.Warn("작업 실패, 재시도 예정",
			zap.String("operation", opName),
			zap.Int("attempt", attempt),
			zap.Error(err),
		)
	}

	rm.logger.Error("최대 재시도 횟수 초과",
		zap.String("operation", opName),
		zap.Int("max_retries", rm.config.MaxRetries),
		zap.Error(lastErr),
	)

	return lastErr
}

// RecoverContainer는 Container 복구를 시도합니다.
func (rm *RecoveryManager) RecoverContainer(ctx context.Context, runner *Runner) error {
	rm.logger.Info("Container 복구 시작",
		zap.String("runner_id", runner.ID),
		zap.String("status", runner.Status),
	)

	// 1. 기존 Container 정리
	if runner.ContainerID != "" {
		if err := runner.Stop(ctx); err != nil {
			rm.logger.Warn("기존 Container 정리 중 오류",
				zap.String("container_id", runner.ContainerID),
				zap.Error(err),
			)
		}
	}

	// 2. 새 Container 시작 (재시도 적용)
	return rm.RetryOperation(ctx, "RecoverContainer", func() error {
		return runner.Start(ctx)
	})
}
