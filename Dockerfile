# Build stage
FROM golang:1.24-alpine AS builder

# Install necessary build tools
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o cnap \
    ./cmd/cnap

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies only
RUN apk --no-cache add \
    ca-certificates \
    tzdata

# Create non-root user
RUN addgroup -g 1000 cnap && \
    adduser -D -u 1000 -G cnap cnap

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/cnap .

# Copy configs if they exist
COPY --from=builder /build/configs ./configs

# Create data directory for SQLite
RUN mkdir -p /app/data && \
    chown -R cnap:cnap /app

# Switch to non-root user
USER cnap

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD ["/app/cnap", "health"]

# Run the application
ENTRYPOINT ["/app/cnap"]
CMD ["start"]
