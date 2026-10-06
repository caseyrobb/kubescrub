# KubeScrub

KubeScrub is a Kubernetes cluster hygiene scanner. It produces a versioned JSON Plan documenting stale resources and over-permissive RBAC. Deletions happen only through the apply command and are gated by multiple safety checks — **report is the product, apply is opt-in**.

> **Safety:** scan is read-only only when run with the `kubescrub-scan` ServiceAccount (via `kubectl --as=`). Running with a user kubeconfig has full read access to that context. Apply deletes only objects annotated with `kubescrub.io/allow-delete=true` and only when `--apply --yes` is provided.

## What scan reports

KubeScrub runs four built-in checkers:

| Checker | What it finds |
|---------|---------------|
| `workload` | Zero-replica Deployments/StatefulSets, missing image tags, orphan ReplicaSets, deprecated API versions |
| `pvc` | Unbound (pending) PVCs, bound PVCs with no active pod mounting |
| `crd` | Deprecated CRD versions, CR objects not on the storage version |
| `rbac` | Unused roles, wildcard permissions, escalation verbs, cluster-admin bindings |

## What apply can delete

Apply can delete resources that pass all safety gates and carry the annotation `kubescrub.io/allow-delete=true`:

- Completed Jobs
- ReplicaSets without a Deployment owner (orphan)

## What KubeScrub will never delete

| Kind | Reason |
|------|--------|
| `CustomResourceDefinition` | Hard-block (code-level) |
| `ClusterRole` | Hard-block (code-level) |
| `ClusterRoleBinding` | Hard-block (code-level) |
| `Role` | Hard-block (code-level) |
| `RoleBinding` | Hard-block (code-level) |
| `Deployment` | `SafeToApply` is always `false` |
| `StatefulSet` | `SafeToApply` is always `false` |
| `PersistentVolumeClaim` (Bound/Available) | `SafeToApply` is `false` for bound PVCs |
| `Job` | `SafeToApply` is `false` |

Additional execution gates: `SuggestedAction` must be `"delete"`, the live object must have the `kubescrub.io/allow-delete=true` annotation, and the namespace must not be in `policy.yaml`'s `excludeNamespaces`.

## Build and test

```bash
make build      # go build -o bin/kubescrub ./cmd/kubescrub
make test       # go test ./...
make tidy       # go mod tidy
```

Requires Go 1.23+.

## Scan

```bash
kubescrub scan
kubescrub scan --out plan.json
kubescrub scan --namespaces production,staging
kubescrub scan --policy my-policy.yaml
```

| Flag | Default | Description |
|------|---------|-------------|
| `--policy` | `policy.yaml` | Path to policy configuration file |
| `--out` | stdout | Write Plan JSON to FILE |
| `--namespaces` | all | Comma-separated list of namespaces to scan |
| `--format` | json | Output format (only `json` is supported) |
| `--kubeconfig` | _(see below)_ | Path to kubeconfig file |
| `--context` | _(see below)_ | Kube context to use |

## Apply

Dry-run (`--apply` not set) performs server-side dry-run deletes and prints a summary. Nothing is deleted.

```bash
kubescrub apply --plan plan.json
```

Real deletion:

```bash
kubescrub apply --plan plan.json --apply --yes
kubescrub apply --plan plan.json --checks workload,pvc --apply --yes
kubescrub apply --plan plan.json --reason completed-job --apply --yes
kubescrub apply --plan plan.json --namespace kubescrub-messy --apply --yes
kubescrub apply --plan plan.json --context kind-dev --apply --yes
```

| Flag | Default | Description |
|------|---------|-------------|
| `--plan` | _(required)_ | Path to scan report JSON file |
| `--checks` | all | Comma-separated check names to include |
| `--reason` | all | Comma-separated finding reasons to include |
| `--namespace` | all | Exact namespace; cluster-scoped findings kept |
| `--max-plan-age` | `24h` | Maximum age of the scan plan |
| `--context` | none | Require kube context to match `plan.cluster` |
| `--apply` | false | Enable real deletion (requires `--yes`) |
| `--yes` | false | Acknowledge real deletion |

### Refuse rules

Real deletion (`--apply --yes`) **refuses** when any guard is violated:

1. `--yes` not provided — `--apply` without `--yes` is always an error.
2. Plan age exceeds `--max-plan-age` (default 24h).
3. `--context` does not match `plan.cluster`.

Dry-run mode continues with warnings only.

## Kubeconfig

KubeScrub respects the same kubeconfig precedence as kubectl. Set `--kubeconfig` to override the file path, or use `--context` to select a specific context from the loaded file:

1. `--kubeconfig` flag, if set.
2. `KUBECONFIG` environment variable (colon-separated list on Linux/macOS).
3. Default `~/.kube/config`.

The `--context` flag selects the context to use; without it, the loaded file's `current-context` is used.

Install least-privilege ClusterRoles for scan and apply operations:

```bash
kubectl apply -f deploy/rbac-scan.yaml
kubectl apply -f deploy/rbac-apply.yaml
```

| Identity | ServiceAccount | Verbs |
|----------|---------------|-------|
| scan | `kubescrub-scan` (in `kubescrub-system`) | get, list, watch |
| apply | `kubescrub-apply` (in `kubescrub-system`) | get, list, delete (jobs, replicasets, PVCs only) |

> **Never reuse the scan identity for apply.** They are separate ClusterRoles and ServiceAccounts.

See [`deploy/rbac-scan.yaml`](deploy/rbac-scan.yaml) and [`deploy/rbac-apply.yaml`](deploy/rbac-apply.yaml) for full manifests. Uncomment the `ClusterRoleBinding` sections to bind them to users or service accounts.

## Podman Kind (testing)

```bash
export KIND_EXPERIMENTAL_PROVIDER=podman
./scripts/kind.sh reset
go test -tags=kind ./internal/app/...
```

Cluster: `kubescrub-dev` · Context: `kind-kubescrub-dev` · Fixture: `testdata/clusters/messy.yaml`

See [`testdata/clusters/README.md`](testdata/clusters/README.md) for details.

## License

[MIT](LICENSE) — Copyright (c) 2026 Casey Robb.
