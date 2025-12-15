package taskrunner

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSettingsManager_OpenCodeConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "settings-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	wm := NewWorkspaceManager(zap.NewNop(), WorkspaceConfig{BaseDir: tmpDir})
	sm := NewSettingsManager(wm, zap.NewNop())

	ctx := context.Background()
	_, err = wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)

	// 기본 설정 조회
	config, err := sm.GetOpenCodeConfig("test-agent")
	require.NoError(t, err)
	assert.NotEmpty(t, config.Model)

	// 설정 업데이트
	config.Model = "gpt-4"
	config.Timeout = 60
	err = sm.UpdateOpenCodeConfig("test-agent", config)
	require.NoError(t, err)

	// 업데이트 확인
	updated, err := sm.GetOpenCodeConfig("test-agent")
	require.NoError(t, err)
	assert.Equal(t, "gpt-4", updated.Model)
	assert.Equal(t, 60, updated.Timeout)
}

func TestSettingsManager_OpenCodeConfig_NotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "settings-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	wm := NewWorkspaceManager(zap.NewNop(), WorkspaceConfig{BaseDir: tmpDir})
	sm := NewSettingsManager(wm, zap.NewNop())

	_, err = sm.GetOpenCodeConfig("nonexistent")
	assert.Error(t, err)
}

func TestSettingsManager_OpenCodeConfig_DefaultWhenFileNotExist(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "settings-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	wm := NewWorkspaceManager(zap.NewNop(), WorkspaceConfig{BaseDir: tmpDir})
	sm := NewSettingsManager(wm, zap.NewNop())

	ctx := context.Background()
	ws, err := wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)

	// config.json 삭제
	_ = os.Remove(ws.ConfigPath)

	// 기본 설정이 반환되어야 함
	config, err := sm.GetOpenCodeConfig("test-agent")
	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet-4-20250514", config.Model)
	assert.Equal(t, 120, config.Timeout)
}

func TestSettingsManager_MCPConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "settings-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	wm := NewWorkspaceManager(zap.NewNop(), WorkspaceConfig{BaseDir: tmpDir})
	sm := NewSettingsManager(wm, zap.NewNop())

	ctx := context.Background()
	_, err = wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)

	// MCP 서버 추가
	err = sm.AddMCPServer("test-agent", "git", MCPServer{
		Command: "mcp-server-git",
		Args:    []string{},
		Enabled: true,
	})
	require.NoError(t, err)

	// 추가 확인
	config, err := sm.GetMCPConfig("test-agent")
	require.NoError(t, err)
	assert.Contains(t, config.MCPServers, "git")
	assert.Equal(t, "mcp-server-git", config.MCPServers["git"].Command)

	// MCP 서버 제거
	err = sm.RemoveMCPServer("test-agent", "git")
	require.NoError(t, err)

	config, err = sm.GetMCPConfig("test-agent")
	require.NoError(t, err)
	assert.NotContains(t, config.MCPServers, "git")
}

func TestSettingsManager_MCPConfig_Multiple(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "settings-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	wm := NewWorkspaceManager(zap.NewNop(), WorkspaceConfig{BaseDir: tmpDir})
	sm := NewSettingsManager(wm, zap.NewNop())

	ctx := context.Background()
	_, err = wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)

	// 여러 MCP 서버 추가
	servers := map[string]MCPServer{
		"git": {
			Command: "mcp-server-git",
			Enabled: true,
		},
		"filesystem": {
			Command: "mcp-server-filesystem",
			Args:    []string{"/workspace"},
			Enabled: true,
		},
	}

	for name, server := range servers {
		err = sm.AddMCPServer("test-agent", name, server)
		require.NoError(t, err)
	}

	// 확인
	config, err := sm.GetMCPConfig("test-agent")
	require.NoError(t, err)
	assert.Len(t, config.MCPServers, 2)
	assert.Contains(t, config.MCPServers, "git")
	assert.Contains(t, config.MCPServers, "filesystem")
}

func TestSettingsManager_UpdateMCPConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "settings-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	wm := NewWorkspaceManager(zap.NewNop(), WorkspaceConfig{BaseDir: tmpDir})
	sm := NewSettingsManager(wm, zap.NewNop())

	ctx := context.Background()
	_, err = wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)

	// MCP 설정 직접 업데이트
	mcpConfig := &MCPConfig{
		MCPServers: map[string]MCPServer{
			"custom": {
				Command: "custom-mcp",
				Args:    []string{"--port", "8080"},
				Env: map[string]string{
					"API_KEY": "test-key",
				},
				Enabled: true,
			},
		},
	}

	err = sm.UpdateMCPConfig("test-agent", mcpConfig)
	require.NoError(t, err)

	// 확인
	config, err := sm.GetMCPConfig("test-agent")
	require.NoError(t, err)
	assert.Contains(t, config.MCPServers, "custom")
	assert.Equal(t, "custom-mcp", config.MCPServers["custom"].Command)
	assert.Equal(t, "test-key", config.MCPServers["custom"].Env["API_KEY"])
}

