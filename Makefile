# Middleware — build, test, and lint entry points.
# Copyright 2026 The Middleware Authors.

SHELL := /bin/sh
GO ?= go

# Optional local-only overrides (git-ignored).
-include .env.local
export
BIN_DIR := bin
GO_FILES := $(shell find . -name '*.go' -not -path './workflow_engine/*' -not -path './vendor/*')

.PHONY: all build test lint fmt fmt-check vet run clean dev-up dev-down migrate proto harness-test

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
	@command -v buf >/dev/null 2>&1 || { echo "buf is not installed"; exit 1; }
	buf generate

harness-test:
	PYTHONPATH=harness/src python3 -m unittest discover -s harness/tests -v
