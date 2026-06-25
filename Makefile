# rosa-hyperfleet-zoa Makefile
IMAGE_REPO ?= quay.io/slopezz/zoa-tools
IMAGE_TAG ?= latest
GIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
CONTAINER_RUNTIME ?= $(shell command -v podman 2>/dev/null || echo docker)

CLI_BIN := bin/zoa
CLI_VERSION ?= $(GIT_SHA)

.PHONY: all build build-cli image image-push image-clean test verify fmt fmt-check lint clean help

all: build-cli image

build: build-cli ## Alias for build-cli

# --- CLI targets ---

build-cli: ## Build the zoa CLI binary
	@mkdir -p bin
	go build -ldflags "-X main.version=$(CLI_VERSION)" -o $(CLI_BIN) ./cmd/zoa/

test: ## Run unit tests
	go test -race ./...

fmt: ## Format Go code
	gofmt -w .
	goimports -w .

fmt-check: ## Check formatting (read-only, fails if unformatted)
	@test -z "$$(gofmt -l .)" || (echo "Files not formatted:"; gofmt -l .; exit 1)
	@test -z "$$(goimports -l .)" || (echo "Files need goimports:"; goimports -l .; exit 1)

lint: ## Lint Go code
	@if command -v golangci-lint > /dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, running go vet instead"; \
		go vet ./...; \
	fi

verify: fmt-check lint test ## Run all checks (CI-safe, read-only)

clean: ## Remove CLI build artifacts (bin/)
	rm -rf bin/

# --- Container image targets ---

image: ## Build multi-arch image (amd64 + arm64). Use NOCACHE=1 to force fresh downloads.
	$(CONTAINER_RUNTIME) manifest rm $(IMAGE_REPO):$(IMAGE_TAG) 2>/dev/null || true
	$(CONTAINER_RUNTIME) rmi $(IMAGE_REPO):$(IMAGE_TAG) 2>/dev/null || true
	$(CONTAINER_RUNTIME) build $(if $(NOCACHE),--no-cache) --platform linux/amd64 \
		--manifest $(IMAGE_REPO):$(IMAGE_TAG) \
		-f Containerfile .
	$(CONTAINER_RUNTIME) build $(if $(NOCACHE),--no-cache) --platform linux/arm64 \
		--manifest $(IMAGE_REPO):$(IMAGE_TAG) \
		-f Containerfile .

image-push: image ## Build multi-arch and push manifest list
	@if [ "$(GIT_SHA)" = "unknown" ]; then echo "ERROR: no git commit — refusing to push untracked image"; exit 1; fi
	$(CONTAINER_RUNTIME) manifest push $(IMAGE_REPO):$(IMAGE_TAG)
	$(CONTAINER_RUNTIME) manifest push $(IMAGE_REPO):$(IMAGE_TAG) docker://$(IMAGE_REPO):$(GIT_SHA)

image-clean: ## Remove built container images
	$(CONTAINER_RUNTIME) rmi $(IMAGE_REPO):$(IMAGE_TAG) 2>/dev/null || true
	$(CONTAINER_RUNTIME) rmi $(IMAGE_REPO):$(GIT_SHA) 2>/dev/null || true

# --- Help ---

help: ## Show this help
	@echo "rosa-hyperfleet-zoa targets:"
	@echo ""
	@echo "  CLI:"
	@echo "    make build        Build ./bin/zoa (alias: build-cli)"
	@echo "    make test         Run unit tests (go test -race)"
	@echo "    make fmt          Format Go code (mutates files)"
	@echo "    make fmt-check    Check formatting (read-only, CI-safe)"
	@echo "    make lint         Lint Go code"
	@echo "    make verify       Run fmt-check + lint + test (CI-safe)"
	@echo "    make clean        Remove CLI build artifacts (bin/)"
	@echo ""
	@echo "  Container Image:"
	@echo "    make image        Build multi-arch image (amd64 + arm64)"
	@echo "    make image-push   Build multi-arch + push manifest list"
	@echo "    make image-clean  Remove built container images"
	@echo ""
	@echo "  Configuration:"
	@echo "    IMAGE_REPO=$(IMAGE_REPO)"
	@echo "    IMAGE_TAG=$(IMAGE_TAG)"
	@echo "    GIT_SHA=$(GIT_SHA)"
	@echo "    CONTAINER_RUNTIME=$(CONTAINER_RUNTIME)"
