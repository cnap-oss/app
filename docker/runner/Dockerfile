# =============================================================================
# Stage 1: Builder - OpenCode 설치
# =============================================================================
FROM node:24-slim AS builder

# 빌드 의존성 설치
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    && rm -rf /var/lib/apt/lists/*

# OpenCode CLI 전역 설치
RUN npm install -g opencode-ai

# =============================================================================
# Stage 2: Runtime - 최종 이미지
# =============================================================================
FROM node:24-slim AS runtime

# 메타데이터
LABEL maintainer="cnap-team"
LABEL description="CNAP Runner Container with OpenCode Server"
LABEL version="1.0.0"

# 런타임 의존성 설치
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# 비루트 사용자 생성 (보안)
# GID/UID 충돌 방지를 위해 높은 번호 사용
RUN groupadd --gid 10000 opencode \
    && useradd --uid 10000 --gid opencode --shell /bin/bash --create-home opencode

# OpenCode CLI 복사
COPY --from=builder /usr/local/lib/node_modules /usr/local/lib/node_modules
COPY --from=builder /usr/local/bin/opencode /usr/local/bin/opencode

# 작업 디렉토리 설정
WORKDIR /workspace

# 권한 설정
RUN chown -R opencode:opencode /workspace

# 비루트 사용자로 전환
USER opencode

# 환경 변수
ENV NODE_ENV=production
ENV OPENCODE_DATA_DIR=/workspace/.opencode

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:3000/health || exit 1

# OpenCode Server 포트
EXPOSE 3000

# 엔트리포인트
ENTRYPOINT ["opencode", "serve"]
CMD ["--port", "3000", "--hostname", "0.0.0.0"]
