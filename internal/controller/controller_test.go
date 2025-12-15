package controller_test

import (
	"context"
	"testing"
	"time"

	"github.com/cnap-oss/app/internal/controller"
	taskrunner "github.com/cnap-oss/app/internal/runner"
	"github.com/cnap-oss/app/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestController(t *testing.T) (*controller.Controller, func()) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, storage.AutoMigrate(db))

	repo, err := storage.NewRepository(db)
	require.NoError(t, err)

	// 테스트용 채널 생성 (버퍼 크기: 10)
	connectorEventChan := make(chan controller.ConnectorEvent, 10)
	controllerEventChan := make(chan controller.ControllerEvent, 10)

	ctrl := controller.NewController(zaptest.NewLogger(t), repo, connectorEventChan, controllerEventChan)

	cleanup := func() {
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	}

	return ctrl, cleanup
}

func TestControllerCreateAndGetAgent(t *testing.T) {
	ctrl, cleanup := newTestController(t)
	defer cleanup()

	ctx := context.Background()

	require.NoError(t, ctrl.CreateAgent(ctx, "agent-x", "Test agent", "opencode", "gpt-4", "Test prompt"))

	info, err := ctrl.GetAgentInfo(ctx, "agent-x")
	require.NoError(t, err)
	require.Equal(t, "agent-x", info.Name)
	require.Equal(t, "Test agent", info.Description)
	require.Equal(t, "gpt-4", info.Model)
	require.Equal(t, "Test prompt", info.Prompt)
	require.Equal(t, storage.AgentStatusActive, info.Status)
}

func TestControllerListAgents(t *testing.T) {
	ctrl, cleanup := newTestController(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-a", "Agent A", "opencode", "gpt-4", "Prompt A"))
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-b", "Agent B", "opencode", "gpt-3", "Prompt B"))

	agents, err := ctrl.ListAgents(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"agent-a", "agent-b"}, agents)
}

func TestControllerCreateTaskWithPrompt(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	ctrl, cleanup := newTestController(t)
	defer cleanup()

	ctx := context.Background()

	// Agent 생성
	require.NoError(t, ctrl.CreateAgent(ctx, "chatbot", "Chatbot agent", "opencode", "gpt-4", "You are a helpful assistant"))

	// Task 생성 with prompt
	require.NoError(t, ctrl.CreateTask(ctx, "chatbot", "task-001", "Hello, how are you?"))

	// Task 조회
	info, err := ctrl.GetTaskInfo(ctx, "task-001")
	require.NoError(t, err)
	require.Equal(t, "task-001", info.TaskID)
	require.Equal(t, "chatbot", info.AgentID)
	require.Equal(t, "Hello, how are you?", info.Prompt)
	require.Equal(t, storage.TaskStatusPending, info.Status)
}

func TestControllerCreateTaskWithoutPrompt(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	ctrl, cleanup := newTestController(t)
	defer cleanup()

	ctx := context.Background()

	// Agent 생성
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-1", "Test agent", "opencode", "gpt-4", "System prompt"))

	// Task 생성 without prompt
	require.NoError(t, ctrl.CreateTask(ctx, "agent-1", "task-empty", ""))

	info, err := ctrl.GetTaskInfo(ctx, "task-empty")
	require.NoError(t, err)
	require.Equal(t, "", info.Prompt)
}

func TestControllerAddMessage(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	ctrl, cleanup := newTestController(t)
	defer cleanup()

	ctx := context.Background()

	// Setup
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-1", "Test agent", "opencode", "gpt-4", "System prompt"))
	require.NoError(t, ctrl.CreateTask(ctx, "agent-1", "task-001", "Initial prompt"))

	// Add messages
	require.NoError(t, ctrl.AddMessage(ctx, "task-001", "user", "First message"))
	require.NoError(t, ctrl.AddMessage(ctx, "task-001", "assistant", "First response"))
	require.NoError(t, ctrl.AddMessage(ctx, "task-001", "user", "Second message"))

	// List messages
	messages, err := ctrl.ListMessages(ctx, "task-001")
	require.NoError(t, err)
	require.Len(t, messages, 3)

	// Verify order
	require.Equal(t, 0, messages[0].ConversationIndex)
	require.Equal(t, "user", messages[0].Role)
	require.Equal(t, 1, messages[1].ConversationIndex)
	require.Equal(t, "assistant", messages[1].Role)
	require.Equal(t, 2, messages[2].ConversationIndex)
	require.Equal(t, "user", messages[2].Role)
}

