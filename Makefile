.PHONY: help build image test coverage lint fmt clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-10s %s\n", $$1, $$2}'

build: ## Build the yeet binary
	go build -o yeet ./cmd/yeet

image: ## Build the yeet container image locally with ko
	ko build --local --platform=linux/$$(go env GOARCH) ./cmd/yeet

test: ## Run tests
	go test ./...

coverage: ## Run tests with repo-wide coverage output
	go test ./... -covermode=atomic -coverpkg=./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

lint: ## Run linter
	golangci-lint run ./...

fmt: ## Format code
	golangci-lint fmt ./...

clean: ## Remove build artifacts
	rm -f yeet coverage.out
