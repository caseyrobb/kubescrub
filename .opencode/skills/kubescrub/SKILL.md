---
name: kubescrub
description: Domain rules for KubeScrub cluster hygiene checks
---

- ReplicaSets owned by a Deployment are revision history, not orphans.
- Pending PVC is unbound; Bound + no pod is unused, not unbound.
- StatefulSet scale-down keeps PVCs on purpose.
- CR storage version lives on the CRD spec, not on kubectl api-resources preferred version.
- Zero-replica Deployments/StatefulSets are reported, not deleted.
- System namespaces are excluded unless policy says otherwise.
- Apply requires kubescrub.io/allow-delete=true and --apply.
- Local clusters are Kind on Podman (`KIND_EXPERIMENTAL_PROVIDER=podman`), context `kind-kubescrub-dev`.
