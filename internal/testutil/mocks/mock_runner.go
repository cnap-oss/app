package mocks

import (
	"context"
	"fmt"

	taskrunner "github.com/cnap-oss/app/internal/runner"
)

// MockRunner는 테스트용 TaskRunner 구현입니다.
type MockRunner struct {
	// Responses는 taskID별 응답을 정의합니다.
	Responses map[string]string

	// Errors는 taskID별 에러를 정의합니다.
	Errors map[string]error

	// Calls는 Run 호출 기록입니다.
	Calls []*taskrunner.RunRequest

	// DefaultResponse는 Responses에 없는 경우 사용할 기본 응답입니다.
	DefaultResponse string

	// Callback은 등록된 콜백입니다.
	Callback taskrunner.StatusCallback
}

// NewMockRunner는 새로운 MockRunner를 생성합니다.
func NewMockRunner(callback taskrunner.StatusCallback) *MockRunner {
	return &MockRunner{
		Responses:       make(map[string]string),
		Errors:          make(map[string]error),
		Calls:           make([]*taskrunner.RunRequest, 0),
		DefaultResponse: "Mock response",
		Callback:        callback,
	}
}

// ensure MockRunner implements TaskRunner
var _ taskrunner.TaskRunner = (*MockRunner)(nil)

// Run implements TaskRunner interface (비동기).
func (m *MockRunner) Run(ctx context.Context, req *taskrunner.RunRequest) error {
	// 호출 기록
	m.Calls = append(m.Calls, req)

	// 비동기로 콜백 호출
	go func() {
		// 에러 체크
		if err, ok := m.Errors[req.TaskID]; ok {
			if m.Callback != nil {
				_ = m.Callback.OnError(req.TaskID, err)
			}
			return
		}

		// 응답 조회
		response := m.DefaultResponse
		if resp, ok := m.Responses[req.TaskID]; ok {
			response = resp
		}

		result := &taskrunner.RunResult{
			Agent:   req.Model,
			Name:    req.TaskID,
			Success: true,
			Output:  response,
			Error:   nil,
		}

		if m.Callback != nil {
			_ = m.Callback.OnComplete(req.TaskID, result)
		}
	}()

	return nil
}

// SetResponse는 특정 taskID에 대한 응답을 설정합니다.
func (m *MockRunner) SetResponse(taskID, response string) {
	m.Responses[taskID] = response
}

// SetError는 특정 taskID에 대한 에러를 설정합니다.
func (m *MockRunner) SetError(taskID string, err error) {
	m.Errors[taskID] = err
}

// SetErrorMessage는 특정 taskID에 대한 에러 메시지를 설정합니다.
func (m *MockRunner) SetErrorMessage(taskID, message string) {
	m.Errors[taskID] = fmt.Errorf("%s", message)
}

// GetCallCount는 Run 호출 횟수를 반환합니다.
func (m *MockRunner) GetCallCount() int {
	return len(m.Calls)
}

// GetLastCall은 마지막 Run 호출을 반환합니다.
func (m *MockRunner) GetLastCall() *taskrunner.RunRequest {
	if len(m.Calls) == 0 {
		return nil
	}
	return m.Calls[len(m.Calls)-1]
}

// Reset은 모든 호출 기록을 초기화합니다.
func (m *MockRunner) Reset() {
	m.Calls = make([]*taskrunner.RunRequest, 0)
}
