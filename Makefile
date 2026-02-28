.PHONY: build test lint fmt clean

build:
	go build -o yeet ./cmd/yeet

test:
	go test ./...

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...

clean:
	rm -f yeet
