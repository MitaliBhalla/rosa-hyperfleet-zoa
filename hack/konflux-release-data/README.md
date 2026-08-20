# konflux-release-data MR checklist

Copy these files into `gitlab.cee.redhat.com/releng/konflux-release-data` and open an MR.

```
tenants-config/cluster/kflux-prd-rh02/tenants/rosa-tenant/
├── zoa-lambda-enterprise-contract-policy.yaml   # new
├── kustomization.yaml                           # add policy + zoa-runner overlay
├── overlay/zoa-lambda/main/                     # replaces overlay/rosa-hyperfleet-zoa/main
└── overlay/zoa-runner/main/                     # new
```

After the release-data MR merges, copy the Tekton pipelines from this directory into the repo `.tekton/`:

- `zoa-lambda-pull-request.yaml` / `zoa-lambda-push.yaml`
- `zoa-runner-pull-request.yaml` / `zoa-runner-push.yaml`

Then remove the legacy `rosa-hyperfleet-zoa-*.yaml` pipelines and update openshift/release branch protection.

Run `build-manifests.sh` in `tenants-config/` before opening the MR.

Unblocks EC failures on PR builds: `rpm_signature` (Amazon Linux 2023 key) and `base_image_registries` (`public.ecr.aws`).

**MR:** https://gitlab.cee.redhat.com/releng/konflux-release-data/-/merge_requests/21738
