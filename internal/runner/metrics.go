package taskrunner

import (
	"sync/atomic"
	"time"
)

// Metrics는 Runner 메트릭을 수집합니다.
type Metrics struct {
	// Container 메트릭
	ContainersCreated int64
	ContainersStarted int64
	ContainersStopped int64
	ContainersFailed  int64

	// Task 메트릭
	TasksExecuted  int64
	TasksSucceeded int64
	TasksFailed    int64

	// 타이밍 메트릭
	TotalExecutionTime  int64 // 나노초
	ContainerStartTime  int64 // 나노초 (평균 계산용 합계)
	ContainerStartCount int64

	// 에러 메트릭
	ErrorsTotal     int64
	ErrorsRecovered int64
	RetriesTotal    int64
}

// GlobalMetrics는 전역 메트릭 인스턴스입니다.
var GlobalMetrics = &Metrics{}

// IncrementContainersCreated는 생성된 Container 수를 증가시킵니다.
func (m *Metrics) IncrementContainersCreated() {
	atomic.AddInt64(&m.ContainersCreated, 1)
}

// IncrementContainersStarted는 시작된 Container 수를 증가시킵니다.
func (m *Metrics) IncrementContainersStarted() {
	atomic.AddInt64(&m.ContainersStarted, 1)
}

// IncrementContainersStopped는 중지된 Container 수를 증가시킵니다.
func (m *Metrics) IncrementContainersStopped() {
	atomic.AddInt64(&m.ContainersStopped, 1)
}

// IncrementContainersFailed는 실패한 Container 수를 증가시킵니다.
func (m *Metrics) IncrementContainersFailed() {
	atomic.AddInt64(&m.ContainersFailed, 1)
}

// RecordTaskExecution은 Task 실행을 기록합니다.
func (m *Metrics) RecordTaskExecution(success bool, duration time.Duration) {
	atomic.AddInt64(&m.TasksExecuted, 1)
	atomic.AddInt64(&m.TotalExecutionTime, int64(duration))

	if success {
		atomic.AddInt64(&m.TasksSucceeded, 1)
	} else {
		atomic.AddInt64(&m.TasksFailed, 1)
	}
}

// RecordContainerStartTime은 Container 시작 시간을 기록합니다.
func (m *Metrics) RecordContainerStartTime(duration time.Duration) {
	atomic.AddInt64(&m.ContainerStartTime, int64(duration))
	atomic.AddInt64(&m.ContainerStartCount, 1)
}

// RecordError는 에러를 기록합니다.
func (m *Metrics) RecordError(recovered bool) {
	atomic.AddInt64(&m.ErrorsTotal, 1)
	if recovered {
		atomic.AddInt64(&m.ErrorsRecovered, 1)
	}
}

// RecordRetry는 재시도를 기록합니다.
func (m *Metrics) RecordRetry() {
	atomic.AddInt64(&m.RetriesTotal, 1)
}

// GetSnapshot은 현재 메트릭 스냅샷을 반환합니다.
func (m *Metrics) GetSnapshot() MetricsSnapshot {
	return MetricsSnapshot{
		ContainersCreated:   atomic.LoadInt64(&m.ContainersCreated),
		ContainersStarted:   atomic.LoadInt64(&m.ContainersStarted),
		ContainersStopped:   atomic.LoadInt64(&m.ContainersStopped),
		ContainersFailed:    atomic.LoadInt64(&m.ContainersFailed),
		TasksExecuted:       atomic.LoadInt64(&m.TasksExecuted),
		TasksSucceeded:      atomic.LoadInt64(&m.TasksSucceeded),
		TasksFailed:         atomic.LoadInt64(&m.TasksFailed),
		AvgExecutionTimeMs:  m.calculateAvgExecutionTime(),
		AvgContainerStartMs: m.calculateAvgContainerStart(),
		ErrorsTotal:         atomic.LoadInt64(&m.ErrorsTotal),
		ErrorsRecovered:     atomic.LoadInt64(&m.ErrorsRecovered),
		RetriesTotal:        atomic.LoadInt64(&m.RetriesTotal),
	}
}

// Reset은 모든 메트릭을 초기화합니다.
func (m *Metrics) Reset() {
	atomic.StoreInt64(&m.ContainersCreated, 0)
	atomic.StoreInt64(&m.ContainersStarted, 0)
	atomic.StoreInt64(&m.ContainersStopped, 0)
	atomic.StoreInt64(&m.ContainersFailed, 0)
	atomic.StoreInt64(&m.TasksExecuted, 0)
	atomic.StoreInt64(&m.TasksSucceeded, 0)
	atomic.StoreInt64(&m.TasksFailed, 0)
	atomic.StoreInt64(&m.TotalExecutionTime, 0)
	atomic.StoreInt64(&m.ContainerStartTime, 0)
	atomic.StoreInt64(&m.ContainerStartCount, 0)
	atomic.StoreInt64(&m.ErrorsTotal, 0)
	atomic.StoreInt64(&m.ErrorsRecovered, 0)
	atomic.StoreInt64(&m.RetriesTotal, 0)
}

func (m *Metrics) calculateAvgExecutionTime() float64 {
	executed := atomic.LoadInt64(&m.TasksExecuted)
	if executed == 0 {
		return 0
	}
	totalNs := atomic.LoadInt64(&m.TotalExecutionTime)
	return float64(totalNs) / float64(executed) / 1e6 // 나노초 -> 밀리초
}

func (m *Metrics) calculateAvgContainerStart() float64 {
	count := atomic.LoadInt64(&m.ContainerStartCount)
	if count == 0 {
		return 0
	}
	totalNs := atomic.LoadInt64(&m.ContainerStartTime)
	return float64(totalNs) / float64(count) / 1e6
}

// MetricsSnapshot은 메트릭 스냅샷입니다.
type MetricsSnapshot struct {
	ContainersCreated   int64   `json:"containers_created"`
	ContainersStarted   int64   `json:"containers_started"`
	ContainersStopped   int64   `json:"containers_stopped"`
	ContainersFailed    int64   `json:"containers_failed"`
	TasksExecuted       int64   `json:"tasks_executed"`
	TasksSucceeded      int64   `json:"tasks_succeeded"`
	TasksFailed         int64   `json:"tasks_failed"`
	AvgExecutionTimeMs  float64 `json:"avg_execution_time_ms"`
	AvgContainerStartMs float64 `json:"avg_container_start_ms"`
	ErrorsTotal         int64   `json:"errors_total"`
	ErrorsRecovered     int64   `json:"errors_recovered"`
	RetriesTotal        int64   `json:"retries_total"`
}
