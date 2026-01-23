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
	find config/ -type f ! -name .gitkeep ! -name qBittorrent.conf ! -name feeds.json -exec rm -fv {} \;
	@echo "✅ qBittorrent config and feed content cleaned up"
	@echo "✅ Cleanup completed"

test:
	go test -v ./...

force-test:
	go test -v -count=1 ./...

lint:
	golangci-lint run ./...

security:
	@command -v govulncheck >/dev/null 2>&1 && govulncheck ./... || echo "govulncheck not installed; skipping"
	@command -v trivy >/dev/null 2>&1 && trivy fs --quiet . || echo "trivy not installed; skipping"

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
	docker buildx build --platform linux/amd64,linux/arm64 -t $(IMAGE_NAME):$(TAG) --load .
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

