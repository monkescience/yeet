BINARY  := yeet
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || printf 'dev')
GO_TEST_COVERAGE_VERSION := v2.19.0 # renovate: datasource=go depName=github.com/vladopajic/go-test-coverage/v2

.PHONY: help build snapshot image test test-unit test-blackbox coverage check-coverage lint fmt generate clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-10s %s\n", $$1, $$2}'

build: ## Build the yeet binary
	go build -trimpath -ldflags "-s -w -X $(shell go list -m)/internal/build.version=$(VERSION)" -o bin/$(BINARY) ./cmd/$(BINARY)

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

check-coverage: ## Check coverage against the thresholds in .testcoverage.yml
	go run github.com/vladopajic/go-test-coverage/v2@$(GO_TEST_COVERAGE_VERSION) --config=./.testcoverage.yml $(COVERAGE_FLAGS)

lint: ## Run linter
	golangci-lint run ./...

fmt: ## Format code
	golangci-lint fmt ./...

generate: ## Run code generators (no-op when no //go:generate directives)
	go generate ./...

clean: ## Remove build artifacts
	rm -rf bin/ coverage/
