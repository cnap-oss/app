package taskrunner

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
)

// LifecycleManager는 Container 수명을 관리합니다.
type LifecycleManager interface {
	// Start는 수명 관리자를 시작합니다.
	Start(ctx context.Context) error

	// Stop은 수명 관리자를 중지합니다.
	Stop(ctx context.Context) error

	// RegisterRunner는 Runner를 수명 관리 대상으로 등록합니다.
	RegisterRunner(runner *Runner) error

	// UnregisterRunner는 Runner를 수명 관리 대상에서 제거합니다.
	UnregisterRunner(runnerID string) error

	// NotifyActivity는 Runner의 활동을 알립니다.
	NotifyActivity(runnerID string)

	// GetRunnerStats는 Runner 상태 통계를 반환합니다.
	GetRunnerStats() *LifecycleStats
}

// LifecycleConfig는 수명 관리 설정입니다.
type LifecycleConfig struct {
	// 유휴 타임아웃: 이 시간 동안 활동이 없으면 Container 종료
	IdleTimeout time.Duration

	// 최대 실행 시간: Container 최대 실행 시간
	MaxRuntime time.Duration

	// 정리 주기: 유휴 Container 검사 주기
	CleanupInterval time.Duration

	// 최대 동시 Container 수
	MaxConcurrentContainers int

	// Graceful shutdown 타임아웃
	ShutdownTimeout time.Duration
}

// DefaultLifecycleConfig는 기본 수명 관리 설정을 반환합니다.
func DefaultLifecycleConfig() LifecycleConfig {
	return LifecycleConfig{
		IdleTimeout:             getEnvOrDefaultDuration("CNAP_RUNNER_IDLE_TIMEOUT", 5*time.Minute),
		MaxRuntime:              getEnvOrDefaultDuration("CNAP_RUNNER_MAX_RUNTIME", 30*time.Minute),
		CleanupInterval:         getEnvOrDefaultDuration("CNAP_RUNNER_CLEANUP_INTERVAL", 1*time.Minute),
		MaxConcurrentContainers: getEnvOrDefaultInt("CNAP_RUNNER_MAX_CONTAINERS", 10),
		ShutdownTimeout:         getEnvOrDefaultDuration("CNAP_RUNNER_SHUTDOWN_TIMEOUT", 30*time.Second),
	}
}

// LifecycleStats는 수명 관리 통계입니다.
type LifecycleStats struct {
	TotalRunners     int
	ActiveRunners    int
	IdleRunners      int
	StoppedRunners   int
	TerminatedByIdle int64
	TerminatedByMax  int64
}

// runnerState는 Runner의 수명 상태를 추적합니다.
type runnerState struct {
	Runner       *Runner
	StartTime    time.Time
	LastActivity time.Time
	IsIdle       bool
}

// lifecycleManager는 LifecycleManager 구현체입니다.
type lifecycleManager struct {
	config  LifecycleConfig
	runners map[string]*runnerState
	mu      sync.RWMutex
	logger  *zap.Logger

	// 통계
	terminatedByIdle int64
	terminatedByMax  int64

	// 정리 루프 제어
	stopChan chan struct{}
	wg       sync.WaitGroup
	stopped  bool
	running  bool
	stopMu   sync.Mutex
}

// NewLifecycleManager는 새 LifecycleManager를 생성합니다.
func NewLifecycleManager(logger *zap.Logger, config ...LifecycleConfig) LifecycleManager {
	cfg := DefaultLifecycleConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &lifecycleManager{
		config:   cfg,
		runners:  make(map[string]*runnerState),
		logger:   logger,
		stopChan: make(chan struct{}),
	}
}

// Start implements LifecycleManager.
func (lm *lifecycleManager) Start(ctx context.Context) error {
	lm.stopMu.Lock()
	defer lm.stopMu.Unlock()

	// 이미 실행 중이면 무시
	if lm.running {
		lm.logger.Debug("수명 관리자가 이미 실행 중")
		return nil
	}

	// 이미 중지되었다면 재시작 준비
	if lm.stopped {
		lm.stopChan = make(chan struct{})
		lm.stopped = false
	}

	lm.running = true

	lm.logger.Info("수명 관리자 시작",
		zap.Duration("idle_timeout", lm.config.IdleTimeout),
		zap.Duration("max_runtime", lm.config.MaxRuntime),
		zap.Duration("cleanup_interval", lm.config.CleanupInterval),
	)

	// 정리 루프 시작
	lm.wg.Add(1)
	go lm.cleanupLoop(ctx)

	return nil
}

// Stop implements LifecycleManager.
func (lm *lifecycleManager) Stop(ctx context.Context) error {
	lm.stopMu.Lock()
	if lm.stopped {
		lm.stopMu.Unlock()
		lm.logger.Debug("수명 관리자가 이미 중지됨")
		return nil
	}
	lm.stopped = true
	lm.running = false
	lm.stopMu.Unlock()

	lm.logger.Info("수명 관리자 중지 시작")

	// 정리 루프 중지
	close(lm.stopChan)

	// 대기 (타임아웃 적용)
	done := make(chan struct{})
	go func() {
		lm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		lm.logger.Info("수명 관리자 정상 중지됨")
	case <-time.After(lm.config.ShutdownTimeout):
		lm.logger.Warn("수명 관리자 중지 타임아웃")
	case <-ctx.Done():
		lm.logger.Warn("수명 관리자 중지 컨텍스트 취소됨")
	}

	// 모든 Runner 정리
	return lm.cleanupAllRunners(ctx)
}

