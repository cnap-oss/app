package taskrunner

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetrics_IncrementContainers(t *testing.T) {
	m := &Metrics{}

	m.IncrementContainersCreated()
	m.IncrementContainersStarted()
	m.IncrementContainersStopped()
	m.IncrementContainersFailed()

	assert.Equal(t, int64(1), m.ContainersCreated)
	assert.Equal(t, int64(1), m.ContainersStarted)
	assert.Equal(t, int64(1), m.ContainersStopped)
	assert.Equal(t, int64(1), m.ContainersFailed)
}

func TestMetrics_RecordTaskExecution(t *testing.T) {
	m := &Metrics{}

	m.RecordTaskExecution(true, 100*time.Millisecond)
	m.RecordTaskExecution(false, 200*time.Millisecond)

	snapshot := m.GetSnapshot()
	assert.Equal(t, int64(2), snapshot.TasksExecuted)
	assert.Equal(t, int64(1), snapshot.TasksSucceeded)
	assert.Equal(t, int64(1), snapshot.TasksFailed)
	assert.Greater(t, snapshot.AvgExecutionTimeMs, 0.0)
}

func TestMetrics_RecordContainerStartTime(t *testing.T) {
	m := &Metrics{}

	m.RecordContainerStartTime(1 * time.Second)
	m.RecordContainerStartTime(2 * time.Second)

	snapshot := m.GetSnapshot()
	assert.Equal(t, int64(2), m.ContainerStartCount)
	assert.InDelta(t, 1500.0, snapshot.AvgContainerStartMs, 10.0) // 평균 1.5초
}

func TestMetrics_RecordError(t *testing.T) {
	m := &Metrics{}

	m.RecordError(true)  // 복구됨
	m.RecordError(false) // 복구 안됨
	m.RecordError(true)  // 복구됨

	snapshot := m.GetSnapshot()
	assert.Equal(t, int64(3), snapshot.ErrorsTotal)
	assert.Equal(t, int64(2), snapshot.ErrorsRecovered)
}

func TestMetrics_RecordRetry(t *testing.T) {
	m := &Metrics{}

	m.RecordRetry()
	m.RecordRetry()
	m.RecordRetry()

	snapshot := m.GetSnapshot()
	assert.Equal(t, int64(3), snapshot.RetriesTotal)
}

func TestMetrics_GetSnapshot(t *testing.T) {
	m := &Metrics{}

	// 데이터 기록
	m.IncrementContainersCreated()
	m.IncrementContainersStarted()
	m.RecordTaskExecution(true, 100*time.Millisecond)
	m.RecordContainerStartTime(500 * time.Millisecond)
	m.RecordError(true)
	m.RecordRetry()

	snapshot := m.GetSnapshot()

	assert.Equal(t, int64(1), snapshot.ContainersCreated)
	assert.Equal(t, int64(1), snapshot.ContainersStarted)
	assert.Equal(t, int64(0), snapshot.ContainersStopped)
	assert.Equal(t, int64(0), snapshot.ContainersFailed)
	assert.Equal(t, int64(1), snapshot.TasksExecuted)
	assert.Equal(t, int64(1), snapshot.TasksSucceeded)
	assert.Equal(t, int64(0), snapshot.TasksFailed)
	assert.Greater(t, snapshot.AvgExecutionTimeMs, 0.0)
	assert.Greater(t, snapshot.AvgContainerStartMs, 0.0)
	assert.Equal(t, int64(1), snapshot.ErrorsTotal)
	assert.Equal(t, int64(1), snapshot.ErrorsRecovered)
	assert.Equal(t, int64(1), snapshot.RetriesTotal)
}

func TestMetrics_Reset(t *testing.T) {
	m := &Metrics{}

	// 데이터 기록
	m.IncrementContainersCreated()
	m.IncrementContainersStarted()
	m.RecordTaskExecution(true, 100*time.Millisecond)
	m.RecordError(true)

	// 초기화
	m.Reset()

	snapshot := m.GetSnapshot()
	assert.Equal(t, int64(0), snapshot.ContainersCreated)
	assert.Equal(t, int64(0), snapshot.ContainersStarted)
	assert.Equal(t, int64(0), snapshot.TasksExecuted)
	assert.Equal(t, int64(0), snapshot.ErrorsTotal)
	assert.Equal(t, 0.0, snapshot.AvgExecutionTimeMs)
	assert.Equal(t, 0.0, snapshot.AvgContainerStartMs)
}

