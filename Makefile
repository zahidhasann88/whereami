# whereami build helpers. Run `make help` for the list of targets.

BINARY  := whereami
PKG     := ./cmd/whereami
VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
GOBIN   ?= $(shell go env GOPATH)/bin

.DEFAULT_GOAL := build

.PHONY: help build test vet lint fmt fmt-check install clean cross check

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build bin/whereami
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(PKG)

test: ## Run unit and integration tests
	go test -count=1 ./...

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint (v1.64.8, see CONTRIBUTING.md)
	golangci-lint run ./...

fmt: ## Format all Go files
	gofmt -s -w cmd internal

fmt-check: ## Fail if any Go file is not gofmt-clean
	@test -z "$$(gofmt -s -l cmd internal)" || (echo "gofmt needed:"; gofmt -s -l cmd internal; exit 1)

check: fmt-check vet test ## Formatting, vet and tests (no linter binary required)

install: ## Install whereami into GOBIN
	go install -trimpath -ldflags "$(LDFLAGS)" $(PKG)

cross: ## Cross-compile for every release platform into dist/
	@for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do \
		os=$${t%/*}; arch=$${t#*/}; ext=""; [ "$$os" = windows ] && ext=.exe; \
		echo "building $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" \
			-o dist/$(BINARY)_$${os}_$${arch}$$ext $(PKG) || exit 1; \
	done

clean: ## Remove build outputs
	rm -rf bin dist coverage.out
