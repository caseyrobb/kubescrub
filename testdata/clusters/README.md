# Kind fixture cluster

Provider is Podman:

```bash
export KIND_EXPERIMENTAL_PROVIDER=podman
```

## Quick start

```bash
./scripts/kind.sh reset    # create cluster, apply fixture, wait ready
bin/kubescrub scan --policy testdata/clusters/policy-kind.yaml
```

Or manually:

```bash
kind create cluster --name kubescrub-dev --wait 60s
kubectl --context kind-kubescrub-dev apply -f testdata/clusters/messy.yaml
kubectl --context kind-kubescrub-dev wait --for=condition=Established crd/widgets.scrub.kubescrub.io --timeout=60s
kubectl --context kind-kubescrub-dev apply -f testdata/clusters/messy.yaml
```

## Scan with short ages

```bash
bin/kubescrub scan --policy testdata/clusters/policy-kind.yaml
```

## Kind lifecycle commands

```bash
./scripts/kind.sh start     # create if missing, wait for API
./scripts/kind.sh stop      # kind delete cluster
./scripts/kind.sh restart   # stop + start
./scripts/kind.sh reset     # restart + fixture + ready
./scripts/kind.sh apply     # re-apply fixture only
./scripts/kind.sh status    # exists / context / cluster-info
./scripts/kind.sh ready     # wait jobs/PVCs
```

## Golden comparison

```bash
./scripts/kind.sh reset
go test -tags=kind ./internal/app/...
```

The test compares a stable projection against `testdata/clusters/expected-scan.json`.
Set `KUBESCRUB_KIND_AUTO=1` to have the test auto-run `scripts/kind.sh reset` if the cluster is down.

## Fixture objects and expected findings

The fixture (`testdata/clusters/messy.yaml`) creates these objects in namespace `kubescrub-messy`:

| Object | Kind | Expected check finding |
|--------|------|----------------------|
| `zero-web` | Deployment (0 replicas) | workload / zero-replicas |
| `zero-db` | StatefulSet (0 replicas) | workload / zero-replicas |
| `live-web` | Deployment (1 replica, healthy) | **no** zero-replica finding |
| `live-web-legacy` | ReplicaSet (no ownerRef) | workload / orphan-replicaset |
| `old-success` | Job (completed) | workload / completed-job |
| `old-failed` | Job (failed) | workload / completed-job |
| `pending-unbound` | PVC (Pending) | pvc / unbound-pvc |
| `unused-bound` | PVC (Bound, no pod mounts) | pvc / unused-pvc |
| `messy-wildcard` | Role (wildcard apiGroups/resources/verbs) | rbac / rbac-wildcard |
| `messy-dangling` | RoleBinding → SA `does-not-exist` | rbac / rbac-dangling-subject |
| `messy-unused` | Role (no RoleBinding) | rbac / rbac-unused-role |
| `messy-escalate` | Role (bind/escalate/impersonate) | rbac / rbac-privilege-verb |
| `kubescrub-messy-admin` | ClusterRoleBinding → cluster-admin | rbac / rbac-cluster-admin |
| `widgets.scrub.kubescrub.io` | CRD (deprecated v1alpha1, storage v1) | crd / crd-version-deprecated |
| `rusty` | Widget CR (on v1alpha1, not storage v1) | crd / cr-not-storage-version |

**Negative expectations:**
- No findings from `kube-system`, `kube-public`, or `kube-node-lease`
- `live-web` Deployment is NOT a zero-replica finding
- Widget/CR rows have `check=crd`

**Known gaps:**
- The deprecated checker (`internal/checks/deprecated`) returns empty — it has not been implemented yet.
- The workload checker does not detect deprecated API versions from `last-applied-configuration` annotations. The zero-web Deployment has an annotation referencing `apps/v1beta1`, but the scanner only detects resources currently served on deprecated GVRs via dynamic listing. On a modern cluster with only v1 resources, this produces no findings.

## Cleanup

```bash
KIND_EXPERIMENTAL_PROVIDER=podman kind delete cluster --name kubescrub-dev
```
