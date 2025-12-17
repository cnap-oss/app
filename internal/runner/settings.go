package taskrunner

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/cnap-oss/app/internal/common"
	"go.uber.org/zap"
)

// SettingsManager는 Container 설정을 관리합니다.
type SettingsManager interface {
	// GetOpenCodeConfig는 OpenCode 설정을 반환합니다.
	GetOpenCodeConfig(agentID string) (*OpenCodeConfig, error)

	// UpdateOpenCodeConfig는 OpenCode 설정을 업데이트합니다.
	UpdateOpenCodeConfig(agentID string, config *OpenCodeConfig) error

	// GetMCPConfig는 MCP 설정을 반환합니다.
	GetMCPConfig(agentID string) (*MCPConfig, error)

	// UpdateMCPConfig는 MCP 설정을 업데이트합니다.
	UpdateMCPConfig(agentID string, config *MCPConfig) error

	// AddMCPServer는 MCP 서버를 추가합니다.
	AddMCPServer(agentID string, name string, server MCPServer) error

	// RemoveMCPServer는 MCP 서버를 제거합니다.
	RemoveMCPServer(agentID string, name string) error

	// BuildContainerEnv는 Container 환경 변수를 생성합니다.
	BuildContainerEnv(agentID string, additionalEnv map[string]string) ([]string, error)
}

// OpenCodeConfig는 OpenCode 설정입니다.
type OpenCodeConfig struct {
	Model    string                 `json:"model"`
	Provider string                 `json:"provider,omitempty"`
	Theme    string                 `json:"theme,omitempty"`
	Timeout  int                    `json:"timeout,omitempty"`
	Features map[string]bool        `json:"features,omitempty"`
	Limits   map[string]int         `json:"limits,omitempty"`
	Custom   map[string]interface{} `json:"custom,omitempty"`
}

// MCPConfig는 MCP 설정입니다.
type MCPConfig struct {
	MCPServers map[string]MCPServer `json:"mcpServers"`
}

// MCPServer는 MCP 서버 설정입니다.
type MCPServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Enabled bool              `json:"enabled,omitempty"`
}

// settingsManager는 SettingsManager 구현체입니다.
type settingsManager struct {
	workspaceManager WorkspaceManager
	logger           *zap.Logger
}

// NewSettingsManager는 새 SettingsManager를 생성합니다.
func NewSettingsManager(wm WorkspaceManager, logger *zap.Logger) SettingsManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &settingsManager{
		workspaceManager: wm,
		logger:           logger,
	}
}

// GetOpenCodeConfig implements SettingsManager.
func (sm *settingsManager) GetOpenCodeConfig(agentID string) (*OpenCodeConfig, error) {
	ws, err := sm.workspaceManager.GetWorkspace(agentID)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(ws.ConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 기본 설정 반환
			return &OpenCodeConfig{
				Model:   "claude-sonnet-4-20250514",
				Timeout: 120,
			}, nil
		}
		return nil, fmt.Errorf("설정 파일 읽기 실패: %w", err)
	}

	var config OpenCodeConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("설정 파싱 실패: %w", err)
	}

	return &config, nil
}

// UpdateOpenCodeConfig implements SettingsManager.
func (sm *settingsManager) UpdateOpenCodeConfig(agentID string, config *OpenCodeConfig) error {
	ws, err := sm.workspaceManager.GetWorkspace(agentID)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("설정 직렬화 실패: %w", err)
	}

	if err := os.WriteFile(ws.ConfigPath, data, 0644); err != nil {
		return fmt.Errorf("설정 저장 실패: %w", err)
	}

	sm.logger.Info("OpenCode 설정 업데이트됨",
		zap.String("agent_id", agentID),
	)

	return nil
}

