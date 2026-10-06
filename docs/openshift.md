# OpenShift checks

OpenShift checks run only when the API server serves their respective API group. A plain Kubernetes cluster is not an error — these checks discover their APIs and return no findings if the group is absent.

## v1

### route (route.openshift.io/v1)

Scans Routes for misconfigured backends:

- **route-missing-service**: Route targets a Service name that does not exist in that namespace.
- **route-port-mismatch**: Route `spec.port.targetPort` is not present as a port on the target Service.
- **route-no-endpoints**: Route targets a Service that exists but has zero ready EndpointSlices.

`SafeToApply` is `false` and `SuggestedAction` is `report` — Routes are never deleted.

### csv (operators.coreos.com/v1alpha1)

Scans ClusterServiceVersions against Subscription status:

- **csv-not-current**: CSV exists but is not named by any Subscription's `status.currentCSV` in that namespace.
- **csv-stuck**: CSV is in `Failed` or `Pending` phase for more than one hour. Applies to both current and non-current CSVs.

`SafeToApply` is `false` for all CSV findings — the current CSV is never a delete candidate.

## Discovery rule

If `route.openshift.io` is not available, the `route` check returns no findings and no error.
If `operators.coreos.com` is not available, the `csv` check returns no findings and no error.
Excluded namespaces (from policy + kube-system, kube-public, kube-node-lease) are skipped.

## Later

Missing CatalogSource, overlapping OperatorGroups, zero-replica DeploymentConfigs, old completed Builds, unreferenced ImageStreams, unreferenced custom SCCs. No MachineSet, ClusterOperator, or image-pruner deletes.

## Identity

Scan needs get/list on routes, services, endpointslices, clusterserviceversions, subscriptions. Keep these in the optional ClusterRole `kubescrub-scan-openshift` (deployed alongside `deploy/rbac-scan.yaml`), not in the default Kubernetes scan role.
