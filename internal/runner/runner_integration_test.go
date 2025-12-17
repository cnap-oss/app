package taskrunner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cnap-oss/app/internal/runner/opencode"
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

// mockCallback은 테스트용 콜백 구현입니다.
type mockCallback struct {
	mu               sync.Mutex
	sessionID        string
	messages         []string
	completeResult   *RunResult
	error            error
	onStartedCalled  int
	onEventCalled    int
	onCompleteCalled int
	onErrorCalled    int
	t                *testing.T // 테스트 로깅용
}

func newMockCallback(t *testing.T) *mockCallback {
	return &mockCallback{t: t}
}

func (m *mockCallback) OnStarted(taskID string, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionID = sessionID
	m.onStartedCalled++
	m.t.Logf("[Callback] OnStarted: taskID=%s, sessionID=%s", taskID, sessionID)
	return nil
}

func (m *mockCallback) OnEvent(taskID string, event *opencode.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// text 파트에서 텍스트 추출
	if event.Type == "message.part.updated" {
		if props, ok := event.Properties["part"].(map[string]interface{}); ok {
			if partType, _ := props["type"].(string); partType == "text" {
				if text, ok := props["text"].(string); ok {
					m.messages = append(m.messages, text)
				}
			}
		}
	}

	m.onEventCalled++
	m.t.Logf("[Callback] OnEvent #%d: taskID=%s, eventType=%s",
		m.onEventCalled, taskID, event.Type)
	return nil
}

func (m *mockCallback) OnComplete(taskID string, result *RunResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completeResult = result
	m.onCompleteCalled++
	m.t.Logf("[Callback] OnComplete: taskID=%s, success=%v, output_length=%d", taskID, result.Success, len(result.Output))
	return nil
}

func (m *mockCallback) OnError(taskID string, err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.error = err
	m.onErrorCalled++
	m.t.Logf("[Callback] OnError: taskID=%s, error=%v", taskID, err)
	return nil
}

func (m *mockCallback) GetMessages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.messages...)
}

func (m *mockCallback) GetCompleteResult() *RunResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.completeResult
}

// 실제 OpenCode API와 통신하는 통합 테스트입니다.
// - Docker가 실행 중이어야 합니다.
// - cnap-runner 이미지가 빌드되어 있어야 합니다.
// - OPENCODE_API_KEY 또는 OPENCODE_API_KEY가 설정되어 있어야 합니다.
func TestRunner_RealAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping real API call")
	}

	// API 키 확인 (OPENCODE_API_KEY 또는 레거시 OPENCODE_API_KEY)
	apiKey := os.Getenv("OPENCODE_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENCODE_API_KEY")
	}
	if apiKey == "" {
		t.Skip("OPENCODE_API_KEY or OPENCODE_API_KEY not set; skipping real API call")
	}

	// 콜백 생성
	callback := newMockCallback(t)

	// Runner 생성
	runner, err := NewRunner(
		"integration-test",
		AgentInfo{
			AgentID: "test-agent",
			Model:   "grok-code",
		},
		callback,
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

	// AI API 호출 (비동기 모드)
	err = runner.Run(ctx, &RunRequest{
		TaskID: "integration-test",
		Model:  "grok-code",
		Messages: []opencode.ChatMessage{
			{Role: "user", Content: "AI란 무엇인가?"},
		},
	})
	require.NoError(t, err)

	// 완료 대기
	select {
	case <-time.After(120 * time.Second):
		t.Fatal("timeout waiting for completion")
	default:
		time.Sleep(10 * time.Second) // 응답 대기
	}

	res := callback.GetCompleteResult()
	require.NotNil(t, res)
	require.True(t, res.Success)
	require.NotEmpty(t, res.Output)

	fmt.Printf("agent=%s name=%s success=%v output=%q\n", res.Agent, res.Name, res.Success, res.Output)
}

