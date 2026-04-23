.PHONY: build install test test-race lint fmt vet tidy clean snapshot help

BIN  := bitgit
PKG  := ./cmd/bitgit

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.Date=$(DATE)

build: ## Build local binary into ./$(BIN)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)

install: ## go install bitgit into $GOBIN
	go install -trimpath -ldflags "$(LDFLAGS)" $(PKG)

test: ## Run tests
	go test ./...

test-race: ## Run tests with race detector + coverage
	go test -race -coverprofile=coverage.txt -covermode=atomic ./...

fmt: ## gofmt -s -w
	gofmt -s -w .

vet: ## go vet
	go vet ./...

lint: vet ## golangci-lint run
	golangci-lint run

tidy: ## go mod tidy
	go mod tidy

snapshot: ## Local goreleaser dry-run (no publish)
	goreleaser release --snapshot --clean

clean: ## Remove build artifacts
	rm -rf dist $(BIN) coverage.txt coverage.html

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := build
