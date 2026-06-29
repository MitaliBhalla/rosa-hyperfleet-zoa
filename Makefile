.PHONY: all build clean install test fmt fmt-check vet lint verify tidy image image-push image-clean help

BINARY_NAME=zoa
BUILD_DIR=./bin
IMAGE_REPO ?= quay.io/slopezz/zoa-tools
IMAGE_TAG ?= latest
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
VERSION = 0.1.0
CONTAINER_RUNTIME ?= $(shell command -v podman 2>/dev/null || echo docker)
VERSION_PKG=github.com/openshift-online/rosa-hyperfleet-zoa/internal/version
LDFLAGS=-ldflags "-X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).GitCommit=$(GIT_COMMIT) -X $(VERSION_PKG).BuildDate=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)"

all: fmt vet lint test build
	@echo ""
	@echo "✓ All checks and build completed successfully!"

help:
	@echo "Available targets:"
	@echo ""
	@echo "Build & Install:"
	@echo "  all             - Run all checks (fmt, vet, lint, test, build)"
	@echo "  build           - Build the $(BINARY_NAME) binary"
	@echo "  clean           - Remove built binaries and test artifacts"
	@echo "  install         - Install $(BINARY_NAME) to GOPATH/bin"
	@echo "  tidy            - Tidy go modules"
	@echo ""
	@echo "Testing:"
	@echo "  test            - Run unit tests"
	@echo ""
	@echo "Code Quality:"
	@echo "  fmt             - Format Go code with gofmt"
	@echo "  fmt-check       - Check if Go code is formatted (non-destructive)"
	@echo "  vet             - Run go vet for suspicious code"
	@echo "  lint            - Run golangci-lint"
	@echo "  verify          - Run all checks (fmt-check, vet, lint)"
	@echo ""
	@echo "Container Image:"
	@echo "  image           - Build multi-arch image (amd64 + arm64)"
	@echo "  image-push      - Build multi-arch + push manifest list"
	@echo "  image-clean     - Remove built container images"
	@echo ""
	@echo "  help            - Show this help message"

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/zoa/
	@echo "✓ Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

clean:
	@echo "Cleaning up..."
	@rm -f $(BUILD_DIR)/$(BINARY_NAME)
	@rm -f coverage.out
	@echo "✓ Clean complete"

install:
	@echo "Installing $(BINARY_NAME) to GOPATH/bin..."
	@go install $(LDFLAGS) ./cmd/zoa/
	@echo "✓ Installation complete"

tidy:
	@echo "Tidying go modules..."
	@GOFLAGS=-mod=mod go mod tidy
	@echo "✓ Tidy complete"

test:
	@echo "Running unit tests..."
	@go test -v -race -coverprofile=coverage.out ./internal/... ./cmd/... 2>.test-stderr; \
	rc=$$?; \
	if [ $$rc -ne 0 ] && grep -q "no such tool.*covdata" .test-stderr; then \
		echo ""; \
		echo "ERROR: Tests passed but coverage failed — missing 'covdata' tool (GVM issue)."; \
		echo ""; \
		echo "Fix with:"; \
		echo "  chmod u+w \$$(go env GOROOT)/pkg/tool/linux_amd64/"; \
		echo "  GOWORK=off go build -o \$$(go env GOROOT)/pkg/tool/linux_amd64/covdata cmd/covdata"; \
		echo ""; \
		echo "Or install Go from https://go.dev/dl/ instead of GVM."; \
		rm -f .test-stderr; \
		exit 1; \
	elif [ $$rc -ne 0 ]; then \
		cat .test-stderr >&2; \
		rm -f .test-stderr; \
		exit $$rc; \
	fi; \
	rm -f .test-stderr
	@echo "✓ Unit tests passed"

# Code Quality Targets
fmt:
	@echo "Formatting Go code..."
	@gofmt -w -s .
	@echo "✓ Formatting complete"

fmt-check:
	@echo "Checking code formatting..."
	@files=$$(gofmt -l .); \
	if [ -n "$$files" ]; then \
		echo "The following files are not formatted:"; \
		echo "$$files"; \
		echo ""; \
		echo "Run 'make fmt' to format them"; \
		exit 1; \
	fi
	@echo "✓ All files are properly formatted"

vet:
	@echo "Running go vet..."
	@go vet ./...
	@echo "✓ go vet passed"

lint:
	@echo "Running golangci-lint..."
	@if ! command -v golangci-lint > /dev/null 2>&1; then \
		echo "Error: golangci-lint not found"; \
		echo "Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.62.2"; \
		exit 1; \
	fi
	@golangci-lint run --timeout=5m ./...
	@echo "✓ golangci-lint passed"

verify: fmt-check vet lint
	@echo ""
	@echo "✓ All verification checks passed!"


# --- Container image targets ---

image: ## Build multi-arch image (amd64 + arm64)
	$(CONTAINER_RUNTIME) manifest rm $(IMAGE_REPO):$(IMAGE_TAG) 2>/dev/null || true
	$(CONTAINER_RUNTIME) rmi $(IMAGE_REPO):$(IMAGE_TAG) 2>/dev/null || true
	$(CONTAINER_RUNTIME) build $(if $(NOCACHE),--no-cache) --platform linux/amd64 \
		--manifest $(IMAGE_REPO):$(IMAGE_TAG) \
		-f Containerfile .
	$(CONTAINER_RUNTIME) build $(if $(NOCACHE),--no-cache) --platform linux/arm64 \
		--manifest $(IMAGE_REPO):$(IMAGE_TAG) \
		-f Containerfile .

image-push: image ## Build multi-arch and push manifest list
	@if [ "$(GIT_COMMIT)" = "unknown" ]; then echo "ERROR: no git commit — refusing to push untracked image"; exit 1; fi
	$(CONTAINER_RUNTIME) manifest push $(IMAGE_REPO):$(IMAGE_TAG)
	$(CONTAINER_RUNTIME) manifest push $(IMAGE_REPO):$(IMAGE_TAG) docker://$(IMAGE_REPO):$(GIT_COMMIT)

image-clean: ## Remove built container images
	$(CONTAINER_RUNTIME) rmi $(IMAGE_REPO):$(IMAGE_TAG) 2>/dev/null || true
	$(CONTAINER_RUNTIME) rmi $(IMAGE_REPO):$(GIT_COMMIT) 2>/dev/null || true
