.PHONY: all build clean install test fmt fmt-check vet lint verify tidy \
       image image-runner image-push image-push-runner help

BINARY_NAME = zoa
BUILD_DIR   = ./bin

# Container images
IMAGE_REPO        ?= quay.io/slopezz/zoa-lambda
RUNNER_IMAGE_REPO ?= quay.io/slopezz/zoa-runner
IMAGE_TAG         ?= latest
GIT_COMMIT        = $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

CONTAINER_RUNTIME ?= $(shell command -v podman 2>/dev/null || echo docker)

# Tools
TOOLS_DIR     := ./hack/tools
TOOLS_BIN_DIR := $(TOOLS_DIR)/bin
GOLANGCI_LINT := $(abspath $(TOOLS_BIN_DIR)/golangci-lint)

$(GOLANGCI_LINT): $(TOOLS_DIR)/go.mod
	cd $(TOOLS_DIR); go build -tags=tools -o $(abspath $(TOOLS_BIN_DIR))/golangci-lint github.com/golangci/golangci-lint/v2/cmd/golangci-lint

VERSION     = 0.2.0
VERSION_PKG = github.com/openshift-online/rosa-hyperfleet-zoa/internal/version
LDFLAGS     = -ldflags "-X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).GitCommit=$(GIT_COMMIT) -X $(VERSION_PKG).BuildDate=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)"

# =============================================================================
# Default
# =============================================================================

all: verify test build

# =============================================================================
# Build
# =============================================================================

build:
	@mkdir -p $(BUILD_DIR)
	@go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/zoa/
	@echo "✓ $(BUILD_DIR)/$(BINARY_NAME)"

clean:
	@rm -rf $(BUILD_DIR) coverage.out

install:
	@go install $(LDFLAGS) ./cmd/zoa/

tidy:
	@go mod tidy

# =============================================================================
# Test
# =============================================================================

test:
	@go test -race -coverprofile=coverage.out ./...

# =============================================================================
# Code Quality
# =============================================================================

fmt:
	@gofmt -w -s .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Run 'make fmt'" && gofmt -l . && exit 1)

vet:
	@go vet ./...

lint: $(GOLANGCI_LINT)
	@$(GOLANGCI_LINT) run --timeout=5m ./...

verify: fmt-check vet lint

# =============================================================================
# Container Images
# =============================================================================

image:
	$(CONTAINER_RUNTIME) build \
		--platform linux/amd64 \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t $(IMAGE_REPO):$(IMAGE_TAG) \
		-f Containerfile .

image-runner:
	$(CONTAINER_RUNTIME) build \
		--platform linux/amd64 \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t $(RUNNER_IMAGE_REPO):$(IMAGE_TAG) \
		-f Containerfile.runner .

image-push: image
	$(CONTAINER_RUNTIME) push $(IMAGE_REPO):$(IMAGE_TAG)
	$(CONTAINER_RUNTIME) tag $(IMAGE_REPO):$(IMAGE_TAG) $(IMAGE_REPO):$(GIT_COMMIT)
	$(CONTAINER_RUNTIME) push $(IMAGE_REPO):$(GIT_COMMIT)

image-push-runner: image-runner
	$(CONTAINER_RUNTIME) push $(RUNNER_IMAGE_REPO):$(IMAGE_TAG)
	$(CONTAINER_RUNTIME) tag $(RUNNER_IMAGE_REPO):$(IMAGE_TAG) $(RUNNER_IMAGE_REPO):$(GIT_COMMIT)
	$(CONTAINER_RUNTIME) push $(RUNNER_IMAGE_REPO):$(GIT_COMMIT)

# =============================================================================
# Help
# =============================================================================

help:
	@echo "Build:"
	@echo "  build              Build zoa CLI (./bin/zoa)"
	@echo "  install            Install zoa to GOPATH/bin"
	@echo "  clean              Remove build artifacts"
	@echo ""
	@echo "Test & Quality:"
	@echo "  test               Run unit tests with race detection"
	@echo "  verify             fmt-check + vet + lint"
	@echo "  fmt                Format code"
	@echo ""
	@echo "Images:"
	@echo "  image              Build zoa-lambda image (primary)"
	@echo "  image-runner       Build zoa-runner image (async Jobs + CLI)"
	@echo "  image-push         Build + push zoa-lambda"
	@echo "  image-push-runner  Build + push zoa-runner"