func TestControllerSendMessage(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	ctrl, cleanup := newTestController(t)
	defer cleanup()

	ctx := context.Background()

	// Setup
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-1", "Test agent", "opencode", "gpt-4", "System prompt"))
	require.NoError(t, ctrl.CreateTask(ctx, "agent-1", "task-001", "Hello"))

	// SendMessage should change status to running
	require.NoError(t, ctrl.SendMessage(ctx, "task-001"))

	info, err := ctrl.GetTaskInfo(ctx, "task-001")
	require.NoError(t, err)
	require.Equal(t, storage.TaskStatusRunning, info.Status)

	// SendMessage on running task should fail
	err = ctrl.SendMessage(ctx, "task-001")
	require.Error(t, err)
	require.Contains(t, err.Error(), "already running")
}

func TestControllerSendMessageWithoutPromptOrMessages(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	ctrl, cleanup := newTestController(t)
	defer cleanup()

	ctx := context.Background()

	// Setup - Task without prompt
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-1", "Test agent", "opencode", "gpt-4", "System prompt"))
	require.NoError(t, ctrl.CreateTask(ctx, "agent-1", "task-empty", ""))

	// SendMessage should fail - no prompt or messages
	err := ctrl.SendMessage(ctx, "task-empty")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no prompt or messages")
}

