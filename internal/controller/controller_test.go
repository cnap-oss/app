package controller_test

import (
	"context"
	"os"
	"testing"

	"github.com/cnap-oss/app/internal/controller"
	"github.com/cnap-oss/app/internal/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestController(t *testing.T) (*controller.Controller, func()) {
	t.Helper()

	// Set mock API key for testing
	_ = os.Setenv("OPEN_CODE_API_KEY", "test-api-key")

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, storage.AutoMigrate(db))

	repo, err := storage.NewRepository(db)
	require.NoError(t, err)

	ctrl := controller.NewController(zaptest.NewLogger(t), repo)

	cleanup := func() {
		_ = os.Unsetenv("OPEN_CODE_API_KEY")
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

	require.NoError(t, ctrl.CreateAgent(ctx, "agent-x", "Test agent", "gpt-4", "Test prompt"))

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
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-a", "Agent A", "gpt-4", "Prompt A"))
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-b", "Agent B", "gpt-3", "Prompt B"))

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
	require.NoError(t, ctrl.CreateAgent(ctx, "chatbot", "Chatbot agent", "gpt-4", "You are a helpful assistant"))

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
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-1", "Test agent", "gpt-4", "System prompt"))

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
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-1", "Test agent", "gpt-4", "System prompt"))
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
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-1", "Test agent", "gpt-4", "System prompt"))
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
	require.NoError(t, ctrl.CreateAgent(ctx, "agent-1", "Test agent", "gpt-4", "System prompt"))
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
	require.NoError(t, ctrl.CreateAgent(ctx, "chatbot", "Friendly chatbot", "gpt-4", "You are a helpful assistant"))

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
