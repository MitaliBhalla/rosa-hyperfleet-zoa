.PHONY: all build dist print-version clean install test fmt fmt-check vet lint verify tidy verify-mod \
       image image-runner image-push image-push-runner help

BINARY_NAME = zoa
BUILD_DIR   = ./bin
DIST_DIR    ?= ./dist

# Cross-compiled CLI artifacts for GitHub Releases (kubectl-style).
CLI_PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
HASH_CMD      := $(shell command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo "shasum -a 256")

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

VERSION     = 0.3.1
VERSION_PKG = github.com/openshift-online/rosa-hyperfleet-zoa/internal/version
VERSION_LDFLAGS = -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).GitCommit=$(GIT_COMMIT) -X $(VERSION_PKG).BuildDate=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS         = -ldflags "$(VERSION_LDFLAGS)"
DIST_LDFLAGS    = -ldflags "-s -w $(VERSION_LDFLAGS)"

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

# print-version is the single parser used by CI so we don't grep Makefile by hand.
print-version:
	@echo $(VERSION)

# Cross-compile the CLI for GitHub Releases. Same version ldflags as `build`,
# plus -s -w (stripped) and -trimpath so artifacts are smaller and more reproducible.
dist:
	@rm -rf $(DIST_DIR)
	@mkdir -p $(DIST_DIR)
	@for platform in $(CLI_PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		out="$(DIST_DIR)/zoa-$$os-$$arch"; \
		echo "Building $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath $(DIST_LDFLAGS) -o "$$out" ./cmd/zoa/; \
	done
	@cd $(DIST_DIR) && $(HASH_CMD) zoa-* > SHA256SUMS
	@echo "✓ $(DIST_DIR)"
	@ls -lh $(DIST_DIR)

clean:
	@rm -rf $(BUILD_DIR) $(DIST_DIR) coverage.out

install:
	@go install $(LDFLAGS) ./cmd/zoa/

tidy:
	@go mod tidy
	@cd $(TOOLS_DIR) && go mod tidy

verify-mod: tidy
	@git diff --exit-code go.mod go.sum $(TOOLS_DIR)/go.mod $(TOOLS_DIR)/go.sum

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
	@echo "  dist               Cross-compile CLI for GitHub Releases (./dist)"
	@echo "  print-version      Print the CLI VERSION from this Makefile"
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
