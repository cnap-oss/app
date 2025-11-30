# CLI 통합 테스트 가이드

## 개요

`scripts/integration_test.sh`는 Controller-RunnerManager 연동의 전체 플로우를 검증하는 CLI 통합 테스트 스크립트입니다.

## 테스트 시나리오

### 시나리오 1: 기본 Agent/Task 라이프사이클
기본적인 Agent 및 Task CRUD 작업과 실행 흐름을 검증합니다.

**테스트 단계:**
1. Agent 생성
2. Agent 목록 확인
3. Agent 상세 조회
4. Task 생성
5. Task 목록 확인
6. Task 실행
7. Task 상태 확인 (polling)
8. Agent 삭제

### 시나리오 2: 멀티턴 대화
여러 메시지를 주고받는 대화 시나리오를 검증합니다.

**테스트 단계:**
1. Agent 및 Task 생성
2. 메시지 추가 (실행 전)
3. 메시지 목록 확인
4. Task 실행
5. 결과 확인

### 시나리오 3: Task 취소
실행 중인 Task를 취소하는 기능을 검증합니다.

**테스트 단계:**
1. 긴 작업 생성 및 실행
2. 실행 중 취소
3. 취소 상태 확인

### 시나리오 4: 에러 처리
다양한 에러 상황을 검증합니다.

**테스트 단계:**
1. 존재하지 않는 Agent로 Task 생성 시도
2. 중복 실행 방지 확인
3. 빈 프롬프트 실행 시도

## 사전 준비

### 1. 환경 변수 설정

```bash
# PostgreSQL 데이터베이스 URL (필수)
export DATABASE_URL="postgres://cnap:cnap@localhost:5432/cnap_test?sslmode=disable"

# OpenCode API Key (필수, 실제 API 호출 시)
export OPEN_CODE_API_KEY="your-api-key-here"
```

### 2. PostgreSQL 실행

#### Docker Compose 사용 (권장)

```bash
# PostgreSQL 및 애플리케이션 컨테이너 시작
docker compose -f docker/docker-compose.yml up -d

# 테스트 데이터베이스 생성 (첫 실행 시)
docker exec -it cnap-unified psql -U cnap -c "CREATE DATABASE cnap_test;"
```

#### 로컬 PostgreSQL 사용

```bash
# 테스트 데이터베이스 생성
createdb -U postgres cnap_test

# 사용자 및 권한 설정
psql -U postgres -c "CREATE USER cnap WITH PASSWORD 'cnap';"
psql -U postgres -c "GRANT ALL PRIVILEGES ON DATABASE cnap_test TO cnap;"
```

### 3. 바이너리 빌드

```bash
make build
```

## 실행 방법

### 전체 테스트 실행

```bash
./scripts/integration_test.sh
```

### 예상 출력

```
========================================
CLI 통합 테스트 시작
========================================
========================================
테스트 환경 설정
========================================
✓ 환경 설정 완료
========================================
시나리오 1: 기본 Agent/Task 라이프사이클
========================================
ℹ 1. Agent 생성
✓ Agent 생성 성공
ℹ 2. Agent 목록 확인
✓ Agent 목록 확인 성공
...
========================================
테스트 결과
========================================
✓ 모든 통합 테스트 통과 (4/4)
```

## 트러블슈팅

### 데이터베이스 연결 오류

```
Error: failed to connect to database
```

**해결책:**
1. PostgreSQL이 실행 중인지 확인
2. DATABASE_URL 환경 변수 확인
3. 네트워크 연결 및 포트 확인

### API Key 오류

```
Error: OPEN_CODE_API_KEY not set
```

**해결책:**
```bash
export OPEN_CODE_API_KEY="your-api-key"
```

### 바이너리 없음

```
Error: ./bin/cnap not found
```

**해결책:**
```bash
make build
```

### 테스트 데이터 충돌

이전 테스트 실행으로 인한 데이터가 남아있을 경우:

```bash
# 테스트 데이터베이스 초기화
psql -U cnap -d cnap_test -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
```

## CI/CD 통합

### GitHub Actions 예제

```yaml
name: Integration Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest

    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_USER: cnap
          POSTGRES_PASSWORD: cnap
          POSTGRES_DB: cnap_test
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432

    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.23'

      - name: Build
        run: make build

      - name: Run Integration Tests
        env:
          DATABASE_URL: postgres://cnap:cnap@localhost:5432/cnap_test?sslmode=disable
          OPEN_CODE_API_KEY: ${{ secrets.OPEN_CODE_API_KEY }}
        run: ./scripts/integration_test.sh
```

## 주의사항

1. **API 비용**: 실제 OpenCode API를 호출하므로 비용이 발생할 수 있습니다. 테스트 환경에서는 mock API를 사용하는 것을 권장합니다.

2. **데이터베이스 격리**: 프로덕션 데이터베이스와 분리된 테스트 전용 데이터베이스를 사용하세요.

3. **타임아웃**: Task 실행 시간이 길어질 수 있으므로 충분한 타임아웃을 설정하세요.

4. **병렬 실행**: 현재 스크립트는 순차 실행을 가정합니다. 병렬 테스트 실행 시 ID 충돌에 주의하세요.

## 향후 개선 사항

- [ ] Mock OpenCode API 서버 통합
- [ ] 병렬 테스트 실행 지원
- [ ] 더 상세한 검증 로직 추가
- [ ] 성능 측정 및 벤치마킹
- [ ] 테스트 데이터 자동 정리
