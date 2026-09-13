# Migrating a Deployment to Argo Rollouts blue/green

For any DB-backed app whose customer-facing workload is a plain `Deployment`:
a rolling update deliberately runs old and new pods together, so one URL can
serve two versions. Blue/green fixes that; this is the one-time migration.

It is **staged** because Argo CD auto-sync can reach production without a Kargo
promotion, so every step must be safe on its own.

## Prerequisites

- Argo Rollouts is installed on the target cluster.
- The app's Argo CD Application carries the selector ignore (see
  `argocd/applicationset.yaml`):
  ```yaml
  ignoreDifferences:
    - kind: Service
      jsonPointers: [/spec/selector]
  syncPolicy:
    syncOptions: [RespectIgnoreDifferences=true]
  ```
  Without it, a sync reverts the Rollouts-owned Service selector and one URL
  serves two versions again.
- Readiness is dependency-aware (e.g. `/ready` checks the database or the
  upstream service), so a broken green is never promoted.

## Steps

1. **Add the Rollout beside the live Deployment** with the **identical image**
   and the same pod template/selector, plus a `previewService`. Keep the
   Deployment serving. The controller moves the `activeService` to the green
   ReplicaSet only once green is Ready. Because the image is identical, any
   transient overlap is behaviorally identical — no version mix even if the
   selector is momentarily broad.
2. **Verify** green is Ready and the active Service selector carries a
   `rollouts-pod-template-hash`; the old ReplicaSet receives no traffic.
3. **Idle the blue Deployment** (`replicas: 0`). Safe: green already serves.
4. **Delete the Deployment.** Put the active Services and any ServiceAccount in
   the Rollout's manifest files, not the Deployment's, so deletion cannot remove
   the Rollout's dependencies. Argo CD `prune` defaults **false** — enable it on
   the Application, or `kubectl delete` explicitly.

## Guard test (copy into the repo's deploy test)

```go
for _, name := range []string{"<app>-api", "<app>-web"} {
    rollout := findResource(t, resources, "Rollout", name)
    if rollout.Spec.Strategy.BlueGreen.ActiveService != name {
        t.Errorf("%s activeService must be %q", name, name)
    }
    if rollout.Spec.Strategy.BlueGreen.PreviewService != name+"-preview" {
        t.Errorf("%s previewService must be %q-preview", name, name)
    }
    findResource(t, resources, "Service", name)          // active
    findResource(t, resources, "Service", name+"-preview")
    // During the migration window ONLY, additionally assert the Rollout runs the
    // same image as the still-present transitional Deployment (no version mix).
    // After the Deployment is removed, drop that assertion — the parity guard
    // lives in the transitional step, not in steady state.
}
```

Also assert the Application config so the selector guard cannot silently regress:
decode `argocd/apps.yaml` and require `ignoreDifferences` of `kind: Service` with
`jsonPointers: [/spec/selector]` plus `RespectIgnoreDifferences=true`.

## Gotcha: a probe must not outrun the image

Changing the readiness probe path (e.g. to a new `/ready`) **before** the image
that serves it is promoted makes every pod carrying the new probe NotReady —
including the blue stack, if it re-rolls. During the accounts migration this
stuck the green web ReplicaSet (probe `404`), and a blue re-roll would have taken
web down. Ship the route in the image first (build + promote), then switch the
probe; if the image is behind, point readiness at the existing dependency-free
path until the new image is live. Migration manifests and their images promote
separately, so order them deliberately.

## Reference implementations

- `patty-accounts/deploy/base/{api,web}-rollout.yaml` — converted (staging + production)
- `patty-corp/deploy/base/rollout.yaml` — converted (staging + production)
