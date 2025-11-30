# CLI 사용법

CNAP CLI(`cnap`)는 Controller·Connector 서버를 구동하고 Agent/Task를 직접 관리하기 위한 단일 실행 파일입니다. 이 문서는 로컬에서 CLI를 빌드하고, 핵심 명령어를 사용하는 방법을 빠르게 익히기 위한 가이드입니다.

## 목차
- [사전 준비](#사전-준비)
- [빌드](#빌드)
- [빠른 시작](#빠른-시작)
- [주요 명령어](#주요-명령어)
  - [서비스 제어](#서비스-제어)
  - [Agent 관리](#agent-관리)
  - [Task 관리](#task-관리)
- [필수/주요 환경 변수](#필수주요-환경-변수)
- [자주 겪는 오류](#자주-겪는-오류)
- [추가 자료](#추가-자료)

---

## 사전 준비

- **Go 1.23+**, **Make** 설치
- 데이터베이스
  - `DATABASE_URL`이 없으면 기본 SQLite 파일(`./data/cnap.db`)이 자동 사용됩니다.
  - PostgreSQL을 쓰는 경우 `DATABASE_URL`을 설정하세요.
- OpenCode 호출이 필요한 Task 실행(`cnap task run`, `cnap task send`) 시 `OPEN_CODE_API_KEY`가 필요합니다.

## 빌드

```bash
# 프로젝트 루트에서
make build
# 또는
go build -o bin/cnap ./cmd/cnap

# 확인
./bin/cnap --version
./bin/cnap --help
```

## 빠른 시작

1. (선택) PostgreSQL 사용 시 환경 변수 설정
   ```bash
   export DATABASE_URL="postgres://cnap:cnap@localhost:5432/cnap?sslmode=disable"
   ```
2. OpenCode 호출을 사용할 경우
   ```bash
   export OPEN_CODE_API_KEY="your-api-key"
   ```
3. CLI 실행 예시
   ```bash
   # Agent 생성
   ./bin/cnap agent create

   # Task 생성 및 실행
   ./bin/cnap task create support-bot task-001 --prompt "첫 메시지"
   ./bin/cnap task run task-001
   ```

## 주요 명령어

### 서비스 제어

- `cnap start`  
  Controller와 Connector 서버를 동시에 시작합니다. SIGINT/SIGTERM을 받으면 30초 동안 Graceful shutdown을 수행합니다.

- `cnap health`  
  프로세스 기동 없이 CLI가 정상인지 확인합니다. `OK`가 출력되면 CLI 실행이 가능함을 의미합니다.

### Agent 관리

- `cnap agent create`  
  대화형 입력으로 Agent를 생성합니다. 한글 입력은 NFC로 정규화되어 DB에 안전하게 저장됩니다.

- `cnap agent list`  
  Agent 목록을 테이블 형태로 출력합니다.

- `cnap agent view <agent-name>`  
  단일 Agent의 상세 정보를 확인합니다.

- `cnap agent edit <agent-name>`  
  설명/모델/프롬프트를 대화형으로 수정합니다(이름은 변경 불가).

- `cnap agent delete <agent-name>`  
  확인 프롬프트 후 Agent를 삭제(`deleted` 상태로 변경)합니다.

### Task 관리

- `cnap task create <agent-name> <task-id> [--prompt|-p <text>]`  
  특정 Agent에 Task를 생성합니다. `--prompt`로 초기 프롬프트를 저장할 수 있습니다.

- `cnap task list <agent-name>`  
  Agent별 Task 목록을 조회합니다.

- `cnap task view <task-id>`  
  단일 Task의 상세 정보와 프롬프트를 확인합니다.

- `cnap task update-status <task-id> <status>`  
  상태를 직접 변경합니다. 지원 상태: `pending`, `running`, `completed`, `failed`, `canceled`.

- `cnap task run <task-id>`  
  Pending Task를 실행합니다. OpenCode 호출을 위해 `OPEN_CODE_API_KEY`가 필요합니다.

- `cnap task send <task-id>`  
  메시지 전송 후 실행을 트리거합니다. Runner 호출을 수행하므로 `OPEN_CODE_API_KEY`가 필요합니다.

- `cnap task cancel <task-id>`  
  실행 중인 Task를 취소합니다.

- `cnap task add-message <task-id> <message>`  
  Task에 메시지를 추가합니다(실행 트리거 없음).

- `cnap task messages <task-id>`  
  메시지 인덱스와 파일 경로를 조회합니다.

## 필수/주요 환경 변수

| 변수 | 필수 | 설명 | 기본값 |
| --- | --- | --- | --- |
| `DATABASE_URL` |  | PostgreSQL DSN | 설정 없을 시 `./data/cnap.db` (SQLite) |
| `SQLITE_DATABASE` |  | SQLite 파일 경로 override | `./data/cnap.db` |
| `OPEN_CODE_API_KEY` | Task 실행 시 필요 | Runner가 OpenCode API를 호출할 때 사용 | 없음 |
| `LOG_LEVEL` |  | 로그 레벨 (`debug`, `info`, `warn`, `error`) | 개발 모드: `debug`, 프로덕션: `info` |
| `ENV` |  | `production` 설정 시 zap 프로덕션 로거 사용 | 빈 값(개발 모드) |
| `DB_MAX_IDLE`, `DB_MAX_OPEN`, `DB_CONN_LIFETIME`, `DB_SKIP_DEFAULT_TXN`, `DB_PREPARE_STMT`, `DB_DISABLE_AUTO_PING` |  | GORM 커넥션 풀/옵션 튜닝 | 문서에 기재된 기본값 사용 |

## 자주 겪는 오류

- **`database is locked` (SQLite)**: 다른 프로세스에서 파일 잠금이 오래 유지될 때 발생할 수 있습니다. 불필요한 CLI 프로세스를 종료하거나 SQLite 파일을 별도 경로(`SQLITE_DATABASE`)로 분리하세요.
- **`OPEN_CODE_API_KEY` not set**: `task run/send` 시 필수이므로 환경 변수를 설정해야 합니다.
- **DB 연결 실패 (PostgreSQL)**: `DATABASE_URL` 포맷과 접근 권한을 확인하고, 방화벽/포트 설정을 점검하세요.
- **한글이 깨지는 경우**: CLI가 입력을 NFC로 정규화하므로 일반적으로 문제없지만, 터미널 인코딩이 UTF-8인지 확인하세요.

## 추가 자료

- Controller 기반 세부 명령 설명: [docs/controller-cli-guide.md](./controller-cli-guide.md)
- CLI 동작 점검 시나리오: [docs/cli-testing-guide.md](./cli-testing-guide.md)
- 통합 테스트/환경 구성: [docs/integration_testing.md](./integration_testing.md)
