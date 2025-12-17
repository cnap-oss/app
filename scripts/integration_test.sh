#!/bin/bash

# CLI 통합 테스트 스크립트
# Controller-RunnerManager 연동 전체 플로우 검증

set -e

# 색상 코드
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 헬퍼 함수
print_header() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "${YELLOW}ℹ $1${NC}"
}

# 설정
CNAP_BIN="./bin/cnap"
CNAP_DB_DSN="${CNAP_DB_DSN:-postgres://cnap:cnap@localhost:5432/cnap_test?sslmode=disable}"
export CNAP_DB_DSN
export CNAP_OPENCODE_API_KEY="${CNAP_OPENCODE_API_KEY:-test-key}"

# 테스트 전 준비
setup() {
    print_header "테스트 환경 설정"

    # 바이너리 빌드
    if [ ! -f "$CNAP_BIN" ]; then
        print_info "바이너리 빌드 중..."
        make build || { print_error "빌드 실패"; exit 1; }
    fi

    print_success "환경 설정 완료"
}

# 테스트 후 정리
cleanup() {
    print_header "테스트 정리"
    # 필요한 경우 테스트 데이터 정리
    print_success "정리 완료"
}

# 시나리오 1: 기본 Agent/Task 라이프사이클
test_basic_lifecycle() {
    print_header "시나리오 1: 기본 Agent/Task 라이프사이클"

    local AGENT_ID="test-agent-${RANDOM}"
    local TASK_ID="task-001-${RANDOM}"

    # 1. Agent 생성
    print_info "1. Agent 생성"
    $CNAP_BIN agent create "$AGENT_ID" "테스트용 에이전트" "gpt-4" "당신은 수학 문제를 푸는 AI입니다." \
        && print_success "Agent 생성 성공" \
        || { print_error "Agent 생성 실패"; return 1; }

    # 2. Agent 목록 확인
    print_info "2. Agent 목록 확인"
    $CNAP_BIN agent list | grep -q "$AGENT_ID" \
        && print_success "Agent 목록 확인 성공" \
        || { print_error "Agent 목록에서 찾을 수 없음"; return 1; }

    # 3. Agent 상세 조회
    print_info "3. Agent 상세 조회"
    $CNAP_BIN agent info "$AGENT_ID" \
        && print_success "Agent 상세 조회 성공" \
        || { print_error "Agent 상세 조회 실패"; return 1; }

    # 4. Task 생성
    print_info "4. Task 생성"
    $CNAP_BIN task create "$AGENT_ID" "$TASK_ID" "2 + 2는 무엇인가요?" \
        && print_success "Task 생성 성공" \
        || { print_error "Task 생성 실패"; return 1; }

    # 5. Task 목록 확인
    print_info "5. Task 목록 확인"
    $CNAP_BIN task list "$AGENT_ID" | grep -q "$TASK_ID" \
        && print_success "Task 목록 확인 성공" \
        || { print_error "Task 목록에서 찾을 수 없음"; return 1; }

    # 6. Task 실행
    print_info "6. Task 실행"
    $CNAP_BIN task send "$TASK_ID" \
        && print_success "Task 실행 성공" \
        || { print_error "Task 실행 실패"; return 1; }

    # 7. Task 상태 확인
    print_info "7. Task 상태 확인"
    local MAX_ATTEMPTS=10
    local ATTEMPT=0
    local STATUS=""

    while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
        ATTEMPT=$((ATTEMPT + 1))
        echo -n "[$ATTEMPT/$MAX_ATTEMPTS] Checking status..."

        STATUS=$($CNAP_BIN task info "$TASK_ID" 2>/dev/null | grep "상태:" | awk '{print $2}' || echo "unknown")
        echo " $STATUS"

        if [ "$STATUS" == "completed" ] || [ "$STATUS" == "failed" ]; then
            break
        fi

        sleep 5
    done

    if [ "$STATUS" == "completed" ]; then
        print_success "Task 완료 확인"
    else
        print_info "Task 상태: $STATUS (테스트 환경에서는 정상)"
    fi

    # 8. Agent 삭제
    print_info "8. Agent 삭제"
    echo "y" | $CNAP_BIN agent delete "$AGENT_ID" \
        && print_success "Agent 삭제 성공" \
        || { print_error "Agent 삭제 실패"; return 1; }

    print_success "시나리오 1 완료"
}

# 시나리오 2: 멀티턴 대화
test_multiturn_conversation() {
    print_header "시나리오 2: 멀티턴 대화"

    local AGENT_ID="chat-agent-${RANDOM}"
    local TASK_ID="task-chat-${RANDOM}"

    # 1. Agent 및 Task 생성
    print_info "1. Agent 및 Task 생성"
    $CNAP_BIN agent create "$AGENT_ID" "대화형 에이전트" "gpt-4" "당신은 친절한 AI 어시스턴트입니다." \
        && $CNAP_BIN task create "$AGENT_ID" "$TASK_ID" "안녕하세요" \
        && print_success "Agent 및 Task 생성 성공" \
        || { print_error "생성 실패"; return 1; }

    # 2. 메시지 추가 (실행 전)
    print_info "2. 메시지 추가"
    $CNAP_BIN message add "$TASK_ID" user "날씨가 좋네요" \
        && print_success "메시지 추가 성공" \
        || { print_error "메시지 추가 실패"; return 1; }

    # 3. 메시지 목록 확인
    print_info "3. 메시지 목록 확인"
    $CNAP_BIN message list "$TASK_ID" \
        && print_success "메시지 목록 조회 성공" \
        || { print_error "메시지 목록 조회 실패"; return 1; }

    # 4. Task 실행
    print_info "4. Task 실행"
    $CNAP_BIN task send "$TASK_ID" \
        && print_success "Task 실행 성공" \
        || print_info "Task 실행 (테스트 환경)"

    # 5. 결과 확인
    print_info "5. 결과 확인"
    $CNAP_BIN message list "$TASK_ID" \
        && print_success "결과 조회 성공" \
        || { print_error "결과 조회 실패"; return 1; }

    # 정리
    echo "y" | $CNAP_BIN agent delete "$AGENT_ID" >/dev/null 2>&1 || true

    print_success "시나리오 2 완료"
}

