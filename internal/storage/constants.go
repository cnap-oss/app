package storage

const (
	AgentStatusActive  = "active"
	AgentStatusIdle    = "idle"
	AgentStatusBusy    = "busy"
	AgentStatusDeleted = "deleted"

	// AI Provider types
	ProviderOpenCode  = "opencode"
	ProviderGemini    = "gemini"
	ProviderClaude    = "claude"
	ProviderOpenAI    = "openai"
	ProviderXAI       = "xai"

	TaskStatusPending   = "pending"
	TaskStatusRunning   = "running"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
	TaskStatusCanceled  = "canceled"
	TaskStatusDeleted   = "deleted"

	RunStepStatusPending   = "pending"
	RunStepStatusRunning   = "running"
	RunStepStatusCompleted = "completed"
	RunStepStatusFailed    = "failed"

	RunStepTypeSystem     = "system"
	RunStepTypeTool       = "tool"
	RunStepTypeModel      = "model"
	RunStepTypeCheckpoint = "checkpoint"

	MessageRoleUser      = "user"
	MessageRoleAssistant = "assistant"
	MessageRoleSystem    = "system"
)
