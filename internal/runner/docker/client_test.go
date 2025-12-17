package docker

import (
	"context"
	"io"
	"strings"
	"testing"
)

// MockDockerClient는 테스트용 Mock Client입니다.
type MockDockerClient struct {
	CreateContainerFunc  func(ctx context.Context, config ContainerConfig) (string, error)
	StartContainerFunc   func(ctx context.Context, containerID string) error
	StopContainerFunc    func(ctx context.Context, containerID string, timeout int) error
	RemoveContainerFunc  func(ctx context.Context, containerID string) error
	ContainerLogsFunc    func(ctx context.Context, containerID string) (io.ReadCloser, error)
	ContainerInspectFunc func(ctx context.Context, containerID string) (ContainerInfo, error)
	PingFunc             func(ctx context.Context) error
	CloseFunc            func() error
}

func (m *MockDockerClient) CreateContainer(ctx context.Context, config ContainerConfig) (string, error) {
	if m.CreateContainerFunc != nil {
		return m.CreateContainerFunc(ctx, config)
	}
	return "mock-container-id", nil
}

func (m *MockDockerClient) StartContainer(ctx context.Context, containerID string) error {
	if m.StartContainerFunc != nil {
		return m.StartContainerFunc(ctx, containerID)
	}
	return nil
}

func (m *MockDockerClient) StopContainer(ctx context.Context, containerID string, timeout int) error {
	if m.StopContainerFunc != nil {
		return m.StopContainerFunc(ctx, containerID, timeout)
	}
	return nil
}

func (m *MockDockerClient) RemoveContainer(ctx context.Context, containerID string) error {
	if m.RemoveContainerFunc != nil {
		return m.RemoveContainerFunc(ctx, containerID)
	}
	return nil
}

func (m *MockDockerClient) ContainerLogs(ctx context.Context, containerID string) (io.ReadCloser, error) {
	if m.ContainerLogsFunc != nil {
		return m.ContainerLogsFunc(ctx, containerID)
	}
	return io.NopCloser(strings.NewReader("mock logs")), nil
}

func (m *MockDockerClient) ContainerInspect(ctx context.Context, containerID string) (ContainerInfo, error) {
	if m.ContainerInspectFunc != nil {
		return m.ContainerInspectFunc(ctx, containerID)
	}
	return ContainerInfo{
		ID:    containerID,
		Name:  "mock-container",
		State: "running",
		Ports: map[string]string{
			"3000/tcp": "8080",
		},
	}, nil
}

func (m *MockDockerClient) Ping(ctx context.Context) error {
	if m.PingFunc != nil {
		return m.PingFunc(ctx)
	}
	return nil
}

func (m *MockDockerClient) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// 인터페이스 구현 확인
var _ DockerClient = (*MockDockerClient)(nil)

func TestMockClient_CreateContainer(t *testing.T) {
	ctx := context.Background()
	mock := &MockDockerClient{}

	config := ContainerConfig{
		Image: "test-image:latest",
		Name:  "test-container",
		Env:   []string{"ENV=test"},
	}

	containerID, err := mock.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	if containerID != "mock-container-id" {
		t.Errorf("Expected container ID 'mock-container-id', got '%s'", containerID)
	}
}

func TestMockClient_StartContainer(t *testing.T) {
	ctx := context.Background()
	mock := &MockDockerClient{}

	err := mock.StartContainer(ctx, "test-container-id")
	if err != nil {
		t.Fatalf("StartContainer failed: %v", err)
	}
}

func TestMockClient_StopContainer(t *testing.T) {
	ctx := context.Background()
	mock := &MockDockerClient{}

	err := mock.StopContainer(ctx, "test-container-id", 10)
	if err != nil {
		t.Fatalf("StopContainer failed: %v", err)
	}
}

