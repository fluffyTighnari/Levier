# Levier — build, test, and lint entry points.
# Copyright 2026 The Levier Authors.

SHELL := /bin/sh
GO ?= go
PYTHON ?= $(if $(wildcard execution/harness/.venv/bin/python),execution/harness/.venv/bin/python,$(if $(wildcard execution/harness/.venv/Scripts/python.exe),execution/harness/.venv/Scripts/python.exe,python3))
MCP_PYTHON ?= $(if $(wildcard execution/mcp_runtime/.venv/bin/python),execution/mcp_runtime/.venv/bin/python,$(if $(wildcard execution/mcp_runtime/.venv/Scripts/python.exe),execution/mcp_runtime/.venv/Scripts/python.exe,python3))
PYTHONPATH_SEP ?= $(if $(filter Windows_NT,$(OS)),;,:)
CONTRACT_PYTHONPATH := execution/agent_core/src$(PYTHONPATH_SEP)execution/harness/src/gen$(PYTHONPATH_SEP)execution/harness/src
BUF ?= $(shell command -v buf 2>/dev/null || printf '%s\n' "$(HOME)/go/bin/buf")
PROTOC_BIN ?= $(HOME)/go/bin

# Optional local-only overrides (git-ignored).
-include .env.local
export
BIN_DIR := bin
GO_FILES := $(shell find . -name '*.go' -not -path './workflow_engine/*' -not -path './vendor/*')
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/fluffyTighnari/levier/internal/version.Version=$(VERSION) \
	-X github.com/fluffyTighnari/levier/internal/version.Commit=$(COMMIT) \
	-X github.com/fluffyTighnari/levier/internal/version.Built=$(BUILD_TIME)

.PHONY: all build test lint fmt fmt-check vet run clean dev-up dev-down migrate proto proto-check proto-lint contract-check harness-test mcp-runtime-test sdk-test image-harness image-mcp-runtime counter-agent counter-agent-test http-smoke retrofit-e2e mcp-e2e deps-check structure-check scripts-check size-report daemon-check agent-core-test mvp-demo mvp-demo-react fault-matrix secrets-check docker-up docker-down docker-smoke docker-logs docker-reset release-dist

all: build

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/levier ./cmd/levier

release-dist:
	VERSION="$(VERSION)" COMMIT="$(COMMIT)" BUILT="$(BUILD_TIME)" sh scripts/build-release.sh

test:
	$(GO) test ./...

lint: fmt-check vet secrets-check structure-check scripts-check

fmt-check:
	@files="$$(gofmt -l $(GO_FILES))"; \
	if [ -n "$$files" ]; then \
		echo "gofmt required for:"; echo "$$files"; exit 1; \
	fi

vet:
	$(GO) vet ./...

fmt:
	gofmt -w $(GO_FILES)

run:
	$(GO) run ./cmd/levier

migrate:
	$(GO) run ./cmd/migrate -config levier.example.yaml

clean:
	rm -rf $(BIN_DIR)

dev-up:
	docker compose up -d postgres

dev-down:
	docker compose down

proto:
	@command -v $(BUF) >/dev/null 2>&1 || { echo "buf is not installed"; exit 1; }
	PATH="$(PROTOC_BIN):$$PATH" $(BUF) generate

proto-check: proto
	@git diff --exit-code -- gen execution/harness/src/gen

proto-lint:
	$(BUF) lint
	$(BUF) build -o /dev/null
	$(BUF) breaking --against '.git#branch=main'

contract-check: proto-lint
	$(GO) test ./internal/protocol -run TestContractGolden -v
	PYTHONPATH="$(CONTRACT_PYTHONPATH)" $(PYTHON) tests/contract/test_contract.py -v

harness-test:
	PYTHONPATH="$(CONTRACT_PYTHONPATH)" $(PYTHON) -m unittest discover -s execution/harness/tests -v

mcp-runtime-test:
	PYTHONPATH="execution/mcp_runtime/src" $(MCP_PYTHON) -m unittest discover -s execution/mcp_runtime/tests -v

sdk-test:
	PYTHONPATH="sdk/python/src" $(PYTHON) -m unittest discover -s sdk/python/tests -v

image-harness:
	docker build -f deploy/harness.Dockerfile -t levier-harness:dev .

image-mcp-runtime:
	docker build -f deploy/mcp-runtime.Dockerfile -t levier-mcp-runtime:dev .

counter-agent:
	$(PYTHON) examples/counter_agent/main.py

http-smoke:
	$(PYTHON) examples/http_client/client.py

retrofit-e2e:
	./tests/e2e/retrofit_e2e.sh

mcp-e2e:
	./tests/e2e/mcp_e2e.sh

deps-check:
	./tests/checks/check_deps.sh

structure-check:
	bash tests/checks/check_structure.sh

scripts-check:
	@for script in scripts/*.sh; do sh -n "$$script"; done

size-report:
	./tests/checks/size_report.sh

daemon-check:
	go run ./cmd/levier daemon check

agent-core-test:
	PYTHONPATH=execution/agent_core/src python3 -m unittest discover -s execution/agent_core/tests -v

mvp-demo:
	./tests/e2e/mvp_demo.sh echo

mvp-demo-react:
	./tests/e2e/mvp_demo.sh react

fault-matrix:
	python3 tests/e2e/fault_matrix.py

secrets-check:
	python3 tests/checks/check_secrets.py

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

docker-smoke: docker-up
	docker compose --profile smoke run --rm smoke

docker-logs:
	docker compose logs -f

docker-reset:
	docker compose down -v

counter-agent-test:
	cd examples/counter_agent && python3 -m unittest discover -s . -p 'test_*.py' -v
