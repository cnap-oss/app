# CLI 빠른 시작 가이드

CNAP CLI를 사용한 실전 파이프라인 가이드입니다. 이 문서는 Agent 생성부터 Task 실행까지 전체 흐름을 단계별로 안내합니다.

## 목차
- [사전 준비](#사전-준비)
- [지원 AI 모델](#지원-ai-모델)
- [기본 파이프라인](#기본-파이프라인)
- [멀티 프로바이더 사용법](#멀티-프로바이더-사용법)
- [멀티턴 대화 파이프라인](#멀티턴-대화-파이프라인)
- [고급 사용법](#고급-사용법)
- [문제 해결](#문제-해결)
- [주요 특징](#주요-특징)

---

## 사전 준비

### 1. 빌드

```bash
make build
# 또는
go build -o bin/cnap ./cmd/cnap
```

### 2. 환경 변수 설정

CNAP은 여러 AI 프로바이더를 지원합니다. 사용할 프로바이더의 API 키를 설정하세요.

```bash
# OpenCode API Key (기본값 - 19개 모델 지원)
export OPEN_CODE_API_KEY="your-opencode-key"

# 또는 다른 프로바이더 API 키
export ANTHROPIC_API_KEY="sk-ant-xxx"  # Claude 직접 사용 시 (추후 지원)
export OPENAI_API_KEY="sk-proj-xxx"    # OpenAI 직접 사용 시
export XAI_API_KEY="your-xai-key"      # xAI 직접 사용 시
export GEMINI_API_KEY="your-key"       # Gemini 직접 사용 시 (추후 지원)

# 데이터베이스 (선택 - 미설정 시 SQLite 사용)
export DATABASE_URL="postgres://cnap:cnap@localhost:5432/cnap?sslmode=disable"
```

**참고**: CLI에서 Agent 생성 시 API 키가 없으면 입력 프롬프트가 표시됩니다.

### 3. 상태 확인

```bash
./bin/cnap health
# 출력: OK
```

---

## 지원 AI 모델

CNAP은 5개의 AI 프로바이더와 다양한 모델을 지원합니다.

### OpenCode (추천 🌟)

**특징**: 단일 API로 19개 모델 사용 가능

| 카테고리 | 모델 ID | 설명 |
|---------|---------|------|
| **Claude** | `claude-opus-4-5` | 최고 성능 |
|  | `claude-sonnet-4-5` | 균형잡힌 성능/비용 |
|  | `claude-haiku-4-5` | 빠른 응답 |
| **GPT** | `gpt-5.1` | 최신 GPT |
|  | `gpt-5.1-codex` | 코드 특화 |
|  | `gpt-5-nano` | 경량 모델 |
| **Gemini** | `gemini-3-pro` | Google AI |
| **xAI** | `grok-code` | 코드 작업 최적화 |
| **중국 AI** | `qwen3-coder` | Alibaba |
|  | `glm-4.6` | Zhipu AI |
|  | `kimi-k2` | Moonshot AI |

**사용 예시**:
```bash
export OPEN_CODE_API_KEY="your-key"
# provider를 opencode로 설정하고 원하는 모델 선택
```

### OpenAI

**특징**: OpenAI 공식 API 직접 사용

| 모델 ID | 설명 |
|---------|------|
| `gpt-5.1` | 최신 GPT 모델 |
| `gpt-5` | GPT-5 |
| `gpt-5.1-codex` | 코드 특화 |

**사용 예시**:
```bash
export OPENAI_API_KEY="sk-proj-xxx"
# provider를 openai로 설정
```

### xAI

**특징**: xAI의 Grok 모델

| 모델 ID | 설명 |
|---------|------|
| `grok-code` | 코드 작업 최적화 |

**사용 예시**:
```bash
export XAI_API_KEY="your-key"
# provider를 xai로 설정
```

### Claude & Gemini (추후 지원)

**현재 상태**: OpenCode provider를 통해 사용 가능

직접 API는 추후 구현 예정입니다. 현재는 `opencode` provider로 Claude/Gemini 모델을 실행할 수 있습니다.

---

## 기본 파이프라인

단일 프롬프트로 Task를 실행하는 가장 기본적인 흐름입니다.

### Step 1: Agent 생성

```bash
./bin/cnap agent create
```

**대화형 입력:**
```
Agent 이름: my-assistant
설명: 개인 비서 AI
프로바이더 (opencode/gemini/claude/openai/xai) [opencode]: opencode
모델 (예: claude-sonnet-4-5): claude-sonnet-4-5
✓ OPEN_CODE_API_KEY가 환경 변수에서 발견되었습니다.
프롬프트 (역할 정의): 당신은 친절하고 도움이 되는 AI 비서입니다.
```

**출력:**
```
✓ Agent 'my-assistant' 생성 완료 (Provider: opencode, Model: claude-sonnet-4-5)
```

**팁**: 비대화형으로 생성하려면:
```bash
echo -e "my-assistant\n개인 비서 AI\nopencode\nclaude-sonnet-4-5\n당신은 친절하고 도움이 되는 AI 비서입니다." | ./bin/cnap agent create
```

**API 키가 없는 경우:**
```
Agent 이름: my-bot
설명: AI 비서
프로바이더 (opencode/gemini/claude/openai/xai) [opencode]: openai
모델 (예: gpt-5.1): gpt-5.1
⚠ OPENAI_API_KEY가 설정되지 않았습니다.
API Key를 입력하세요 (Enter를 누르면 건너뛰기): sk-proj-xxxxx
✓ OPENAI_API_KEY가 설정되었습니다.
프롬프트 (역할 정의): 친절한 AI입니다.
```

### Step 2: Agent 확인

```bash
./bin/cnap agent list
```

**출력:**
```
NAME           STATUS  MODEL                DESCRIPTION   CREATED
----           ------  -----                -----------   -------
my-assistant   active  claude-sonnet-4-5   개인 비서 AI   2025-11-30 23:15
```

**상세 정보 확인:**
```bash
./bin/cnap agent view my-assistant
```

**출력:**
```
=== Agent 정보: my-assistant ===

이름:        my-assistant
상태:        active
프로바이더:  opencode
모델:        claude-sonnet-4-5
설명:        개인 비서 AI
프롬프트:
당신은 친절하고 도움이 되는 AI 비서입니다.

생성일:      2025-11-30 23:15:30
수정일:      2025-11-30 23:15:30
```

### Step 3: Task 생성 (프롬프트 포함)

```bash
./bin/cnap task create my-assistant task-001 --prompt "2+2는 얼마인가요?"
```

**출력:**
```
✓ Task 'task-001' 생성 완료 (Agent: my-assistant, Prompt: 2+2는 얼마인가요?)
```

**프롬프트 없이 생성:**
```bash
./bin/cnap task create my-assistant task-002
```

### Step 4: Task 확인

```bash
./bin/cnap task view task-001
```

**출력:**
```
=== Task 정보: task-001 ===

Task ID:     task-001
Agent ID:    my-assistant
상태:        pending
프롬프트:    2+2는 얼마인가요?
생성일:      2025-11-30 23:16:01
수정일:      2025-11-30 23:16:01
```

### Step 5: Task 실행

```bash
./bin/cnap task send task-001
```

**출력:**
```
✓ Task 'task-001' 실행이 트리거되었습니다.
```

**내부 동작:**
1. Task 상태를 `running`으로 변경
2. Runner가 없으면 자동 재생성 (🎯 핵심 기능!)
3. Provider별 API 호출 (opencode, openai, xai 등)
4. 백그라운드에서 실행

### Step 6: 실행 상태 확인

```bash
./bin/cnap task view task-001
```

**출력:**
```
=== Task 정보: task-001 ===

Task ID:     task-001
Agent ID:    my-assistant
상태:        running  # pending → running으로 변경됨
프롬프트:    2+2는 얼마인가요?
...
```

---

## 멀티 프로바이더 사용법

여러 AI 프로바이더를 선택하여 사용하는 방법입니다.

### OpenCode로 Claude 사용 (추천 🌟)

가장 간편한 방법입니다. 단일 API 키로 19개 모델을 모두 사용할 수 있습니다.

```bash
# 1. API 키 설정
export OPEN_CODE_API_KEY="your-key"

# 2. Agent 생성 (비대화형)
echo -e "claude-bot\nClaude AI\nopencode\nclaude-sonnet-4-5\n친절한 AI입니다" | ./bin/cnap agent create

# 3. Task 실행
./bin/cnap task create claude-bot task-001 --prompt "안녕하세요"
./bin/cnap task send task-001
```

### OpenAI 직접 API 사용

OpenAI 공식 API를 직접 호출합니다.

```bash
# 1. API 키 설정
export OPENAI_API_KEY="sk-proj-xxxxx"

# 2. Agent 생성
echo -e "gpt-bot\nGPT AI\nopenai\ngpt-5.1\n친절한 AI입니다" | ./bin/cnap agent create

# 3. Task 실행
./bin/cnap task create gpt-bot task-002 --prompt "Hello"
./bin/cnap task send task-002
```

### xAI Grok 사용

xAI의 Grok 코드 특화 모델을 사용합니다.

```bash
# 1. API 키 설정
export XAI_API_KEY="your-xai-key"

# 2. Agent 생성
echo -e "grok-bot\nGrok Code AI\nxai\ngrok-code\nYou are a coding assistant" | ./bin/cnap agent create

# 3. Task 실행
./bin/cnap task create grok-bot task-003 --prompt "Write a Python function"
./bin/cnap task send task-003
```

### 여러 프로바이더 동시 사용

하나의 시스템에서 여러 프로바이더를 동시에 사용할 수 있습니다.

```bash
# 모든 API 키 설정
export OPEN_CODE_API_KEY="your-opencode-key"
export OPENAI_API_KEY="sk-proj-xxx"
export XAI_API_KEY="your-xai-key"

# 각기 다른 프로바이더로 Agent 생성
echo -e "claude-bot\nClaude\nopencode\nclaude-sonnet-4-5\nClaude AI" | ./bin/cnap agent create
echo -e "gpt-bot\nGPT\nopenai\ngpt-5.1\nOpenAI" | ./bin/cnap agent create
echo -e "grok-bot\nGrok\nxai\ngrok-code\nGrok AI" | ./bin/cnap agent create

# Agent 목록 확인
./bin/cnap agent list
```

**출력:**
```
NAME          STATUS  MODEL                DESCRIPTION  CREATED
----          ------  -----                -----------  -------
claude-bot    active  claude-sonnet-4-5   Claude       2025-12-01 00:10
gpt-bot       active  gpt-5.1              OpenAI       2025-12-01 00:11
grok-bot      active  grok-code            Grok AI      2025-12-01 00:12
```

### Agent 수정 (프로바이더 변경)

기존 Agent의 프로바이더를 변경할 수 있습니다.

```bash
./bin/cnap agent edit my-assistant
```

**대화형 입력:**
```
설명 (현재: 개인 비서 AI): [Enter로 스킵]
프로바이더 (현재: opencode) [opencode/gemini/claude/openai/xai]: openai
모델 (현재: claude-sonnet-4-5): gpt-5.1
프롬프트 (현재: 당신은...): [Enter로 스킵]
```

---

## 멀티턴 대화 파이프라인

여러 메시지를 주고받는 대화형 Task 실행 방법입니다.

### Step 1: 프롬프트 없는 Task 생성

```bash
./bin/cnap task create my-assistant chat-001
```

### Step 2: 첫 번째 메시지 추가

```bash
./bin/cnap task add-message chat-001 "안녕하세요!"
```

**출력:**
```
✓ Task 'chat-001'에 메시지가 추가되었습니다.
```

### Step 3: 메시지 목록 확인

```bash
./bin/cnap task messages chat-001
```

**출력:**
```
INDEX  ROLE  FILE PATH                           CREATED
-----  ----  ---------                           -------
0      user  data/messages/chat-001/0000.json   2025-11-30 14:16
```

### Step 4: Task 실행

```bash
./bin/cnap task send chat-001
```

### Step 5: 추가 메시지 대화

AI 응답 후 계속 대화하려면:

```bash
# 상태를 pending으로 변경
./bin/cnap task update-status chat-001 pending

# 두 번째 메시지 추가
./bin/cnap task add-message chat-001 "날씨는 어때요?"

# 다시 실행
./bin/cnap task send chat-001
```

---

## 고급 사용법

### Task 취소

실행 중인 Task를 중단합니다:

```bash
./bin/cnap task cancel task-001
```

### Task 상태 직접 변경

```bash
./bin/cnap task update-status task-001 completed
```

**사용 가능한 상태:**
- `pending` - 대기 중
- `running` - 실행 중
- `completed` - 완료
- `failed` - 실패
- `canceled` - 취소됨

### Agent 수정

```bash
./bin/cnap agent edit my-assistant
```

**대화형 입력:**
```
설명 (현재: 개인 비서 AI): 고급 AI 비서
모델 (현재: gpt-4): [Enter로 스킵]
프롬프트 (현재: 당신은...): [Enter로 스킵]
```

### Agent 삭제

```bash
./bin/cnap agent delete my-assistant
```

**확인 프롬프트:**
```
Agent 'my-assistant'을(를) 삭제하시겠습니까? (y/N): y
✓ Agent 'my-assistant' 삭제 완료
```

### Task 목록 조회

특정 Agent의 모든 Task 확인:

```bash
./bin/cnap task list my-assistant
```

**출력:**
```
TASK ID   STATUS     CREATED           UPDATED
-------   ------     -------           -------
task-001  running    2025-11-30 23:16  2025-11-30 23:16
chat-001  pending    2025-11-30 23:17  2025-11-30 23:17
```

---

## 문제 해결

### API 키 관련 에러

#### "OPEN_CODE_API_KEY not set"

**원인**: OpenCode provider 사용 시 API 키가 없음

**해결:**
```bash
export OPEN_CODE_API_KEY="your-key"
```

#### "환경 변수 ANTHROPIC_API_KEY가 설정되어 있지 않습니다"

**원인**: Claude provider를 직접 사용하려 했지만 API 키가 없음

**해결 1**: OpenCode로 전환 (추천)
```bash
# OpenCode provider로 Claude 모델 사용
export OPEN_CODE_API_KEY="your-key"
# Agent 생성 시 provider를 opencode로 선택
```

**해결 2**: Claude API 키 설정 (추후 지원)
```bash
export ANTHROPIC_API_KEY="sk-ant-xxx"
# 현재는 claude provider 직접 API가 구현되지 않아 에러 발생
# opencode provider를 사용하세요
```

#### 다른 Provider API 키 에러

| Provider | 환경 변수 | 상태 |
|----------|----------|------|
| openai   | `OPENAI_API_KEY` | ✅ 지원 |
| xai      | `XAI_API_KEY` | ✅ 지원 |
| gemini   | `GEMINI_API_KEY` | ⏳ 추후 지원 |
| claude   | `ANTHROPIC_API_KEY` | ⏳ 추후 지원 |

### "database is locked" (SQLite)

**원인**: 동시 접근으로 인한 DB 잠금

**해결 1**: 다른 프로세스 종료
```bash
pkill cnap
```

**해결 2**: PostgreSQL 사용
```bash
export DATABASE_URL="postgres://cnap:cnap@localhost:5432/cnap?sslmode=disable"
```

### "agent not found" 에러

**원인**: 존재하지 않는 Agent 참조

**해결:**
```bash
# 등록된 Agent 목록 확인
./bin/cnap agent list
```

### "runner not found" 에러 발생하지 않음!

이전에는 별도 프로세스에서 `task send` 실행 시 이 에러가 발생했으나, **PR #59 이후 자동 해결됩니다**.

**내부 동작:**
- Task 실행 시 Runner가 없으면 자동으로 재생성
- CLI 단일 실행 프로세스에서도 정상 동작
- 로그에 "Runner not found, recreating..." 메시지 표시

---

## 주요 특징

### 🌐 멀티 프로바이더 지원 (PR #61)

**특징**: 5개 AI 프로바이더 선택 가능

**지원 프로바이더:**
- **OpenCode** (추천): 19개 모델 통합 API
- **OpenAI**: GPT 공식 API
- **xAI**: Grok 코드 특화
- **Claude/Gemini**: 추후 직접 API 지원 (현재 OpenCode로 사용 가능)

**장점:**
- Provider별 최적 모델 선택
- 비용/성능 트레이드오프 조정
- API 키 자동 관리
- 여러 Provider 동시 사용 가능

**예시:**
```bash
# OpenCode로 Claude 사용 (추천)
echo -e "bot1\nAI\nopencode\nclaude-sonnet-4-5\nAI" | ./bin/cnap agent create

# OpenAI 직접 사용
echo -e "bot2\nAI\nopenai\ngpt-5.1\nAI" | ./bin/cnap agent create

# xAI Grok 사용
echo -e "bot3\nAI\nxai\ngrok-code\nAI" | ./bin/cnap agent create
```

### 🎯 Runner 자동 재생성 (PR #59)

**문제**:
- 이전에는 `task create`와 `task send`를 별도 프로세스로 실행하면 실패
- RunnerManager가 프로세스 메모리에만 존재하여 프로세스 종료 시 사라짐

**해결**:
- `executeTask` 메서드에서 Runner가 없으면 Agent 정보로 자동 재생성
- Runner는 Stateless 설계이므로 재생성해도 기능 동일
- 모든 상태는 DB/파일에 저장되어 있음

**실제 동작:**
```bash
# 프로세스 1
./bin/cnap task create my-assistant task-001 --prompt "Hello"

# 프로세스 2 (완전히 새로운 프로세스)
./bin/cnap task send task-001  # ✅ 성공! (자동 재생성)
```

### 📁 메시지 파일 저장

메시지는 DB에 인덱스만 저장하고, 실제 내용은 JSON 파일로 저장:

```
data/
└── messages/
    └── task-001/
        ├── 0000.json  # 첫 번째 메시지
        ├── 0001.json  # 두 번째 메시지
        └── 0002.json  # 세 번째 메시지
```

**장점:**
- 대용량 메시지 처리 효율적
- 파일 시스템 기반 백업 용이
- DB 크기 절감

### 🗄️ 유연한 데이터베이스

**SQLite (기본값):**
```bash
# 별도 설정 없이 바로 사용
./bin/cnap agent create
# 데이터 위치: ./data/cnap.db
```

**PostgreSQL:**
```bash
export DATABASE_URL="postgres://cnap:cnap@localhost:5432/cnap?sslmode=disable"
./bin/cnap agent create
```

### 🔧 NFC 정규화

한글 입력이 자동으로 NFC 정규화되어 저장:
- macOS와 Linux 간 호환성 보장
- 파일명 충돌 방지
- 안정적인 검색 지원

---

## 완전한 예제 스크립트

### 기본 예제 (OpenCode)

```bash
#!/bin/bash
set -e

# 환경 설정
export OPEN_CODE_API_KEY="your-key"

# 1. Agent 생성 (Claude 모델)
echo -e "math-tutor\n수학 선생님\nopencode\nclaude-sonnet-4-5\n수학 문제를 풀어주는 선생님입니다." | ./bin/cnap agent create

# 2. Task 생성 및 실행
./bin/cnap task create math-tutor homework-001 --prompt "2의 10승은?"
./bin/cnap task send homework-001

# 3. 상태 확인
sleep 2
./bin/cnap task view homework-001

# 4. 멀티턴 대화
./bin/cnap task create math-tutor chat-001
./bin/cnap task add-message chat-001 "안녕하세요"
./bin/cnap task send chat-001

sleep 5

# 상태를 pending으로 변경 후 추가 메시지
./bin/cnap task update-status chat-001 pending
./bin/cnap task add-message chat-001 "미적분을 알려주세요"
./bin/cnap task send chat-001

# 5. 결과 확인
./bin/cnap task list math-tutor
./bin/cnap task messages chat-001

echo "✓ 전체 파이프라인 완료"
```

### 멀티 프로바이더 예제

```bash
#!/bin/bash
set -e

# 여러 Provider API 키 설정
export OPEN_CODE_API_KEY="your-opencode-key"
export OPENAI_API_KEY="sk-proj-xxx"
export XAI_API_KEY="your-xai-key"

# 1. 각기 다른 프로바이더로 Agent 생성
echo "=== Creating agents with different providers ==="

# OpenCode로 Claude 사용
echo -e "claude-bot\nClaude AI\nopencode\nclaude-sonnet-4-5\n친절한 Claude AI" | ./bin/cnap agent create

# OpenAI 직접 사용
echo -e "gpt-bot\nGPT AI\nopenai\ngpt-5.1\n친절한 GPT AI" | ./bin/cnap agent create

# xAI Grok 사용
echo -e "grok-bot\nGrok AI\nxai\ngrok-code\nCode-focused AI" | ./bin/cnap agent create

# 2. Agent 목록 확인
echo "=== Agent list ==="
./bin/cnap agent list

# 3. 각 Agent로 Task 실행
echo "=== Running tasks ==="

./bin/cnap task create claude-bot task-c1 --prompt "안녕하세요"
./bin/cnap task send task-c1

./bin/cnap task create gpt-bot task-g1 --prompt "Hello"
./bin/cnap task send task-g1

./bin/cnap task create grok-bot task-x1 --prompt "Write a Python function"
./bin/cnap task send task-x1

# 4. 결과 확인
sleep 3
echo "=== Task status ==="
./bin/cnap task view task-c1
./bin/cnap task view task-g1
./bin/cnap task view task-x1

echo "✓ 멀티 프로바이더 테스트 완료"
```

---

## 다음 단계

- [통합 테스트 가이드](./integration_testing.md) - 자동화된 테스트 실행
- [Controller CLI 상세 가이드](./controller-cli-guide.md) - 모든 명령어 설명
- [Docker 가이드](./docker-guide.md) - 컨테이너 환경에서 실행
- [Task Runner 실행 이슈](./task-runner-execution-issue.md) - Runner 자동 재생성 배경

---

## 참고 자료

### 관련 PR
- [PR #61](https://github.com/cnap-oss/app/pull/61) - 멀티 AI 프로바이더 지원 (최신)
- [PR #59](https://github.com/cnap-oss/app/pull/59) - Runner 자동 재생성 구현
- [PR #56](https://github.com/cnap-oss/app/pull/56) - Controller-RunnerManager 통합 테스트
- [PR #57](https://github.com/cnap-oss/app/pull/57) - CLI 통합 테스트 스크립트

### 프로젝트 개요
- [CLAUDE.md](../CLAUDE.md) - 프로젝트 전체 구조
- [README.md](../README.md) - 프로젝트 소개
