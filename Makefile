APP ?= memoryd
MCP_APP ?= agent-memory-mcp
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -X github.com/mordor-forge/agent-memory/internal/version.Name=agent-memory         	-X github.com/mordor-forge/agent-memory/internal/version.Version=$(VERSION)         	-X github.com/mordor-forge/agent-memory/internal/version.Commit=$(COMMIT)         	-X github.com/mordor-forge/agent-memory/internal/version.Date=$(DATE)

.PHONY: fmt test integration build build-mcp run migrate doctor dev-up dev-down dev-reset dev-env

fmt:
	@files="$$(find cmd internal -name '*.go' -print)";         	if [ -n "$$files" ]; then gofmt -w $$files; fi

test:
	go test ./...

integration:
	go test -tags=integration ./internal/store/cockroach/...

build:
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/$(APP) ./cmd/memoryd

build-mcp:
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/$(MCP_APP) ./cmd/agent-memory-mcp

run:
	go run ./cmd/memoryd serve

migrate:
	go run ./cmd/memoryd migrate

doctor:
	go run ./cmd/memoryd doctor

dev-up:
	./scripts/dev-cockroach-up.sh

dev-down:
	./scripts/dev-cockroach-down.sh

dev-reset:
	./scripts/dev-cockroach-reset.sh

dev-env:
	./scripts/dev-env.sh
