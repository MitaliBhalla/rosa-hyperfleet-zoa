# rosa-hyperfleet-zoa

Zero Operator Access (ZOA) CLI and tooling for the ROSA HCP Hyperfleet platform. Provides a Go CLI for executing Trusted Actions against target clusters and the container image used by TA Jobs.

**Tech stack**: Go, Cobra, AWS SDK v2 (SigV4), Ginkgo/Gomega

## Key Directories

| Path | Purpose |
|------|---------|
| `cmd/zoa/` | Binary entry point |
| `internal/commands/` | Cobra CLI subcommands (run, catalog, describe, history, audit) |
| `internal/client/` | Platform API client (SigV4-signed HTTP requests) |
| `trusted-actions/` | Trusted Action definitions (YAML: script, RBAC, params) |
| `image/` | Runner and uploader entrypoint scripts baked into zoa-tools image |
| `Containerfile` | zoa-tools container image (kubectl + aws-cli + entrypoints) |

## Commands

```bash
make build        # Build ./bin/zoa
make test         # Unit tests (go test -race)
make verify       # fmt-check + vet + lint
make fmt          # Auto-format Go code
make image        # Build the zoa-tools container image
make image-push   # Push to registry
```

## Important Context

- **SigV4 authentication**: All API calls are signed with AWS SigV4 against the regional API Gateway
- **Trusted Actions**: YAML files in `trusted-actions/` defining `name`, `scope`, `type`, `params`, `rbac`, `script` — consumed by the platform repo at a pinned commit/hash
- **Security invariant**: TA templates must never request secrets access in `clusters-*` namespaces
- **Two privilege scopes**: `kube-api` (Kubernetes operations, per-execution SA) and `aws-api` (AWS CLI, static SA with Pod Identity)
- **Conventional commits**: Use `feat:`, `fix:`, `docs:`, `chore:` prefixes
- **Architecture**: See README.md for the system diagram

Include AGENTS.md
