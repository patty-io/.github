# ADR-0001 — Code and data ship together: migrate first, expand/contract, blue/green

- **Status:** Accepted
- **Date:** 2026-09-12
- **Applies to:** every Patty app that reads a database (patty-corp site, patty-accounts, …)

## Context

A release changes two things at once: the code (container image) and the data (schema + content).
Kubernetes defaults make this hazardous:

- A `RollingUpdate` **deliberately runs old and new pods together**, so a single URL can serve two
  versions mid-release. That is invisible for compatible API changes and very visible for a
  user-facing content or brand change.
- Self-hosted, multi-instance Next.js keeps its cache **per pod**; `revalidateTag()` only
  invalidates the pod that receives the call. A migration can therefore change the database while
  live pods keep serving pre-migration pages until their own TTL expires.
- Migrations run in a separate Job; if the order (data before code) is not enforced, new code
  starts against an un-migrated database.

We were bitten by exactly these: a promotion showed old and new content intermittently, and CMS
data changed under running pods.

## Decision

For every DB-backed app:

1. **Migrate before the code — PreSync, never after.** Each app has a PreSync hook Job
   running its migrator (a dedicated migrator image, or the app image with a migrate
   entrypoint). A failed migration aborts the sync, so new code never starts against an
   un-migrated database. Data/content changes ship **as a migration in a release** so they ride
   this hook.
2. **Migrations are expand/contract.** Never rename or drop in the same release that the code stops
   using the old shape: add the new shape → backfill (or dual-write) → switch reads → remove the
   old shape in a later release. This makes the brief old/new overlap safe for schema.
3. **Never cache database truth per instance.** Read CMS data live, or use a shared cache handler
   with `refreshTags()`. The default per-pod file-system cache is not a correctness tool.
4. **Customer-facing apps use a blue/green Rollout** (Argo Rollouts), not a rolling Deployment. The
   controller flips the active Service to the new ReplicaSet only once every new pod is Ready, so
   one URL never serves two versions. Readiness must be **dependency-aware** (e.g. a DB check) so a
   broken green is never promoted; liveness and the load-balancer check stay dependency-free.
5. **Argo CD must ignore the Rollout-managed Service selector.** Argo Rollouts adds
   `rollouts-pod-template-hash` to the active/preview Service selectors; without
   `ignoreDifferences: [{kind: Service, jsonPointers: [/spec/selector]}]` (+
   `RespectIgnoreDifferences=true`) a sync reverts the selector and re-introduces mixed versions.

## Consequences

- **Positive:** code and data are consistent once a pod is Ready; no mixed-version window for
  customer-facing apps; one place documents the release contract; gradual dial-ups (canary) become
  a strategy change, not a rewrite.
- **Negative / costs:** blue/green runs extra pods briefly; expand/contract requires multi-step
  releases for breaking schema changes; every app's Argo CD Application needs the
  `ignoreDifferences` entry; readiness now fails closed when the DB is unreachable (deliberate).
- **Operational:** `deploy/README.md` in each repo links here and holds the app-specific commands.

## References

- Argo Rollouts — BlueGreen, and Migrating to Rollouts (`workloadRef`, side-by-side).
- Argo CD — Resource Hooks (PreSync) and `ignoreDifferences`.
- Next.js — Self-hosting / ISR: per-instance cache, `refreshTags()`.
- Harness / Prisma — the expand-and-contract pattern.
