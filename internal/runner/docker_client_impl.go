package taskrunner

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

// RealDockerClient는 실제 Docker SDK를 사용하는 DockerClient 구현체입니다.
type RealDockerClient struct {
	client *client.Client
}

// NewDockerClient는 새로운 RealDockerClient를 생성합니다.
func NewDockerClient() (DockerClient, error) {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("Docker 클라이언트 생성 실패: %w", err)
	}

	return &RealDockerClient{client: cli}, nil
}

// CreateContainer implements DockerClient.
func (d *RealDockerClient) CreateContainer(ctx context.Context, config ContainerConfig) (string, error) {
	// Container 설정 구성
	containerConfig := &container.Config{
		Image:  config.Image,
		Env:    config.Env,
		Labels: config.Labels,
	}

	// 포트 설정
	if config.PortBinding != nil {
		exposedPorts := nat.PortSet{}
		containerPort := nat.Port(config.PortBinding.ContainerPort + "/tcp")
		exposedPorts[containerPort] = struct{}{}
		containerConfig.ExposedPorts = exposedPorts
	}

	// 호스트 설정 구성
	hostConfig := &container.HostConfig{
		AutoRemove: false,
	}

	// 포트 바인딩 설정
	if config.PortBinding != nil {
		containerPort := nat.Port(config.PortBinding.ContainerPort + "/tcp")
		hostPort := config.PortBinding.HostPort
		if hostPort == "" {
			hostPort = "0" // 동적 포트 할당
		}

		hostConfig.PortBindings = nat.PortMap{
			containerPort: []nat.PortBinding{
				{
					HostIP:   "127.0.0.1",
					HostPort: hostPort,
				},
			},
		}
	}

	// 볼륨 마운트 설정
	if len(config.Mounts) > 0 {
		binds := make([]string, 0, len(config.Mounts))
		for _, m := range config.Mounts {
			bind := fmt.Sprintf("%s:%s", m.Source, m.Target)
			binds = append(binds, bind)
		}
		hostConfig.Binds = binds
	}

	// Container 생성
	resp, err := d.client.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		nil, // networkingConfig
		nil, // platform
		config.Name,
	)
	if err != nil {
		return "", fmt.Errorf("Container 생성 실패: %w", err)
	}

	return resp.ID, nil
}

// StartContainer implements DockerClient.
func (d *RealDockerClient) StartContainer(ctx context.Context, containerID string) error {
	if err := d.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("Container 시작 실패: %w", err)
	}
	return nil
}

// StopContainer implements DockerClient.
func (d *RealDockerClient) StopContainer(ctx context.Context, containerID string, timeout int) error {
	var stopOptions container.StopOptions
	if timeout > 0 {
		stopOptions.Timeout = &timeout
	}

	if err := d.client.ContainerStop(ctx, containerID, stopOptions); err != nil {
		return fmt.Errorf("Container 중지 실패: %w", err)
	}
	return nil
}

// RemoveContainer implements DockerClient.
func (d *RealDockerClient) RemoveContainer(ctx context.Context, containerID string) error {
	removeOptions := container.RemoveOptions{
		Force:         true,
		RemoveVolumes: false,
	}

	if err := d.client.ContainerRemove(ctx, containerID, removeOptions); err != nil {
		return fmt.Errorf("Container 삭제 실패: %w", err)
	}
	return nil
}

// ContainerLogs implements DockerClient.
func (d *RealDockerClient) ContainerLogs(ctx context.Context, containerID string) (io.ReadCloser, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     false,
		Timestamps: false,
	}

	logs, err := d.client.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return nil, fmt.Errorf("Container 로그 조회 실패: %w", err)
	}

	return logs, nil
}

// ContainerInspect implements DockerClient.
func (d *RealDockerClient) ContainerInspect(ctx context.Context, containerID string) (ContainerInfo, error) {
	inspect, err := d.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return ContainerInfo{}, fmt.Errorf("Container 조회 실패: %w", err)
	}

	// 포트 매핑 정보 추출
	ports := make(map[string]string)
	if inspect.NetworkSettings != nil && inspect.NetworkSettings.Ports != nil {
		for containerPort, bindings := range inspect.NetworkSettings.Ports {
			if len(bindings) > 0 {
				ports[string(containerPort)] = bindings[0].HostPort
			}
		}
	}

	// IP 주소 추출
	ipAddress := ""
	if inspect.NetworkSettings != nil {
		ipAddress = inspect.NetworkSettings.IPAddress
	}

	// 종료 코드 추출
	exitCode := 0
	if inspect.State != nil {
		exitCode = inspect.State.ExitCode
	}

	// 상태 정보 구성
	info := ContainerInfo{
		ID:        inspect.ID,
		Name:      inspect.Name,
		ImageID:   inspect.Image,
		Ports:     ports,
		Labels:    inspect.Config.Labels,
		IPAddress: ipAddress,
		ExitCode:  exitCode,
	}

	if inspect.State != nil {
		info.State = inspect.State.Status
		info.Status = inspect.State.Status
		info.StartedAt = inspect.State.StartedAt
		info.FinishedAt = inspect.State.FinishedAt
		info.Error = inspect.State.Error
	}

	return info, nil
}

// Ping implements DockerClient.
func (d *RealDockerClient) Ping(ctx context.Context) error {
	_, err := d.client.Ping(ctx)
	if err != nil {
		return fmt.Errorf("Docker daemon 연결 실패: %w", err)
	}
	return nil
}

// Close implements DockerClient.
func (d *RealDockerClient) Close() error {
	if d.client != nil {
		return d.client.Close()
	}
	return nil
}

// GetContainerPort는 Container에 매핑된 호스트 포트를 반환합니다.
func GetContainerPort(ctx context.Context, dockerClient DockerClient, containerID string, containerPort string) (int, error) {
	info, err := dockerClient.ContainerInspect(ctx, containerID)
	if err != nil {
		return 0, fmt.Errorf("Container 조회 실패: %w", err)
	}

	portKey := containerPort + "/tcp"
	hostPort, ok := info.Ports[portKey]
	if !ok {
		return 0, fmt.Errorf("포트 매핑을 찾을 수 없음: %s", containerPort)
	}

	port, err := strconv.Atoi(hostPort)
	if err != nil {
		return 0, fmt.Errorf("포트 파싱 실패: %w", err)
	}

	return port, nil
}

// 인터페이스 구현 확인
var _ DockerClient = (*RealDockerClient)(nil)
