#!/bin/bash
set -e

# CNAP 설치 스크립트
# macOS 및 Linux 지원

# 색상 정의
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 로그 함수
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 배너 출력
print_banner() {
    echo ""
    echo "╔═══════════════════════════════════════╗"
    echo "║   CNAP Agent 설치 스크립트           ║"
    echo "╚═══════════════════════════════════════╝"
    echo ""
}

# OS 및 아키텍처 감지
detect_platform() {
    local os=$(uname -s | tr '[:upper:]' '[:lower:]')
    local arch=$(uname -m)

    case "$os" in
        linux*)
            OS="linux"
            ;;
        darwin*)
            OS="darwin"
            ;;
        *)
            log_error "지원하지 않는 운영체제입니다: $os"
            log_error "macOS 또는 Linux에서만 설치가 가능합니다."
            exit 1
            ;;
    esac

    case "$arch" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            log_error "지원하지 않는 아키텍처입니다: $arch"
            log_error "amd64 또는 arm64만 지원됩니다."
            exit 1
            ;;
    esac

    log_info "감지된 플랫폼: ${OS}-${ARCH}"
}

# GitHub 저장소 정보
GITHUB_REPO="cnap-oss/app"
INSTALL_DIR="$HOME/.cnap"
BIN_DIR="$INSTALL_DIR/bin"
CONFIG_DIR="$INSTALL_DIR"

