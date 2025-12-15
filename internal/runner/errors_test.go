package taskrunner

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunnerError(t *testing.T) {
	err := NewRunnerError("Start", "test-runner", errors.New("failed"))

	assert.Equal(t, "runner[test-runner] Start: failed", err.Error())
	assert.Equal(t, "failed", err.Unwrap().Error())
}

func TestRunnerError_NoRunnerID(t *testing.T) {
	err := NewRunnerError("Start", "", errors.New("failed"))

	assert.Equal(t, "runner Start: failed", err.Error())
}

func TestContainerError(t *testing.T) {
	err := NewContainerError("Start", "container-123", errors.New("failed"), true)

	assert.Equal(t, "container[container-123] Start: failed", err.Error())
	assert.True(t, err.Recoverable)
	assert.Equal(t, "failed", err.Unwrap().Error())
}

func TestIsRecoverable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "APITimeout",
			err:      ErrAPITimeout,
			expected: true,
		},
		{
			name:     "APIConnectionFailed",
			err:      ErrAPIConnectionFailed,
			expected: true,
		},
		{
			name:     "ContainerUnhealthy",
			err:      ErrContainerUnhealthy,
			expected: true,
		},
		{
			name:     "ContainerError with Recoverable=true",
			err:      NewContainerError("Start", "test", errors.New("failed"), true),
			expected: true,
		},
		{
			name:     "ContainerError with Recoverable=false",
			err:      NewContainerError("Start", "test", errors.New("failed"), false),
			expected: false,
		},
		{
			name:     "ContainerNotFound",
			err:      ErrContainerNotFound,
			expected: false,
		},
		{
			name:     "MaxContainersReached",
			err:      ErrMaxContainersReached,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRecoverable(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "APITimeout",
			err:      ErrAPITimeout,
			expected: true,
		},
		{
			name:     "APIConnectionFailed",
			err:      ErrAPIConnectionFailed,
			expected: true,
		},
		{
			name:     "ContainerStartFailed",
			err:      ErrContainerStartFailed,
			expected: true,
		},
		{
			name:     "ContainerNotFound",
			err:      ErrContainerNotFound,
			expected: false,
		},
		{
			name:     "RunnerNotReady",
			err:      ErrRunnerNotReady,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestErrorWrapping(t *testing.T) {
	baseErr := errors.New("base error")
	runnerErr := NewRunnerError("Run", "test-runner", baseErr)

	// errors.Is로 래핑된 에러 확인
	assert.True(t, errors.Is(runnerErr, baseErr))

	// errors.As로 타입 확인
	var re *RunnerError
	assert.True(t, errors.As(runnerErr, &re))
	assert.Equal(t, "Run", re.Op)
	assert.Equal(t, "test-runner", re.RunnerID)
}

func TestContainerErrorWrapping(t *testing.T) {
	baseErr := errors.New("connection failed")
	containerErr := NewContainerError("Create", "container-123", baseErr, true)

	// errors.Is로 래핑된 에러 확인
	assert.True(t, errors.Is(containerErr, baseErr))

	// errors.As로 타입 확인
	var ce *ContainerError
	assert.True(t, errors.As(containerErr, &ce))
	assert.Equal(t, "Create", ce.Op)
	assert.Equal(t, "container-123", ce.ContainerID)
	assert.True(t, ce.Recoverable)
}

func TestPredefinedErrors(t *testing.T) {
	// Container 에러
	assert.NotNil(t, ErrContainerNotFound)
	assert.NotNil(t, ErrContainerStartFailed)
	assert.NotNil(t, ErrContainerStopFailed)
	assert.NotNil(t, ErrContainerNotRunning)
	assert.NotNil(t, ErrContainerUnhealthy)

	// Runner 에러
	assert.NotNil(t, ErrRunnerNotReady)
	assert.NotNil(t, ErrRunnerAlreadyExists)
	assert.NotNil(t, ErrRunnerNotFound)

	// 리소스 에러
	assert.NotNil(t, ErrMaxContainersReached)
	assert.NotNil(t, ErrInsufficientResources)

	// 통신 에러
	assert.NotNil(t, ErrAPITimeout)
	assert.NotNil(t, ErrAPIConnectionFailed)

	// 작업 공간 에러
	assert.NotNil(t, ErrWorkspaceNotFound)
	assert.NotNil(t, ErrWorkspaceCreateFailed)
}

func TestIsRecoverable_WithWrappedErrors(t *testing.T) {
	// ContainerError로 래핑된 에러
	baseErr := errors.New("network timeout")
	containerErr := NewContainerError("Start", "test", baseErr, true)

	assert.True(t, IsRecoverable(containerErr))

	// RunnerError로 래핑된 복구 가능한 에러
	runnerErr := NewRunnerError("Start", "test", ErrAPITimeout)
	assert.True(t, IsRecoverable(runnerErr))
}

func TestErrorMessages(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		contains string
	}{
		{
			name:     "RunnerError with ID",
			err:      NewRunnerError("Start", "runner-1", errors.New("timeout")),
			contains: "runner[runner-1] Start: timeout",
		},
		{
			name:     "RunnerError without ID",
			err:      NewRunnerError("Stop", "", errors.New("failed")),
			contains: "runner Stop: failed",
		},
		{
			name:     "ContainerError",
			err:      NewContainerError("Remove", "abc123", errors.New("in use"), false),
			contains: "container[abc123] Remove: in use",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, tt.err.Error(), tt.contains)
		})
	}
}
