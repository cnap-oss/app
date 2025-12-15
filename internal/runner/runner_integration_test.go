package taskrunner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func init() {
	// 프로젝트 루트의 .env 파일 로드
	// 테스트 파일은 internal/runner에 있으므로 두 단계 위로 올라감
	rootDir := filepath.Join("..", "..")
	envPath := filepath.Join(rootDir, ".env")

	// .env 파일이 없어도 에러를 무시 (CI 환경 등에서는 환경 변수로 직접 설정할 수 있음)
	_ = godotenv.Load(envPath)
}

// 실제 OpenCode API와 통신하는 통합 테스트입니다.
// - Docker가 실행 중이어야 합니다.
// - cnap-runner 이미지가 빌드되어 있어야 합니다.
// - OPENCODE_API_KEY 또는 OPEN_CODE_API_KEY가 설정되어 있어야 합니다.
func TestRunner_RealAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping real API call")
	}

	// API 키 확인 (OPENCODE_API_KEY 또는 레거시 OPEN_CODE_API_KEY)
	apiKey := os.Getenv("OPENCODE_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPEN_CODE_API_KEY")
	}
	if apiKey == "" {
		t.Skip("OPENCODE_API_KEY or OPEN_CODE_API_KEY not set; skipping real API call")
	}

	// Runner 생성
	runner, err := NewRunner(
		"integration-test",
		AgentInfo{
			AgentID: "test-agent",
			Model:   "grok-code",
		},
		zap.NewExample(),
	)
	require.NoError(t, err)

	// Container 시작
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	err = runner.Start(ctx)
	require.NoError(t, err)

	// 테스트 종료 시 Container 정리
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_ = runner.Stop(cleanupCtx)
	}()

	// AI API 호출
	res, err := runner.Run(ctx, &RunRequest{
		TaskID: "integration-test",
		Model:  "grok-code",
		Messages: []ChatMessage{
			{Role: "user", Content: "AI란 무엇인가?"},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Success)
	require.NotEmpty(t, res.Output)
	
	fmt.Printf("agent=%s name=%s success=%v output=%q\n", res.Agent, res.Name, res.Success, res.Output)
}