# 최신 릴리스 버전 가져오기
get_latest_version() {
    log_info "최신 릴리스 버전 확인 중..."
    
    LATEST_VERSION=$(curl -sL "https://api.github.com/repos/$GITHUB_REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    
    if [ -z "$LATEST_VERSION" ]; then
        log_error "최신 릴리스 버전을 가져올 수 없습니다."
        log_error "GitHub API 접근에 문제가 있거나 릴리스가 없습니다."
        exit 1
    fi
    
    log_success "최신 버전: $LATEST_VERSION"
}

# 디렉토리 생성
create_directories() {
    log_info "설치 디렉토리 생성 중..."
    
    mkdir -p "$BIN_DIR"
    mkdir -p "$CONFIG_DIR"
    
    log_success "디렉토리 생성 완료: $INSTALL_DIR"
}

# 바이너리 다운로드
download_binary() {
    log_info "CNAP 바이너리 다운로드 중..."
    
    BINARY_NAME="cnap-${LATEST_VERSION}-${OS}-${ARCH}"
    DOWNLOAD_URL="https://github.com/$GITHUB_REPO/releases/download/${LATEST_VERSION}/${BINARY_NAME}"
    
    log_info "다운로드 URL: $DOWNLOAD_URL"
    
    if ! curl -fsSL -o "$BIN_DIR/cnap" "$DOWNLOAD_URL"; then
        log_error "바이너리 다운로드에 실패했습니다."
        log_error "URL을 확인해주세요: $DOWNLOAD_URL"
        exit 1
    fi
    
    chmod +x "$BIN_DIR/cnap"
    log_success "바이너리 다운로드 및 실행 권한 설정 완료"
}

# 체크섬 검증
verify_checksum() {
    log_info "체크섬 검증 중..."
    
    CHECKSUM_URL="https://github.com/$GITHUB_REPO/releases/download/${LATEST_VERSION}/${BINARY_NAME}.sha256"
    
    if curl -fsSL -o /tmp/cnap.sha256 "$CHECKSUM_URL" 2>/dev/null; then
        cd "$BIN_DIR"
        if command -v shasum &> /dev/null; then
            shasum -a 256 -c /tmp/cnap.sha256
        elif command -v sha256sum &> /dev/null; then
            sha256sum -c /tmp/cnap.sha256
        else
            log_warn "체크섬 검증 도구를 찾을 수 없습니다. 검증을 건너뜁니다."
            return
        fi
        log_success "체크섬 검증 완료"
        rm /tmp/cnap.sha256
    else
        log_warn "체크섬 파일을 다운로드할 수 없습니다. 검증을 건너뜁니다."
    fi
}

# Docker 설치 확인
check_docker() {
    log_info "Docker 설치 확인 중..."
    
    if command -v docker &> /dev/null; then
        log_success "Docker가 이미 설치되어 있습니다."
        
        # Docker 실행 확인
        if docker ps &> /dev/null; then
            log_success "Docker가 정상적으로 실행 중입니다."
        else
            log_warn "Docker가 설치되어 있지만 실행되고 있지 않습니다."
            log_warn "Docker를 시작해주세요: sudo systemctl start docker (Linux) 또는 Docker Desktop 실행 (macOS)"
        fi
        return 0
    fi
    
    log_warn "Docker가 설치되어 있지 않습니다."
    echo ""
    echo "CNAP은 작업 실행을 위해 Docker가 필요합니다."
    echo ""
    
    if [ "$OS" = "darwin" ]; then
        echo "macOS에서 Docker 설치 방법:"
        echo "1. Docker Desktop 다운로드: https://www.docker.com/products/docker-desktop"
        echo "2. 또는 Homebrew 사용: brew install --cask docker"
    else
        echo "Linux에서 Docker 설치 방법:"
        echo "1. 공식 설치 스크립트 사용:"
        echo "   curl -fsSL https://get.docker.com -o get-docker.sh"
        echo "   sudo sh get-docker.sh"
        echo ""
        echo "2. 사용자를 docker 그룹에 추가:"
        echo "   sudo usermod -aG docker \$USER"
        echo "   newgrp docker"
    fi
    
    echo ""
    read -p "Docker를 자동으로 설치하시겠습니까? (y/N): " -n 1 -r
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        install_docker
    else
        log_warn "Docker 설치를 건너뜁니다. 나중에 수동으로 설치해주세요."
    fi
}

# Docker 자동 설치
install_docker() {
    if [ "$OS" = "darwin" ]; then
        if command -v brew &> /dev/null; then
            log_info "Homebrew를 사용하여 Docker Desktop 설치 중..."
            brew install --cask docker
            log_success "Docker Desktop 설치 완료"
            log_warn "Docker Desktop 앱을 실행해주세요."
        else
            log_error "Homebrew가 설치되어 있지 않습니다."
            log_error "https://www.docker.com/products/docker-desktop 에서 직접 다운로드해주세요."
        fi
    else
        log_info "Docker 설치 중..."
        
        if [ ! -f get-docker.sh ]; then
            curl -fsSL https://get.docker.com -o get-docker.sh
        fi
        
        sudo sh get-docker.sh
        
        log_info "현재 사용자를 docker 그룹에 추가 중..."
        sudo usermod -aG docker "$USER"
        
        log_success "Docker 설치 완료"
        log_warn "변경 사항을 적용하려면 로그아웃 후 다시 로그인하거나 'newgrp docker'를 실행하세요."
        
        rm -f get-docker.sh
    fi
}

# 설정 파일 생성
create_config() {
    log_info "설정 파일 생성 중..."
    
    CONFIG_FILE="$CONFIG_DIR/config.yml"
    
    if [ -f "$CONFIG_FILE" ]; then
        log_warn "설정 파일이 이미 존재합니다: $CONFIG_FILE"
        read -p "기존 설정 파일을 덮어쓰시겠습니까? (y/N): " -n 1 -r
        echo ""
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "설정 파일 생성을 건너뜁니다."
            return
        fi
    fi
    
    echo ""
    echo "════════════════════════════════════════"
    echo "  Discord Bot Token 설정"
    echo "════════════════════════════════════════"
    echo ""
    echo "Discord Bot Token 발급 방법:"
    echo "1. https://discord.com/developers/applications 접속"
    echo "2. 'New Application' 클릭하여 새 애플리케이션 생성"
    echo "3. 'Bot' 메뉴로 이동하여 'Add Bot' 클릭"
    echo "4. 'Reset Token' 클릭하여 토큰 복사"
    echo "5. 'Privileged Gateway Intents'에서 다음 항목 활성화:"
    echo "   - MESSAGE CONTENT INTENT"
    echo "   - SERVER MEMBERS INTENT"
    echo "6. 'OAuth2' > 'URL Generator'에서 다음 권한 선택:"
    echo "   - Scopes: bot, applications.commands"
    echo "   - Bot Permissions: Send Messages, Read Messages, etc."
    echo "7. 생성된 URL로 봇을 서버에 초대"
    echo ""
    read -p "Discord Bot Token을 입력하세요: " DISCORD_TOKEN
    
    echo ""
    echo "════════════════════════════════════════"
    echo "  OpenCode Zen API Key 설정"
    echo "════════════════════════════════════════"
    echo ""
    echo "OpenCode Zen API Key 발급 방법:"
    echo "1. https://opencode.ai/auth 접속"
    echo "2. 회원가입 또는 로그인"
    echo "3. 대시보드에서 'API Keys' 메뉴로 이동"
    echo "4. 'Create New API Key' 클릭"
    echo "5. API Key 이름 설정 및 생성"
    echo "6. 생성된 API Key 복사 (한 번만 표시됨)"
    echo ""
    read -p "OpenCode Zen API Key를 입력하세요: " OPENCODE_API_KEY
    
    # config.yml 생성
    cat > "$CONFIG_FILE" << EOF
# CNAP Configuration File
# Generated by install.sh

# Application Configuration
app:
  env: production
  log_level: info

# Database Configuration
database:
  # SQLite를 기본으로 사용 (별도 설정 불필요)
  dsn: ""
  log_level: info
  max_idle_conns: 5
  max_open_conns: 20
  conn_max_lifetime: 30m
  skip_default_txn: true
  prepare_stmt: false
  disable_automatic_ping: false

# Discord Bot Configuration
discord:
  token: "${DISCORD_TOKEN}"

# API Keys Configuration
api_keys:
  opencode: "${OPENCODE_API_KEY}"
  anthropic: ""
  openai: ""

# Runner Configuration
runner:
  # Docker 이미지는 기본값 사용
  image: ""
  workspace_dir: ""

# Directory Configuration
directory:
  workspace_base_dir: ""
  sqlite_database: ""
EOF
    
    chmod 600 "$CONFIG_FILE"
    log_success "설정 파일 생성 완료: $CONFIG_FILE"
}

# Shell 프로파일 업데이트 안내
setup_shell_profile() {
    echo ""
    echo "════════════════════════════════════════"
    echo "  Shell 프로파일 설정"
    echo "════════════════════════════════════════"
    echo ""
    log_info "CNAP을 PATH에 추가하려면 아래 명령어를 실행하세요:"
    echo ""
    
    # 현재 쉘 감지
    CURRENT_SHELL=$(basename "$SHELL")
    
    PATH_EXPORT="export PATH=\"\$HOME/.cnap/bin:\$PATH\""
    
    case "$CURRENT_SHELL" in
        bash)
            echo "# Bash 사용자:"
            echo "echo '$PATH_EXPORT' >> ~/.bashrc"
            echo "source ~/.bashrc"
            ;;
        zsh)
            echo "# Zsh 사용자:"
            echo "echo '$PATH_EXPORT' >> ~/.zshrc"
            echo "source ~/.zshrc"
            ;;
        fish)
            echo "# Fish 사용자:"
            echo "set -Ux fish_user_paths \$HOME/.cnap/bin \$fish_user_paths"
            ;;
        *)
            echo "# 기타 쉘:"
            echo "적절한 프로파일 파일에 다음을 추가하세요:"
            echo "$PATH_EXPORT"
            ;;
    esac
    
    echo ""
    echo "또는 모든 쉘에 대해:"
    echo "  Bash:  echo '$PATH_EXPORT' >> ~/.bashrc && source ~/.bashrc"
    echo "  Zsh:   echo '$PATH_EXPORT' >> ~/.zshrc && source ~/.zshrc"
    echo "  Fish:  set -Ux fish_user_paths \$HOME/.cnap/bin \$fish_user_paths"
    echo ""
    
    read -p "현재 쉘($CURRENT_SHELL)의 프로파일을 자동으로 업데이트하시겠습니까? (y/N): " -n 1 -r
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        case "$CURRENT_SHELL" in
            bash)
                echo "$PATH_EXPORT" >> ~/.bashrc
                log_success "~/.bashrc 업데이트 완료"
                ;;
            zsh)
                echo "$PATH_EXPORT" >> ~/.zshrc
                log_success "~/.zshrc 업데이트 완료"
                ;;
            fish)
                fish -c "set -Ux fish_user_paths \$HOME/.cnap/bin \$fish_user_paths"
                log_success "Fish 경로 업데이트 완료"
                ;;
            *)
                log_warn "자동 업데이트는 bash, zsh, fish만 지원됩니다."
                ;;
        esac
    fi
}

# 설치 완료 메시지
print_completion() {
    echo ""
    echo "╔═══════════════════════════════════════╗"
    echo "║   CNAP 설치 완료!                    ║"
    echo "╚═══════════════════════════════════════╝"
    echo ""
    log_success "설치 위치: $BIN_DIR/cnap"
    log_success "설정 파일: $CONFIG_DIR/config.yml"
    echo ""
    echo "다음 단계:"
    echo "1. 새 터미널을 열거나 쉘 프로파일을 다시 로드하세요"
    echo "2. 'cnap --version'을 실행하여 설치를 확인하세요"
    echo "3. 'cnap --help'를 실행하여 사용 가능한 명령어를 확인하세요"
    echo ""
    echo "시작하기:"
    echo "  cnap agent create      # 새 에이전트 생성"
    echo "  cnap task create       # 작업 생성"
    echo "  cnap task send         # 작업 실행"
    echo ""
    echo "문서: https://cnap-oss.github.io/"
    echo ""
}

# 메인 실행
main() {
    print_banner
    detect_platform
    get_latest_version
    create_directories
    download_binary
    verify_checksum
    check_docker
    create_config
    setup_shell_profile
    print_completion
}

# 스크립트 실행
main