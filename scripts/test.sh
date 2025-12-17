#!/bin/bash

# 테스트 실행 스크립트
# 단위 테스트, 통합 테스트, 벤치마크 테스트, 커버리지 리포트 생성

set -e

# 색상 코드
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

# 기본 설정
COVERAGE_DIR="coverage"
COVERAGE_FILE="${COVERAGE_DIR}/coverage.out"
COVERAGE_HTML="${COVERAGE_DIR}/coverage.html"
TEST_TIMEOUT="10m"
RACE_DETECTOR="-race"

# 커버리지 디렉토리 생성
mkdir -p "${COVERAGE_DIR}"

# 명령행 인자 파싱
RUN_UNIT=false
RUN_INTEGRATION=false
RUN_BENCHMARK=false
RUN_COVERAGE=false
VERBOSE=false
SHORT=false

show_usage() {
    cat << EOF
사용법: $0 [옵션]

옵션:
    -u, --unit              단위 테스트 실행
    -i, --integration       통합 테스트 실행
    -b, --benchmark         벤치마크 테스트 실행
    -c, --coverage          커버리지 리포트 생성
    -a, --all              모든 테스트 실행 (기본값)
    -v, --verbose          자세한 출력
    -s, --short            짧은 테스트만 실행
    --no-race              race detector 비활성화
    -h, --help             도움말 표시

예제:
    $0 --unit --coverage          # 단위 테스트 및 커버리지
    $0 --integration              # 통합 테스트만
    $0 --all                      # 모든 테스트
    $0 --benchmark --verbose      # 벤치마크 (자세히)

EOF
}

# 인자가 없으면 모든 테스트 실행
if [ $# -eq 0 ]; then
    RUN_UNIT=true
    RUN_INTEGRATION=true
    RUN_BENCHMARK=false
    RUN_COVERAGE=true
fi

while [[ $# -gt 0 ]]; do
    case $1 in
        -u|--unit)
            RUN_UNIT=true
            shift
            ;;
        -i|--integration)
            RUN_INTEGRATION=true
            shift
            ;;
        -b|--benchmark)
            RUN_BENCHMARK=true
            shift
            ;;
        -c|--coverage)
            RUN_COVERAGE=true
            shift
            ;;
        -a|--all)
            RUN_UNIT=true
            RUN_INTEGRATION=true
            RUN_BENCHMARK=true
            RUN_COVERAGE=true
            shift
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -s|--short)
            SHORT=true
            shift
            ;;
        --no-race)
            RACE_DETECTOR=""
            shift
            ;;
        -h|--help)
            show_usage
            exit 0
            ;;
        *)
            print_error "알 수 없는 옵션: $1"
            show_usage
            exit 1
            ;;
    esac
done

# 테스트 옵션 구성
TEST_FLAGS=""
if [ "$VERBOSE" = true ]; then
    TEST_FLAGS="$TEST_FLAGS -v"
fi
if [ "$SHORT" = true ]; then
    TEST_FLAGS="$TEST_FLAGS -short"
fi

# 단위 테스트 실행
run_unit_tests() {
    print_header "단위 테스트 실행"
    
    if [ "$RUN_COVERAGE" = true ]; then
        print_info "커버리지 수집과 함께 단위 테스트 실행..."
        go test $TEST_FLAGS $RACE_DETECTOR \
            -timeout="${TEST_TIMEOUT}" \
            -coverprofile="${COVERAGE_FILE}" \
            -covermode=atomic \
            ./... \
            -tags=unit \
            || { print_error "단위 테스트 실패"; exit 1; }
    else
        print_info "단위 테스트 실행..."
        go test $TEST_FLAGS $RACE_DETECTOR \
            -timeout="${TEST_TIMEOUT}" \
            ./... \
            -tags=unit \
            || { print_error "단위 테스트 실패"; exit 1; }
    fi
    
    print_success "단위 테스트 성공"
}

