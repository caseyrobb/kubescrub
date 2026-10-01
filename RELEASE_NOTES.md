# v0.1.0 Release Notes

## What's new

KubeScrub v0.1.0 is the first public release. It ships a cluster hygiene scanner and a gated deletion apply command.

### Four built-in checkers

| Checker | Purpose |
|---------|---------|
| `workload` | Zero-replica Deployments/StatefulSets, missing image tags, orphan ReplicaSets, deprecated APIs |
| `pvc` | Unbound (pending) PVCs, bound PVCs with no active pod mounting |
| `crd` | Deprecated CRD versions, CR objects not on the storage version |
| `rbac` | Unused roles, wildcard permissions, escalation verbs, cluster-admin bindings |

### Plan schema: `kubescrub.io/v1`

The `scan` command produces a versioned JSON Plan:

```json
{
  "apiVersion": "kubescrub.io/v1",
  "kind": "Plan",
  "planVersion": 1,
  "cluster": "<kube context>",
  "generatedAt": "2025-01-01T00:00:00Z",
  "policyPath": "policy.yaml",
  "kubescrubVersion": "0.1.0",
  "findings": [...]
}
```

The `apply` command also accepts the legacy bare `[]Finding` array for backward compatibility.

### RBAC identities

Two separate ServiceAccounts enforce least-privilege:

| Identity | ClusterRole | Verbs |
|----------|------------|-------|
| `kubescrub-scan` (SA: `kubescrub-scan`) | `kubescrub-scan` | get, list, watch |
| `kubescrub-apply` (SA: `kubescrub-apply`) | `kubescrub-apply` | get, list, delete (jobs, replicasets, PVCs only) |

Install manifests: `deploy/rbac-scan.yaml`, `deploy/rbac-apply.yaml`.

### Apply refuse rules

The `apply` command **refuses** real deletion (`--apply --yes`) when any of these conditions hold:

1. `--yes` not provided — `--apply` without `--yes` always errors.
2. Plan age exceeds `--max-plan-age` (default 24h).
3. `--context` does not match `plan.cluster`.

Dry-run (`--apply` not set) never deletes; violations only produce warnings.

### Binary releases

- `kubescrub-linux-amd64`
- `kubescrub-linux-arm64`
- `kubescrub-darwin-amd64`
- `kubescrub-darwin-arm64`

SHA-256 checksums are included in each release asset.

## Known limitations

- Podman-in-Kind cluster tests are not part of CI (requires privileged host). Use `make kind-test` locally.
- The `deprecated` checker has been removed (its responsibility is covered by `workload`).
- Per-namespace thresholds are not yet implemented.