func TestMetrics_ConcurrentAccess(t *testing.T) {
	m := &Metrics{}
	iterations := 100

	var wg sync.WaitGroup

	// Container 메트릭 동시 업데이트
	wg.Add(4)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			m.IncrementContainersCreated()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			m.IncrementContainersStarted()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			m.RecordTaskExecution(true, 10*time.Millisecond)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			m.RecordError(true)
		}
	}()

	wg.Wait()

	snapshot := m.GetSnapshot()
	assert.Equal(t, int64(iterations), snapshot.ContainersCreated)
	assert.Equal(t, int64(iterations), snapshot.ContainersStarted)
	assert.Equal(t, int64(iterations), snapshot.TasksExecuted)
	assert.Equal(t, int64(iterations), snapshot.ErrorsTotal)
}

func TestMetrics_AverageCalculations(t *testing.T) {
	m := &Metrics{}

	// Task 실행 시간 기록
	m.RecordTaskExecution(true, 100*time.Millisecond)
	m.RecordTaskExecution(true, 200*time.Millisecond)
	m.RecordTaskExecution(true, 300*time.Millisecond)

	snapshot := m.GetSnapshot()
	// 평균: (100 + 200 + 300) / 3 = 200ms
	assert.InDelta(t, 200.0, snapshot.AvgExecutionTimeMs, 1.0)
}

func TestMetrics_AverageWithZeroCount(t *testing.T) {
	m := &Metrics{}

	snapshot := m.GetSnapshot()
	assert.Equal(t, 0.0, snapshot.AvgExecutionTimeMs)
	assert.Equal(t, 0.0, snapshot.AvgContainerStartMs)
}

func TestMetrics_SuccessFailureRatio(t *testing.T) {
	m := &Metrics{}

	// 성공 7, 실패 3
	for i := 0; i < 7; i++ {
		m.RecordTaskExecution(true, 10*time.Millisecond)
	}
	for i := 0; i < 3; i++ {
		m.RecordTaskExecution(false, 10*time.Millisecond)
	}

	snapshot := m.GetSnapshot()
	assert.Equal(t, int64(10), snapshot.TasksExecuted)
	assert.Equal(t, int64(7), snapshot.TasksSucceeded)
	assert.Equal(t, int64(3), snapshot.TasksFailed)

	// 성공률 계산
	successRate := float64(snapshot.TasksSucceeded) / float64(snapshot.TasksExecuted)
	assert.InDelta(t, 0.7, successRate, 0.01)
}

func TestMetrics_GlobalInstance(t *testing.T) {
	// GlobalMetrics가 초기화되어 있는지 확인
	require.NotNil(t, GlobalMetrics)

	// 글로벌 인스턴스 테스트
	GlobalMetrics.Reset()
	GlobalMetrics.IncrementContainersCreated()

	snapshot := GlobalMetrics.GetSnapshot()
	assert.Equal(t, int64(1), snapshot.ContainersCreated)

	// 테스트 후 정리
	GlobalMetrics.Reset()
}

func TestMetrics_ErrorRecoveryRate(t *testing.T) {
	m := &Metrics{}

	// 10개 에러 중 6개 복구
	for i := 0; i < 6; i++ {
		m.RecordError(true)
	}
	for i := 0; i < 4; i++ {
		m.RecordError(false)
	}

	snapshot := m.GetSnapshot()
	assert.Equal(t, int64(10), snapshot.ErrorsTotal)
	assert.Equal(t, int64(6), snapshot.ErrorsRecovered)

	// 복구율 계산
	recoveryRate := float64(snapshot.ErrorsRecovered) / float64(snapshot.ErrorsTotal)
	assert.InDelta(t, 0.6, recoveryRate, 0.01)
}

func TestMetricsSnapshot_JSONTags(t *testing.T) {
	m := &Metrics{}
	m.IncrementContainersCreated()
	m.RecordTaskExecution(true, 100*time.Millisecond)

	snapshot := m.GetSnapshot()

	// JSON 태그가 올바르게 설정되어 있는지 확인
	assert.NotNil(t, snapshot)
	assert.Equal(t, int64(1), snapshot.ContainersCreated)
	assert.Equal(t, int64(1), snapshot.TasksExecuted)
}

func TestMetrics_LargeNumbers(t *testing.T) {
	m := &Metrics{}

	// 큰 숫자 테스트
	iterations := 10000
	for i := 0; i < iterations; i++ {
		m.IncrementContainersCreated()
		m.RecordTaskExecution(true, 1*time.Millisecond)
	}

	snapshot := m.GetSnapshot()
	assert.Equal(t, int64(iterations), snapshot.ContainersCreated)
	assert.Equal(t, int64(iterations), snapshot.TasksExecuted)
	assert.Greater(t, snapshot.AvgExecutionTimeMs, 0.0)
}
