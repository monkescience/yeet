VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || printf 'dev')
LDFLAGS := -X github.com/monkescience/yeet/internal/build.version=$(VERSION)

.PHONY: help build snapshot image test test-unit test-blackbox coverage coverage-html lint fmt clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-10s %s\n", $$1, $$2}'

build: ## Build the yeet binary
	go build -ldflags "$(LDFLAGS)" -o yeet ./cmd/yeet

snapshot: ## Build a local release snapshot with goreleaser
	goreleaser release --snapshot --clean

image: ## Build the yeet container image locally with ko
	VERSION=$(VERSION) ko build --local --platform=linux/$$(go env GOARCH) ./cmd/yeet

test: ## Run all tests (unit + blackbox)
	go test ./...

test-unit: ## Run unit tests only (skips blackbox via -short)
	go test -short ./...

test-blackbox: ## Run end-to-end blackbox tests against the compiled binary
	go test ./tests/...

coverage: ## Run tests with repo-wide coverage output (merges unit + blackbox subprocess coverage)
	rm -rf coverage && mkdir -p coverage
	go test ./... -covermode=atomic -coverpkg=./... -coverprofile=coverage/unit.out
	if [ -s coverage/process.out ]; then \
		printf 'mode: atomic\n' > coverage.out && \
		tail -n +2 coverage/unit.out >> coverage.out && \
		tail -n +2 coverage/process.out >> coverage.out; \
	else \
		cp coverage/unit.out coverage.out; \
	fi
	go tool cover -func=coverage.out

coverage-html: coverage ## Generate an HTML coverage report
	go tool cover -html=coverage.out -o coverage.html

lint: ## Run linter
	golangci-lint run ./...

fmt: ## Format code
	golangci-lint fmt ./...

clean: ## Remove build artifacts
	rm -f yeet coverage.out coverage.html
	rm -rf coverage
