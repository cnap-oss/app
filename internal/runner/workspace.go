package taskrunner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/cnap-oss/app/internal/common"
	"go.uber.org/zap"
)

// WorkspaceManager는 Agent 작업 공간을 관리합니다.
type WorkspaceManager interface {
	// CreateWorkspace는 Agent용 작업 공간을 생성합니다.
	CreateWorkspace(ctx context.Context, agentID string) (*Workspace, error)

	// GetWorkspace는 기존 작업 공간을 조회합니다.
	GetWorkspace(agentID string) (*Workspace, error)

	// DeleteWorkspace는 작업 공간을 삭제합니다.
	DeleteWorkspace(ctx context.Context, agentID string, force bool) error

	// ListWorkspaces는 모든 작업 공간을 나열합니다.
	ListWorkspaces(ctx context.Context) ([]*Workspace, error)

	// CleanupStaleWorkspaces는 오래된 작업 공간을 정리합니다.
	CleanupStaleWorkspaces(ctx context.Context, maxAgeDays int) error
}

// Workspace는 Agent 작업 공간 정보입니다.
type Workspace struct {
	AgentID       string `json:"agent_id"`
	BasePath      string `json:"base_path"`       // 전체 경로 (./data/workspace/{agent_id})
	OpenCodeDir   string `json:"opencode_dir"`    // .opencode 디렉토리 경로
	ProjectDir    string `json:"project_dir"`     // project 디렉토리 경로
	LogDir        string `json:"log_dir"`         // logs 디렉토리 경로
	ConfigPath    string `json:"config_path"`     // config.json 경로
	MCPConfigPath string `json:"mcp_config_path"` // mcp.json 경로
}

// WorkspaceConfig는 WorkspaceManager 설정입니다.
type WorkspaceConfig struct {
	BaseDir           string // 기본 디렉토리 (기본: ./data/workspace)
	DefaultConfigPath string // 기본 OpenCode 설정 템플릿 경로
	DefaultMCPPath    string // 기본 MCP 설정 템플릿 경로
	MaxDiskUsageMB    int64  // 최대 디스크 사용량 (MB, 기본: 1024)
}

// DefaultWorkspaceConfig는 기본 Workspace 설정을 반환합니다.
func DefaultWorkspaceConfig() WorkspaceConfig {
	configsDir := common.GetConfigsDir()
	return WorkspaceConfig{
		BaseDir:           common.GetWorkspaceDir(),
		DefaultConfigPath: getEnvOrDefault("CNAP_WORKSPACE_DEFAULT_CONFIG", filepath.Join(configsDir, "opencode", "default-config.json")),
		DefaultMCPPath:    getEnvOrDefault("CNAP_WORKSPACE_DEFAULT_MCP", filepath.Join(configsDir, "opencode", "default-mcp.json")),
		MaxDiskUsageMB:    getEnvOrDefaultInt64("CNAP_WORKSPACE_MAX_DISK_MB", 1024),
	}
}

// workspaceManager는 WorkspaceManager 구현체입니다.
type workspaceManager struct {
	config     WorkspaceConfig
	workspaces map[string]*Workspace
	mu         sync.RWMutex
	logger     *zap.Logger
}

// NewWorkspaceManager는 새 WorkspaceManager를 생성합니다.
func NewWorkspaceManager(logger *zap.Logger, config ...WorkspaceConfig) WorkspaceManager {
	cfg := DefaultWorkspaceConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	wm := &workspaceManager{
		config:     cfg,
		workspaces: make(map[string]*Workspace),
		logger:     logger,
	}

	// 기본 디렉토리 생성
	if err := os.MkdirAll(cfg.BaseDir, 0755); err != nil {
		logger.Error("기본 디렉토리 생성 실패", zap.Error(err))
	}

	return wm
}

// CreateWorkspace implements WorkspaceManager.
func (wm *workspaceManager) CreateWorkspace(ctx context.Context, agentID string) (*Workspace, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	// 이미 존재하는지 확인
	if ws, exists := wm.workspaces[agentID]; exists {
		return ws, nil
	}

	basePath := filepath.Join(wm.config.BaseDir, agentID)

	ws := &Workspace{
		AgentID:       agentID,
		BasePath:      basePath,
		OpenCodeDir:   filepath.Join(basePath, ".opencode"),
		ProjectDir:    filepath.Join(basePath, "project"),
		LogDir:        filepath.Join(basePath, "logs"),
		ConfigPath:    filepath.Join(basePath, ".opencode", "config.json"),
		MCPConfigPath: filepath.Join(basePath, ".opencode", "mcp.json"),
	}

	// 디렉토리 구조 생성
	dirs := []string{
		ws.BasePath,
		ws.OpenCodeDir,
		ws.ProjectDir,
		ws.LogDir,
		filepath.Join(ws.OpenCodeDir, "sessions"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("디렉토리 생성 실패 (%s): %w", dir, err)
		}
	}

	// 기본 설정 파일 복사
	if err := wm.initializeConfigs(ws); err != nil {
		wm.logger.Warn("기본 설정 초기화 실패",
			zap.String("agent_id", agentID),
			zap.Error(err),
		)
	}

	wm.workspaces[agentID] = ws
	wm.logger.Info("작업 공간 생성됨",
		zap.String("agent_id", agentID),
		zap.String("path", basePath),
	)

	return ws, nil
}

