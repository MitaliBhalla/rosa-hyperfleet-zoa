# AGENTS.md

This file provides guidance to AI coding assistants when working with this repository.

## Project Overview

**rosa-hyperfleet-zoa** is the Zero Operator Access (ZOA) CLI, tooling, and Trusted Action definitions repository for the ROSA HCP Hyperfleet platform. ZOA ensures operators have no persistent, interactive, or unaudited access to customer infrastructure — all operational actions are executed through pre-defined, audited Trusted Actions via the Platform API.

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

## Key Technologies

- **Language**: Go (CLI + tests)
- **CLI framework**: Cobra
- **Authentication**: AWS SigV4 against regional API Gateway
- **Container image**: kubectl + aws-cli on minimal base
- **Testing**: Ginkgo/Gomega
- **CI**: OpenShift CI (Prow + ci-operator)

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

### Formatting

- Go: `gofmt` / `goimports`
- YAML: 2-space indentation
- Conventional commits: `feat:`, `fix:`, `docs:`, `chore:` prefixes

## Related Repositories

| Repository | What it contains |
|---|---|
| [rosa-hyperfleet](https://github.com/openshift-online/rosa-hyperfleet) | Terraform infra, Helm charts, ArgoCD deployment, CI runner |
| [rosa-hyperfleet-api](https://github.com/openshift-online/rosa-hyperfleet-api) | Execution engine, REST API, reconciler, DynamoDB stores |
