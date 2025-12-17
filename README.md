# CNAP

## 프로젝트 구조

```text
cnap-app/
├── cmd/                  # 메인 애플리케이션
├── internal/             # 내부 패키지
│   ├── connector/             # Discord 봇
│   ├── controller/       # 에이전트 관리 및 서버 제어
│   ├── runner/          # OpenCode 러너
│   └── storage/         # GORM 기반 영속 계층
├── go.mod
├── Makefile
└── README.md
```

## 시스템 요구사항

- **OS**: Linux (커널 3.10+)
- **Go**: 1.23+
- **runc**: 최신 버전
- **권한**: root

## 개발 도구 설정

### 필수 도구 설치

#### 1. golangci-lint 설치

`golangci-lint`는 여러 Go linter를 통합 실행하는 도구로, CI에서 코드 품질 검사에 사용됩니다.

**macOS (Homebrew):**

```bash
brew install golangci-lint
```

**Linux:**

```bash
# Binary 직접 설치 (최신 버전)
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin

# 또는 go install 사용
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

**설치 확인:**

```bash
golangci-lint --version
```

#### 2. 기타 개발 도구

**Make** (빌드 자동화):

```bash
# macOS
brew install make

# Linux (대부분 기본 설치됨)
sudo apt-get install make  # Debian/Ubuntu
sudo yum install make       # RHEL/CentOS
```

### 개발 워크플로우

#### 코드 포매팅

```bash
# Go 표준 포매터 실행
make fmt

# 또는 직접 실행
go fmt ./...
gofmt -s -w .
```

#### Lint 검사

```bash
# golangci-lint 실행 (프로젝트 루트의 .golangci.toml 설정 사용)
make lint

# 또는 직접 실행
golangci-lint run

# 특정 디렉토리만 검사
golangci-lint run ./internal/...

# 자동 수정 가능한 이슈 수정
golangci-lint run --fix
```

#### 전체 검사 실행

```bash
# 포매팅, Lint, 테스트를 한번에 실행
make check
```

### CI 통과를 위한 체크리스트

PR을 생성하기 전에 다음 명령어들이 모두 성공하는지 확인하세요:

```bash
# 1. 코드 포매팅
make fmt

# 2. Lint 검사
make lint

# 3. 테스트 실행
make test

# 또는 한번에 실행
make check
```

### Makefile 주요 명령어

| 명령어               | 설명                               |
| -------------------- | ---------------------------------- |
| `make build`         | 바이너리 빌드 (`bin/cnap`)         |
| `make fmt`           | 코드 포매팅 (`gofmt`, `goimports`) |
| `make lint`          | golangci-lint 실행                 |
| `make test`          | 모든 테스트 실행                   |
| `make test-coverage` | 커버리지 리포트 생성               |
| `make check`         | fmt + lint + test 실행             |
| `make clean`         | 빌드 산출물 삭제                   |
| `make docker-build`  | Docker 이미지 빌드                 |

### 4. 빌드

```bash
make build
# 또는
go build -o cnap ./cmd/cnap
```

### 5. 설정

CNAP은 두 가지 방법으로 설정할 수 있습니다:

#### 방법 1: 환경 변수 (기본)

```bash
# .env 파일 생성
cp .env.example .env

# 필수 설정 입력
# - DISCORD_TOKEN: Discord 봇 토큰
# - OPENCODE_API_KEY: OpenCode API 키
```

#### 방법 2: YAML 설정 파일

```bash
# config.yml 파일 생성
cp config.example.yml config.yml

# config.yml 편집하여 설정 입력
```

**YAML 설정 우선순위:**

- YAML 파일에 정의된 값이 기본값으로 사용됩니다
- 환경 변수가 있으면 YAML 값을 오버라이드합니다
- CLI 실행 시 `--config` 플래그로 YAML 파일 경로를 지정할 수 있습니다

**주요 설정 항목:**

| 설정             | 환경 변수           | YAML 경로            | 설명                   |
| ---------------- | ------------------- | -------------------- | ---------------------- |
| Discord 토큰     | `DISCORD_TOKEN`     | `discord.token`      | Discord 봇 토큰 (필수) |
| OpenCode API 키  | `OPENCODE_API_KEY`  | `api_keys.opencode`  | OpenCode API 키 (필수) |
| Anthropic API 키 | `ANTHROPIC_API_KEY` | `api_keys.anthropic` | Anthropic API 키       |
| OpenAI API 키    | `OPENAI_API_KEY`    | `api_keys.openai`    | OpenAI API 키          |
| 실행 환경        | `ENV`               | `app.env`            | development/production |
| 로그 레벨        | `LOG_LEVEL`         | `app.log_level`      | debug/info/warn/error  |
| 데이터베이스 URL | `DATABASE_URL`      | `database.dsn`       | PostgreSQL DSN         |
| Runner 이미지    | `RUNNER_IMAGE`      | `runner.image`       | Docker 이미지 이름     |

전체 설정 항목은 [`config.example.yml`](config.example.yml) 또는 [`.env.example`](.env.example)을 참고하세요.

### 6. CLI 빠른 시작

CNAP CLI를 사용한 기본 파이프라인입니다. 자세한 내용은 [CLI 빠른 시작 가이드](docs/cli-quickstart-guide.md)를 참고하세요.

```bash
# 환경 변수 설정
export OPENCODE_API_KEY="your-api-key"