func TestMockClient_RemoveContainer(t *testing.T) {
	ctx := context.Background()
	mock := &MockDockerClient{}

	err := mock.RemoveContainer(ctx, "test-container-id")
	if err != nil {
		t.Fatalf("RemoveContainer failed: %v", err)
	}
}

func TestMockClient_ContainerLogs(t *testing.T) {
	ctx := context.Background()
	mock := &MockDockerClient{}

	logs, err := mock.ContainerLogs(ctx, "test-container-id")
	if err != nil {
		t.Fatalf("ContainerLogs failed: %v", err)
	}
	defer logs.Close()

	// 로그 읽기 테스트
	buf := make([]byte, 100)
	n, err := logs.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Failed to read logs: %v", err)
	}

	if n == 0 {
		t.Error("Expected to read some logs")
	}
}

func TestMockClient_ContainerInspect(t *testing.T) {
	ctx := context.Background()
	mock := &MockDockerClient{}

	info, err := mock.ContainerInspect(ctx, "test-container-id")
	if err != nil {
		t.Fatalf("ContainerInspect failed: %v", err)
	}

	if info.ID != "test-container-id" {
		t.Errorf("Expected container ID 'test-container-id', got '%s'", info.ID)
	}

	if info.State != "running" {
		t.Errorf("Expected state 'running', got '%s'", info.State)
	}

	if info.Ports["3000/tcp"] != "8080" {
		t.Errorf("Expected port mapping '3000/tcp' -> '8080', got '%s'", info.Ports["3000/tcp"])
	}
}

func TestMockClient_Ping(t *testing.T) {
	ctx := context.Background()
	mock := &MockDockerClient{}

	err := mock.Ping(ctx)
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestMockClient_Close(t *testing.T) {
	mock := &MockDockerClient{}

	err := mock.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestGetContainerPort(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		containerID   string
		containerPort string
		mockPorts     map[string]string
		expectedPort  int
		expectError   bool
	}{
		{
			name:          "성공: 포트 매핑 조회",
			containerID:   "test-container",
			containerPort: "3000",
			mockPorts: map[string]string{
				"3000/tcp": "8080",
			},
			expectedPort: 8080,
			expectError:  false,
		},
		{
			name:          "실패: 포트 매핑 없음",
			containerID:   "test-container",
			containerPort: "3000",
			mockPorts:     map[string]string{},
			expectedPort:  0,
			expectError:   true,
		},
		{
			name:          "실패: 잘못된 포트 형식",
			containerID:   "test-container",
			containerPort: "3000",
			mockPorts: map[string]string{
				"3000/tcp": "invalid",
			},
			expectedPort: 0,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockDockerClient{
				ContainerInspectFunc: func(ctx context.Context, containerID string) (ContainerInfo, error) {
					return ContainerInfo{
						ID:    containerID,
						Ports: tt.mockPorts,
					}, nil
				},
			}

			port, err := GetContainerPort(ctx, mock, tt.containerID, tt.containerPort)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if port != tt.expectedPort {
					t.Errorf("Expected port %d, got %d", tt.expectedPort, port)
				}
			}
		})
	}
}

func TestContainerConfig(t *testing.T) {
	config := ContainerConfig{
		Image: "cnap-runner:latest",
		Name:  "test-runner",
		Env:   []string{"ENV=production"},
		Mounts: []MountConfig{
			{
				Source: "/host/path",
				Target: "/container/path",
			},
		},
		PortBinding: &PortConfig{
			HostPort:      "8080",
			ContainerPort: "3000",
		},
		Labels: map[string]string{
			"app": "cnap",
		},
	}

	if config.Image != "cnap-runner:latest" {
		t.Errorf("Expected image 'cnap-runner:latest', got '%s'", config.Image)
	}

	if len(config.Mounts) != 1 {
		t.Errorf("Expected 1 mount, got %d", len(config.Mounts))
	}

	if config.PortBinding.ContainerPort != "3000" {
		t.Errorf("Expected container port '3000', got '%s'", config.PortBinding.ContainerPort)
	}
}
