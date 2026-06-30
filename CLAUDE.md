# rosa-hyperfleet-zoa

This file provides guidance to AI coding assistants when working with this repository.

## Project Overview

**rosa-hyperfleet-zoa** is the Zero Operator Access (ZOA) CLI, tooling, and Trusted Action definitions repository for the ROSA HCP Hyperfleet platform. ZOA ensures operators have no persistent, interactive, or unaudited access to customer infrastructure — all operational actions are executed through pre-defined, audited Trusted Actions via the Platform API.

**Tech stack**: Go 1.25, Cobra, AWS SDK v2 (SigV4), OpenShift CI (Prow + ci-operator)

**Key Goals:**

- **Zero standing access**: Operators interact exclusively through the audited API
- **Least-privilege execution**: Per-execution RBAC scoped to exactly what the TA declares
- **Immutable audit trail**: Every action logged with caller identity, target, jira, and duration
- **Defense-in-depth**: API-level secrets protection, write cooldowns, three-layer timeout model

## Architecture Overview

This repo provides the operator-facing tooling for the ZOA system:

1. **ZOA CLI** (`cmd/zoa/`) — Go CLI that authenticates via SigV4 and calls the Platform API
2. **zoa-tools image** (`Containerfile` + `image/`) — Container image with runner/uploader entrypoints used by TA Jobs on target clusters
3. **Trusted Actions** (`trusted-actions/`) — Trusted Action definitions consumed by the platform repo at a pinned commit/hash

The full execution flow: CLI → API Gateway → Platform API → DynamoDB + Maestro → ManifestWork → Job on target cluster → S3 output → CLI retrieves results.

## Key Directories

| Path | Purpose |
|------|---------|
| `cmd/zoa/` | Binary entry point |
| `internal/cli/` | Cobra CLI commands (run, get, runs, actions, describe, audit, logs) |
| `internal/client/` | Platform API client (SigV4-signed HTTP requests) |
| `internal/output/` | Output formatting (table, JSON — kubectl-style) |
| `internal/version/` | Version info (injected via ldflags at build time) |
| `trusted-actions/` | Trusted Action definitions (YAML: script, RBAC, params) |
| `image/` | Runner and uploader entrypoint scripts baked into zoa-tools image |
| `ci/` | CI scripts and Containerfile for OpenShift CI build root |
| `Containerfile` | zoa-tools container image (kubectl + aws-cli + entrypoints) |

## Key Technologies

- **Language**: Go 1.25 (CLI + tests)
- **CLI framework**: Cobra
- **Authentication**: AWS SigV4 against regional API Gateway (via `API_URL` env var)
- **Container image**: kubectl + aws-cli on minimal base
- **Testing**: Go stdlib `testing` (unit tests), Ginkgo/Gomega (future integration/E2E)
- **CI**: OpenShift CI (Prow + ci-operator)
- **Linting**: golangci-lint v2.12+ (required, no fallback; config in `.golangci.yml`)

## Build & Test Commands

```bash
make all          # Run all checks and build (fmt, vet, lint, test, build)
make build        # Build ./bin/zoa (injects version, commit, build date)
make install      # Install zoa to GOPATH/bin
make test         # Unit tests (go test -v -race -coverprofile)
make verify       # fmt-check + vet + lint (CI-safe, read-only)
make fmt          # Format Go code (gofmt -w -s)
make fmt-check    # Check formatting (read-only, fails if unformatted)
make vet          # Run go vet for suspicious code
make lint         # Run golangci-lint (errors if not installed)
make tidy         # Tidy go modules (go mod tidy)
make clean        # Remove build artifacts and coverage files
make image        # Build zoa-tools multi-arch container image
make image-push   # Build and push manifest list to registry
make image-clean  # Remove built container images
make help         # Show all available targets
```

## Important Context

- **SigV4 authentication**: All API calls are signed with AWS SigV4 against the regional API Gateway
- **Environment**: Set `API_URL` to the regional API Gateway endpoint (e.g. `export API_URL="https://<id>.execute-api.<region>.amazonaws.com/prod"`)
- **Versioning**: Semantic version from git tags, git commit, and build date injected via `-ldflags`
- **Trusted Actions**: YAML files in `trusted-actions/` defining `name`, `scope`, `type`, `params`, `rbac`, `script` — consumed by the platform repo at a pinned commit/hash
- **Security invariant**: TA templates must never request secrets access in `clusters-*` namespaces
- **Two privilege scopes**: `kube-api` (Kubernetes operations, per-execution SA) and `aws-api` (AWS CLI, static SA with Pod Identity)
- **Conventional commits**: Use `feat:`, `fix:`, `docs:`, `chore:` prefixes
- **golangci-lint required**: Must be installed locally (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`)
- **Architecture**: See README.md for the system diagram

## Development Guidelines

### Agent Usage

- **Use the architect agent** for changes to Trusted Action RBAC patterns or security-sensitive code
- **Use the code-reviewer agent** for CLI code quality review

### Security Guidelines

- **Never** access secrets in `clusters-*` namespaces from Trusted Actions
- **Always** use least-privilege RBAC in Trusted Action `rbac` sections
- **Never** hardcode credentials — use Pod Identity for AWS access, SigV4 for API auth
- **Always** validate and sanitize params in Trusted Action scripts (`set -euo pipefail`)
- **Never** log sensitive data (tokens, credentials, customer content) in Trusted Action scripts

### Trusted Action Conventions

- Scope: `kube-api` (Kubernetes operations) or `aws-api` (AWS CLI operations)
- Type: `read` (no cooldown) or `write` (cooldown enforced)
- Script must write structured output to `/artifacts/output.json`
- RBAC must follow least-privilege: only the verbs/resources the TA actually needs
- `timeout_seconds` should be set appropriately (default: 1800s)

### Developer Workflow

- Run `make all` before pushing to ensure everything passes (format, vet, lint, test, build)
- `make verify` runs the same read-only checks as CI (fmt-check, vet, lint)
- `golangci-lint` is a hard requirement — install with: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`

### Formatting

- Go: `gofmt -s` (no goimports dependency)
- YAML: 2-space indentation
- Conventional commits: `feat:`, `fix:`, `docs:`, `chore:` prefixes

## Related Repositories

| Repository | What it contains |
|---|---|
| [rosa-hyperfleet](https://github.com/openshift-online/rosa-hyperfleet) | Terraform infra, Helm charts, ArgoCD deployment, CI runner |
| [rosa-hyperfleet-api](https://github.com/openshift-online/rosa-hyperfleet-api) | Execution engine, REST API, reconciler, DynamoDB stores |