// RegisterRunner implements LifecycleManager.
func (lm *lifecycleManager) RegisterRunner(runner *Runner) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// 최대 Container 수 확인
	activeCount := lm.countActiveRunnersLocked()
	if activeCount >= lm.config.MaxConcurrentContainers {
		return fmt.Errorf("최대 동시 Container 수 초과 (%d/%d)",
			activeCount, lm.config.MaxConcurrentContainers)
	}

	now := time.Now()
	lm.runners[runner.ID] = &runnerState{
		Runner:       runner,
		StartTime:    now,
		LastActivity: now,
		IsIdle:       false,
	}

	lm.logger.Info("Runner 등록됨",
		zap.String("runner_id", runner.ID),
		zap.Int("total_runners", len(lm.runners)),
	)

	return nil
}

// UnregisterRunner implements LifecycleManager.
func (lm *lifecycleManager) UnregisterRunner(runnerID string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if _, exists := lm.runners[runnerID]; !exists {
		return fmt.Errorf("runner를 찾을 수 없음: %s", runnerID)
	}

	delete(lm.runners, runnerID)

	lm.logger.Info("Runner 등록 해제됨",
		zap.String("runner_id", runnerID),
		zap.Int("remaining_runners", len(lm.runners)),
	)

	return nil
}

// NotifyActivity implements LifecycleManager.
func (lm *lifecycleManager) NotifyActivity(runnerID string) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if state, exists := lm.runners[runnerID]; exists {
		state.LastActivity = time.Now()
		state.IsIdle = false
	}
}

// GetRunnerStats implements LifecycleManager.
func (lm *lifecycleManager) GetRunnerStats() *LifecycleStats {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	stats := &LifecycleStats{
		TotalRunners:     len(lm.runners),
		TerminatedByIdle: lm.terminatedByIdle,
		TerminatedByMax:  lm.terminatedByMax,
	}

	for _, state := range lm.runners {
		switch state.Runner.Status {
		case RunnerStatusReady, RunnerStatusRunning:
			if state.IsIdle {
				stats.IdleRunners++
			} else {
				stats.ActiveRunners++
			}
		case RunnerStatusStopped, RunnerStatusFailed:
			stats.StoppedRunners++
		}
	}

	return stats
}

// cleanupLoop는 주기적으로 유휴 Container를 정리합니다.
func (lm *lifecycleManager) cleanupLoop(ctx context.Context) {
	defer lm.wg.Done()

	ticker := time.NewTicker(lm.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lm.performCleanup(ctx)
		case <-lm.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

// performCleanup은 유휴 및 만료된 Container를 정리합니다.
func (lm *lifecycleManager) performCleanup(ctx context.Context) {
	lm.mu.Lock()
	now := time.Now()
	runnersToCleanup := make([]*Runner, 0)

	for id, state := range lm.runners {
		// 유휴 타임아웃 확인
		idleDuration := now.Sub(state.LastActivity)
		if idleDuration > lm.config.IdleTimeout {
			state.IsIdle = true

			// Ready 상태인 유휴 Runner만 정리
			if state.Runner.Status == RunnerStatusReady {
				lm.logger.Info("유휴 Runner 정리 예정",
					zap.String("runner_id", id),
					zap.Duration("idle_duration", idleDuration),
				)
				runnersToCleanup = append(runnersToCleanup, state.Runner)
				lm.terminatedByIdle++
			}
		}

		// 최대 실행 시간 확인
		runtime := now.Sub(state.StartTime)
		if runtime > lm.config.MaxRuntime {
			lm.logger.Warn("최대 실행 시간 초과 Runner 정리 예정",
				zap.String("runner_id", id),
				zap.Duration("runtime", runtime),
			)
			runnersToCleanup = append(runnersToCleanup, state.Runner)
			lm.terminatedByMax++
		}
	}
	lm.mu.Unlock()

	// Container 정리 (락 해제 후)
	for _, runner := range runnersToCleanup {
		if err := runner.Stop(ctx); err != nil {
			lm.logger.Warn("Runner 정리 중 오류",
				zap.String("runner_id", runner.ID),
				zap.Error(err),
			)
		}
		_ = lm.UnregisterRunner(runner.ID)
	}
}

// cleanupAllRunners는 모든 Runner를 정리합니다.
func (lm *lifecycleManager) cleanupAllRunners(ctx context.Context) error {
	lm.mu.Lock()
	runners := make([]*Runner, 0, len(lm.runners))
	for _, state := range lm.runners {
		runners = append(runners, state.Runner)
	}
	lm.mu.Unlock()

	var lastErr error
	for _, runner := range runners {
		if err := runner.Stop(ctx); err != nil {
			lm.logger.Warn("Runner 정리 중 오류",
				zap.String("runner_id", runner.ID),
				zap.Error(err),
			)
			lastErr = err
		}
	}

	return lastErr
}

// countActiveRunnersLocked는 활성 Runner 수를 반환합니다. (락 보유 상태에서 호출)
func (lm *lifecycleManager) countActiveRunnersLocked() int {
	count := 0
	for _, state := range lm.runners {
		if state.Runner.Status == RunnerStatusReady ||
			state.Runner.Status == RunnerStatusRunning ||
			state.Runner.Status == RunnerStatusStarting {
			count++
		}
	}
	return count
}

// getEnvOrDefaultDuration은 환경 변수에서 duration을 읽거나 기본값을 반환합니다.
func getEnvOrDefaultDuration(key string, defaultVal time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal
	}
	return d
}

// getEnvOrDefaultInt는 환경 변수에서 정수를 읽거나 기본값을 반환합니다.
func getEnvOrDefaultInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}

// 인터페이스 구현 확인
var _ LifecycleManager = (*lifecycleManager)(nil)
