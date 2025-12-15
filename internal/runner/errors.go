package taskrunner

import (
	"errors"
	"fmt"
)

// 기본 에러 타입
var (
	// Container 관련 에러
	ErrContainerNotFound    = errors.New("container를 찾을 수 없음")
	ErrContainerStartFailed = errors.New("container 시작 실패")
	ErrContainerStopFailed  = errors.New("container 중지 실패")
	ErrContainerNotRunning  = errors.New("container가 실행 중이 아님")
	ErrContainerUnhealthy   = errors.New("container 상태 비정상")

	// Runner 관련 에러
	ErrRunnerNotReady      = errors.New("runner가 준비되지 않음")
	ErrRunnerAlreadyExists = errors.New("runner가 이미 존재함")
	ErrRunnerNotFound      = errors.New("runner를 찾을 수 없음")

	// 리소스 관련 에러
	ErrMaxContainersReached  = errors.New("최대 container 수 초과")
	ErrInsufficientResources = errors.New("리소스 부족")

	// 통신 관련 에러
	ErrAPITimeout          = errors.New("API 요청 타임아웃")
	ErrAPIConnectionFailed = errors.New("API 연결 실패")

	// 작업 공간 관련 에러
	ErrWorkspaceNotFound     = errors.New("작업 공간을 찾을 수 없음")
	ErrWorkspaceCreateFailed = errors.New("작업 공간 생성 실패")
)

// RunnerError는 Runner 관련 에러를 래핑합니다.
type RunnerError struct {
	Op       string // 작업명 (예: "Start", "Stop", "Run")
	RunnerID string // Runner ID
	Err      error  // 원본 에러
}

func (e *RunnerError) Error() string {
	if e.RunnerID != "" {
		return fmt.Sprintf("runner[%s] %s: %v", e.RunnerID, e.Op, e.Err)
	}
	return fmt.Sprintf("runner %s: %v", e.Op, e.Err)
}

func (e *RunnerError) Unwrap() error {
	return e.Err
}

// NewRunnerError는 새 RunnerError를 생성합니다.
func NewRunnerError(op, runnerID string, err error) *RunnerError {
	return &RunnerError{
		Op:       op,
		RunnerID: runnerID,
		Err:      err,
	}
}

// ContainerError는 Container 관련 에러를 래핑합니다.
type ContainerError struct {
	Op          string // 작업명
	ContainerID string // Container ID
	Err         error  // 원본 에러
	Recoverable bool   // 복구 가능 여부
}

func (e *ContainerError) Error() string {
	return fmt.Sprintf("container[%s] %s: %v", e.ContainerID, e.Op, e.Err)
}

func (e *ContainerError) Unwrap() error {
	return e.Err
}

// NewContainerError는 새 ContainerError를 생성합니다.
func NewContainerError(op, containerID string, err error, recoverable bool) *ContainerError {
	return &ContainerError{
		Op:          op,
		ContainerID: containerID,
		Err:         err,
		Recoverable: recoverable,
	}
}

// IsRecoverable는 에러가 복구 가능한지 확인합니다.
func IsRecoverable(err error) bool {
	var containerErr *ContainerError
	if errors.As(err, &containerErr) {
		return containerErr.Recoverable
	}

	// 특정 에러 타입 확인
	switch {
	case errors.Is(err, ErrAPITimeout):
		return true
	case errors.Is(err, ErrAPIConnectionFailed):
		return true
	case errors.Is(err, ErrContainerUnhealthy):
		return true
	default:
		return false
	}
}

// IsRetryable는 재시도 가능한 에러인지 확인합니다.
func IsRetryable(err error) bool {
	switch {
	case errors.Is(err, ErrAPITimeout):
		return true
	case errors.Is(err, ErrAPIConnectionFailed):
		return true
	case errors.Is(err, ErrContainerStartFailed):
		return true
	default:
		return false
	}
}
