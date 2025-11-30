package taskrunner

import (
	"sync"

	"go.uber.org/zap"
)

// RunnerManager manages Runner instances.
type RunnerManager struct {
	runners map[string]*Runner
	mu      sync.RWMutex
}

var (
	instance *RunnerManager
	once     sync.Once
)

// GetRunnerManager returns the singleton instance of RunnerManager.
func GetRunnerManager() *RunnerManager {
	once.Do(func() {
		instance = &RunnerManager{
			runners: make(map[string]*Runner),
		}
	})
	return instance
}

// CreateRunner creates a new Runner and adds it to the manager.
func (rm *RunnerManager) CreateRunner(taskId string, _ AgentInfo, logger *zap.Logger, opts ...RunnerOption) *Runner {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if logger == nil {
		logger = zap.NewNop()
	}

	runner := NewRunner(logger, opts...)
	runner.ID = taskId
	runner.Status = "Pending" // Initial status

	rm.runners[taskId] = runner
	return runner
}

// ListRunner returns a list of all Runners.
func (rm *RunnerManager) ListRunner() *[]Runner {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	runnersList := make([]Runner, 0, len(rm.runners))
	for _, runner := range rm.runners {
		if runner != nil {
			runnersList = append(runnersList, *runner)
		}
	}
	return &runnersList
}

// DeleteRunner removes a Runner by its ID.
func (rm *RunnerManager) DeleteRunner(taskId string) *Runner {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	runner, exists := rm.runners[taskId]
	if !exists {
		return nil
	}

	delete(rm.runners, taskId)
	return runner
}
