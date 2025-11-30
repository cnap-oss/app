package taskrunner

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// mockAgentInfo is a mock AgentInfo for testing.
func mockAgentInfo() AgentInfo {
	return AgentInfo{
		AgentID: "test-agent",
		Model:   "grok-code",
		Prompt:  "test prompt",
	}
}

func TestRunnerManager_Singleton(t *testing.T) {
	rm1 := GetRunnerManager()
	rm2 := GetRunnerManager()

	assert.Equal(t, rm1, rm2, "GetRunnerManager should return the same instance")
}

func TestRunnerManager_CRUD(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	rm := GetRunnerManager()

	// Ensure clean state for test (though singleton persists, so we might need to clear it if tests run in same process)
	// Since we can't easily reset the singleton once, we just work with what we have or clear the map manually.
	rm.mu.Lock()
	rm.runners = make(map[string]*Runner)
	rm.mu.Unlock()

	agent := mockAgentInfo()
	taskId := "task-1"

	// Create
	runner := rm.CreateRunner(taskId, agent, zap.NewNop())
	assert.NotNil(t, runner)
	assert.Equal(t, taskId, runner.ID)
	assert.Equal(t, "Pending", runner.Status)

	// List
	runners := rm.ListRunner()
	assert.NotNil(t, runners)
	assert.Len(t, *runners, 1)
	assert.Equal(t, taskId, (*runners)[0].ID)

	// Delete
	deletedRunner := rm.DeleteRunner(taskId)
	assert.NotNil(t, deletedRunner)
	assert.Equal(t, taskId, deletedRunner.ID)

	// List after delete
	runners = rm.ListRunner()
	assert.Len(t, *runners, 0)

	// Delete non-existent
	nilRunner := rm.DeleteRunner("non-existent")
	assert.Nil(t, nilRunner)
}

// TestRunnerManager_GetRunner tests the GetRunner method
func TestRunnerManager_GetRunner(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	rm := GetRunnerManager()
	rm.mu.Lock()
	rm.runners = make(map[string]*Runner)
	rm.mu.Unlock()

	agent := mockAgentInfo()
	taskId := "task-get"

	// Get non-existent runner
	runner := rm.GetRunner(taskId)
	assert.Nil(t, runner)

	// Create runner
	createdRunner := rm.CreateRunner(taskId, agent, zap.NewNop())
	assert.NotNil(t, createdRunner)

	// Get existing runner
	runner = rm.GetRunner(taskId)
	assert.NotNil(t, runner)
	assert.Equal(t, taskId, runner.ID)
	assert.Equal(t, createdRunner, runner)
}

// TestRunnerManager_ConcurrentAccess tests concurrent access to RunnerManager
func TestRunnerManager_ConcurrentAccess(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	rm := GetRunnerManager()
	rm.mu.Lock()
	rm.runners = make(map[string]*Runner)
	rm.mu.Unlock()

	agent := mockAgentInfo()
	numGoroutines := 100

	// Concurrently create runners
	done := make(chan bool, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			taskId := fmt.Sprintf("task-%d", id)
			runner := rm.CreateRunner(taskId, agent, zap.NewNop())
			assert.NotNil(t, runner)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify all runners were created
	runners := rm.ListRunner()
	assert.Len(t, *runners, numGoroutines)

	// Concurrently read and delete runners
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			taskId := fmt.Sprintf("task-%d", id)

			// Get runner
			runner := rm.GetRunner(taskId)
			assert.NotNil(t, runner)

			// Delete runner
			deleted := rm.DeleteRunner(taskId)
			assert.NotNil(t, deleted)

			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify all runners were deleted
	runners = rm.ListRunner()
	assert.Len(t, *runners, 0)
}

// TestRunnerManager_NilLogger tests CreateRunner with nil logger
func TestRunnerManager_NilLogger(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	rm := GetRunnerManager()
	rm.mu.Lock()
	rm.runners = make(map[string]*Runner)
	rm.mu.Unlock()

	agent := mockAgentInfo()
	taskId := "task-nil-logger"

	// CreateRunner should handle nil logger gracefully
	runner := rm.CreateRunner(taskId, agent, nil)
	assert.NotNil(t, runner)
	assert.Equal(t, taskId, runner.ID)
	assert.NotNil(t, runner.logger) // Should have initialized with zap.NewNop()
}