# Agent 생성
echo -e "my-bot\nAI 비서\ngpt-4\n친절한 AI입니다" | ./bin/cnap agent create

# Task 생성 및 실행
./bin/cnap task create my-bot task-001 --prompt "안녕하세요"
./bin/cnap task send task-001

# 상태 확인
./bin/cnap task view task-001
```

**주요 기능:**

- ✅ 프로세스 재시작 후에도 Task 실행 가능 (Runner 자동 재생성)
- ✅ SQLite 기본 지원 (별도 DB 설정 불필요)
- ✅ 멀티턴 대화 지원
- ✅ 메시지 파일 시스템 저장

### 7. 테스트

저장소 동작과 컨트롤러 흐름은 GORM의 인메모리(SQLite) 드라이버를 활용한 단위 테스트로 검증됩니다.

#### 단위 테스트

```bash
# 모든 단위 테스트 실행
go test ./...

# 커버리지와 함께 실행
make test-coverage
```

#### 통합 테스트

CLI 통합 테스트는 전체 플로우(Agent 생성 → Task 실행 → 상태 확인)를 검증합니다.

```bash
# 사전 준비: PostgreSQL 실행 및 환경 변수 설정
export DATABASE_URL="postgres://cnap:cnap@localhost:5432/cnap_test?sslmode=disable"
export OPEN_CODE_API_KEY="your-api-key"

# 통합 테스트 실행
./scripts/integration_test.sh
```

자세한 내용은 [통합 테스트 가이드](docs/integration_testing.md)를 참고하세요.

## 데이터베이스 설정

CNAP은 PostgreSQL과 GORM을 사용하여 다음 엔티티를 관리합니다.

- `agents`: 로직 멀티테넌시를 위한 에이전트 메타데이터
- `tasks`: 에이전트별 작업 실행 단위
- `msg_index`: 메시지 본문이 저장된 로컬 JSON 파일 경로 인덱스
- `run_steps`: 작업 단계 기록
- `checkpoints`: Git 스냅샷(해시) 기록

### 데이터베이스 환경 변수

| 변수                   | 필수 | 설명                                               | 기본값             |
| ---------------------- | ---- | -------------------------------------------------- | ------------------ |
| `DATABASE_URL`         |      | PostgreSQL 연결 DSN                                | SQLite (로컬 파일) |
| `DB_LOG_LEVEL`         |      | GORM 로그 레벨 (`silent`, `error`, `warn`, `info`) | `warn`             |
| `DB_MAX_IDLE`          |      | 연결 풀 idle 개수                                  | `5`                |
| `DB_MAX_OPEN`          |      | 연결 풀 최대 개수                                  | `20`               |
| `DB_CONN_LIFETIME`     |      | 연결 최대 수명 (예: `1h`)                          | `30m`              |
| `DB_SKIP_DEFAULT_TXN`  |      | 기본 트랜잭션 생략 여부 (`true`/`false`)           | `true`             |
| `DB_PREPARE_STMT`      |      | Prepare statement 캐시 활성화 여부                 | `false`            |
| `DB_DISABLE_AUTO_PING` |      | GORM 자동 `Ping` 비활성화                          | `false`            |

애플리케이션이 시작될 때 자동으로 스키마 마이그레이션을 수행하며, 메시지 본문은 데이터베이스가 아닌 로컬 JSON 파일로 유지됩니다.

### Docker Compose로 애플리케이션 실행

`docker/docker-compose.yml`은 CNAP 애플리케이션과 PostgreSQL을 함께 실행합니다. 데이터 및 메시지 파일은 `.gitignore`에 포함된 `docker-data/` 경로에 저장됩니다.

```bash
docker compose -f docker/docker-compose.yml up -d
```

환경 변수는 다음과 같이 기본값을 재정의할 수 있습니다.

- 데이터베이스: `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_PORT`
- 애플리케이션: `APP_ENV`, `APP_LOG_LEVEL`

## 라이선스

MIT License

- 데이터베이스: `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_PORT`
- 애플리케이션: `APP_ENV`, `APP_LOG_LEVEL`

## 라이선스

MIT License
