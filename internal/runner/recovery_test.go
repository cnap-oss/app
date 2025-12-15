package taskrunner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDefaultRecoveryConfig(t *testing.T) {
	config := DefaultRecoveryConfig()

	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 1*time.Second, config.InitialBackoff)
	assert.Equal(t, 30*time.Second, config.MaxBackoff)
	assert.Equal(t, 2.0, config.BackoffFactor)
}

func TestRecoveryManager_RetryOperation_Success(t *testing.T) {
	rm := NewRecoveryManager(zap.NewNop())
	ctx := context.Background()

	called := 0
	err := rm.RetryOperation(ctx, "test-op", func() error {
		called++
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, called)
}

func TestRecoveryManager_RetryOperation_SuccessAfterRetry(t *testing.T) {
	rm := NewRecoveryManager(zap.NewNop(), RecoveryConfig{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		BackoffFactor:  2.0,
	})
	ctx := context.Background()

	called := 0
	err := rm.RetryOperation(ctx, "test-op", func() error {
		called++
		if called < 3 {
			return ErrAPITimeout // 재시도 가능한 에러
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 3, called)
}

func TestRecoveryManager_RetryOperation_MaxRetriesExceeded(t *testing.T) {
	rm := NewRecoveryManager(zap.NewNop(), RecoveryConfig{
		MaxRetries:     2,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		BackoffFactor:  2.0,
	})
	ctx := context.Background()

	called := 0
	err := rm.RetryOperation(ctx, "test-op", func() error {
		called++
		return ErrAPITimeout // 항상 실패
	})

	require.Error(t, err)
	assert.Equal(t, ErrAPITimeout, err)
	assert.Equal(t, 3, called) // 초기 시도 + 2번 재시도
}

func TestRecoveryManager_RetryOperation_NonRetryableError(t *testing.T) {
	rm := NewRecoveryManager(zap.NewNop())
	ctx := context.Background()

	called := 0
	nonRetryableErr := errors.New("non-retryable")
	err := rm.RetryOperation(ctx, "test-op", func() error {
		called++
		return nonRetryableErr
	})

	require.Error(t, err)
	assert.Equal(t, nonRetryableErr, err)
	assert.Equal(t, 1, called) // 재시도 없이 즉시 반환
}

func TestRecoveryManager_RetryOperation_ContextCanceled(t *testing.T) {
	rm := NewRecoveryManager(zap.NewNop(), RecoveryConfig{
		MaxRetries:     3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		BackoffFactor:  2.0,
	})

	ctx, cancel := context.WithCancel(context.Background())

	called := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := rm.RetryOperation(ctx, "test-op", func() error {
		called++
		return ErrAPITimeout // 재시도 가능한 에러
	})

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.True(t, called >= 1) // 최소 1번 호출
}

func TestRecoveryManager_BackoffProgression(t *testing.T) {
	rm := NewRecoveryManager(zap.NewNop(), RecoveryConfig{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
		BackoffFactor:  2.0,
	})
	ctx := context.Background()

	start := time.Now()
	called := 0

	err := rm.RetryOperation(ctx, "test-op", func() error {
		called++
		return ErrAPITimeout
	})

	duration := time.Since(start)

	require.Error(t, err)
	assert.Equal(t, 4, called) // 초기 + 3번 재시도

	// 백오프: 10ms + 20ms + 40ms = 70ms
	// MaxBackoff로 제한: 10ms + 20ms + 50ms = 80ms (근사)
	assert.True(t, duration >= 70*time.Millisecond, "백오프 시간이 너무 짧음")
	assert.True(t, duration < 200*time.Millisecond, "백오프 시간이 너무 김")
}

func TestRecoveryManager_RetryOperation_DifferentErrors(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		shouldRetry   bool
		expectedCalls int
	}{
		{
			name:          "APITimeout - should retry",
			err:           ErrAPITimeout,
			shouldRetry:   true,
			expectedCalls: 4, // 1 + 3 retries
		},
		{
			name:          "APIConnectionFailed - should retry",
			err:           ErrAPIConnectionFailed,
			shouldRetry:   true,
			expectedCalls: 4,
		},
		{
			name:          "ContainerStartFailed - should retry",
			err:           ErrContainerStartFailed,
			shouldRetry:   true,
			expectedCalls: 4,
		},
		{
			name:          "ContainerNotFound - should not retry",
			err:           ErrContainerNotFound,
			shouldRetry:   false,
			expectedCalls: 1,
		},
		{
			name:          "RunnerNotReady - should not retry",
			err:           ErrRunnerNotReady,
			shouldRetry:   false,
			expectedCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm := NewRecoveryManager(zap.NewNop(), RecoveryConfig{
				MaxRetries:     3,
				InitialBackoff: 1 * time.Millisecond,
				MaxBackoff:     10 * time.Millisecond,
				BackoffFactor:  2.0,
			})
			ctx := context.Background()

			called := 0
			err := rm.RetryOperation(ctx, "test-op", func() error {
				called++
				return tt.err
			})

			require.Error(t, err)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.expectedCalls, called)
		})
	}
}

func TestNewRecoveryManager_WithConfig(t *testing.T) {
	customConfig := RecoveryConfig{
		MaxRetries:     5,
		InitialBackoff: 2 * time.Second,
		MaxBackoff:     60 * time.Second,
		BackoffFactor:  3.0,
	}

	rm := NewRecoveryManager(zap.NewNop(), customConfig)

	assert.Equal(t, customConfig.MaxRetries, rm.config.MaxRetries)
	assert.Equal(t, customConfig.InitialBackoff, rm.config.InitialBackoff)
	assert.Equal(t, customConfig.MaxBackoff, rm.config.MaxBackoff)
	assert.Equal(t, customConfig.BackoffFactor, rm.config.BackoffFactor)
}

func TestNewRecoveryManager_WithoutConfig(t *testing.T) {
	rm := NewRecoveryManager(zap.NewNop())

	defaultConfig := DefaultRecoveryConfig()
	assert.Equal(t, defaultConfig.MaxRetries, rm.config.MaxRetries)
	assert.Equal(t, defaultConfig.InitialBackoff, rm.config.InitialBackoff)
	assert.Equal(t, defaultConfig.MaxBackoff, rm.config.MaxBackoff)
	assert.Equal(t, defaultConfig.BackoffFactor, rm.config.BackoffFactor)
}

func TestNewRecoveryManager_NilLogger(t *testing.T) {
	rm := NewRecoveryManager(nil)

	assert.NotNil(t, rm.logger)
}
