# Patty shared delivery workflows

This repository owns the organization-wide GitHub Actions and Kargo release
metadata contract. Product repositories call the reusable workflow; they do
not copy its build implementation.

## Container releases

Use the **Patty container release** starter workflow, then replace its `images`
JSON with one entry per deployable image. Each Harbor robot remains scoped to
the caller's project. Callers should pin a released major version such as `v1`;
production repositories may pin the exact workflow commit for reproducibility.

The workflow publishes immutable `sha-<commit>-run-<run-id>-<attempt>` images with OCI source,
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

## Shared promotion mechanics

`kargo/promote-app-task.yaml` defines `ClusterPromotionTask/patty-promote-app`:
the generic clone → pin every Freight image digest into the overlay → commit →
push sequence. App Stages reference it and keep only what is app-specific (the
release-metadata tasks and `argocd-update`/`argocd-wait`). The tooling-cluster
owner applies it alongside `release-metadata-task.yaml`; apply both **before**
merging any Stage that references them, or every promotion fails with
"task not found".

## Delivery standards

- `docs/adr/0001-code-and-data-release-standard.md` — the release standard every
  DB-backed app follows (migrate-first, expand/contract, no per-instance DB
  cache, blue/green with dependency-aware readiness).
- `docs/deployment-to-rollout.md` — the staged Deployment → Rollout blue/green
  migration procedure, with a copyable guard test.
- `argocd/applicationset.yaml` — the single source for each app/env Argo CD
  Application, including the blue/green Service-selector guard
  (`ignoreDifferences` + `RespectIgnoreDifferences`).

## Validation

```sh
sh scripts/release-workflow.test.sh
```
