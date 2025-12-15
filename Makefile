.PHONY: build clean test fmt lint deps run help run-local test-local dev clean-db

# Binary name
BINARY_NAME=cnap
BUILD_DIR=bin
GO=go

# Build variables
VERSION?=$(shell git describe --tags --always --dirty)
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Local development variables
SQLITE_DB_DIR=./data
SQLITE_DB_FILE=$(SQLITE_DB_DIR)/cnap.db

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/cnap

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(BINARY_NAME)
	@$(GO) clean

test: ## Run tests
	@echo "Running tests..."
	$(GO) test -v -race -coverprofile=coverage.out ./...

test-coverage: test ## Run tests with coverage report
	@echo "Generating coverage report..."
	$(GO) tool cover -html=coverage.out -o coverage.html

fmt: ## Format code
	@echo "Formatting code..."
	$(GO) fmt ./...
	@which goimports > /dev/null 2>&1 || (echo "Installing goimports..." && $(GO) install golang.org/x/tools/cmd/goimports@latest)
	goimports -w .

lint: ## Run linter
	@echo "Running linter..."
	@which golangci-lint > /dev/null 2>&1 || (echo "Installing golangci-lint..." && $(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GO) mod download
	$(GO) mod tidy

deps-update: ## Update dependencies
	@echo "Updating dependencies..."
	$(GO) get -u ./...
	$(GO) mod tidy

run: build ## Build and run the application
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME)

install: ## Install the binary to GOPATH/bin
	@echo "Installing $(BINARY_NAME)..."
	$(GO) install $(LDFLAGS) ./cmd/cnap

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):$(VERSION) .

vendor: ## Create vendor directory
	@echo "Vendoring dependencies..."
	$(GO) mod vendor

check: fmt lint test ## Run format, lint, and test

# Local development commands (SQLite, no Docker required)

run-local: ## Run with SQLite (no Docker needed)
	@echo "Running $(BINARY_NAME) with SQLite..."
	@mkdir -p $(SQLITE_DB_DIR)
	@unset DATABASE_URL && $(GO) run $(LDFLAGS) ./cmd/cnap start

dev: build ## Build and run locally with SQLite
	@echo "Running $(BINARY_NAME) locally with SQLite..."
	@mkdir -p $(SQLITE_DB_DIR)
	@if [ -f .env ]; then \
		echo "Loading environment from .env file..."; \
		export $$(cat .env | grep -v '^#' | xargs) && unset DATABASE_URL && ./$(BUILD_DIR)/$(BINARY_NAME) start; \
	else \
		unset DATABASE_URL && ./$(BUILD_DIR)/$(BINARY_NAME) start; \
	fi

test-local: ## Run tests with in-memory SQLite
	@echo "Running tests with in-memory SQLite..."
	@SQLITE_DATABASE=":memory:" $(GO) test -v -race -coverprofile=coverage.out ./...

clean-db: ## Remove local SQLite database
	@echo "Removing local SQLite database..."
	@rm -f $(SQLITE_DB_FILE)
	@echo "Database removed: $(SQLITE_DB_FILE)"