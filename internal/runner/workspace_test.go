package taskrunner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestWorkspaceManager_CreateWorkspace(t *testing.T) {
	// 임시 디렉토리 생성
	tmpDir, err := os.MkdirTemp("", "workspace-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config := WorkspaceConfig{
		BaseDir: tmpDir,
	}
	wm := NewWorkspaceManager(zap.NewNop(), config)

	ctx := context.Background()
	ws, err := wm.CreateWorkspace(ctx, "test-agent")

	require.NoError(t, err)
	assert.Equal(t, "test-agent", ws.AgentID)
	assert.DirExists(t, ws.BasePath)
	assert.DirExists(t, ws.OpenCodeDir)
	assert.DirExists(t, ws.ProjectDir)
	assert.DirExists(t, ws.LogDir)
	assert.DirExists(t, filepath.Join(ws.OpenCodeDir, "sessions"))

	// 설정 파일 생성 확인
	assert.FileExists(t, ws.ConfigPath)
	assert.FileExists(t, ws.MCPConfigPath)
}

func TestWorkspaceManager_CreateWorkspace_Idempotent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config := WorkspaceConfig{BaseDir: tmpDir}
	wm := NewWorkspaceManager(zap.NewNop(), config)

	ctx := context.Background()

	// 첫 번째 생성
	ws1, err := wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)

	// 두 번째 생성 (같은 결과 반환)
	ws2, err := wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)

	assert.Equal(t, ws1.BasePath, ws2.BasePath)
	assert.Equal(t, ws1.AgentID, ws2.AgentID)
}

func TestWorkspaceManager_GetWorkspace(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config := WorkspaceConfig{BaseDir: tmpDir}
	wm := NewWorkspaceManager(zap.NewNop(), config)

	ctx := context.Background()
	_, err = wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)

	// 캐시에서 조회
	ws, err := wm.GetWorkspace("test-agent")
	require.NoError(t, err)
	assert.Equal(t, "test-agent", ws.AgentID)
}

func TestWorkspaceManager_GetWorkspace_FromDisk(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// 수동으로 디렉토리 생성
	agentDir := filepath.Join(tmpDir, "disk-agent")
	err = os.MkdirAll(agentDir, 0755)
	require.NoError(t, err)

	config := WorkspaceConfig{BaseDir: tmpDir}
	wm := NewWorkspaceManager(zap.NewNop(), config)

	// 디스크에서 조회
	ws, err := wm.GetWorkspace("disk-agent")
	require.NoError(t, err)
	assert.Equal(t, "disk-agent", ws.AgentID)
	assert.Equal(t, agentDir, ws.BasePath)
}

func TestWorkspaceManager_GetWorkspace_NotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config := WorkspaceConfig{BaseDir: tmpDir}
	wm := NewWorkspaceManager(zap.NewNop(), config)

	_, err = wm.GetWorkspace("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "작업 공간을 찾을 수 없음")
}

func TestWorkspaceManager_DeleteWorkspace(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config := WorkspaceConfig{BaseDir: tmpDir}
	wm := NewWorkspaceManager(zap.NewNop(), config)

	ctx := context.Background()
	ws, err := wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)
	assert.DirExists(t, ws.BasePath)

	// 강제 삭제
	err = wm.DeleteWorkspace(ctx, "test-agent", true)
	require.NoError(t, err)
	assert.NoDirExists(t, ws.BasePath)
}

func TestWorkspaceManager_DeleteWorkspace_SafeMode(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config := WorkspaceConfig{BaseDir: tmpDir}
	wm := NewWorkspaceManager(zap.NewNop(), config)

	ctx := context.Background()
	ws, err := wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)

	// 안전 삭제 (현재 구현은 force와 동일하게 동작)
	err = wm.DeleteWorkspace(ctx, "test-agent", false)
	require.NoError(t, err)
	// 현재는 즉시 삭제됨
	assert.NoDirExists(t, ws.BasePath)
}

func TestWorkspaceManager_ListWorkspaces(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config := WorkspaceConfig{BaseDir: tmpDir}
	wm := NewWorkspaceManager(zap.NewNop(), config)

	ctx := context.Background()

	// 빈 목록
	workspaces, err := wm.ListWorkspaces(ctx)
	require.NoError(t, err)
	assert.Len(t, workspaces, 0)

	// 작업 공간 생성
	_, err = wm.CreateWorkspace(ctx, "agent-1")
	require.NoError(t, err)
	_, err = wm.CreateWorkspace(ctx, "agent-2")
	require.NoError(t, err)

	// 목록 조회
	workspaces, err = wm.ListWorkspaces(ctx)
	require.NoError(t, err)
	assert.Len(t, workspaces, 2)

	// 에이전트 ID 확인
	agentIDs := make(map[string]bool)
	for _, ws := range workspaces {
		agentIDs[ws.AgentID] = true
	}
	assert.True(t, agentIDs["agent-1"])
	assert.True(t, agentIDs["agent-2"])
}

