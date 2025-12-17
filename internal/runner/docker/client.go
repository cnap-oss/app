package docker

import (
	"context"
	"io"
)

// DockerClient는 Docker Container 관리를 위한 인터페이스입니다.
// 테스트 시 mock 구현을 주입할 수 있도록 인터페이스로 정의합니다.
type DockerClient interface {
	// CreateContainer는 새로운 Container를 생성합니다.
	CreateContainer(ctx context.Context, config ContainerConfig) (containerID string, err error)

	// StartContainer는 Container를 시작합니다.
	StartContainer(ctx context.Context, containerID string) error

	// StopContainer는 Container를 중지합니다.
	// timeout은 강제 종료까지 대기할 시간(초)입니다.
	StopContainer(ctx context.Context, containerID string, timeout int) error

	// RemoveContainer는 Container를 삭제합니다.
	RemoveContainer(ctx context.Context, containerID string) error

	// ContainerLogs는 Container의 로그를 반환합니다.
	ContainerLogs(ctx context.Context, containerID string) (io.ReadCloser, error)

	// ContainerInspect는 Container의 상세 정보를 반환합니다.
	ContainerInspect(ctx context.Context, containerID string) (ContainerInfo, error)

	// Ping은 Docker daemon과의 연결을 확인합니다.
	Ping(ctx context.Context) error

	// Close는 클라이언트 연결을 종료합니다.
	Close() error
}

// ContainerConfig는 Container 생성 설정입니다.
type ContainerConfig struct {
	Image       string            // 이미지 이름 (cnap-runner:latest)
	Name        string            // 컨테이너 이름
	Env         []string          // 환경 변수
	Mounts      []MountConfig     // 볼륨 마운트
	PortBinding *PortConfig       // 포트 바인딩
	Labels      map[string]string // 라벨
}

// MountConfig는 볼륨 마운트 설정입니다.
type MountConfig struct {
	Source string // 호스트 경로
	Target string // 컨테이너 경로
}

// PortConfig는 포트 바인딩 설정입니다.
type PortConfig struct {
	HostPort      string // 호스트 포트 (0이면 랜덤 포트)
	ContainerPort string // 컨테이너 포트
}

// ContainerInfo는 Container의 상세 정보입니다.
type ContainerInfo struct {
	ID         string            // Container ID
	Name       string            // Container 이름
	State      string            // Container 상태 (running, exited, etc.)
	Status     string            // Container 상태 설명
	ImageID    string            // 이미지 ID
	Ports      map[string]string // 포트 매핑 (containerPort -> hostPort)
	Labels     map[string]string // 라벨
	IPAddress  string            // IP 주소
	StartedAt  string            // 시작 시간
	FinishedAt string            // 종료 시간
	ExitCode   int               // 종료 코드
	Error      string            // 에러 메시지 (있는 경우)
}