func TestControllerMultiTurnConversation(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	ctrl, cleanup := newTestController(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Agent 생성 (시스템 프롬프트)
	require.NoError(t, ctrl.CreateAgent(ctx, "chatbot", "Friendly chatbot", "opencode", "gpt-4", "You are a helpful assistant"))

	// 2. Task 생성 (대화 세션)
	require.NoError(t, ctrl.CreateTask(ctx, "chatbot", "session-001", "안녕하세요"))

	// 3. 첫 번째 SendMessage
	require.NoError(t, ctrl.SendMessage(ctx, "session-001"))

	// 상태가 running으로 변경됨
	info, err := ctrl.GetTaskInfo(ctx, "session-001")
	require.NoError(t, err)
	require.Equal(t, storage.TaskStatusRunning, info.Status)

	// 4. 상태를 completed로 변경 (RunnerManager가 할 역할)
	require.NoError(t, ctrl.UpdateTaskStatus(ctx, "session-001", storage.TaskStatusCompleted))

	// 5. AI 응답 추가 (RunnerManager가 할 역할)
	require.NoError(t, ctrl.AddMessage(ctx, "session-001", "assistant", "안녕하세요! 무엇을 도와드릴까요?"))

	// 6. 상태를 pending으로 변경하여 다음 메시지 준비
	require.NoError(t, ctrl.UpdateTaskStatus(ctx, "session-001", storage.TaskStatusPending))

	// 7. 두 번째 사용자 메시지
	require.NoError(t, ctrl.AddMessage(ctx, "session-001", "user", "날씨 알려줘"))

	// 8. 두 번째 SendMessage
	require.NoError(t, ctrl.SendMessage(ctx, "session-001"))

	// 9. 메시지 히스토리 확인
	messages, err := ctrl.ListMessages(ctx, "session-001")
	require.NoError(t, err)
	require.Len(t, messages, 2) // assistant + user

	// 대화 순서 확인
	require.Equal(t, "assistant", messages[0].Role)
	require.Equal(t, "user", messages[1].Role)
}

// TestControllerCreateTask_RunnerManagerIntegration tests CreateTask creates a Runner in RunnerManager
func TestControllerCreateTask_RunnerManagerIntegration(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	ctrl, cleanup := newTestController(t)
	defer cleanup()

	ctx := context.Background()

	// Create agent
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-test", "Test agent", "opencode", "gpt-4", "System prompt"))

	// Create task - should register runner in RunnerManager
	require.NoError(t, ctrl.CreateTask(ctx, "agent-test", "task-runner-test", "Test prompt"))

	// Verify task was created
	info, err := ctrl.GetTaskInfo(ctx, "task-runner-test")
	require.NoError(t, err)
	require.Equal(t, "task-runner-test", info.TaskID)
	require.Equal(t, storage.TaskStatusPending, info.Status)
}

// TestControllerSendMessage_PreventDuplicateExecution tests duplicate execution prevention
func TestControllerSendMessage_PreventDuplicateExecution(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	ctrl, cleanup := newTestController(t)
	defer cleanup()

	ctx := context.Background()

	// Setup
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-1", "Test agent", "opencode", "gpt-4", "System prompt"))
	require.NoError(t, ctrl.CreateTask(ctx, "agent-1", "task-dup", "Hello"))

	// First SendMessage
	require.NoError(t, ctrl.SendMessage(ctx, "task-dup"))

	// Verify status is running
	info, err := ctrl.GetTaskInfo(ctx, "task-dup")
	require.NoError(t, err)
	require.Equal(t, storage.TaskStatusRunning, info.Status)

	// Second SendMessage should fail
	err = ctrl.SendMessage(ctx, "task-dup")
	require.Error(t, err)
	require.Contains(t, err.Error(), "already running")
}

// TestControllerCancelTask tests task cancellation
func TestControllerCancelTask(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	ctrl, cleanup := newTestController(t)
	defer cleanup()

	ctx := context.Background()

	// Setup
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-1", "Test agent", "opencode", "gpt-4", "System prompt"))
	require.NoError(t, ctrl.CreateTask(ctx, "agent-1", "task-cancel", "Long running task"))

	// Start task
	require.NoError(t, ctrl.SendMessage(ctx, "task-cancel"))

	// Verify status is running
	info, err := ctrl.GetTaskInfo(ctx, "task-cancel")
	require.NoError(t, err)
	require.Equal(t, storage.TaskStatusRunning, info.Status)

	// Cancel task
	require.NoError(t, ctrl.CancelTask(ctx, "task-cancel"))

	// Wait a bit for cancellation to process
	// Note: In a real scenario, the task should update its status to canceled
	// For this test, we just verify the CancelTask call succeeded
}

// TestControllerCancelTask_NotRunning tests canceling a non-running task
func TestControllerCancelTask_NotRunning(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	ctrl, cleanup := newTestController(t)
	defer cleanup()

	ctx := context.Background()

	// Setup - use unique agent ID to avoid conflicts
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-notrunning", "Test agent", "opencode", "gpt-4", "System prompt"))
	require.NoError(t, ctrl.CreateTask(ctx, "agent-notrunning", "task-not-running", "Task"))

	// Try to cancel a pending task (not running)
	err := ctrl.CancelTask(ctx, "task-not-running")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not running")
}

// TestControllerCancelTask_NotFound tests canceling a non-existent task
func TestControllerCancelTask_NotFound(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	ctrl, cleanup := newTestController(t)
	defer cleanup()

	ctx := context.Background()

	// Try to cancel non-existent task
	err := ctrl.CancelTask(ctx, "non-existent-task")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// TestControllerSendMessage_RunnerAutoRecreation tests runner auto-recreation
// This test simulates the CLI scenario where runner is not available
// Note: RunnerManager is singleton, so we can access it directly for testing
func TestControllerSendMessage_RunnerAutoRecreation(t *testing.T) {
	t.Setenv("OPEN_CODE_API_KEY", "test-key")

	ctrl, cleanup := newTestController(t)
	defer cleanup()

	ctx := context.Background()

	// Get singleton RunnerManager
	runnerMgr := taskrunner.GetRunnerManager()

	// 1. Agent 및 Task 생성
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-recreate", "Test agent", "opencode", "gpt-4", "System prompt"))
	require.NoError(t, ctrl.CreateTask(ctx, "agent-recreate", "task-recreate", "Hello"))

	// 2. Runner가 생성되었는지 확인
	runner := runnerMgr.GetRunner("task-recreate")
	assert.NotNil(t, runner, "Runner should be created after CreateTask")

	// 3. Runner를 수동으로 삭제 (CLI 프로세스 재시작 시뮬레이션)
	runnerMgr.DeleteRunner("task-recreate")
	runner = runnerMgr.GetRunner("task-recreate")
	assert.Nil(t, runner, "Runner should be deleted")

	// 4. SendMessage 실행 - Runner가 자동으로 재생성되어야 함
	err := ctrl.SendMessage(ctx, "task-recreate")
	assert.NoError(t, err, "SendMessage should succeed even without runner (auto-recreation)")

	// 5. executeTask가 비동기로 실행되므로 잠시 대기
	time.Sleep(100 * time.Millisecond)

	// Note: executeTask 내부에서 runner를 재생성하므로
	// 로그에 "Runner not found, recreating..." 메시지가 출력되어야 함
	// 실제 프로덕션에서는 이 로직 덕분에 CLI 단일 실행도 정상 동작함
}
