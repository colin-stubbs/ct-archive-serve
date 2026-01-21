.PHONY: help test lint security build-ct-archive-serve

BIN_DIR ?= bin

help:
	@echo "Targets:"
	@echo "  test                  Run unit tests"
	@echo "  lint                  Run golangci-lint"
	@echo "  security              Run govulncheck and trivy (if installed)"
	@echo "  build-ct-archive-serve Build ct-archive-serve binary"

test:
	go test ./...

lint:
	golangci-lint run ./...

security:
	@command -v govulncheck >/dev/null 2>&1 && govulncheck ./... || echo "govulncheck not installed; skipping"
	@command -v trivy >/dev/null 2>&1 && trivy fs --quiet . || echo "trivy not installed; skipping"

build-ct-archive-serve:
	@mkdir -p "$(BIN_DIR)"
	go build -o "$(BIN_DIR)/ct-archive-serve" ./cmd/ct-archive-serve

