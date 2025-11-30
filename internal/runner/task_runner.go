package taskrunner

import "context"

// TaskRunner는 Task 실행을 위한 인터페이스입니다.
type TaskRunner interface {
	// Run은 주어진 메시지들로 AI API를 호출하고 결과를 반환합니다.
	Run(ctx context.Context, req *RunRequest) (*RunResult, error)
}

// RunRequest는 TaskRunner 실행 요청입니다.
type RunRequest struct {
	TaskID       string
	Model        string
	SystemPrompt string
	Messages     []ChatMessage
}

// ensure Runner implements TaskRunner interface
var _ TaskRunner = (*Runner)(nil)

// Run implements TaskRunner interface.
func (r *Runner) Run(ctx context.Context, req *RunRequest) (*RunResult, error) {
	// 시스템 프롬프트와 메시지를 결합
	messages := make([]ChatMessage, 0, len(req.Messages)+1)
	if req.SystemPrompt != "" {
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}
	messages = append(messages, req.Messages...)

	// 마지막 사용자 메시지를 prompt로 사용
	var prompt string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			prompt = messages[i].Content
			break
		}
	}

	// API 호출
	result, err := r.RunWithResult(ctx, req.Model, req.TaskID, prompt)
	if err != nil {
		return nil, err
	}

	return result, nil
}
