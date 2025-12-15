package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/cnap-oss/app/internal/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// CreateAgent는 새로운 에이전트를 생성합니다.
func (c *Controller) CreateAgent(ctx context.Context, agentID, description, provider, model, prompt string) error {
	c.logger.Info("Creating agent",
		zap.String("agent_id", agentID),
		zap.String("provider", provider),
		zap.String("model", model),
	)

	if c.repo == nil {
		return fmt.Errorf("controller: repository is not configured")
	}

	payload := &storage.Agent{
		AgentID:     agentID,
		Description: description,
		Provider:    provider,
		Model:       model,
		Prompt:      prompt,
		Status:      storage.AgentStatusActive,
	}

	if err := c.repo.CreateAgent(ctx, payload); err != nil {
		c.logger.Error("Failed to persist agent", zap.Error(err))
		return err
	}

	c.logger.Info("Agent created successfully",
		zap.String("agent", agentID),
		zap.Int64("id", payload.ID),
	)
	return nil
}

// DeleteAgent는 기존 에이전트를 삭제합니다.
func (c *Controller) DeleteAgent(ctx context.Context, agent string) error {
	c.logger.Info("Deleting agent",
		zap.String("agent", agent),
	)

	if c.repo == nil {
		return fmt.Errorf("controller: repository is not configured")
	}

	if err := c.repo.UpsertAgentStatus(ctx, agent, storage.AgentStatusDeleted); err != nil {
		return err
	}

	c.logger.Info("Agent deleted successfully",
		zap.String("agent", agent),
	)
	return nil
}

// GetAgentInfo는 특정 에이전트의 정보를 반환합니다.
func (c *Controller) GetAgentInfo(ctx context.Context, agent string) (*AgentInfo, error) {
	c.logger.Info("Getting agent info",
		zap.String("agent", agent),
	)

	if c.repo == nil {
		return nil, fmt.Errorf("controller: repository is not configured")
	}

	rec, err := c.repo.GetAgent(ctx, agent)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("agent not found: %s", agent)
		}
		return nil, err
	}

	info := &AgentInfo{
		Name:        rec.AgentID,
		Description: rec.Description,
		Provider:    rec.Provider,
		Model:       rec.Model,
		Prompt:      rec.Prompt,
		Status:      rec.Status,
		CreatedAt:   rec.CreatedAt,
		UpdatedAt:   rec.UpdatedAt,
	}

	c.logger.Info("Retrieved agent info",
		zap.String("agent", agent),
		zap.String("status", info.Status),
	)
	return info, nil
}

// UpdateAgent는 에이전트 정보를 수정합니다.
func (c *Controller) UpdateAgent(ctx context.Context, agentID, description, provider, model, prompt string) error {
	c.logger.Info("Updating agent",
		zap.String("agent_id", agentID),
	)

	if c.repo == nil {
		return fmt.Errorf("controller: repository is not configured")
	}

	// Agent 존재 여부 확인
	if _, err := c.repo.GetAgent(ctx, agentID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("agent not found: %s", agentID)
		}
		return err
	}

	agent := &storage.Agent{
		AgentID:     agentID,
		Description: description,
		Provider:    provider,
		Model:       model,
		Prompt:      prompt,
	}

	if err := c.repo.UpdateAgent(ctx, agent); err != nil {
		c.logger.Error("Failed to update agent", zap.Error(err))
		return err
	}

	c.logger.Info("Agent updated successfully", zap.String("agent", agentID))
	return nil
}

// ListAgents는 모든 에이전트 목록을 반환합니다.
func (c *Controller) ListAgents(ctx context.Context) ([]string, error) {
	c.logger.Info("Listing agents")

	if c.repo == nil {
		return nil, fmt.Errorf("controller: repository is not configured")
	}

	records, err := c.repo.ListAgents(ctx)
	if err != nil {
		return nil, err
	}

	agents := make([]string, 0, len(records))
	for _, rec := range records {
		agents = append(agents, rec.AgentID)
	}

	c.logger.Info("Listed agents",
		zap.Int("count", len(agents)),
	)
	return agents, nil
}

// ListAgentsWithInfo는 상세 정보를 포함한 에이전트 목록을 반환합니다.
func (c *Controller) ListAgentsWithInfo(ctx context.Context) ([]*AgentInfo, error) {
	c.logger.Info("Listing agents with info")

	if c.repo == nil {
		return nil, fmt.Errorf("controller: repository is not configured")
	}

	records, err := c.repo.ListAgents(ctx)
	if err != nil {
		return nil, err
	}

	agents := make([]*AgentInfo, 0, len(records))
	for _, rec := range records {
		agents = append(agents, &AgentInfo{
			Name:        rec.AgentID,
			Description: rec.Description,
			Model:       rec.Model,
			Prompt:      rec.Prompt,
			Status:      rec.Status,
			CreatedAt:   rec.CreatedAt,
			UpdatedAt:   rec.UpdatedAt,
		})
	}

	c.logger.Info("Listed agents with info",
		zap.Int("count", len(agents)),
	)
	return agents, nil
}

// ValidateAgent는 에이전트 이름의 유효성을 검증합니다.
func (c *Controller) ValidateAgent(agent string) error {
	if agent == "" {
		return fmt.Errorf("agent name cannot be empty")
	}

	if len(agent) > 64 {
		return fmt.Errorf("agent name too long (max 64 characters)")
	}

	// 추가 검증 로직
	return nil
}