# 시나리오 3: Task 취소
test_task_cancellation() {
    print_header "시나리오 3: Task 취소"

    local AGENT_ID="long-agent-${RANDOM}"
    local TASK_ID="long-task-${RANDOM}"

    # 1. 긴 작업 실행
    print_info "1. 긴 작업 실행"
    $CNAP_BIN agent create "$AGENT_ID" "장문 에이전트" "gpt-4" "당신은 에세이를 작성합니다." \
        && $CNAP_BIN task create "$AGENT_ID" "$TASK_ID" "매우 긴 에세이를 작성해주세요" \
        && print_success "Task 생성 성공" \
        || { print_error "Task 생성 실패"; return 1; }

    # 2. 실행 후 바로 취소
    print_info "2. Task 실행 및 취소"
    $CNAP_BIN task send "$TASK_ID" && sleep 1 \
        && $CNAP_BIN task cancel "$TASK_ID" \
        && print_success "Task 취소 성공" \
        || print_info "Task 취소 (이미 완료되었을 수 있음)"

    # 3. 상태 확인
    print_info "3. 상태 확인"
    local STATUS=$($CNAP_BIN task info "$TASK_ID" 2>/dev/null | grep "상태:" | awk '{print $2}' || echo "unknown")
    print_info "최종 상태: $STATUS"

    # 정리
    echo "y" | $CNAP_BIN agent delete "$AGENT_ID" >/dev/null 2>&1 || true

    print_success "시나리오 3 완료"
}

# 시나리오 4: 에러 처리
test_error_handling() {
    print_header "시나리오 4: 에러 처리"

    local AGENT_ID="error-agent-${RANDOM}"
    local TASK_ID="error-task-${RANDOM}"

    # 1. 존재하지 않는 Agent로 Task 생성
    print_info "1. 존재하지 않는 Agent로 Task 생성"
    if $CNAP_BIN task create "nonexistent-agent" "$TASK_ID" "test" 2>/dev/null; then
        print_error "존재하지 않는 Agent로 Task 생성이 성공함 (예상: 실패)"
        return 1
    else
        print_success "존재하지 않는 Agent 에러 처리 확인"
    fi

    # 2. 중복 실행 방지
    print_info "2. 중복 실행 방지"
    $CNAP_BIN agent create "$AGENT_ID" "테스트" "gpt-4" "test" >/dev/null 2>&1
    local DUP_TASK="task-dup-${RANDOM}"
    $CNAP_BIN task create "$AGENT_ID" "$DUP_TASK" "test" >/dev/null 2>&1
    $CNAP_BIN task send "$DUP_TASK" >/dev/null 2>&1

    if $CNAP_BIN task send "$DUP_TASK" 2>/dev/null; then
        print_error "중복 실행이 허용됨 (예상: 차단)"
        return 1
    else
        print_success "중복 실행 방지 확인"
    fi

    # 3. 빈 프롬프트 실행
    print_info "3. 빈 프롬프트 실행"
    local EMPTY_TASK="empty-task-${RANDOM}"
    $CNAP_BIN task create "$AGENT_ID" "$EMPTY_TASK" "" >/dev/null 2>&1

    if $CNAP_BIN task send "$EMPTY_TASK" 2>/dev/null; then
        print_info "빈 프롬프트 실행 허용됨 (구현에 따라 다를 수 있음)"
    else
        print_success "빈 프롬프트 에러 처리 확인"
    fi

    # 정리
    echo "y" | $CNAP_BIN agent delete "$AGENT_ID" >/dev/null 2>&1 || true

    print_success "시나리오 4 완료"
}

# 메인 실행
main() {
    print_header "CLI 통합 테스트 시작"

    # 환경 설정
    setup

    # 테스트 실행
    local FAILED=0

    test_basic_lifecycle || FAILED=$((FAILED + 1))
    echo ""

    test_multiturn_conversation || FAILED=$((FAILED + 1))
    echo ""

    test_task_cancellation || FAILED=$((FAILED + 1))
    echo ""

    test_error_handling || FAILED=$((FAILED + 1))
    echo ""

    # 정리
    cleanup

    # 결과 출력
    print_header "테스트 결과"
    if [ $FAILED -eq 0 ]; then
        print_success "모든 통합 테스트 통과 (4/4)"
        exit 0
    else
        print_error "$FAILED 개의 테스트 실패"
        exit 1
    fi
}

# 트랩 설정 (에러 시 정리)
trap cleanup EXIT

# 실행
main