// GetWorkspace implements WorkspaceManager.
func (wm *workspaceManager) GetWorkspace(agentID string) (*Workspace, error) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	if ws, exists := wm.workspaces[agentID]; exists {
		return ws, nil
	}

	// 캐시에 없으면 디스크에서 확인
	basePath := filepath.Join(wm.config.BaseDir, agentID)
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("작업 공간을 찾을 수 없음: %s", agentID)
	}

	ws := &Workspace{
		AgentID:       agentID,
		BasePath:      basePath,
		OpenCodeDir:   filepath.Join(basePath, ".opencode"),
		ProjectDir:    filepath.Join(basePath, "project"),
		LogDir:        filepath.Join(basePath, "logs"),
		ConfigPath:    filepath.Join(basePath, ".opencode", "config.json"),
		MCPConfigPath: filepath.Join(basePath, ".opencode", "mcp.json"),
	}

	return ws, nil
}

// DeleteWorkspace implements WorkspaceManager.
func (wm *workspaceManager) DeleteWorkspace(ctx context.Context, agentID string, force bool) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	basePath := filepath.Join(wm.config.BaseDir, agentID)

	if !force {
		// 안전 삭제: 일정 기간 후 삭제 예약
		// 실제 구현에서는 삭제 예약 로직 추가
		wm.logger.Info("작업 공간 삭제 예약",
			zap.String("agent_id", agentID),
		)
	}

	if err := os.RemoveAll(basePath); err != nil {
		return fmt.Errorf("작업 공간 삭제 실패: %w", err)
	}

	delete(wm.workspaces, agentID)
	wm.logger.Info("작업 공간 삭제됨",
		zap.String("agent_id", agentID),
	)

	return nil
}

// ListWorkspaces implements WorkspaceManager.
func (wm *workspaceManager) ListWorkspaces(ctx context.Context) ([]*Workspace, error) {
	entries, err := os.ReadDir(wm.config.BaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Workspace{}, nil
		}
		return nil, fmt.Errorf("디렉토리 읽기 실패: %w", err)
	}

	var workspaces []*Workspace
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		ws, err := wm.GetWorkspace(entry.Name())
		if err != nil {
			wm.logger.Warn("작업 공간 조회 실패",
				zap.String("name", entry.Name()),
				zap.Error(err),
			)
			continue
		}
		workspaces = append(workspaces, ws)
	}

	return workspaces, nil
}

// CleanupStaleWorkspaces implements WorkspaceManager.
func (wm *workspaceManager) CleanupStaleWorkspaces(ctx context.Context, maxAgeDays int) error {
	// TODO: 구현 - 오래된 작업 공간 정리
	// 마지막 수정 시간 기준으로 정리
	return nil
}

// initializeConfigs는 기본 설정 파일을 복사합니다.
func (wm *workspaceManager) initializeConfigs(ws *Workspace) error {
	// 기본 OpenCode 설정 복사
	if err := wm.copyIfNotExists(wm.config.DefaultConfigPath, ws.ConfigPath); err != nil {
		return fmt.Errorf("config.json 초기화 실패: %w", err)
	}

	// 기본 MCP 설정 복사
	if err := wm.copyIfNotExists(wm.config.DefaultMCPPath, ws.MCPConfigPath); err != nil {
		return fmt.Errorf("mcp.json 초기화 실패: %w", err)
	}

	return nil
}

// copyIfNotExists는 대상 파일이 없으면 소스 파일을 복사합니다.
func (wm *workspaceManager) copyIfNotExists(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return nil // 이미 존재함
	}

	// 소스 파일이 없으면 기본 설정 생성
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return wm.createDefaultConfig(dst)
	}

	srcData, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, srcData, 0644)
}

// createDefaultConfig는 기본 설정 파일을 생성합니다.
func (wm *workspaceManager) createDefaultConfig(path string) error {
	var config interface{}

	if filepath.Base(path) == "config.json" {
		config = map[string]interface{}{
			"model":   "claude-sonnet-4-20250514",
			"theme":   "dark",
			"timeout": 120,
		}
	} else if filepath.Base(path) == "mcp.json" {
		config = map[string]interface{}{
			"mcpServers": map[string]interface{}{},
		}
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// getEnvOrDefault는 환경 변수를 읽거나 기본값을 반환합니다.
func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

// getEnvOrDefaultInt64는 환경 변수를 int64로 읽거나 기본값을 반환합니다.
func getEnvOrDefaultInt64(key string, defaultValue int64) int64 {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.ParseInt(val, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// 인터페이스 구현 확인
var _ WorkspaceManager = (*workspaceManager)(nil)