// 스트리밍 모드 콜백 테스트
func TestRunner_RealAPI_WithCallback(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping real API call")
	}

	// API 키 확인
	apiKey := os.Getenv("OPENCODE_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENCODE_API_KEY")
	}
	if apiKey == "" {
		t.Skip("OPENCODE_API_KEY or OPENCODE_API_KEY not set; skipping real API call")
	}

	// 콜백 생성
	callback := newMockCallback(t)

	// Runner 생성
	runner, err := NewRunner(
		"integration-test-callback",
		AgentInfo{
			AgentID: "test-agent",
			Model:   "grok-code",
		},
		callback,
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
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := runner.Stop(cleanupCtx); err != nil {
			t.Logf("Failed to stop runner: %v", err)
		}
	}()

	t.Log("========== Starting AI API Call ==========")

	// AI API 호출 (비동기 모드)
	err = runner.Run(ctx, &RunRequest{
		TaskID: "integration-test-callback",
		Model:  "grok-code",
		Messages: []opencode.ChatMessage{
			{Role: "user", Content: "간단히 'Hello, World!'라고만 답해줘."},
		},
	})
	require.NoError(t, err)

	// 완료 대기
	time.Sleep(30 * time.Second)

	res := callback.GetCompleteResult()
	require.NotNil(t, res)
	require.True(t, res.Success)
	require.NotEmpty(t, res.Output)

	t.Log("========== API Call Completed ==========")

	// 콜백 검증
	messages := callback.GetMessages()
	t.Logf("\n========== Callback Statistics ==========")
	t.Logf("OnEvent called: %d times", callback.onEventCalled)
	t.Logf("OnComplete called: %d times", callback.onCompleteCalled)
	t.Logf("OnError called: %d times", callback.onErrorCalled)
	t.Logf("Messages received count: %d", len(messages))

	// 이벤트가 수신되었는지 확인 (빈 응답 문제 해결 검증)
	require.Greater(t, callback.onEventCalled, 0, "OnEvent should be called at least once")

	// 완료 콜백 확인
	require.Equal(t, 1, callback.onCompleteCalled, "OnComplete should be called exactly once")
	require.NotNil(t, callback.GetCompleteResult())

	// 에러 콜백이 호출되지 않았는지 확인
	require.Equal(t, 0, callback.onErrorCalled, "OnError should not be called")
	require.Nil(t, callback.error)

	// 전체 출력 확인
	fullOutput := strings.Join(messages, "")
	t.Logf("\n========== Output ==========")
	t.Logf("Full output from messages: %s", fullOutput)
	t.Logf("Result output: %s", res.Output)
	t.Logf("Result success: %v", res.Success)

	fmt.Printf("\n========== Final Result ==========\n")
	fmt.Printf("agent=%s name=%s success=%v output=%q\n", res.Agent, res.Name, res.Success, res.Output)
}

// 빈 응답 처리 테스트
func TestRunner_RealAPI_EmptyResponseHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping real API call")
	}

	// API 키 확인
	apiKey := os.Getenv("OPENCODE_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENCODE_API_KEY")
	}
	if apiKey == "" {
		t.Skip("OPENCODE_API_KEY or OPENCODE_API_KEY not set; skipping real API call")
	}

	// 콜백 생성
	callback := newMockCallback(t)

	// Runner 생성
	runner, err := NewRunner(
		"integration-test-empty",
		AgentInfo{
			AgentID: "test-agent",
			Model:   "grok-code",
		},
		callback,
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
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := runner.Stop(cleanupCtx); err != nil {
			t.Logf("Failed to stop runner: %v", err)
		}
	}()

	// AI API 호출 - 여러 번 시도하여 빈 응답 처리 검증
	for i := 0; i < 3; i++ {
		t.Logf("Attempt %d/3", i+1)

		err := runner.Run(ctx, &RunRequest{
			TaskID: fmt.Sprintf("integration-test-empty-%d", i),
			Model:  "grok-code",
			Messages: []opencode.ChatMessage{
				{Role: "user", Content: "안녕하세요"},
			},
		})

		// 빈 응답이어도 에러가 발생하지 않아야 함
		require.NoError(t, err, "Run should not return error")

		// 잠시 대기
		time.Sleep(10 * time.Second)

		res := callback.GetCompleteResult()
		if res != nil {
			require.True(t, res.Success, "Success should be true even with empty response")
			t.Logf("Attempt %d result: output_length=%d", i+1, len(res.Output))
		}
	}

	t.Logf("Total OnEvent calls: %d", callback.onEventCalled)
	t.Logf("Total OnComplete calls: %d", callback.onCompleteCalled)
	t.Logf("Total OnError calls: %d", callback.onErrorCalled)
}
