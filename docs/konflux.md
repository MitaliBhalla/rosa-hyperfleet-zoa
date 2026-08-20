# Konflux

ZOA container images build on Konflux (`rosa-tenant` / `kflux-prd-rh02`).

| Phase | Component | Containerfile | Quay image |
|-------|-----------|---------------|------------|
| **Now** | `rosa-hyperfleet-zoa` | `Containerfile` | `quay.io/.../rosa-hyperfleet-zoa` |
| **After release-data MR** | `zoa-lambda` | `Containerfile` | `quay.io/.../zoa-lambda` |
| **After release-data MR** | `zoa-runner` | `Containerfile.runner` | `quay.io/.../zoa-runner` |

Application: `rosa-hyperfleet-zoa` ([Konflux UI](https://konflux-ui.apps.kflux-prd-rh02.0fk9.p1.openshiftapps.com/ns/rosa-tenant/applications/rosa-hyperfleet-zoa/activity))

The `zoa` CLI is built into the `zoa-runner` image and released via `.github/workflows/release-cli.yml` — no separate Konflux component.

## Merge order

1. [konflux-release-data MR !21738](https://gitlab.cee.redhat.com/releng/konflux-release-data/-/merge_requests/21738) — EC policy + new components
2. Copy `.tekton/zoa-lambda-*` and `.tekton/zoa-runner-*` from `hack/konflux-release-data/` into `.tekton/`; remove legacy `rosa-hyperfleet-zoa-*.yaml`
3. `openshift/release` branch protection — add new Konflux required checks

## EC note

`zoa-lambda` uses `public.ecr.aws` Lambda bases. Enterprise Contract requires the tenant policy exception in release-data MR !21738.
