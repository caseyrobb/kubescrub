# KubeScrub

Developer reference. For operator instructions, see [README.md](README.md).

- [License](LICENSE) — MIT, Copyright (c) 2026 Casey Robb
- [Security policy](SECURITY.md)
- [CI workflow](.github/workflows/ci.yml) — runs `go test` and `go build` on push/PR
- [Release workflow](.github/workflows/release.yml) — cross-compiles on `v*` tags
- [RBAC manifests](deploy/) — scan and apply identities

## For developers

### Stack

- Go, cobra, client-go
- One Check implementation per package under `internal/checks`
- All output is `[]report.Finding` from `internal/report/finding.go`

### Hard rules

- Never kubectl delete / patch a live cluster unless the user named `kind cluster kubescrub-dev` explicitly.
- Default kube client is the current context; tests use envtest or `kind kubescrub-dev`.
- Do not shell out to popeye/kor/kubent for the primary path.
- No new dependencies without saying why.
- Every checker needs a fixture YAML + golden findings test.
- `SafeToApply` is false unless the object is terminal and outside system namespaces.
- Prefer Limit/Continue lists; do not load the whole cluster into memory.

### Makefile targets

```bash
make build    # go build -o bin/kubescrub ./cmd/kubescrub
make test     # go test ./...
make tidy     # go mod tidy
make release-check  # test + build for pre-release verification
make kind-reset    # scripts/kind.sh reset (Podman Kind cluster)
make kind-test     # go test -tags=kind ./internal/app/...
```

### How to add a checker

1. Read `.opencode/skills/add-checker/SKILL.md`
2. Implement `checks.Check`
3. Register in `internal/app/registry.go`
4. Add `testdata/fixtures/<check>/`
5. `go test ./internal/checks/...`

### OpenCode config

`opencode.jsonc` and `.opencode/` are local development aids. They may stay in the repo only if they contain **no** API keys or private endpoints. If either file references a local `baseURL` or contains credentials:

1. Add a `.gitignore` entry for that file.
2. Remove the secret/baseURL from the file.

The current `opencode.jsonc` contains a private endpoint (`http://spark.redcomet.ca:8000/v1`) and must be gitignored.

### Kubeconfig precedence

KubeScrub follows kubectl's kubeconfig precedence (set via `--kubeconfig` flag and/or the `KUBECONFIG` env variable):

1. `--kubeconfig` flag, if set.
2. `KUBECONFIG` environment variable (colon-separated list on Linux/macOS, same as client-go).
3. Default `~/.kube/config`.

The `--context` flag selects the context to use from the loaded file; without it, the file's `current-context` is used. Both `scan` and `apply` commands use the same resolution.

### Plan document

Plans are versioned JSON documents using `kubescrub.io/v1`. The `scan` command
produces a Plan wrapper. The `apply` command reads it.

- `kubescrub scan` — run all checks, print Plan JSON to stdout
- `kubescrub scan --out FILE` — write Plan JSON to FILE (also print to stdout)
- `kubescrub apply --plan FILE` — read a Plan JSON and filter findings for apply-mode deletion

Apply flags:

- `--checks string` — comma-separated check names (e.g. `workload,pvc`); empty = all
- `--reason string` — comma-separated finding reasons; empty = all
- `--namespace string` — exact namespace filter; cluster-scoped findings (empty namespace) stay included
- `--max-plan-age duration` — max age of the scan plan; default `24h`. Refuses `--apply` when exceeded (warns in dry-run)
- `--context string` — required kube context to match `plan.cluster` when non-empty. Refuses `--apply` on mismatch (warns in dry-run)

Guard rules:

- `--apply` without `--yes` → refuses and deletes nothing (existing)
- `--apply --yes` + plan older than `--max-plan-age` → refuses and deletes nothing
- `--apply --yes` + `--context` mismatch → refuses and deletes nothing
- Dry-run (`--apply` not set) → never deletes; guards only warn

Examples:
```
kubescrub apply --plan report.json --checks workload,pvc
kubescrub apply --plan report.json --reason completed-job
kubescrub apply --plan report.json --namespace kubescrub-messy
kubescrub apply --plan report.json --max-plan-age 1h --context kind-dev
```

Plan JSON structure:
```json
{
  "apiVersion": "kubescrub.io/v1",
  "kind": "Plan",
  "planVersion": 1,
  "cluster": "<kube context>",
  "generatedAt": "<RFC3339>",
  "policyPath": "policy.yaml",
  "kubescrubVersion": "",
  "findings": [ ...report.Finding... ]
}
```

The apply command loader (`internal/apply/plan.go`) also accepts a legacy bare
`[]Finding` JSON array for backward compatibility with reports generated before
the Plan wrapper was introduced.

Every `Finding.ID` is stable and non-empty, formatted as:
`check|namespace|kind|name|reason|version` (empty namespace allowed). IDs are
generated at scan-assembly time via `Plan.AddFinding` if the checker did not
already set one. No timestamps appear in IDs.

### RBAC identities

Scan and apply use **separate** identities. Never reuse the same binding.

| Operation | ClusterRole | ServiceAccount | Verbs |
|-----------|------------|----------------|-------|
| scan | `kubescrub-scan` | `kubescrub-scan` | get, list, watch (read-only) |
| apply | `kubescrub-apply` | `kubescrub-apply` | get, list, delete (jobs, replicasets, PVCs only) |

Install both with `kubectl apply -f deploy/rbac-scan.yaml` and
`kubectl apply -f deploy/rbac-apply.yaml`. See `deploy/README.md` for binding
examples.

### Kind cluster lifecycle (Podman)

Scripts in `scripts/kind.sh` manage a Podman-backed Kind cluster.

```bash
./scripts/kind.sh start     # create cluster if missing, wait until API is up
./scripts/kind.sh stop      # kind delete cluster --name kubescrub-dev
./scripts/kind.sh restart   # stop then start
./scripts/kind.sh reset     # restart, apply fixture, wait ready
./scripts/kind.sh status    # exists / context / cluster-info
./scripts/kind.sh apply     # kubectl apply messy.yaml, wait CRD established, re-apply
./scripts/kind.sh ready     # wait jobs/PVCs to settle
```

The script handles `Delegate/cgroup` errors on first create by retrying via
`systemd-run`.

Agent-friendly auto-reset:

```bash
KUBESCRUB_KIND_AUTO=1 go test -tags=kind ./internal/app/...
```

When `KUBESCRUB_KIND_AUTO=1`, the test runs `scripts/kind.sh reset` before scanning.

Golden comparison:

```bash
./scripts/kind.sh reset
go test -tags=kind ./internal/app/...
```

The test compares a stable projection of findings against
`testdata/clusters/expected-scan.json`.

### Kind (Podman)

Kind runs on Podman. Always set `KIND_EXPERIMENTAL_PROVIDER=podman`.
Cluster name: `kubescrub-dev`
Kube context: `kind-kubescrub-dev`
Fixture: `testdata/clusters/messy.yaml`
Kind policy overlay: `testdata/clusters/policy-kind.yaml`
Never use Docker as the Kind provider in docs, scripts, or shell commands.