func TestWorkspaceManager_ListWorkspaces_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config := WorkspaceConfig{BaseDir: tmpDir}
	wm := NewWorkspaceManager(zap.NewNop(), config)

	ctx := context.Background()
	workspaces, err := wm.ListWorkspaces(ctx)
	require.NoError(t, err)
	assert.Len(t, workspaces, 0)
}

func TestWorkspaceManager_ListWorkspaces_NonexistentDir(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "nonexistent-workspace-dir")

	config := WorkspaceConfig{BaseDir: tmpDir}
	wm := NewWorkspaceManager(zap.NewNop(), config)

	ctx := context.Background()
	workspaces, err := wm.ListWorkspaces(ctx)
	require.NoError(t, err)
	assert.Len(t, workspaces, 0)
}

func TestWorkspaceManager_DefaultConfig(t *testing.T) {
	config := DefaultWorkspaceConfig()

	assert.Equal(t, "./data/workspace", config.BaseDir)
	assert.Equal(t, "./data/configs/opencode/default-config.json", config.DefaultConfigPath)
	assert.Equal(t, "./data/configs/opencode/default-mcp.json", config.DefaultMCPPath)
	assert.Equal(t, int64(1024), config.MaxDiskUsageMB)
}

func TestWorkspaceManager_WithCustomConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	customConfig := WorkspaceConfig{
		BaseDir:        tmpDir,
		MaxDiskUsageMB: 2048,
	}

	wm := NewWorkspaceManager(zap.NewNop(), customConfig)

	ctx := context.Background()
	ws, err := wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)

	assert.Contains(t, ws.BasePath, tmpDir)
}

func TestWorkspaceManager_CreateDefaultConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config := WorkspaceConfig{
		BaseDir: tmpDir,
		// 템플릿 경로 설정 안 함 (기본 설정 생성)
	}
	wm := NewWorkspaceManager(zap.NewNop(), config)

	ctx := context.Background()
	ws, err := wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)

	// config.json 생성 확인
	assert.FileExists(t, ws.ConfigPath)
	data, err := os.ReadFile(ws.ConfigPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "claude-sonnet-4-20250514")

	// mcp.json 생성 확인
	assert.FileExists(t, ws.MCPConfigPath)
	data, err = os.ReadFile(ws.MCPConfigPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "mcpServers")
}

func TestWorkspaceManager_PathsCorrect(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config := WorkspaceConfig{BaseDir: tmpDir}
	wm := NewWorkspaceManager(zap.NewNop(), config)

	ctx := context.Background()
	ws, err := wm.CreateWorkspace(ctx, "test-agent")
	require.NoError(t, err)

	// 경로 검증
	expectedBase := filepath.Join(tmpDir, "test-agent")
	assert.Equal(t, expectedBase, ws.BasePath)
	assert.Equal(t, filepath.Join(expectedBase, ".opencode"), ws.OpenCodeDir)
	assert.Equal(t, filepath.Join(expectedBase, "project"), ws.ProjectDir)
	assert.Equal(t, filepath.Join(expectedBase, "logs"), ws.LogDir)
	assert.Equal(t, filepath.Join(expectedBase, ".opencode", "config.json"), ws.ConfigPath)
	assert.Equal(t, filepath.Join(expectedBase, ".opencode", "mcp.json"), ws.MCPConfigPath)
}

func Test_getEnvOrDefault(t *testing.T) {
	// 환경 변수가 없을 때
	val := getEnvOrDefault("NONEXISTENT_VAR", "default")
	assert.Equal(t, "default", val)

	// 환경 변수가 있을 때
	_ = os.Setenv("TEST_VAR", "custom")
	defer os.Unsetenv("TEST_VAR")

	val = getEnvOrDefault("TEST_VAR", "default")
	assert.Equal(t, "custom", val)
}

func Test_getEnvOrDefaultInt64(t *testing.T) {
	// 환경 변수가 없을 때
	val := getEnvOrDefaultInt64("NONEXISTENT_VAR", 100)
	assert.Equal(t, int64(100), val)

	// 환경 변수가 있을 때
	_ = os.Setenv("TEST_INT_VAR", "200")
	defer os.Unsetenv("TEST_INT_VAR")

	val = getEnvOrDefaultInt64("TEST_INT_VAR", 100)
	assert.Equal(t, int64(200), val)

	// 유효하지 않은 값일 때
	_ = os.Setenv("TEST_INVALID_VAR", "invalid")
	defer os.Unsetenv("TEST_INVALID_VAR")

	val = getEnvOrDefaultInt64("TEST_INVALID_VAR", 100)
	assert.Equal(t, int64(100), val)
}
