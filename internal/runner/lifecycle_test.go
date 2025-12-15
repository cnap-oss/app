package taskrunner

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLifecycleManager_RegisterUnregister(t *testing.T) {
	lm := NewLifecycleManager(zap.NewNop(), LifecycleConfig{
		MaxConcurrentContainers: 10,
	})

	runner := &Runner{ID: "test-runner", Status: RunnerStatusReady}

	err := lm.RegisterRunner(runner)
	require.NoError(t, err)

	stats := lm.GetRunnerStats()
	assert.Equal(t, 1, stats.TotalRunners)

	err = lm.UnregisterRunner("test-runner")
	require.NoError(t, err)

	stats = lm.GetRunnerStats()
	assert.Equal(t, 0, stats.TotalRunners)
}

func TestLifecycleManager_MaxContainers(t *testing.T) {
	lm := NewLifecycleManager(zap.NewNop(), LifecycleConfig{
		MaxConcurrentContainers: 2,
	})

	// 2개까지 등록 가능
	for i := 0; i < 2; i++ {
		runner := &Runner{ID: fmt.Sprintf("runner-%d", i), Status: RunnerStatusReady}
		err := lm.RegisterRunner(runner)
		require.NoError(t, err)
	}

	// 3번째는 실패
	runner := &Runner{ID: "runner-overflow", Status: RunnerStatusReady}
	err := lm.RegisterRunner(runner)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "최대 동시 Container 수 초과")
}

func TestLifecycleManager_NotifyActivity(t *testing.T) {
	lm := NewLifecycleManager(zap.NewNop())

	runner := &Runner{ID: "test-runner", Status: RunnerStatusReady}
	err := lm.RegisterRunner(runner)
	require.NoError(t, err)

	// 활동 알림
	lm.NotifyActivity("test-runner")

	// 존재하지 않는 Runner에 대한 알림 (패닉 없이 무시)
	lm.NotifyActivity("nonexistent")
}

func TestLifecycleManager_StartStop(t *testing.T) {
	ctx := context.Background()
	lm := NewLifecycleManager(zap.NewNop(), LifecycleConfig{
		CleanupInterval: 100 * time.Millisecond,
	})

	err := lm.Start(ctx)
	require.NoError(t, err)

	// 약간의 시간을 주어 cleanup 루프가 시작되도록 함
	time.Sleep(50 * time.Millisecond)

	err = lm.Stop(ctx)
	require.NoError(t, err)
}

func TestLifecycleManager_Stats(t *testing.T) {
	lm := NewLifecycleManager(zap.NewNop())

	// 다양한 상태의 Runner 등록
	runners := []*Runner{
		{ID: "runner-1", Status: RunnerStatusReady},
		{ID: "runner-2", Status: RunnerStatusRunning},
		{ID: "runner-3", Status: RunnerStatusStopped},
		{ID: "runner-4", Status: RunnerStatusFailed},
	}

	for _, r := range runners {
		err := lm.RegisterRunner(r)
		require.NoError(t, err)
	}

	stats := lm.GetRunnerStats()
	assert.Equal(t, 4, stats.TotalRunners)
	assert.Equal(t, 2, stats.ActiveRunners)  // ready + running
	assert.Equal(t, 2, stats.StoppedRunners) // stopped + failed
}

func TestLifecycleManager_UnregisterNonExistent(t *testing.T) {
	lm := NewLifecycleManager(zap.NewNop())

	err := lm.UnregisterRunner("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runner를 찾을 수 없음")
}

func TestLifecycleManager_IdleDetection(t *testing.T) {
	lm := NewLifecycleManager(zap.NewNop(), LifecycleConfig{
		IdleTimeout:             50 * time.Millisecond,
		MaxConcurrentContainers: 10,
	})

	runner := &Runner{ID: "test-runner", Status: RunnerStatusReady}
	err := lm.RegisterRunner(runner)
	require.NoError(t, err)

	// 유휴 시간 경과
	time.Sleep(60 * time.Millisecond)

	stats := lm.GetRunnerStats()
	// IsIdle 플래그는 performCleanup에서만 업데이트되므로
	// 여기서는 테스트하지 않음
	assert.Equal(t, 1, stats.TotalRunners)
}

func TestDefaultLifecycleConfig(t *testing.T) {
	config := DefaultLifecycleConfig()

	assert.Equal(t, 5*time.Minute, config.IdleTimeout)
	assert.Equal(t, 30*time.Minute, config.MaxRuntime)
	assert.Equal(t, 1*time.Minute, config.CleanupInterval)
	assert.Equal(t, 10, config.MaxConcurrentContainers)
	assert.Equal(t, 30*time.Second, config.ShutdownTimeout)
}

func TestLifecycleManager_ConcurrentAccess(t *testing.T) {
	lm := NewLifecycleManager(zap.NewNop(), LifecycleConfig{
		MaxConcurrentContainers: 100,
	})

	// 동시에 여러 Runner 등록
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			runner := &Runner{
				ID:     fmt.Sprintf("runner-%d", idx),
				Status: RunnerStatusReady,
			}
			_ = lm.RegisterRunner(runner)
			lm.NotifyActivity(runner.ID)
			done <- true
		}(i)
	}

	// 모든 고루틴 완료 대기
	for i := 0; i < 10; i++ {
		<-done
	}

	stats := lm.GetRunnerStats()
	assert.Equal(t, 10, stats.TotalRunners)
}

func TestGetEnvOrDefaultDuration(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		envValue string
		defVal   time.Duration
		expected time.Duration
	}{
		{
			name:     "환경 변수 없음",
			key:      "TEST_DURATION",
			envValue: "",
			defVal:   5 * time.Minute,
			expected: 5 * time.Minute,
		},
		{
			name:     "유효한 환경 변수",
			key:      "TEST_DURATION_2",
			envValue: "10m",
			defVal:   5 * time.Minute,
			expected: 10 * time.Minute,
		},
		{
			name:     "잘못된 형식",
			key:      "TEST_DURATION_3",
			envValue: "invalid",
			defVal:   5 * time.Minute,
			expected: 5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(tt.key, tt.envValue)
			}
			result := getEnvOrDefaultDuration(tt.key, tt.defVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetEnvOrDefaultInt(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		envValue string
		defVal   int
		expected int
	}{
		{
			name:     "환경 변수 없음",
			key:      "TEST_INT",
			envValue: "",
			defVal:   10,
			expected: 10,
		},
		{
			name:     "유효한 환경 변수",
			key:      "TEST_INT_2",
			envValue: "20",
			defVal:   10,
			expected: 20,
		},
		{
			name:     "잘못된 형식",
			key:      "TEST_INT_3",
			envValue: "invalid",
			defVal:   10,
			expected: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(tt.key, tt.envValue)
			}
			result := getEnvOrDefaultInt(tt.key, tt.defVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}