# 통합 테스트 실행
run_integration_tests() {
    print_header "통합 테스트 실행"
    
    print_info "통합 테스트 환경 확인..."
    
    # 통합 테스트 환경 변수 설정
    export CNAP_ENV=integration
    export CNAP_LOG_LEVEL=info
    
    print_info "통합 테스트 실행..."
    go test $TEST_FLAGS \
        -timeout="${TEST_TIMEOUT}" \
        ./tests/integration/... \
        -tags=integration \
        || { print_error "통합 테스트 실패"; exit 1; }
    
    print_success "통합 테스트 성공"
}

# 벤치마크 테스트 실행
run_benchmark_tests() {
    print_header "벤치마크 테스트 실행"
    
    print_info "벤치마크 실행..."
    go test $TEST_FLAGS \
        -bench=. \
        -benchmem \
        -benchtime=5s \
        -run=^$ \
        ./... \
        > "${COVERAGE_DIR}/benchmark.txt" \
        || { print_error "벤치마크 실패"; exit 1; }
    
    print_success "벤치마크 성공"
    print_info "벤치마크 결과: ${COVERAGE_DIR}/benchmark.txt"
    
    if [ "$VERBOSE" = true ]; then
        cat "${COVERAGE_DIR}/benchmark.txt"
    fi
}

# 커버리지 리포트 생성
generate_coverage_report() {
    print_header "커버리지 리포트 생성"
    
    if [ ! -f "${COVERAGE_FILE}" ]; then
        print_error "커버리지 파일을 찾을 수 없음: ${COVERAGE_FILE}"
        return 1
    fi
    
    print_info "커버리지 요약 생성..."
    go tool cover -func="${COVERAGE_FILE}" | tee "${COVERAGE_DIR}/coverage.txt"
    
    print_info "HTML 커버리지 리포트 생성..."
    go tool cover -html="${COVERAGE_FILE}" -o "${COVERAGE_HTML}"
    
    # 총 커버리지 계산
    TOTAL_COVERAGE=$(go tool cover -func="${COVERAGE_FILE}" | grep total | awk '{print $3}')
    
    print_success "커버리지 리포트 생성 완료"
    print_info "HTML 리포트: ${COVERAGE_HTML}"
    print_info "총 커버리지: ${TOTAL_COVERAGE}"
    
    # 커버리지 임계값 확인 (예: 70%)
    THRESHOLD=70.0
    COVERAGE_NUM=$(echo $TOTAL_COVERAGE | sed 's/%//')
    
    if (( $(echo "$COVERAGE_NUM < $THRESHOLD" | bc -l) )); then
        print_error "커버리지가 임계값(${THRESHOLD}%)보다 낮음: ${TOTAL_COVERAGE}"
        exit 1
    else
        print_success "커버리지가 임계값(${THRESHOLD}%)을 만족함: ${TOTAL_COVERAGE}"
    fi
}

# 테스트 전 환경 확인
check_environment() {
    print_header "테스트 환경 확인"
    
    # Go 버전 확인
    GO_VERSION=$(go version)
    print_info "Go 버전: ${GO_VERSION}"
    
    # 의존성 확인
    print_info "의존성 확인..."
    go mod verify || { print_error "의존성 검증 실패"; exit 1; }
    
    # 의존성 다운로드
    print_info "의존성 다운로드..."
    go mod download || { print_error "의존성 다운로드 실패"; exit 1; }
    
    print_success "환경 확인 완료"
}

# 메인 실행 흐름
main() {
    print_header "Go 프로젝트 테스트 시작"
    
    # 환경 확인
    check_environment
    
    # 단위 테스트
    if [ "$RUN_UNIT" = true ]; then
        run_unit_tests
    fi
    
    # 통합 테스트
    if [ "$RUN_INTEGRATION" = true ]; then
        run_integration_tests
    fi
    
    # 벤치마크 테스트
    if [ "$RUN_BENCHMARK" = true ]; then
        run_benchmark_tests
    fi
    
    # 커버리지 리포트
    if [ "$RUN_COVERAGE" = true ] && [ -f "${COVERAGE_FILE}" ]; then
        generate_coverage_report
    fi
    
    print_header "모든 테스트 완료"
    print_success "테스트가 성공적으로 완료되었습니다!"
}

# 스크립트 실행
main