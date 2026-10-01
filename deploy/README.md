# KubeScrub RBAC manifests

Least-privilege ClusterRoles for KubeScrub scan and apply operations.

## Apply CLI flags (guard rules)

The `apply` command has four new flags that enforce safety gates:

- `--checks` — comma-separated check names (e.g. `workload,pvc`); empty = all
- `--reason` — comma-separated finding reasons; empty = all
- `--namespace` — exact namespace filter; cluster-scoped findings (empty namespace) stay included
- `--max-plan-age` — max age of the scan plan (default `24h`); refuses `--apply` when exceeded
- `--context` — required kube context to match `plan.cluster`; refuses `--apply` on mismatch

Two hard refuse rules (no deletions when violated):

1. **Plan age guard**: `--apply --yes` with a plan older than `--max-plan-age` is refused.
2. **Context guard**: `--apply --yes` with `--context` that doesn't match `plan.cluster` is refused.

Both guards only warn in dry-run mode. Dry-run (`--apply` not set) never deletes.

## Scan identity

**ClusterRole**: `kubescrub-scan`
**ServiceAccount**: `kubescrub-scan` (namespace `kubescrub-system`)
**Verbs**: `get`, `list`, `watch` only — no create/update/patch/delete

Reads pods, PVCs, PVs, namespaces, service accounts, configmaps, deployments,
replicasets, statefulsets, jobs, RBAC objects, and CRDs.

## Apply identity

**ClusterRole**: `kubescrub-apply`
**ServiceAccount**: `kubescrub-apply` (namespace `kubescrub-system`)
**Verbs**: `get`, `list`, `delete` on jobs, replicasets, persistentvolumeclaims only

The apply command has additional safety gates enforced in code (SafeToApply,
SuggestedAction, annotation gate, namespace exclusion, `--apply --yes` flags).

## Key rule

> **Never reuse the scan identity for apply.** Scan is `kubescrub-scan` (read-only).
> Apply is `kubescrub-apply` (read + delete). They are separate ClusterRoles and
> separate ServiceAccounts.

### 1. Install scan RBAC

```bash
kubectl apply -f deploy/rbac-scan.yaml
```

This creates the `kubescrub-system` namespace, the `kubescrub-scan` ServiceAccount,
and the `kubescrub-scan` ClusterRole.

### 2. Install apply RBAC

```bash
kubectl apply -f deploy/rbac-apply.yaml
```

This creates the `kubescrub-apply` ServiceAccount and ClusterRole.

### 3. Bind the scan ClusterRole

Edit `deploy/rbac-scan.yaml` and uncomment the `ClusterRoleBinding` section at the
bottom. Replace `<YOUR_KUBECONFIG_USER>` with your kubeconfig user name:

```yaml
subjects:
  - kind: User
    name: "alice@company.com"
    apiGroup: rbac.authorization.k8s.io
```

Then bind:

```bash
# Option A: bind to a user
kubectl create clusterrolebinding kubescrub-scan-binding \
  --clusterrole=kubescrub-scan \
  --user=alice@company.com

# Option B: bind to a group
kubectl create clusterrolebinding kubescrub-scan-binding \
  --clusterrole=kubescrub-scan \
  --group=devops-team

# Option C: bind to the ServiceAccount directly
kubectl create clusterrolebinding kubescrub-scan-binding \
  --clusterrole=kubescrub-scan \
  --serviceaccount=kubescrub-system:kubescrub-scan
```

### 4. Bind the apply ClusterRole

Edit `deploy/rbac-apply.yaml` and uncomment the `ClusterRoleBinding` section at
the bottom. Replace `<YOUR_KUBECONFIG_USER>` with your kubeconfig user name.

Then bind:

```bash
kubectl create clusterrolebinding kubescrub-apply-binding \
  --clusterrole=kubescrub-apply \
  --user=alice@company.com
```

### 5. Scan with the identity

```bash
# Using a kubeconfig user
kubectl --user=alice@company.com kubescrub scan

# Using the ServiceAccount
kubectl --as=system:serviceaccount:kubescrub-system:kubescrub-scan \
  kubescrub scan > plan.json

# Apply a scan report
kubectl --as=system:serviceaccount:kubescrub-system:kubescrub-apply \
  kubescrub apply --plan plan.json --apply --yes
```

## Optional ClusterRoles

### CR instance listing (`kubescrub-scan-cr`)

The `kubescrub-scan-cr` ClusterRole grants `get`/`list` on **all** resources
(`*`/`*`). This is required for the **crd checker** to dynamically list custom
resource instances on each GVR. It is still read-only (no delete).

To install:

```bash
kubectl apply -f deploy/rbac-scan.yaml   # includes the CR ClusterRole
kubectl create clusterrolebinding kubescrub-scan-cr-binding \
  --clusterrole=kubescrub-scan-cr \
  --user=alice@company.com
```

### Helm release manifests (`kubescrub-scan-helm`)

The `kubescrub-scan-helm` ClusterRole grants `get`/`list` on Secrets and
ConfigMaps. This is only needed if you want KubeScrub to inspect Helm release
manifests for deprecated API version references. **Do not bind this by default.**

To install:

```bash
kubectl apply -f deploy/rbac-scan.yaml   # includes the Helm ClusterRole
kubectl create clusterrolebinding kubescrub-scan-helm-binding \
  --clusterrole=kubescrub-scan-helm \
  --user=alice@company.com
```

## Rule reference

### kubescrub-scan (ClusterRole)

| API Group               | Resources                                        | Verbs              |
|-------------------------|--------------------------------------------------|--------------------|
| `""` (core)             | pods, persistentvolumeclaims, persistentvolumes, namespaces, serviceaccounts, configmaps | get, list, watch   |
| `apps`                  | deployments, replicasets, statefulsets           | get, list, watch   |
| `batch`                 | jobs                                             | get, list, watch   |
| `rbac.authorization.k8s.io` | roles, rolebindings, clusterroles, clusterrolebindings | get, list, watch |
| `apiextensions.k8s.io`  | customresourcedefinitions                        | get, list, watch   |
| _(nonResourceURLs)_     | `/api`, `/api/*`, `/apis`, `/apis/*`            | get                |

No `create`, `update`, `patch`, `delete`, `bind`, `escalate`, or `impersonate` verbs.

### kubescrub-scan-cr (ClusterRole, optional)

| API Group | Resources | Verbs |
|-----------|-----------|-------|
| `*`       | `*`       | get, list |

Required for the crd checker. Still read-only.

### kubescrub-scan-helm (ClusterRole, optional)

| API Group | Resources | Verbs |
|-----------|-----------|-------|
| `""` (core) | secrets, configmaps | get, list |

Only for Helm release inspection.

### kubescrub-apply (ClusterRole)

| API Group               | Resources               | Verbs               |
|-------------------------|-------------------------|---------------------|
| `batch`                 | jobs                    | get, list, delete   |
| `apps`                  | replicasets             | get, list, delete   |
| `""` (core)             | persistentvolumeclaims  | get, list, delete   |
| `""` (core)             | namespaces              | get                 |

Only these three resource types can be deleted. All other safety gates are
enforced in code.
