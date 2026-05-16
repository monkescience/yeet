BINARY  := yeet
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || printf 'dev')
LDFLAGS := -X $(shell go list -m)/internal/build.version=$(VERSION)

.PHONY: help build snapshot image test test-unit test-blackbox coverage lint fmt clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-10s %s\n", $$1, $$2}'

build: ## Build the yeet binary
	go build -trimpath -ldflags "-s -w $(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

snapshot: ## Build a local release snapshot with goreleaser
	goreleaser release --snapshot --clean

image: ## Build the yeet container image locally with ko
	VERSION=$(VERSION) ko build --local --platform=linux/$$(go env GOARCH) ./cmd/$(BINARY)

test: ## Run all tests (unit + blackbox)
	go test -race -count=1 ./...

test-unit: ## Run unit tests only (skips blackbox via -short)
	go test -short -race ./...

test-blackbox: ## Run end-to-end blackbox tests against the compiled binary
	go test -race ./tests/...

coverage: ## Generate HTML coverage report from coverage/coverage.out
	go tool cover -html=coverage/coverage.out -o coverage/coverage.html

lint: ## Run linter
	golangci-lint run ./...

fmt: ## Format code
	golangci-lint fmt ./...

clean: ## Remove build artifacts
	rm -rf bin/ coverage/
