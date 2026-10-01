# Changelog

## [Unreleased]

## [v0.1.0] — 2026-XX-XX

### Added

- `workload` checker — zero-replica Deployments/StatefulSets, missing image tags, orphan ReplicaSets, deprecated API versions
- `pvc` checker — unbound (pending) PVCs, bound PVCs with no active pod mounting
- `crd` checker — deprecated CRD versions, CR objects not on the storage version
- `rbac` checker — unused roles, wildcard permissions, escalation verbs, cluster-admin bindings
- Plan schema `kubescrub.io/v1` — versioned JSON wrapper for scan findings
- `kubescrub-scan` ServiceAccount and ClusterRole — read-only identity
- `kubescrub-apply` ServiceAccount and ClusterRole — minimal delete identity (jobs, replicasets, PVCs only)
- Apply refuse rules: `--apply` requires `--yes`, plan age guard (`--max-plan-age`), context guard (`--context`)
- Annotation gate: apply only deletes objects with `kubescrub.io/allow-delete=true`
- Filter flags: `--checks`, `--reason`, `--namespace`
