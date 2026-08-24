# Konflux

ZOA container images build on Konflux (`rosa-tenant` / `kflux-prd-rh02`).

| Phase | Component | Containerfile | Quay image |
|-------|-----------|---------------|------------|
| **Now (legacy)** | `rosa-hyperfleet-zoa` | `Containerfile` | `quay.io/.../rosa-hyperfleet-zoa` |
| **Target** | `zoa-lambda` | `Containerfile` | `quay.io/.../zoa-lambda` |
| **Target** | `zoa-runner` | `Containerfile.runner` | `quay.io/.../zoa-runner` |

Application: `rosa-hyperfleet-zoa` ([Konflux UI](https://konflux-ui.apps.kflux-prd-rh02.0fk9.p1.openshiftapps.com/ns/rosa-tenant/applications/rosa-hyperfleet-zoa/activity))

The `zoa` CLI is built into the `zoa-runner` image and released via `.github/workflows/release-cli.yml` — no separate Konflux component.

Release-data components `zoa-lambda` and `zoa-runner` are registered in [konflux-release-data MR !21738](https://gitlab.cee.redhat.com/releng/konflux-release-data/-/merge_requests/21738) (merged). Images use UBI9 bases and pass standard Enterprise Contract policy — no custom ECR or RPM signature exceptions are required.

## Pipelines

| PipelineRun | Component | Trigger |
|-------------|-----------|---------|
| `zoa-lambda-on-pull-request` / `zoa-lambda-on-push` | `zoa-lambda` | `Containerfile`, `cmd/zoa-lambda/**`, shared Go paths |
| `zoa-runner-on-pull-request` / `zoa-runner-on-push` | `zoa-runner` | `Containerfile.runner`, `cmd/zoa-runner/**`, `cmd/zoa/**`, shared Go paths |
| `rosa-hyperfleet-zoa-on-pull-request` / `rosa-hyperfleet-zoa-on-push` | `rosa-hyperfleet-zoa` | All PRs / pushes (legacy; remove after cutover) |
