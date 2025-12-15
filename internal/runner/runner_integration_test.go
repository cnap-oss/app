package taskrunner

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// 실제 OpenCode API와 통신하는 통합 테스트입니다.
// - 네트워크가 가능하고 과금에 동의하는 환경에서만 실행하세요.
// - OPEN_CODE_API_KEY가 설정되어 있지 않으면 스킵합니다.
func TestRunner_RealAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping real API call")
	}

	apiKey := os.Getenv("OPEN_CODE_API_KEY")
	if apiKey == "" {
		t.Skip("OPEN_CODE_API_KEY not set; skipping real API call")
	}

	runner, err := NewRunner(
		"integration-test",
		AgentInfo{
			AgentID: "test-agent",
			Model:   "grok-code",
		},
		zap.NewExample(),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	res, err := runner.Run(ctx, &RunRequest{
		TaskID: "integration-test",
		Model:  "grok-code",
		Messages: []ChatMessage{
			{Role: "user", Content: "AI란 무엇인가?"},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	fmt.Printf("agent=%s name=%s success=%v output=%q\n", res.Agent, res.Name, res.Success, res.Output)
}
