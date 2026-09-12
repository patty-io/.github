# Patty shared delivery workflows

This repository owns the organization-wide GitHub Actions and Kargo release
metadata contract. Product repositories call the reusable workflow; they do
not copy its build implementation.

## Container releases

Use the **Patty container release** starter workflow, then replace its `images`
JSON with one entry per deployable image. Each Harbor robot remains scoped to
the caller's project. Callers should pin a released major version such as `v1`;
production repositories may pin the exact workflow commit for reproducibility.

The workflow publishes immutable `sha-<commit>` images with OCI source,
revision, build, commit, pull-request, and changed-file annotations. Kargo
imports those annotations into Freight. Failed builds remain visible in GitHub
Actions but do not become release candidates.

## Kargo release metadata

The tooling-cluster owner applies `kargo/release-metadata-task.yaml` once. Each
Stage invokes `ClusterPromotionTask/patty-release-metadata` before deployment
steps and supplies one representative `imageRepo`.

For applications with multiple images, the Warehouse must use
`freightCreationCriteria` to require identical immutable tags before creating
Freight. Without that invariant, the shared task must not be used: a single
release alias cannot truthfully identify a mixed-revision artifact set.

The task records the exact source revision, commit and CI links, change
summary, immutable image digest, target environment, and deployed-to-candidate
comparison link in Kargo metadata. Kargo remains authoritative for verification
and promotion status.

## Validation

```sh
sh scripts/release-workflow.test.sh
```

