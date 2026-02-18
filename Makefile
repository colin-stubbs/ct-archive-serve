.PHONY: help test lint security build-ct-archive-serve build-container build-container-multi build clean

# Configuration for local builds
IMAGE_NAME ?= colin-stubbs/ct-archive-serve
DOCKER_HUB_IMAGE_NAME ?= colinstubbs/ct-archive-serve
TAG ?= latest
REGISTRY ?= ghcr.io
REGISTRY_IMAGE ?= $(REGISTRY)/$(IMAGE_NAME)

BIN_DIR ?= bin

help:
	@echo "Targets:"
	@echo "  test                  Run unit tests"
	@echo "  lint                  Run golangci-lint"
	@echo "  security              Run govulncheck and trivy (if installed)"
	@echo "  build                 Build ct-archive-serve binary"
	@echo "  build-container       Build Docker image"
	@echo "  build-container-multi Build multi-platform Docker image"
	@echo "  clean                 Clean up build artifacts"

clean:
	@echo "🧹 Cleaning up build artifacts..."
	rm -fv ./ct-archive-serve
	rm -fv ./bin/ct-archive-serve
	@echo "✅ Build artifacts cleaned up"
	@echo "🧹 Cleaning up qBittorrent config and feed content..."
	find qbittorrent/ -type f ! -name .gitkeep ! -name qBittorrent.conf ! -name feeds.json ! -name download_rules.json -exec rm -fv {} \;
	@echo "✅ qBittorrent config and feed content cleaned up"
	@echo "✅ Cleanup completed"

test:
	go test -v ./... 1>test.log 2>&1
	cat test.log

force-test:
	go test -v -count=1 ./... 1>test.log 2>&1
	cat test.log

lint:
	golangci-lint run ./...

# Security scanning
security: security-trivy security-govulncheck

security-trivy:
	@echo "Running trivy security scan..."
	@if command -v trivy >/dev/null 2>&1; then \
		trivy fs --scanners vuln,misconfig,secret --skip-dirs docs/ .; \
	else \
		echo "trivy not found.;" \
		exit 1; \
	fi

security-govulncheck:
	@echo "Running govulncheck security scan..."
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck -show verbose ./...; \
	else \
		echo "govulncheck not found."; \
		exit 1; \
	fi

# Code quality check (combines linting and security)
quality: lint security
	@echo "✅ Code quality check completed"

# Install development tools
install-tools:
	@echo "🔧 Installing development tools..."
	@if command -v brew >/dev/null 2>&1; then \
		brew install hadolint trivy golangci-lint semgrep govulncheck; \
	elif command -v apt-get >/dev/null 2>&1; then \
		sudo apt-get update && sudo apt-get install -y hadolint trivy semgrep govulncheck; \
		curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.10.1 \
	else \
		echo "⚠️  Package manager not supported, please install manually:"; \
		echo "  - golangci-lint: https://github.com/golangci/golangci-lint"; \
		echo "  - hadolint: https://github.com/hadolint/hadolint"; \
		echo "  - trivy: https://github.com/aquasecurity/trivy"; \
		echo "  - govulncheck: https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck"; \
		echo "  - semgrep: https://semgrep.dev"; \
	fi
	@echo "✅ Tools installation completed"
  
build-ct-archive-serve:
	@mkdir -p "$(BIN_DIR)"
	go build -o "$(BIN_DIR)/ct-archive-serve" ./cmd/ct-archive-serve

build-container:
	@echo "🔨 Building Docker image..."
	docker compose build
	@echo "✅ Build completed"

build-container-multi:
	@echo "🔨 Building multi-platform Docker image..."
	docker buildx create --use --name multi-builder || true
	docker buildx build --platform linux/amd64,linux/arm64 -t $(REGISTRY_IMAGE):$(TAG) --load .
	@echo "✅ Multi-platform build completed"

push-container-docker-hub:
	@echo "📤 Pushing to Docker Hub..."
	docker tag $(IMAGE_NAME):$(TAG) docker.io/$(DOCKER_HUB_IMAGE_NAME):$(TAG)
	docker push docker.io/$(DOCKER_HUB_IMAGE_NAME):$(TAG)
	@echo "✅ Push completed"

# Build commands
build:
	@echo "🔨 Building ct-archive-serve..."
	make build-ct-archive-serve
	@echo "🔨 Building container..."
	make build-container-multi

