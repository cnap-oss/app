package mocks_test

import (
	"context"
	"testing"
	"time"

	taskrunner "github.com/cnap-oss/app/internal/runner"
	"github.com/cnap-oss/app/internal/runner/opencode"
	"github.com/cnap-oss/app/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// MockCallback for testing
type MockCallback struct {
	Result *taskrunner.RunResult
	Error  error
	Done   chan struct{}
}

func NewMockCallback() *MockCallback {
	return &MockCallback{
		Done: make(chan struct{}),
	}
}

func (m *MockCallback) OnStarted(taskID string, sessionID string) error {
	return nil
}

func (m *MockCallback) OnEvent(taskID string, event *opencode.Event) error {
	return nil
}

func (m *MockCallback) OnComplete(taskID string, result *taskrunner.RunResult) error {
	m.Result = result
	close(m.Done)
	return nil
}

func (m *MockCallback) OnError(taskID string, err error) error {
	m.Error = err
	close(m.Done)
	return nil
}

func TestMockRunner_Run(t *testing.T) {
	callback := NewMockCallback()
	mock := mocks.NewMockRunner(callback)
	mock.SetResponse("task-001", "Hello from mock!")

	ctx := context.Background()
	req := &taskrunner.RunRequest{
		TaskID:       "task-001",
		Model:        "gpt-4",
		SystemPrompt: "You are a helpful assistant",
		Messages: []opencode.ChatMessage{
			{Role: "user", Content: "Hi"},
		},
	}

	err := mock.Run(ctx, req)
	require.NoError(t, err)

	// 완료 대기
	select {
	case <-callback.Done:
		require.NotNil(t, callback.Result)
		require.Equal(t, "Hello from mock!", callback.Result.Output)
		require.True(t, callback.Result.Success)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for callback")
	}

	require.Equal(t, 1, mock.GetCallCount())
}

func TestMockRunner_DefaultResponse(t *testing.T) {
	callback := NewMockCallback()
	mock := mocks.NewMockRunner(callback)
	mock.DefaultResponse = "Default response"

	ctx := context.Background()
	req := &taskrunner.RunRequest{
		TaskID: "unknown-task",
		Model:  "gpt-4",
	}

	err := mock.Run(ctx, req)
	require.NoError(t, err)

	// 완료 대기
	select {
	case <-callback.Done:
		require.NotNil(t, callback.Result)
		require.Equal(t, "Default response", callback.Result.Output)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for callback")
	}
}

func TestMockRunner_Error(t *testing.T) {
	callback := NewMockCallback()
	mock := mocks.NewMockRunner(callback)
	mock.SetErrorMessage("task-fail", "API error")

	ctx := context.Background()
	req := &taskrunner.RunRequest{
		TaskID: "task-fail",
		Model:  "gpt-4",
	}

	err := mock.Run(ctx, req)
	require.NoError(t, err) // Run 자체는 성공 (비동기)

	// 에러 콜백 대기
	select {
	case <-callback.Done:
		require.NotNil(t, callback.Error)
		require.Contains(t, callback.Error.Error(), "API error")
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for callback")
	}
}

func TestMockRunner_CallHistory(t *testing.T) {
	callback := NewMockCallback()
	mock := mocks.NewMockRunner(callback)

	ctx := context.Background()

	// 여러 번 호출
	_ = mock.Run(ctx, &taskrunner.RunRequest{TaskID: "task-1"})
	_ = mock.Run(ctx, &taskrunner.RunRequest{TaskID: "task-2"})
	_ = mock.Run(ctx, &taskrunner.RunRequest{TaskID: "task-3"})

	require.Equal(t, 3, mock.GetCallCount())
	require.Equal(t, "task-3", mock.GetLastCall().TaskID)

	// Reset
	mock.Reset()
	require.Equal(t, 0, mock.GetCallCount())
	require.Nil(t, mock.GetLastCall())
}
