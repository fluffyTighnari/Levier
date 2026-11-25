# Levier — build, test, and lint entry points.
# Copyright 2026 The Levier Authors.

SHELL := /bin/sh
GO ?= go
PYTHON ?= $(if $(wildcard execution/react_runtime/.venv/bin/python),execution/react_runtime/.venv/bin/python,$(if $(wildcard execution/react_runtime/.venv/Scripts/python.exe),execution/react_runtime/.venv/Scripts/python.exe,$(if $(filter Windows_NT,$(OS)),python,python3)))
MCP_PYTHON ?= $(if $(wildcard execution/mcp_runtime/.venv/bin/python),execution/mcp_runtime/.venv/bin/python,$(if $(wildcard execution/mcp_runtime/.venv/Scripts/python.exe),execution/mcp_runtime/.venv/Scripts/python.exe,$(if $(filter Windows_NT,$(OS)),python,python3)))
PYTHONPATH_SEP ?= $(if $(filter Windows_NT,$(OS)),;,:)
CONTRACT_PYTHONPATH := execution/agent_core/src$(PYTHONPATH_SEP)execution/runtime_sdk/src$(PYTHONPATH_SEP)execution/react_runtime/src
BUF ?= $(shell command -v buf 2>/dev/null || printf '%s\n' "$(HOME)/go/bin/buf")
PROTOC_BIN ?= $(HOME)/go/bin

# Optional local-only overrides (git-ignored).
-include .env.local
export
BIN_DIR := bin
EXE := $(if $(filter Windows_NT,$(OS)),.exe,)
LEVIER_BIN := $(BIN_DIR)/levier$(EXE)
GO_FILES := $(shell git ls-files '*.go' --cached --others --exclude-standard 2>/dev/null | grep -v '^workflow_engine/' | grep -v '^vendor/' 2>/dev/null)
ifeq ($(strip $(GO_FILES)),)
GO_FILES := $(shell find . -name '*.go' -not -path './workflow_engine/*' -not -path './vendor/*')
endif
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/fluffyTighnari/levier/internal/version.Version=$(VERSION) \
	-X github.com/fluffyTighnari/levier/internal/version.Commit=$(COMMIT) \
	-X github.com/fluffyTighnari/levier/internal/version.Built=$(BUILD_TIME)

.PHONY: all build install wheels test lint fmt fmt-check vet run clean dev-up dev-down migrate proto proto-check proto-lint contract-check react-runtime-test runtime-sdk-test langgraph-runtime-test mcp-runtime-test sdk-test image-runtime image-mcp-runtime counter-agent counter-agent-test http-smoke retrofit-e2e mcp-e2e global-e2e deps-check structure-check scripts-check size-report daemon-check agent-core-test mvp-demo mvp-demo-react fault-matrix secrets-check docker-up docker-down docker-smoke docker-logs docker-reset release-dist

PREFIX ?= $(HOME)/.local/bin

all: build

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(LEVIER_BIN) ./cmd/levier

install: build
	@mkdir -p "$(PREFIX)"
	@cp "$(LEVIER_BIN)" "$(PREFIX)/levier$(EXE)"
	@chmod 0755 "$(PREFIX)/levier$(EXE)" 2>/dev/null || true
	@printf 'installed: %s\n' "$(PREFIX)/levier$(EXE)"
	@case ":$$PATH:" in *":$(PREFIX):"*) ;; *) printf 'PATH hint:\n  export PATH="%s:$$PATH"\n' "$(PREFIX)" ;; esac

wheels:
	PYTHON="$(PYTHON)" sh scripts/build-wheels.sh

release-dist:
	VERSION="$(VERSION)" COMMIT="$(COMMIT)" BUILT="$(BUILD_TIME)" sh scripts/build-release.sh
	PYTHON="$(PYTHON)" sh scripts/build-wheels.sh

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
	$(GO) run ./cmd/levier serve

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
	PATH="$(PROTOC_BIN):$$PATH" $(BUF) generate --template buf.gen.python.yaml --path proto/middleware/runtime/v1

proto-check: proto
	@git diff --exit-code -- gen execution/runtime_sdk/src

proto-lint:
	$(BUF) lint
	$(BUF) build -o /dev/null
	$(BUF) breaking --against '.git#branch=main'

contract-check: proto-lint
	$(GO) test ./internal/protocol -run TestContractGolden -v
	PYTHONPATH="$(CONTRACT_PYTHONPATH)" $(PYTHON) tests/contract/test_contract.py -v

react-runtime-test:
	PYTHONPATH="$(CONTRACT_PYTHONPATH)" $(PYTHON) -m unittest discover -s execution/react_runtime/tests -v

runtime-sdk-test:
	PYTHONPATH="execution/agent_core/src$(PYTHONPATH_SEP)execution/runtime_sdk/src" $(PYTHON) -m unittest discover -s execution/runtime_sdk/tests -v

langgraph-runtime-test:
	PYTHONPATH="execution/runtime_sdk/src$(PYTHONPATH_SEP)execution/langgraph_runtime/src" $(PYTHON) -m unittest discover -s execution/langgraph_runtime/tests -v

mcp-runtime-test:
	PYTHONPATH="execution/mcp_runtime/src" $(MCP_PYTHON) -m unittest discover -s execution/mcp_runtime/tests -v

sdk-test:
	PYTHONPATH="sdk/python/src$(PYTHONPATH_SEP)sdk/levier/src" $(PYTHON) -m unittest discover -s sdk/python/tests -v
	PYTHONPATH="sdk/levier/src$(PYTHONPATH_SEP)sdk/python/src$(PYTHONPATH_SEP)execution/runtime_sdk/src" $(PYTHON) -m unittest discover -s sdk/levier/tests -v

image-runtime:
	docker build -f deploy/runtime.Dockerfile -t levier-runtime:dev .

image-mcp-runtime:
	docker build -f deploy/mcp-runtime.Dockerfile -t levier-mcp-runtime:dev .

counter-agent:
	PYTHONPATH="sdk/levier/src$(PYTHONPATH_SEP)sdk/python/src" $(PYTHON) examples/counter_agent/app.py

http-smoke:
	$(PYTHON) examples/http_client/client.py

retrofit-e2e:
	./tests/e2e/retrofit_e2e.sh

mcp-e2e:
	./tests/e2e/mcp_e2e.sh

global-e2e:
	sh tests/e2e/global_cli_e2e.sh

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
	docker compose up --build -d postgres levier mcp-runtime react-runtime echo-runtime
	@mkdir -p data/agent
	@if [ ! -f data/agent/credentials.json ]; then \
		for i in $$(seq 1 100); do \
			curl -fsS http://127.0.0.1:8080/healthz >/dev/null 2>&1 && break; \
			sleep 0.2; \
		done; \
		LEVIER_ADMIN_TOKEN="$${LEVIER_ADMIN_TOKEN:-admin-change-me}" \
			$(GO) run ./cmd/levier app init \
			--name "$${LEVIER_APP_NAME:-counter-agent}" \
			--server http://127.0.0.1:8080 \
			--credentials-out data/agent/credentials.json \
			--rotate --yes; \
	fi
	docker compose up -d agent

docker-down:
	docker compose down

docker-smoke: docker-up
	docker compose --profile smoke run --rm smoke

docker-logs:
	docker compose logs -f

docker-reset:
	docker compose down -v

counter-agent-test:
	PYTHONPATH="sdk/levier/src$(PYTHONPATH_SEP)sdk/python/src" $(PYTHON) -m unittest discover -s examples/counter_agent/tests -p 'test_*.py' -v
