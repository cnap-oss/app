package taskrunner

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	// Ensure clean state for test
	rm.mu.Lock()
	rm.runners = make(map[string]*Runner)
	rm.mu.Unlock()

	agent := mockAgentInfo()
	taskId := "task-1"
	ctx := context.Background()

	// Create
	runner, err := rm.CreateRunner(ctx, taskId, agent)
	require.NoError(t, err)
	assert.NotNil(t, runner)
	assert.Equal(t, taskId, runner.ID)
	assert.Equal(t, RunnerStatusPending, runner.Status)

	// List
	runners := rm.ListRunner()
	assert.NotNil(t, runners)
	assert.Len(t, runners, 1)
	assert.Equal(t, taskId, runners[0].ID)

	// Delete
	err = rm.DeleteRunner(ctx, taskId)
	require.NoError(t, err)

	// List after delete
	runners = rm.ListRunner()
	assert.Len(t, runners, 0)

	// Delete non-existent
	err = rm.DeleteRunner(ctx, "non-existent")
	assert.NoError(t, err) // Should not error on non-existent
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
	ctx := context.Background()

	// Get non-existent runner
	runner := rm.GetRunner(taskId)
	assert.Nil(t, runner)

	// Create runner
	createdRunner, err := rm.CreateRunner(ctx, taskId, agent)
	require.NoError(t, err)
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
	ctx := context.Background()

	// Concurrently create runners
	done := make(chan bool, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			taskId := fmt.Sprintf("task-%d", id)
			runner, err := rm.CreateRunner(ctx, taskId, agent)
			assert.NoError(t, err)
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
	assert.Len(t, runners, numGoroutines)

	// Concurrently read and delete runners
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			taskId := fmt.Sprintf("task-%d", id)

			// Get runner
			runner := rm.GetRunner(taskId)
			assert.NotNil(t, runner)

			// Delete runner
			err := rm.DeleteRunner(ctx, taskId)
			assert.NoError(t, err)

			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify all runners were deleted
	runners = rm.ListRunner()
	assert.Len(t, runners, 0)
}

// TestRunnerManager_GetRunnerCount tests the GetRunnerCount method
func TestRunnerManager_GetRunnerCount(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	rm := GetRunnerManager()
	rm.mu.Lock()
	rm.runners = make(map[string]*Runner)
	rm.mu.Unlock()

	agent := mockAgentInfo()
	ctx := context.Background()

	// Initial count should be 0
	assert.Equal(t, 0, rm.GetRunnerCount())

	// Create runners
	for i := 0; i < 5; i++ {
		taskId := fmt.Sprintf("task-%d", i)
		_, err := rm.CreateRunner(ctx, taskId, agent)
		require.NoError(t, err)
	}

	// Count should be 5
	assert.Equal(t, 5, rm.GetRunnerCount())
}
