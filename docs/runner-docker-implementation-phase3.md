# Runner Docker 구현 명세서 - Phase 3: 작업 공간 및 설정 관리

본 문서는 Runner Docker 구현의 Phase 3 단계에 대한 세부 구현 명세서입니다.

---

## 목차

1. [개요](#1-개요)
2. [FR-006: 작업 공간 관리](#2-fr-006-작업-공간-관리)
3. [FR-007: Container 설정 주입](#3-fr-007-container-설정-주입)
4. [구현 체크리스트](#4-구현-체크리스트)

---

## 1. 개요

### 1.1 Phase 3 목표

Phase 3의 목표는 Agent별 작업 공간과 Container 설정 관리를 구현하는 것입니다:

- Agent별 작업 공간 디렉토리 관리
- 작업 공간과 Container 간 볼륨 마운트
- MCP 설정 및 환경 변수 주입

### 1.2 의존성 관계

```mermaid
graph LR
    FR005[FR-005: 인터페이스 호환성] --> FR006[FR-006: Workspace 관리]
    FR006 --> FR007[FR-007: Container 설정 주입]
```

### 1.3 예상 파일 변경

| 작업 유형 | 파일 경로                           | 설명                      |
| --------- | ----------------------------------- | ------------------------- |
| 신규      | `internal/runner/workspace.go`      | WorkspaceManager 구현     |
| 신규      | `internal/runner/workspace_test.go` | Workspace 테스트          |
| 신규      | `internal/runner/settings.go`       | Container 설정 관리       |
| 수정      | `internal/runner/runner.go`         | Workspace 연동            |
| 수정      | `internal/runner/config.go`         | 설정 항목 추가            |
| 신규      | `configs/opencode/`                 | OpenCode 기본 설정 템플릿 |

---

## 2. FR-006: 작업 공간 관리

### 2.1 요구사항 요약

각 Agent는 Container에 마운트할 작업 공간을 가집니다. 작업 공간은 `./data/workspace` 아래에 생성됩니다.

### 2.2 디렉토리 구조 설계

```
./data/
├── workspace/                      # 작업 공간 루트
│   ├── {agent_id_1}/              # Agent별 디렉토리
│   │   ├── .opencode/             # OpenCode 설정 디렉토리
│   │   │   ├── config.json        # OpenCode 설정
│   │   │   ├── mcp.json           # MCP 서버 설정
│   │   │   └── sessions/          # 세션 데이터
│   │   ├── project/               # 실제 작업 파일
│   │   │   └── ...
│   │   └── logs/                  # 로컬 로그
│   │       └── opencode.log
│   └── {agent_id_2}/
│       └── ...
├── messages/                       # 기존 메시지 저장 경로
│   └── ...
└── configs/                        # 공유 설정 템플릿
    └── opencode/
        ├── default-config.json
        └── default-mcp.json
```

### 2.3 WorkspaceManager 인터페이스 및 구현

```go
// internal/runner/workspace.go

package taskrunner

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sync"

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
    AgentID        string `json:"agent_id"`
    BasePath       string `json:"base_path"`        // 전체 경로 (./data/workspace/{agent_id})
    OpenCodeDir    string `json:"opencode_dir"`     // .opencode 디렉토리 경로
    ProjectDir     string `json:"project_dir"`      // project 디렉토리 경로
    LogDir         string `json:"log_dir"`          // logs 디렉토리 경로
    ConfigPath     string `json:"config_path"`      // config.json 경로
    MCPConfigPath  string `json:"mcp_config_path"`  // mcp.json 경로
}

// WorkspaceConfig는 WorkspaceManager 설정입니다.
type WorkspaceConfig struct {
    BaseDir            string // 기본 디렉토리 (기본: ./data/workspace)
    DefaultConfigPath  string // 기본 OpenCode 설정 템플릿 경로
    DefaultMCPPath     string // 기본 MCP 설정 템플릿 경로
    MaxDiskUsageMB     int64  // 최대 디스크 사용량 (MB, 기본: 1024)
}

// DefaultWorkspaceConfig는 기본 Workspace 설정을 반환합니다.
func DefaultWorkspaceConfig() WorkspaceConfig {
    return WorkspaceConfig{
        BaseDir:           getEnvOrDefault("WORKSPACE_BASE_DIR", "./data/workspace"),
        DefaultConfigPath: getEnvOrDefault("WORKSPACE_DEFAULT_CONFIG", "./data/configs/opencode/default-config.json"),
        DefaultMCPPath:    getEnvOrDefault("WORKSPACE_DEFAULT_MCP", "./data/configs/opencode/default-mcp.json"),
        MaxDiskUsageMB:    getEnvOrDefaultInt64("WORKSPACE_MAX_DISK_MB", 1024),
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

// 인터페이스 구현 확인
var _ WorkspaceManager = (*workspaceManager)(nil)
```

### 2.4 Runner와 Workspace 연동

```go
// internal/runner/runner.go (수정)

// Runner 생성 시 Workspace 연동
func NewRunner(taskID string, agentInfo AgentInfo, logger *zap.Logger, opts ...RunnerOption) (*Runner, error) {
    // ... 기존 코드 ...

    // 작업 공간 경로 설정
    if agentInfo.WorkspacePath != "" {
        r.WorkspacePath = agentInfo.WorkspacePath
    } else {
        r.WorkspacePath = fmt.Sprintf("%s/%s", r.config.WorkspaceBaseDir, agentInfo.AgentID)
    }

    return r, nil
}

// Start 메서드에서 볼륨 마운트 설정
func (r *Runner) Start(ctx context.Context) error {
    // ... 기존 코드 ...

    // Container 설정에 볼륨 마운트 추가
    mounts := []MountConfig{
        {
            Source:   r.WorkspacePath,
            Target:   "/workspace",
            ReadOnly: false,
        },
    }

    // OpenCode 설정 디렉토리도 마운트
    openCodeDir := filepath.Join(r.WorkspacePath, ".opencode")
    if _, err := os.Stat(openCodeDir); err == nil {
        mounts = append(mounts, MountConfig{
            Source:   openCodeDir,
            Target:   "/home/opencode/.opencode",
            ReadOnly: false,
        })
    }

    // ... Container 생성 시 mounts 전달 ...
}
```

### 2.5 커밋 포인트

```
feat(runner): FR-006 Workspace 관리 구현

- WorkspaceManager 인터페이스 정의
- Agent별 작업 공간 생성/삭제/조회
- 디렉토리 구조 자동 생성
- 기본 설정 파일 초기화
- Runner와 Workspace 연동

Refs: FR-006
```

---

## 3. FR-007: Container 설정 주입

### 3.1 요구사항 요약

Container에 설정을 주입하여 MCP 등 다양한 도구 설정이 가능해야 합니다.

### 3.2 설정 구조 설계

#### 3.2.1 OpenCode 설정 (config.json)

```json
{
  "model": "claude-sonnet-4-20250514",
  "provider": "anthropic",
  "theme": "dark",
  "timeout": 120,
  "features": {
    "codeExecution": true,
    "fileOperations": true,
    "webBrowsing": false
  },
  "limits": {
    "maxTokens": 4096,
    "maxTurns": 50
  }
}
```

#### 3.2.2 MCP 설정 (mcp.json)

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "mcp-server-filesystem",
      "args": ["/workspace/project"],
      "env": {}
    },
    "git": {
      "command": "mcp-server-git",
      "args": [],
      "env": {}
    },
    "custom": {
      "command": "custom-mcp-server",
      "args": ["--config", "/workspace/.opencode/custom-mcp.json"],
      "env": {
        "API_KEY": "${CUSTOM_API_KEY}"
      }
    }
  }
}
```

### 3.3 설정 관리자 구현

```go
// internal/runner/settings.go

package taskrunner

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "regexp"
    "strings"

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
    env = append(env, fmt.Sprintf("OPENCODE_DATA_DIR=/home/opencode/.opencode"))
    env = append(env, fmt.Sprintf("OPENCODE_WORKSPACE=/workspace"))

    // 모델 설정
    if config.Model != "" {
        env = append(env, fmt.Sprintf("OPENCODE_MODEL=%s", config.Model))
    }
    if config.Provider != "" {
        env = append(env, fmt.Sprintf("OPENCODE_PROVIDER=%s", config.Provider))
    }

    // API 키 전달 (호스트 환경 변수에서)
    apiKeyEnvs := []string{
        "OPENCODE_API_KEY",
        "ANTHROPIC_API_KEY",
        "OPENAI_API_KEY",
        "GOOGLE_API_KEY",
        "AZURE_OPENAI_API_KEY",
    }
    for _, key := range apiKeyEnvs {
        if val := os.Getenv(key); val != "" {
            env = append(env, fmt.Sprintf("%s=%s", key, val))
        }
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
```

### 3.4 Runner에 SettingsManager 통합

```go
// internal/runner/runner.go (수정)

// Runner 구조체에 SettingsManager 추가
type Runner struct {
    // ... 기존 필드 ...

    settingsManager SettingsManager  // 신규: 설정 관리자
}

// buildEnvironmentVariables 수정
func (r *Runner) buildEnvironmentVariables() []string {
    if r.settingsManager != nil {
        env, err := r.settingsManager.BuildContainerEnv(r.AgentInfo.AgentID, nil)
        if err != nil {
            r.logger.Warn("환경 변수 빌드 실패, 기본값 사용",
                zap.Error(err),
            )
        } else {
            return env
        }
    }

    // 기존 기본 환경 변수 반환
    env := []string{
        fmt.Sprintf("OPENCODE_MODEL=%s", r.AgentInfo.Model),
    }

    // API 키 전달
    if apiKey := os.Getenv("OPENCODE_API_KEY"); apiKey != "" {
        env = append(env, fmt.Sprintf("OPENCODE_API_KEY=%s", apiKey))
    }
    // ... 기존 코드 ...

    return env
}
```

### 3.5 기본 설정 템플릿

```json
// data/configs/opencode/default-config.json
{
  "model": "claude-sonnet-4-20250514",
  "provider": "anthropic",
  "theme": "dark",
  "timeout": 120,
  "features": {
    "codeExecution": true,
    "fileOperations": true,
    "webBrowsing": false
  },
  "limits": {
    "maxTokens": 4096,
    "maxTurns": 50
  }
}
```

```json
// data/configs/opencode/default-mcp.json
{
  "mcpServers": {
    "filesystem": {
      "command": "mcp-server-filesystem",
      "args": ["/workspace/project"],
      "env": {},
      "enabled": true
    }
  }
}
```

### 3.6 커밋 포인트

```
feat(runner): FR-007 Container 설정 주입 구현

- SettingsManager 인터페이스 정의
- OpenCode 및 MCP 설정 관리
- 환경 변수 빌드 및 주입
- ${VAR} 형태의 환경 변수 참조 치환
- 기본 설정 템플릿 추가

Refs: FR-007
```

---

## 4. 구현 체크리스트

### 4.1 Phase 3 구현 순서

| 순서 | 작업                    | 파일                                | 커밋 메시지                                  |
| ---- | ----------------------- | ----------------------------------- | -------------------------------------------- |
| 1    | 기본 설정 디렉토리 구조 | `data/configs/opencode/`            | `chore: 기본 설정 템플릿 디렉토리 추가`      |
| 2    | WorkspaceManager 구현   | `internal/runner/workspace.go`      | `feat(runner): FR-006 WorkspaceManager 구현` |
| 3    | Workspace 테스트        | `internal/runner/workspace_test.go` | `test(runner): Workspace 단위 테스트`        |
| 4    | SettingsManager 구현    | `internal/runner/settings.go`       | `feat(runner): FR-007 SettingsManager 구현`  |
| 5    | Settings 테스트         | `internal/runner/settings_test.go`  | `test(runner): Settings 단위 테스트`         |
| 6    | Runner 연동             | `internal/runner/runner.go`         | `refactor(runner): Workspace/Settings 연동`  |
| 7    | Config 업데이트         | `internal/runner/config.go`         | `feat(runner): Workspace/Settings 설정 추가` |

### 4.2 테스트 전략

#### 4.2.1 WorkspaceManager 테스트

```go
// internal/runner/workspace_test.go

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

    ws, err := wm.GetWorkspace("test-agent")
    require.NoError(t, err)
    assert.Equal(t, "test-agent", ws.AgentID)
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

    err = wm.DeleteWorkspace(ctx, "test-agent", true)
    require.NoError(t, err)
    assert.NoDirExists(t, ws.BasePath)
}

func TestWorkspaceManager_ListWorkspaces(t *testing.T) {
    tmpDir, err := os.MkdirTemp("", "workspace-test-*")
    require.NoError(t, err)
    defer os.RemoveAll(tmpDir)

    config := WorkspaceConfig{BaseDir: tmpDir}
    wm := NewWorkspaceManager(zap.NewNop(), config)

    ctx := context.Background()
    _, err = wm.CreateWorkspace(ctx, "agent-1")
    require.NoError(t, err)
    _, err = wm.CreateWorkspace(ctx, "agent-2")
    require.NoError(t, err)

    workspaces, err := wm.ListWorkspaces(ctx)
    require.NoError(t, err)
    assert.Len(t, workspaces, 2)
}
```

#### 4.2.2 SettingsManager 테스트

```go
// internal/runner/settings_test.go

package taskrunner

import (
    "context"
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

    // MCP 서버 제거
    err = sm.RemoveMCPServer("test-agent", "git")
    require.NoError(t, err)

    config, err = sm.GetMCPConfig("test-agent")
    require.NoError(t, err)
    assert.NotContains(t, config.MCPServers, "git")
}

func TestSettingsManager_BuildContainerEnv(t *testing.T) {
    tmpDir, err := os.MkdirTemp("", "settings-test-*")
    require.NoError(t, err)
    defer os.RemoveAll(tmpDir)

    // 환경 변수 설정
    os.Setenv("ANTHROPIC_API_KEY", "test-key")
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

    // 환경 변수 확인
    assert.Contains(t, env, "ANTHROPIC_API_KEY=test-key")
    assert.Contains(t, env, "CUSTOM_VAR=custom-value")
}

func TestSettingsManager_ResolveEnvVars(t *testing.T) {
    os.Setenv("TEST_VAR", "resolved-value")
    defer os.Unsetenv("TEST_VAR")

    sm := &settingsManager{logger: zap.NewNop()}

    // 치환 테스트
    result := sm.resolveEnvVars("prefix_${TEST_VAR}_suffix")
    assert.Equal(t, "prefix_resolved-value_suffix", result)

    // 없는 변수는 원본 유지
    result = sm.resolveEnvVars("${NONEXISTENT}")
    assert.Equal(t, "${NONEXISTENT}", result)
}
```

### 4.3 예상 파일 구조

```
internal/runner/
├── api.go             # Phase 2
├── api_types.go       # Phase 2
├── api_test.go        # Phase 2
├── config.go          # Phase 1 + 수정
├── docker.go          # Phase 1
├── docker_test.go     # Phase 1
├── manager.go         # Phase 1 + 수정
├── manager_test.go    # Phase 1 + 수정
├── runner.go          # Phase 1 + 수정
├── runner_test.go     # Phase 1 + 수정
├── settings.go        # 신규: Phase 3
├── settings_test.go   # 신규: Phase 3
├── workspace.go       # 신규: Phase 3
├── workspace_test.go  # 신규: Phase 3
└── runner_integration_test.go

data/
├── workspace/         # 런타임에 생성됨
└── configs/
    └── opencode/
        ├── default-config.json
        └── default-mcp.json
```

---

## 다음 단계

Phase 3 완료 후 [Phase 4: 안정화 및 운영](./runner-docker-implementation-phase4.md)으로 진행합니다.

- FR-008: Container 수명 관리
- FR-009: 에러 처리 및 복구
- FR-010: 로깅 및 모니터링
