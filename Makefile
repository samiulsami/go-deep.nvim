.PHONY: build install test-go test-lua test lint fmt ci ci-go ci-nvim clean release help

GO_DIR   := go
BIN_DIR  := bin
BINARY   := go-deep
BIN_PATH := $(BIN_DIR)/$(BINARY)

GOLANGCI_LINT_VERSION ?= v2.5.0
GO_BIN := $(or $(shell go env GOBIN),$(shell go env GOPATH)/bin)
GOLANGCI_LINT := $(GO_BIN)/golangci-lint
GOLANGCI_LINT_CONFIG := ../.golangci.yml

help:
	@echo "Usage: make [target]"
	@echo "Targets:"
	@echo "  build       Build the go-deep binary."
	@echo "  install     Install the go-deep binary to your Go bin directory."
	@echo "  test-go     Run Go tests."
	@echo "  test-lua    Run Lua tests using Neovim."
	@echo "  test        Run all tests (Go and Lua)."
	@echo "  lint        Run linters on the Go code."
	@echo "  fmt         Format the Go code using golangci-lint."
	@echo "  ci          Run all checks (lint, test, build) for CI."
	@echo "  ci-go       Run Go checks (lint, test, build) for CI."
	@echo "  ci-nvim     Run Neovim tests for CI."
	@echo "  release     Build the release artifact."
	@echo "  clean       Remove built binaries and temporary files."


$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build: $(BIN_DIR)
	cd $(GO_DIR) && go build -o ../$(BIN_PATH) .

install: build
	mkdir -p "$(GO_BIN)"
	cp $(BIN_PATH) "$(GO_BIN)/$(BINARY)"

test-go:
	cd $(GO_DIR) && go test ./...

test-lua: build
	nvim --headless -u tests/minimal_init.lua \
		-c "luafile tests/rpc_test.lua" \
		-c "qa!"

test: test-go test-lua

lint:
	@if ! command -v "$(GOLANGCI_LINT)" >/dev/null 2>&1; then \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi
	cd $(GO_DIR) && "$(GOLANGCI_LINT)" run --config $(GOLANGCI_LINT_CONFIG) ./...

fmt:
	@if ! command -v "$(GOLANGCI_LINT)" >/dev/null 2>&1; then \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi
	cd $(GO_DIR) && "$(GOLANGCI_LINT)" fmt --config $(GOLANGCI_LINT_CONFIG) ./...

ci-go: lint test-go build

ci-nvim: build test-lua

ci: ci-go ci-nvim

clean:
	rm -rf $(BIN_DIR) $(GO_DIR)/go-deep

release: build
