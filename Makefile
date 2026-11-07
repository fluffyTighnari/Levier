# Middleware — build, test, and lint entry points.
# Copyright 2026 The Middleware Authors.

SHELL := /bin/sh
GO ?= go
PYTHON ?= $(if $(wildcard harness/.venv/bin/python),harness/.venv/bin/python,python3)
MCP_PYTHON ?= $(if $(wildcard mcp_runtime/.venv/bin/python),mcp_runtime/.venv/bin/python,python3)
BUF ?= $(HOME)/go/bin/buf
PROTOC_BIN ?= $(HOME)/go/bin

# Optional local-only overrides (git-ignored).
-include .env.local
export
BIN_DIR := bin
GO_FILES := $(shell find . -name '*.go' -not -path './workflow_engine/*' -not -path './vendor/*')

.PHONY: all build test lint fmt fmt-check vet run clean dev-up dev-down migrate proto proto-check proto-lint contract-check harness-test mcp-runtime-test image-harness image-mcp-runtime e2e-echo m1-e2e

all: build

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/middleware ./cmd/middleware

test:
	$(GO) test ./...

lint: fmt-check vet

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
	$(GO) run ./cmd/middleware

migrate:
	$(GO) run ./cmd/migrate -config config.example.yaml

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
	@git diff --exit-code -- gen harness/src/gen

proto-lint:
	$(BUF) lint
	$(BUF) build -o /dev/null
	$(BUF) breaking --against '.git#branch=main'

contract-check: proto-lint
	$(GO) test ./internal/protocol -run TestContractGolden -v
	PYTHONPATH=harness/src/gen:harness/src $(PYTHON) tests/contract/test_contract.py -v

harness-test:
	PYTHONPATH=harness/src/gen:harness/src $(PYTHON) -m unittest discover -s harness/tests -v

mcp-runtime-test:
	PYTHONPATH=mcp_runtime/src $(MCP_PYTHON) -m unittest discover -s mcp_runtime/tests -v

image-harness:
	docker build -f deploy/harness.Dockerfile -t middleware-harness:dev .

image-mcp-runtime:
	docker build -f deploy/mcp-runtime.Dockerfile -t middleware-mcp-runtime:dev .

e2e-echo: build
	./scripts/e2e_echo.sh

m1-e2e: build
	./scripts/m1_e2e.sh