func TestSettingsManager_BuildContainerEnv(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "settings-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// 환경 변수 설정
	_ = os.Setenv("ANTHROPIC_API_KEY", "test-key")
	defer os.Unsetenv("ANTHROPIC_API_KEY")

	wm := NewWorkspaceManager(zap.NewNop(), WorkspaceConfig{BaseDir: tmpDir})
	sm := NewSettingsManager(wm, zap.NewNop())

	ctx := context.Background()
	_, err = wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)

	env, err := sm.BuildContainerEnv("test-agent", map[string]string{
		"CUSTOM_VAR": "custom-value",
	})
	require.NoError(t, err)

	// 기본 환경 변수 확인
	assert.Contains(t, env, "OPENCODE_DATA_DIR=/home/opencode/.opencode")
	assert.Contains(t, env, "OPENCODE_WORKSPACE=/workspace")

	// API 키 확인
	assert.Contains(t, env, "ANTHROPIC_API_KEY=test-key")

	// 커스텀 환경 변수 확인
	assert.Contains(t, env, "CUSTOM_VAR=custom-value")
}

func TestSettingsManager_BuildContainerEnv_WithModel(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "settings-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	wm := NewWorkspaceManager(zap.NewNop(), WorkspaceConfig{BaseDir: tmpDir})
	sm := NewSettingsManager(wm, zap.NewNop())

	ctx := context.Background()
	_, err = wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)

	// 모델 설정
	config := &OpenCodeConfig{
		Model:    "gpt-4",
		Provider: "openai",
	}
	err = sm.UpdateOpenCodeConfig("test-agent", config)
	require.NoError(t, err)

	env, err := sm.BuildContainerEnv("test-agent", nil)
	require.NoError(t, err)

	assert.Contains(t, env, "OPENCODE_MODEL=gpt-4")
	assert.Contains(t, env, "OPENCODE_PROVIDER=openai")
}

func TestSettingsManager_BuildContainerEnv_WithMCPEnv(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "settings-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// 호스트 환경 변수 설정
	_ = os.Setenv("MY_SECRET", "secret-value")
	defer os.Unsetenv("MY_SECRET")

	wm := NewWorkspaceManager(zap.NewNop(), WorkspaceConfig{BaseDir: tmpDir})
	sm := NewSettingsManager(wm, zap.NewNop())

	ctx := context.Background()
	_, err = wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)

	// MCP 서버에 환경 변수 참조 추가
	err = sm.AddMCPServer("test-agent", "custom", MCPServer{
		Command: "custom-mcp",
		Env: map[string]string{
			"SECRET_KEY": "${MY_SECRET}",
		},
	})
	require.NoError(t, err)

	env, err := sm.BuildContainerEnv("test-agent", nil)
	require.NoError(t, err)

	// 환경 변수 치환 확인
	assert.Contains(t, env, "SECRET_KEY=secret-value")
}

func TestSettingsManager_ResolveEnvVars(t *testing.T) {
	_ = os.Setenv("TEST_VAR", "resolved-value")
	defer os.Unsetenv("TEST_VAR")

	sm := &settingsManager{logger: zap.NewNop()}

	// 치환 테스트
	result := sm.resolveEnvVars("prefix_${TEST_VAR}_suffix")
	assert.Equal(t, "prefix_resolved-value_suffix", result)

	// 여러 변수 치환
	_ = os.Setenv("VAR1", "value1")
	_ = os.Setenv("VAR2", "value2")
	defer os.Unsetenv("VAR1")
	defer os.Unsetenv("VAR2")

	result = sm.resolveEnvVars("${VAR1}-${VAR2}")
	assert.Equal(t, "value1-value2", result)

	// 없는 변수는 원본 유지
	result = sm.resolveEnvVars("${NONEXISTENT}")
	assert.Equal(t, "${NONEXISTENT}", result)
}

func TestSettingsManager_BuildContainerEnv_NonexistentAgent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "settings-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	wm := NewWorkspaceManager(zap.NewNop(), WorkspaceConfig{BaseDir: tmpDir})
	sm := NewSettingsManager(wm, zap.NewNop())

	// 존재하지 않는 에이전트도 기본 환경 변수 반환
	env, err := sm.BuildContainerEnv("nonexistent", nil)
	require.NoError(t, err)
	assert.Contains(t, env, "OPENCODE_DATA_DIR=/home/opencode/.opencode")
}

func TestSettingsManager_AllAPIKeys(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "settings-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// 여러 API 키 설정
	apiKeys := map[string]string{
		"OPENCODE_API_KEY":     "opencode-key",
		"ANTHROPIC_API_KEY":    "anthropic-key",
		"OPENAI_API_KEY":       "openai-key",
		"GOOGLE_API_KEY":       "google-key",
		"AZURE_OPENAI_API_KEY": "azure-key",
	}

	for key, val := range apiKeys {
		_ = os.Setenv(key, val)
		defer os.Unsetenv(key)
	}

	wm := NewWorkspaceManager(zap.NewNop(), WorkspaceConfig{BaseDir: tmpDir})
	sm := NewSettingsManager(wm, zap.NewNop())

	ctx := context.Background()
	_, err = wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)

	env, err := sm.BuildContainerEnv("test-agent", nil)
	require.NoError(t, err)

	// 모든 API 키가 포함되어야 함
	for key, val := range apiKeys {
		expected := fmt.Sprintf("%s=%s", key, val)
		assert.Contains(t, env, expected)
	}
}

func TestNewSettingsManager_WithNilLogger(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "settings-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	wm := NewWorkspaceManager(zap.NewNop(), WorkspaceConfig{BaseDir: tmpDir})
	sm := NewSettingsManager(wm, nil)

	assert.NotNil(t, sm)
}
