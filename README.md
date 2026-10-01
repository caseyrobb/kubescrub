# KubeScrub

KubeScrub is a Kubernetes cluster hygiene scanner that finds stale resources and over-permissive RBAC. It produces a versioned JSON Plan (`kubescrub.io/v1`) and performs deletions only through an opt-in apply command gated by safety checks — **report is the product, apply is optional**.

## Example finding

```json
{
  "id": "workload-completed-job|kubescrub-messy|Job|completed-job|batch/v1",
  "check": "workload",
  "severity": "info",
  "cluster": "kind-kubescrub-dev",
  "namespace": "kubescrub-messy",
  "version": "batch/v1",
  "kind": "Job",
  "name": "completed-job",
  "message": "Job kubescrub-messy/completed-job has completed with no active pods",
  "reason": "completed-job",
  "suggestedAction": "delete",
  "risk": "Completed jobs consume cluster storage and audit noise",
  "safeToApply": true,
  "evidence": {
    "active": 0,
    "succeeded": 1,
    "failed": 0
  }
}
```

## Installation

### Go install

```bash
go install github.com/caseyrobb/kubescrub@latest
```

> **Release binaries:** Download pre-built binaries from [Releases](https://github.com/caseyrobb/kubescrub/releases) once the first tagged release is published.

### From source

```bash
git clone https://github.com/caseyrobb/kubescrub.git
cd kubescrub
make build
```

Requires Go 1.23+.

## License

[MIT](LICENSE.md) — Copyright (c) 2025 KubeScrub contributors.

## Quick start

### Scan

Scan the current kubeconfig context:

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
| `--format` | json | Output format (currently only JSON) |

The scan command runs four checkers:

| Checker | What it finds |
|---------|--------------|
| `workload` | Zero-replica Deployments/StatefulSets, image tags, orphan ReplicaSets, deprecated APIs |
| `pvc` | Unbound (pending) PVCs, bound PVCs with no active pod mounting |
| `crd` | Deprecated CRD versions, CR objects not on the storage version |
| `rbac` | Unused roles, wildcard permissions, escalation verbs, cluster-admin bindings |

### Apply

Dry-run first (default — nothing is deleted):

```bash
kubescrub apply --plan plan.json
```

Perform real deletions:

```bash
kubescrub apply --plan plan.json --apply --yes
kubescrub apply --plan plan.json --checks workload,pvc --apply --yes
kubescrub apply --plan plan.json --reason completed-job --apply --yes
kubescrub apply --plan plan.json --namespace kubescrub-messy --apply --yes
kubescrub apply --plan plan.json --max-plan-age 1h --context kind-dev --apply --yes
```

| Flag | Default | Description |
|------|---------|-------------|
| `--plan` | _(required)_ | Path to scan report JSON file |
| `--checks` | all | Comma-separated check names (`workload,pvc,crd,rbac`) |
| `--reason` | all | Comma-separated finding reasons |
| `--namespace` | all | Exact namespace; cluster-scoped findings stay included |
| `--max-plan-age` | `24h` | Refuse plans older than this duration |
| `--context` | none | Require kube context to match `plan.cluster` |

Dry-run runs server-side dry-run deletes and prints a summary. `--apply --yes` triggers real deletions.

## What KubeScrub will not delete

KubeScrub enforces multiple safety gates. The following resource types are **never deleted**:

| Category | Resource kinds |
|----------|---------------|
| Hard-block (code-level) | `CustomResourceDefinition`, `ClusterRole`, `ClusterRoleBinding`, `Role`, `RoleBinding` |
| Workloads | `Deployment`, `StatefulSet` (all have `SafeToApply=false`) |
| Bound PVCs | `PersistentVolumeClaim` in `Bound` or `Available` state |
| Jobs | `Job` has `SafeToApply=false` |

At execution time, additional gates are applied:

1. `SuggestedAction` must be `"delete"`
2. `SafeToApply` must be `true`
3. The live object must carry the annotation `kubescrub.io/allow-delete=true`
4. Namespace must not be excluded in `policy.yaml`
5. Plan must not exceed `--max-plan-age`
6. `--context` must match `plan.cluster` (when set)

## Policy configuration

KubeScrub reads a YAML policy file (`--policy`, default `policy.yaml`):

```yaml
excludeNamespaces:
  - kube-system
  - kube-public
  - kube-node-lease
  - local-path-storage
thresholds:
  completedJobAge: 1s
  failedJobAge: 1s
  pendingPVCAge: 1s
  unusedPVCAge: 1s
  unusedReplicaSetAge: 1s
rbac:
  flagWildcards: true
  flagClusterAdmin: true
  ignoreSubjects:
    - system:serviceaccount:kube-system:replicaset-controller
apply:
  requireAnnotation: "kubescrub.io/allow-delete=true"
```

## RBAC

Deploy least-privilege ClusterRoles for scan and apply operations:

```bash
kubectl apply -f deploy/rbac-scan.yaml
kubectl apply -f deploy/rbac-apply.yaml
```

| Operation | ClusterRole | ServiceAccount | Verbs |
|-----------|------------|----------------|-------|
| scan | `kubescrub-scan` | `kubescrub-scan` | get, list, watch (read-only) |
| apply | `kubescrub-apply` | `kubescrub-apply` | get, list, delete (jobs, replicasets, PVCs only) |

See `deploy/rbac-scan.yaml` and `deploy/rbac-apply.yaml` for full manifests. Uncomment the `ClusterRoleBinding` sections to bind them to users or service accounts.

> **Never reuse the scan identity for apply.** They are separate ClusterRoles and separate ServiceAccounts.

## Podman Kind (testing)

```bash
export KIND_EXPERIMENTAL_PROVIDER=podman
./scripts/kind.sh reset
go test -tags=kind ./internal/app/...
```

Cluster: `kubescrub-dev` · Context: `kind-kubescrub-dev` · Fixture: `testdata/clusters/messy.yaml`

Run with automatic cluster reset:

```bash
KUBESCRUB_KIND_AUTO=1 go test -tags=kind ./internal/app/...
```

## Building & testing

```bash
make build      # go build -o bin/kubescrub ./cmd/kubescrub
make test       # go test ./...
make test-cover # go test -coverprofile=coverage.out -count=1 ./...
make tidy       # go mod tidy
```
