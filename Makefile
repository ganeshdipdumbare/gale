.PHONY: build test lint clean install

BINARY := gale
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o bin/$(BINARY) ./cmd/gale

install: build
	cp bin/$(BINARY) $(GOPATH)/bin/$(BINARY)

test:
	go test ./... -count=1 -race -timeout 120s

lint:
	go vet ./...
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || true

clean:
	rm -rf bin/

help:
	@echo "Targets: build, install, test, lint, clean"
