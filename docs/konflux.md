# Konflux

ZOA container images build on Konflux (`rosa-tenant` / `kflux-prd-rh02`).

| Component | Containerfile | Quay image |
|-----------|---------------|------------|
| `zoa-lambda` | `Containerfile` | `quay.io/redhat-user-workloads/rosa-tenant/zoa-lambda` |
| `zoa-runner` | `Containerfile.runner` | `quay.io/redhat-user-workloads/rosa-tenant/zoa-runner` |

Application: `rosa-hyperfleet-zoa` ([Konflux UI](https://konflux-ui.apps.kflux-prd-rh02.0fk9.p1.openshiftapps.com/ns/rosa-tenant/applications/rosa-hyperfleet-zoa/activity))

The `zoa` CLI is built into the `zoa-runner` image and released via `.github/workflows/release-cli.yml` — no separate Konflux component.

## Merge order

1. [konflux-release-data MR](https://gitlab.cee.redhat.com/releng/konflux-release-data/-/merge_requests/21897) — remove legacy `rosa-hyperfleet-zoa` component; finalize `zoa-lambda` / `zoa-runner`
2. Merge this PR (`.tekton/zoa-lambda-*` + `.tekton/zoa-runner-*`)
3. `openshift/release` — update branch protection required Konflux check names
