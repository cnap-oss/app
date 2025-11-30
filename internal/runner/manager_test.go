package taskrunner

import (
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