// GetMCPConfig implements SettingsManager.
func (sm *settingsManager) GetMCPConfig(agentID string) (*MCPConfig, error) {
	ws, err := sm.workspaceManager.GetWorkspace(agentID)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(ws.MCPConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &MCPConfig{
				MCPServers: make(map[string]MCPServer),
			}, nil
		}
		return nil, fmt.Errorf("MCP 설정 읽기 실패: %w", err)
	}

	var config MCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("MCP 설정 파싱 실패: %w", err)
	}

	return &config, nil
}

// UpdateMCPConfig implements SettingsManager.
func (sm *settingsManager) UpdateMCPConfig(agentID string, config *MCPConfig) error {
	ws, err := sm.workspaceManager.GetWorkspace(agentID)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("MCP 설정 직렬화 실패: %w", err)
	}

	if err := os.WriteFile(ws.MCPConfigPath, data, 0644); err != nil {
		return fmt.Errorf("MCP 설정 저장 실패: %w", err)
	}

	sm.logger.Info("MCP 설정 업데이트됨",
		zap.String("agent_id", agentID),
	)

	return nil
}

// AddMCPServer implements SettingsManager.
func (sm *settingsManager) AddMCPServer(agentID string, name string, server MCPServer) error {
	config, err := sm.GetMCPConfig(agentID)
	if err != nil {
		return err
	}

	if config.MCPServers == nil {
		config.MCPServers = make(map[string]MCPServer)
	}

	config.MCPServers[name] = server
	return sm.UpdateMCPConfig(agentID, config)
}

// RemoveMCPServer implements SettingsManager.
func (sm *settingsManager) RemoveMCPServer(agentID string, name string) error {
	config, err := sm.GetMCPConfig(agentID)
	if err != nil {
		return err
	}

	delete(config.MCPServers, name)
	return sm.UpdateMCPConfig(agentID, config)
}

// BuildContainerEnv implements SettingsManager.
func (sm *settingsManager) BuildContainerEnv(agentID string, additionalEnv map[string]string) ([]string, error) {
	config, err := sm.GetOpenCodeConfig(agentID)
	if err != nil {
		sm.logger.Warn("OpenCode 설정 조회 실패, 기본값 사용",
			zap.String("agent_id", agentID),
			zap.Error(err),
		)
		config = &OpenCodeConfig{}
	}

	env := make([]string, 0)

	// 기본 환경 변수
	env = append(env, "OPENCODE_DATA_DIR=/home/opencode/.opencode")
	env = append(env, "OPENCODE_WORKSPACE=/workspace")

	// 모델 설정
	if config.Model != "" {
		env = append(env, fmt.Sprintf("OPENCODE_MODEL=%s", config.Model))
	}
	if config.Provider != "" {
		env = append(env, fmt.Sprintf("OPENCODE_PROVIDER=%s", config.Provider))
	}

	// API 키 전달 (config에서)
	cfg, err := common.LoadConfig()
	if err == nil {
		env = append(env, cfg.GetAPIKeyEnvVars()...)
	}

	// MCP 설정에서 환경 변수 추출 및 치환
	mcpConfig, err := sm.GetMCPConfig(agentID)
	if err == nil {
		for _, server := range mcpConfig.MCPServers {
			for key, val := range server.Env {
				// ${VAR} 형태의 환경 변수 참조 치환
				resolvedVal := sm.resolveEnvVars(val)
				if resolvedVal != "" {
					env = append(env, fmt.Sprintf("%s=%s", key, resolvedVal))
				}
			}
		}
	}

	// 추가 환경 변수
	for key, val := range additionalEnv {
		env = append(env, fmt.Sprintf("%s=%s", key, val))
	}

	return env, nil
}

// resolveEnvVars는 ${VAR} 형태의 환경 변수 참조를 치환합니다.
func (sm *settingsManager) resolveEnvVars(value string) string {
	re := regexp.MustCompile(`\$\{(\w+)\}`)
	return re.ReplaceAllStringFunc(value, func(match string) string {
		varName := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		if val := os.Getenv(varName); val != "" {
			return val
		}
		return match // 치환 실패 시 원본 유지
	})
}

// 인터페이스 구현 확인
var _ SettingsManager = (*settingsManager)(nil)
