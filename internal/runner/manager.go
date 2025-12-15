package taskrunner

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// RunnerManager manages Runner instances.
type RunnerManager struct {
	runners      map[string]*Runner
	dockerClient DockerClient
	mu           sync.RWMutex
	logger       *zap.Logger
}

// RunnerManagerOption은 RunnerManager 옵션입니다.
type RunnerManagerOption func(*RunnerManager)

// WithDockerClientOption은 DockerClient를 주입합니다.
func WithDockerClientOption(client DockerClient) RunnerManagerOption {
	return func(rm *RunnerManager) {
		rm.dockerClient = client
	}
}

// WithLogger는 logger를 설정합니다.
func WithLogger(logger *zap.Logger) RunnerManagerOption {
	return func(rm *RunnerManager) {
		rm.logger = logger
	}
}

var (
	instance *RunnerManager
	once     sync.Once
)

// GetRunnerManager returns the singleton instance of RunnerManager.
func GetRunnerManager(opts ...RunnerManagerOption) *RunnerManager {
	once.Do(func() {
		instance = &RunnerManager{
			runners: make(map[string]*Runner),
			logger:  zap.NewNop(),
		}

		for _, opt := range opts {
			opt(instance)
		}

		// DockerClient가 설정되지 않았으면 새로 생성
		if instance.dockerClient == nil {
			client, err := NewDockerClient()
			if err != nil {
				instance.logger.Fatal("Docker client 생성 실패", zap.Error(err))
			}
			instance.dockerClient = client
		}
	})
	return instance
}

// CreateRunner creates a new Runner and adds it to the manager.
// Container는 생성되지만 시작되지 않습니다. StartRunner()를 별도로 호출해야 합니다.
func (rm *RunnerManager) CreateRunner(ctx context.Context, taskID string, agentInfo AgentInfo, opts ...RunnerOption) (*Runner, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// 이미 존재하는지 확인
	if existing, ok := rm.runners[taskID]; ok {
		return existing, nil
	}

	// DockerClient를 옵션에 추가
	allOpts := append([]RunnerOption{WithDockerClient(rm.dockerClient)}, opts...)

	runner, err := NewRunner(
		taskID,
		agentInfo,
		rm.logger,
		allOpts...,
	)
	if err != nil {
		return nil, fmt.Errorf("runner 생성 실패: %w", err)
	}

	rm.runners[taskID] = runner
	return runner, nil
}

// StartRunner starts a Runner's container.
func (rm *RunnerManager) StartRunner(ctx context.Context, taskID string) error {
	rm.mu.RLock()
	runner, ok := rm.runners[taskID]
	rm.mu.RUnlock()

	if !ok {
		return fmt.Errorf("runner를 찾을 수 없음: %s", taskID)
	}

	return runner.Start(ctx)
}

// GetRunner returns a Runner by its ID.
func (rm *RunnerManager) GetRunner(taskID string) *Runner {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.runners[taskID]
}

// ListRunner returns a list of all Runners.
func (rm *RunnerManager) ListRunner() []*Runner {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	runnersList := make([]*Runner, 0, len(rm.runners))
	for _, runner := range rm.runners {
		if runner != nil {
			runnersList = append(runnersList, runner)
		}
	}
	return runnersList
}

// DeleteRunner removes a Runner by its ID and stops its container.
func (rm *RunnerManager) DeleteRunner(ctx context.Context, taskID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	runner, exists := rm.runners[taskID]
	if !exists {
		return nil
	}

	// Container 중지
	if err := runner.Stop(ctx); err != nil {
		rm.logger.Warn("Runner 중지 중 오류",
			zap.String("task_id", taskID),
			zap.Error(err),
		)
	}

	delete(rm.runners, taskID)
	return nil
}

// Cleanup은 모든 Runner를 정리합니다. (종료 시 호출)
func (rm *RunnerManager) Cleanup(ctx context.Context) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	var lastErr error
	for taskID, runner := range rm.runners {
		if err := runner.Stop(ctx); err != nil {
			rm.logger.Warn("Runner 정리 중 오류",
				zap.String("task_id", taskID),
				zap.Error(err),
			)
			lastErr = err
		}
		delete(rm.runners, taskID)
	}

	return lastErr
}

// GetRunnerCount는 현재 관리 중인 Runner 수를 반환합니다.
func (rm *RunnerManager) GetRunnerCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.runners)
}
