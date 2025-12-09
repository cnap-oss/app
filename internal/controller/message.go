package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cnap-oss/app/internal/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AddMessage adds a message to an existing task without executing it.
// The message will be stored and can be sent later using SendMessage.
func (c *Controller) AddMessage(ctx context.Context, taskID, role, content string) error {
	c.logger.Info("Adding message to task",
		zap.String("task_id", taskID),
		zap.String("role", role),
	)

	if c.repo == nil {
		return fmt.Errorf("controller: repository is not configured")
	}

	// Task 존재 여부 확인
	if _, err := c.repo.GetTask(ctx, taskID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("task not found: %s", taskID)
		}
		return err
	}

	// 메시지를 파일로 저장하고 인덱스 생성
	filePath, err := c.saveMessageToFile(ctx, taskID, role, content)
	if err != nil {
		c.logger.Error("Failed to save message to file", zap.Error(err))
		return err
	}

	if _, err := c.repo.AppendMessageIndex(ctx, taskID, role, filePath); err != nil {
		c.logger.Error("Failed to add message", zap.Error(err))
		return err
	}

	c.logger.Info("Message added successfully",
		zap.String("task_id", taskID),
		zap.String("role", role),
	)
	return nil
}

// SendMessage triggers the execution of a task.
// This method should be called after creating a task and optionally adding messages.
// The actual execution will be handled by the RunnerManager (to be implemented).
func (c *Controller) SendMessage(ctx context.Context, taskID string) error {
	c.logger.Info("Sending message for task",
		zap.String("task_id", taskID),
	)

	if c.repo == nil {
		return fmt.Errorf("controller: repository is not configured")
	}

	// Task 조회
	task, err := c.repo.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("task not found: %s", taskID)
		}
		return err
	}

	// 이미 실행 중인 경우 에러
	if task.Status == storage.TaskStatusRunning {
		return fmt.Errorf("task is already running: %s", taskID)
	}

	// 완료된 작업은 재실행 불가
	if task.Status == storage.TaskStatusCompleted || task.Status == storage.TaskStatusFailed {
		return fmt.Errorf("task is already finished: %s (status: %s)", taskID, task.Status)
	}

	// 메시지 목록 조회
	messages, err := c.repo.ListMessageIndexByTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to list messages: %w", err)
	}

	// 프롬프트나 메시지가 없으면 에러
	if task.Prompt == "" && len(messages) == 0 {
		return fmt.Errorf("no prompt or messages to send for task: %s", taskID)
	}

	// 상태를 running으로 변경
	if err := c.repo.UpsertTaskStatus(ctx, taskID, task.AgentID, storage.TaskStatusRunning); err != nil {
		c.logger.Error("Failed to update task status", zap.Error(err))
		return err
	}

	c.logger.Info("Task execution triggered",
		zap.String("task_id", taskID),
		zap.String("agent_id", task.AgentID),
		zap.Int("message_count", len(messages)),
	)

	// Task 실행을 위한 context 생성 (5분 타임아웃)
	taskCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	// TaskContext 저장
	c.mu.Lock()
	c.taskContexts[taskID] = &TaskContext{
		ctx:    taskCtx,
		cancel: cancel,
	}
	c.mu.Unlock()

	// RunnerManager를 통해 실제 실행 트리거
	go c.executeTask(taskCtx, taskID, task)

	return nil
}

// ListMessages returns all messages for a task in conversation order.
func (c *Controller) ListMessages(ctx context.Context, taskID string) ([]storage.MessageIndex, error) {
	c.logger.Info("Listing messages for task",
		zap.String("task_id", taskID),
	)

	if c.repo == nil {
		return nil, fmt.Errorf("controller: repository is not configured")
	}

	messages, err := c.repo.ListMessageIndexByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	c.logger.Info("Listed messages",
		zap.String("task_id", taskID),
		zap.Int("count", len(messages)),
	)
	return messages, nil
}

// saveMessageToFile saves message content to a file and returns the file path.
// Messages are stored in data/messages/{taskID}/{conversationIndex}.json
func (c *Controller) saveMessageToFile(ctx context.Context, taskID, role, content string) (string, error) {
	// 1. 디렉토리 생성
	dir := filepath.Join("data", "messages", taskID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.logger.Error("Failed to create message directory",
			zap.String("dir", dir),
			zap.Error(err),
		)
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// 2. Conversation index 조회
	messages, err := c.repo.ListMessageIndexByTask(ctx, taskID)
	if err != nil {
		c.logger.Error("Failed to list messages", zap.Error(err))
		return "", fmt.Errorf("failed to list messages: %w", err)
	}
	index := len(messages)

	// 3. JSON 파일 저장
	filename := fmt.Sprintf("%04d.json", index)
	filePath := filepath.Join(dir, filename)

	msg := map[string]interface{}{
		"role":      role,
		"content":   content,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		c.logger.Error("Failed to marshal message", zap.Error(err))
		return "", fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		c.logger.Error("Failed to write file",
			zap.String("path", filePath),
			zap.Error(err),
		)
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	c.logger.Debug("Message saved to file",
		zap.String("task_id", taskID),
		zap.String("role", role),
		zap.String("path", filePath),
	)

	return filePath, nil
}

// readMessageFromFile reads message content from a JSON file.
func (c *Controller) readMessageFromFile(filePath string) (string, error) {
	// 파일 읽기
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// JSON 파싱
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %w", err)
	}

	// content 필드 추출
	content, ok := msg["content"].(string)
	if !ok {
		return "", fmt.Errorf("content field not found or not a string")
	}

	return content, nil
}