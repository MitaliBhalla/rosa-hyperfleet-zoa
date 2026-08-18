# Development Guide

## Prerequisites

- **Go 1.26+** — install from [go.dev/dl](https://go.dev/dl/) or via GVM
- **golangci-lint v2.12+** — `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`
- **Make**
- **AWS CLI v2** — with a profile configured for your environment

## Quick Start

```bash
git clone https://github.com/openshift-online/rosa-hyperfleet-zoa.git
cd rosa-hyperfleet-zoa
make all                    # fmt → vet → lint → test → build
```

## Build Targets

| Target | What it does |
|--------|-------------|
| `make all` | fmt → vet → lint → test → build (run before pushing) |
| `make build` | Build `./bin/zoa` |
| `make install` | Install to `$GOBIN` |
| `make fmt` | Format code (`gofmt -w -s`) |
| `make vet` | Static analysis (`go vet`) |
| `make lint` | Lint (`golangci-lint`) |
| `make test` | Unit tests with coverage and race detection |
| `make verify` | Read-only checks (`fmt-check + vet + lint`) |
| `make tidy` | Clean up `go.mod` / `go.sum` |

## Binaries

This repository produces three binaries:

| Binary | Entry Point | Purpose |
|--------|-------------|---------|
| `zoa` | `cmd/zoa/main.go` | CLI for operators |
| `zoa-lambda` | `cmd/zoa-lambda/main.go` | Lambda function (API + Worker modes, deployed per VPC) |
| `zoa-runner` | `cmd/zoa-runner/main.go` | Async Job runner (executes inside K8s Jobs for async TAs) |

## Testing

```bash
make test                    # All tests with race detection
go test ./pkg/actions/...    # Action tests only
go test ./pkg/handler/...    # Handler tests only
go test ./pkg/executor/...   # Executor tests only
```

### Test Patterns

- Use `k8s.io/client-go/kubernetes/fake` for Kubernetes unit tests
- Use interface mocking for DynamoDB/S3 operations
- Test names follow `"When ... it should ..."` format
- All tests run with `-race` flag

## CI

CI runs via [OpenShift CI (Prow)](https://prow.ci.openshift.org/):

| Job | Script | What it checks |
|-----|--------|----------------|
| `lint` | `ci/lint.sh` | `make fmt-check` + `make lint` |
| `test` | `ci/unit-tests.sh` | `make test` + coverage artifacts |
| `verify` | `ci/verify.sh` | `make verify` |

## Commit Conventions

Use [conventional commits](https://www.conventionalcommits.org/):

```
feat: add new trusted action
fix: handle timeout in dispatch request
docs: update development guide
chore: bump golangci-lint to v2.13.0
```

## Releasing

The CLI version is defined in the `VERSION` variable at the top of the `Makefile`.
On merge to `main`, a GitHub Action checks if the version is new and creates a git tag + GitHub Release automatically.

## Container Images

| Image | Containerfile | Purpose |
|-------|---------------|---------|
| `zoa-lambda` | `Containerfile` | Lambda container (UBI9 + `zoa-lambda` binary + Lambda Web Adapter) |
| `zoa-runner` | `Containerfile.runner` | Async runner (UBI9 + `zoa-runner` + `zoa` CLI, runs inside K8s Jobs) |

```bash
make image          # Build Lambda image
make image-push     # Build and push
```

## GVM Users

If using GVM and `make test` fails with `go: no such tool "covdata"`:

```bash
chmod u+w $(go env GOROOT)/pkg/tool/linux_amd64/
GOWORK=off go build -o $(go env GOROOT)/pkg/tool/linux_amd64/covdata cmd/covdata
```
