package taskrunner

import (
	"testing"
	"time"
)

func TestRunnerMessage_IsText(t *testing.T) {
	tests := []struct {
		name     string
		msgType  RunnerMessageType
		expected bool
	}{
		{"Text message", MessageTypeText, true},
		{"Reasoning message", MessageTypeReasoning, true},
		{"Tool call message", MessageTypeToolCall, false},
		{"Complete message", MessageTypeComplete, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &RunnerMessage{Type: tt.msgType}
			if got := msg.IsText(); got != tt.expected {
				t.Errorf("IsText() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRunnerMessage_IsToolRelated(t *testing.T) {
	tests := []struct {
		name     string
		msgType  RunnerMessageType
		expected bool
	}{
		{"Tool call message", MessageTypeToolCall, true},
		{"Tool result message", MessageTypeToolResult, true},
		{"Text message", MessageTypeText, false},
		{"Complete message", MessageTypeComplete, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &RunnerMessage{Type: tt.msgType}
			if got := msg.IsToolRelated(); got != tt.expected {
				t.Errorf("IsToolRelated() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRunnerMessage_IsTerminal(t *testing.T) {
	tests := []struct {
		name     string
		msgType  RunnerMessageType
		expected bool
	}{
		{"Complete message", MessageTypeComplete, true},
		{"Error message", MessageTypeError, true},
		{"Session aborted message", MessageTypeSessionAborted, true},
		{"Text message", MessageTypeText, false},
		{"Tool call message", MessageTypeToolCall, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &RunnerMessage{Type: tt.msgType}
			if got := msg.IsTerminal(); got != tt.expected {
				t.Errorf("IsTerminal() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRunnerMessageType_Constants(t *testing.T) {
	// 상수가 올바르게 정의되었는지 확인
	if MessageTypeText != "text" {
		t.Errorf("MessageTypeText = %v, want 'text'", MessageTypeText)
	}
	if MessageTypeReasoning != "reasoning" {
		t.Errorf("MessageTypeReasoning = %v, want 'reasoning'", MessageTypeReasoning)
	}
	if MessageTypeToolCall != "tool_call" {
		t.Errorf("MessageTypeToolCall = %v, want 'tool_call'", MessageTypeToolCall)
	}
	if MessageTypeToolResult != "tool_result" {
		t.Errorf("MessageTypeToolResult = %v, want 'tool_result'", MessageTypeToolResult)
	}
	if MessageTypeStatus != "status" {
		t.Errorf("MessageTypeStatus = %v, want 'status'", MessageTypeStatus)
	}
	if MessageTypeProgress != "progress" {
		t.Errorf("MessageTypeProgress = %v, want 'progress'", MessageTypeProgress)
	}
	if MessageTypeComplete != "complete" {
		t.Errorf("MessageTypeComplete = %v, want 'complete'", MessageTypeComplete)
	}
	if MessageTypeError != "error" {
		t.Errorf("MessageTypeError = %v, want 'error'", MessageTypeError)
	}
	if MessageTypeSessionCreated != "session_created" {
		t.Errorf("MessageTypeSessionCreated = %v, want 'session_created'", MessageTypeSessionCreated)
	}
	if MessageTypeSessionAborted != "session_aborted" {
		t.Errorf("MessageTypeSessionAborted = %v, want 'session_aborted'", MessageTypeSessionAborted)
	}
}

func TestRunnerMessage_Construction(t *testing.T) {
	now := time.Now()
	msg := &RunnerMessage{
		Type:      MessageTypeText,
		SessionID: "ses_123",
		MessageID: "msg_456",
		Timestamp: now,
		Content:   "Hello, World!",
	}

	if msg.Type != MessageTypeText {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeText)
	}
	if msg.SessionID != "ses_123" {
		t.Errorf("SessionID = %v, want 'ses_123'", msg.SessionID)
	}
	if msg.MessageID != "msg_456" {
		t.Errorf("MessageID = %v, want 'msg_456'", msg.MessageID)
	}
	if msg.Content != "Hello, World!" {
		t.Errorf("Content = %v, want 'Hello, World!'", msg.Content)
	}
}

func TestToolCallInfo_Construction(t *testing.T) {
	toolCall := &ToolCallInfo{
		ToolID:   "tool_123",
		ToolName: "test_tool",
		Arguments: map[string]any{
			"param1": "value1",
			"param2": 42,
		},
	}

	if toolCall.ToolID != "tool_123" {
		t.Errorf("ToolID = %v, want 'tool_123'", toolCall.ToolID)
	}
	if toolCall.ToolName != "test_tool" {
		t.Errorf("ToolName = %v, want 'test_tool'", toolCall.ToolName)
	}
	if len(toolCall.Arguments) != 2 {
		t.Errorf("Arguments length = %v, want 2", len(toolCall.Arguments))
	}
}

func TestToolResultInfo_Construction(t *testing.T) {
	toolResult := &ToolResultInfo{
		ToolID:   "tool_123",
		ToolName: "test_tool",
		Result:   "Success",
		IsError:  false,
	}

	if toolResult.ToolID != "tool_123" {
		t.Errorf("ToolID = %v, want 'tool_123'", toolResult.ToolID)
	}
	if toolResult.ToolName != "test_tool" {
		t.Errorf("ToolName = %v, want 'test_tool'", toolResult.ToolName)
	}
	if toolResult.Result != "Success" {
		t.Errorf("Result = %v, want 'Success'", toolResult.Result)
	}
	if toolResult.IsError != false {
		t.Errorf("IsError = %v, want false", toolResult.IsError)
	}
}

func TestMessageErrorInfo_Construction(t *testing.T) {
	errInfo := &MessageErrorInfo{
		Code:    "ERR_TEST",
		Message: "Test error message",
		Details: map[string]any{
			"key": "value",
		},
	}

	if errInfo.Code != "ERR_TEST" {
		t.Errorf("Code = %v, want 'ERR_TEST'", errInfo.Code)
	}
	if errInfo.Message != "Test error message" {
		t.Errorf("Message = %v, want 'Test error message'", errInfo.Message)
	}
	if errInfo.Details == nil {
		t.Error("Details should not be nil")
	}
}
