BINARY_NAME ?= go-starter
BIN_DIR ?= bin
PKG ?= ./...
CMD ?= ./cmd/app
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/codingseeker/go-starter/internal/buildinfo.Version=$(VERSION) \
	-X github.com/codingseeker/go-starter/internal/buildinfo.Commit=$(COMMIT) \
	-X github.com/codingseeker/go-starter/internal/buildinfo.BuildDate=$(BUILD_DATE)

.DEFAULT_GOAL := help

.PHONY: help fmt fmt-check lint test test-race test-short cover vet tidy vuln verify build run serve version clean docker-build docker-run

help: ## Show this help
	@echo "go-starter make targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

fmt: ## Format the code
	gofmt -s -w .

fmt-check: ## Fail when the code is not formatted
	@files=$$(gofmt -s -l .); \
	if [ -n "$$files" ]; then \
		echo "These files need gofmt:"; echo "$$files"; exit 1; \
	fi

lint: ## Run golangci-lint, or vet plus a format check when it is unavailable
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, falling back to go vet and gofmt"; \
		$(MAKE) vet fmt-check; \
	fi

vet: ## Run go vet
	go vet $(PKG)

test: ## Run the unit and integration tests
	go test -count=1 $(PKG)

test-race: ## Run the tests with the race detector
	go test -race -count=1 $(PKG)

test-short: ## Run the tests without the slow binary integration tests
	go test -short -count=1 $(PKG)

cover: ## Run the tests and report coverage
	go test -count=1 -coverprofile=coverage.out -covermode=atomic $(PKG)
	@go tool cover -func=coverage.out | tail -n 1

tidy: ## Tidy go.mod and go.sum
	go mod tidy

vuln: ## Scan the dependency graph for known vulnerabilities
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	elif [ -x "$$(go env GOPATH)/bin/govulncheck" ]; then \
		"$$(go env GOPATH)/bin/govulncheck" ./...; \
	else \
		echo "govulncheck not installed:"; \
		echo "  go install golang.org/x/vuln/cmd/govulncheck@v1.1.4"; \
		exit 1; \
	fi

verify: fmt-check vet lint test-race build vuln ## Run every static check, the race tests, a build and a vulnerability scan

build: ## Build the binary into $(BIN_DIR)
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) $(CMD)

run: ## Run the core application
	go run $(CMD) run

serve: ## Run the HTTP API server
	go run $(CMD) serve

version: ## Print the version metadata compiled into the binary
	@$(MAKE) --no-print-directory build >/dev/null
	@$(BIN_DIR)/$(BINARY_NAME) version

clean: ## Remove build and coverage artifacts
	rm -rf $(BIN_DIR) coverage.out

docker-build: ## Build the container image
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t $(BINARY_NAME):$(VERSION) .

docker-run: ## Run the container image
	docker run --rm -p 8080:8080 $(BINARY_NAME):$(VERSION) serve
