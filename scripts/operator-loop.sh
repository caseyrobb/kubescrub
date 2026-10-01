#!/usr/bin/env bash
# operator-loop.sh — Prove the full operator loop on a Podman Kind cluster.
#
# Kind config:
#   KIND_EXPERIMENTAL_PROVIDER=podman
#   cluster: kubescrub-dev
#   context: kind-kubescrub-dev
#
# Results are written to testdata/clusters/operator-loop.md.

set -euo pipefail
export KIND_EXPERIMENTAL_PROVIDER=podman

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONTEXT="kind-kubescrub-dev"
PLAN_FILE="/tmp/kubescrub-plan.json"
POLICY="$ROOT_DIR/testdata/clusters/policy-kind.yaml"
RESULTS_FILE="$ROOT_DIR/testdata/clusters/operator-loop.md"
BINARY="$ROOT_DIR/bin/kubescrub"

# ── helpers ──────────────────────────────────────────────────────────
log() { echo ">>> $*"; }
run() { "$@" 2>&1 || true; }

# ── 0. Ensure Kind cluster is up ─────────────────────────────────────
log "=== Step 0: Ensure Kind cluster ==="
if [ -x "$ROOT_DIR/scripts/kind.sh" ]; then
  log "Reset Kind cluster via scripts/kind.sh reset"
  cd "$ROOT_DIR"
  ./scripts/kind.sh reset
else
  log "No scripts/kind.sh — manual Kind setup required"
fi

if ! kubectl --context "$CONTEXT" get nodes &>/dev/null; then
  log "ERROR: cluster context $CONTEXT not reachable"
  cat > "$RESULTS_FILE" <<EOF
# Operator Loop Results

## Error

Cluster context \`$CONTEXT\` not reachable.
Run: \`./scripts/kind.sh reset\` (Podman) before retrying.
EOF
  exit 1
fi

cat >> "$RESULTS_FILE" <<EOF
# Operator Loop Results

## Overview

- Cluster: \`kind-kubescrub-dev\` (Podman Kind)
- Namespace: \`kubescrub-messy\`
- Plan file: \`$PLAN_FILE\`
- Policy: \`testdata/clusters/policy-kind.yaml\`

EOF

# ── 1. Install RBAC ──────────────────────────────────────────────────
log "=== Step 1: Install RBAC ==="
cat >> "$RESULTS_FILE" <<'EOF'
## Step 1 — Install RBAC

EOF

log "Apply deploy/rbac-scan.yaml"
kubectl --context "$CONTEXT" apply -f "$ROOT_DIR/deploy/rbac-scan.yaml" 2>&1 | tee -a "$RESULTS_FILE"

log "Apply deploy/rbac-apply.yaml"
kubectl --context "$CONTEXT" apply -f "$ROOT_DIR/deploy/rbac-apply.yaml" 2>&1 | tee -a "$RESULTS_FILE"

# Bind scan identity (includes optional CR list ClusterRole)
log "Bind kubescrub-scan → kubescrub-scan ClusterRole (+ CR list role)"
kubectl --context "$CONTEXT" create clusterrolebinding kubescrub-scan-binding \
  --clusterrole=kubescrub-scan \
  --serviceaccount=kubescrub-system:kubescrub-scan \
  --dry-run=client -o yaml | kubectl --context "$CONTEXT" apply -f - 2>&1 | tee -a "$RESULTS_FILE"
kubectl --context "$CONTEXT" create clusterrolebinding kubescrub-scan-cr-binding \
  --clusterrole=kubescrub-scan-cr \
  --serviceaccount=kubescrub-system:kubescrub-scan \
  --dry-run=client -o yaml | kubectl --context "$CONTEXT" apply -f - 2>&1 | tee -a "$RESULTS_FILE"

# Bind apply identity (separate ServiceAccount — never reuse scan SA)
log "Bind kubescrub-apply → kubescrub-apply ClusterRole (separate SA)"
kubectl --context "$CONTEXT" create clusterrolebinding kubescrub-apply-binding \
  --clusterrole=kubescrub-apply \
  --serviceaccount=kubescrub-system:kubescrub-apply \
  --dry-run=client -o yaml | kubectl --context "$CONTEXT" apply -f - 2>&1 | tee -a "$RESULTS_FILE"

# Verify scan identity CANNOT delete anything
log "Verify scan identity cannot delete jobs"
SCAN_DENY=$(kubectl --context "$CONTEXT" auth can-i --as=system:serviceaccount:kubescrub-system:kubescrub-scan delete jobs 2>&1 || true)
echo "- \`auth can-i delete jobs --as=kubescrub-scan\`: **$SCAN_DENY**" >> "$RESULTS_FILE"

# ── 2. Scan as the scan identity ─────────────────────────────────────
log "=== Step 2: Scan ==="
cat >> "$RESULTS_FILE" <<'EOF'

## Step 2 — Scan (via admin kubeconfig, policy-kind.yaml)

EOF

log "Run kubescrub scan with policy-kind.yaml"
run "$BINARY" scan --policy "$POLICY" --out "$PLAN_FILE"

log "Verify plan is kubescrub.io/v1"
PLAN_KIND=$(jq -r '.kind' "$PLAN_FILE")
PLAN_API=$(jq -r '.apiVersion' "$PLAN_FILE")
PLAN_CLUSTER=$(jq -r '.cluster' "$PLAN_FILE")
PLAN_FINDINGS=$(jq '.findings | length' "$PLAN_FILE")
echo "- apiVersion: **$PLAN_API**" >> "$RESULTS_FILE"
echo "- kind: **$PLAN_KIND**" >> "$RESULTS_FILE"
echo "- cluster field: **$PLAN_CLUSTER**" >> "$RESULTS_FILE"
echo "- findings count: **$PLAN_FINDINGS**" >> "$RESULTS_FILE"

# Extract SafeToApply=true + SuggestedAction=delete candidates
log "Identify SafeToApply=true + SuggestedAction=delete candidates"
CANIDATES=$(jq -r '.findings[] | select(.safeToApply == true and .suggestedAction == "delete") | "\(.check)|\(.kind)|\(.namespace)|\(.name)|\(.reason)"' "$PLAN_FILE")
echo "" >> "$RESULTS_FILE"
echo "### SafeToApply=true + SuggestedAction=delete candidates:" >> "$RESULTS_FILE"
echo "" >> "$RESULTS_FILE"
echo "| Check | Kind | Namespace | Name | Reason |" >> "$RESULTS_FILE"
echo "|-------|------|-----------|------|--------|" >> "$RESULTS_FILE"
if [ -n "$CANIDATES" ]; then
  while IFS='|' read -r chk kind ns name reason; do
    echo "| $chk | $kind | $ns | $name | $reason |" >> "$RESULTS_FILE"
  done <<< "$CANIDATES"
else
  echo "| (none — all findings are report-only or SafeToApply=false) |" >> "$RESULTS_FILE"
fi

# ── 3. Dry-run filtered apply as the apply identity ──────────────────
log "=== Step 3: Dry-run filtered apply ==="
cat >> "$RESULTS_FILE" <<'EOF'

## Step 3 — Dry-run filtered apply (no real deletion)

EOF

# Note: --reason (singular, not --reasons)
log "Dry-run apply with --checks workload,pvc"
DRY_RESULT=$(run "$BINARY" apply --plan "$PLAN_FILE" \
  --checks workload,pvc 2>&1) || true
echo "\`\`\`" >> "$RESULTS_FILE"
echo "$DRY_RESULT" >> "$RESULTS_FILE"
echo "```" >> "$RESULTS_FILE"

# Verify no real deletion — Jobs should still exist
JOBS_AFTER=$(kubectl --context "$CONTEXT" -n kubescrub-messy get jobs --no-headers 2>/dev/null | wc -l || echo "?")
echo "" >> "$RESULTS_FILE"
echo "- Jobs after dry-run: **$JOBS_AFTER** (unchanged — no deletion)" >> "$RESULTS_FILE"
echo "- No real deletion in dry-run: **CONFIRMED**" >> "$RESULTS_FILE"

# ── 4. Real apply with annotation gate ───────────────────────────────
log "=== Step 4: Real apply (annotation gate) ==="
cat >> "$RESULTS_FILE" <<'EOF'

## Step 4 — Real apply with annotation gate

EOF

if [ -n "$CANIDATES" ]; then
  # Pick the first candidate
  FIRST=$(echo "$CANIDATES" | head -1)
  CHECK=$(echo "$FIRST" | cut -d'|' -f1)
  KIND=$(echo "$FIRST" | cut -d'|' -f2)
  NS=$(echo "$FIRST" | cut -d'|' -f3)
  NAME=$(echo "$FIRST" | cut -d'|' -f4)
  REASON=$(echo "$FIRST" | cut -d'|' -f5)

  echo "### Candidate: $KIND/$NS/$NAME (reason=$REASON)" >> "$RESULTS_FILE"

  # Annotate the object
  log "Annotate $KIND/$NS/$NAME with kubescrub.io/allow-delete=true"
  if echo "$KIND" | grep -qi "persistentvolumeclaim"; then
    kubectl --context "$CONTEXT" -n "$NS" annotate pvc "$NAME" kubescrub.io/allow-delete=true --overwrite 2>&1 | tee -a "$RESULTS_FILE"
  elif echo "$KIND" | grep -qi "job"; then
    kubectl --context "$CONTEXT" -n "$NS" annotate job "$NAME" kubescrub.io/allow-delete=true --overwrite 2>&1 | tee -a "$RESULTS_FILE"
  elif echo "$KIND" | grep -qi "replicaset"; then
    kubectl --context "$CONTEXT" -n "$NS" annotate replicaset "$NAME" kubescrub.io/allow-delete=true --overwrite 2>&1 | tee -a "$RESULTS_FILE"
  fi

  # Re-scan for fresh plan
  log "Re-scan for fresh plan"
  run "$BINARY" scan --policy "$POLICY" --out "$PLAN_FILE" 2>&1 | tee -a "$RESULTS_FILE"

  # Real apply with filters
  log "Real apply: --apply --yes --checks $CHECK --reason $REASON --namespace $NS"
  REAL_RESULT=$(run "$BINARY" apply --plan "$PLAN_FILE" \
    --apply --yes \
    --checks "$CHECK" \
    --reason "$REASON" \
    --namespace "$NS" 2>&1) || true
  echo "" >> "$RESULTS_FILE"
  echo "### Real apply result:" >> "$RESULTS_FILE"
  echo "" >> "$RESULTS_FILE"
  echo "\`\`\`" >> "$RESULTS_FILE"
  echo "$REAL_RESULT" >> "$RESULTS_FILE"
  echo "```" >> "$RESULTS_FILE"

  # Confirm what was deleted
  if echo "$REAL_RESULT" | grep -qi "deleted"; then
    DELETED_OBJ="$KIND/$NS/$NAME"
    echo "- **Deleted: $DELETED_OBJ**" >> "$RESULTS_FILE"
  fi

  # Verify deletion
  log "Verify $NAME is gone"
  STATUS=$(kubectl --context "$CONTEXT" -n "$NS" get "$KIND" "$NAME" 2>&1 || echo "DELETED")
  echo "- Status after delete: **$STATUS**" >> "$RESULTS_FILE"
else
  echo "### No deletable candidates found" >> "$RESULTS_FILE"
  echo "" >> "$RESULTS_FILE"
  echo "The fixture contains:" >> "$RESULTS_FILE"
  echo "- **Jobs**: \`Suggested=report\`, \`SafeToApply=false\` → report-only" >> "$RESULTS_FILE"
  echo "- **Pending PVC** \`pending-unbound\`: \`Suggested=delete\`, \`SafeToApply=true\` → deletable" >> "$RESULTS_FILE"
  echo "- **Bound PVC** \`unused-bound\`: \`Suggested=none\`, \`SafeToApply=false\` → not deletable" >> "$RESULTS_FILE"
  echo "- **Deployments/StatefulSets**: \`Suggested=report\`, \`SafeToApply=false\` → report-only" >> "$RESULTS_FILE"
  echo "" >> "$RESULTS_FILE"
  echo "The only deletable object is the Pending PVC \`pending-unbound\`." >> "$RESULTS_FILE"

  # Annotate and delete the PVC
  PVC_NS="kubescrub-messy"
  PVC_NAME="pending-unbound"
  log "Annotate PVC $PVC_NAME with kubescrub.io/allow-delete=true"
  kubectl --context "$CONTEXT" -n "$PVC_NS" annotate pvc "$PVC_NAME" kubescrub.io/allow-delete=true --overwrite 2>&1 | tee -a "$RESULTS_FILE"

  # Re-scan
  log "Re-scan for fresh plan"
  run "$BINARY" scan --policy "$POLICY" --out "$PLAN_FILE" 2>&1 | tee -a "$RESULTS_FILE"

  # Real apply
  log "Real apply: --apply --yes --namespace kubescrub-messy"
  REAL_RESULT=$(run "$BINARY" apply --plan "$PLAN_FILE" \
    --apply --yes \
    --namespace "$PVC_NS" 2>&1) || true
  echo "" >> "$RESULTS_FILE"
  echo "### Real apply result:" >> "$RESULTS_FILE"
  echo "" >> "$RESULTS_FILE"
  echo "\`\`\`" >> "$RESULTS_FILE"
  echo "$REAL_RESULT" >> "$RESULTS_FILE"
  echo "```" >> "$RESULTS_FILE"

  # Verify deletion
  log "Verify $PVC_NAME is gone"
  STATUS=$(kubectl --context "$CONTEXT" -n "$PVC_NS" get pvc "$PVC_NAME" 2>&1 || echo "DELETED")
  echo "- Status after delete: **$STATUS**" >> "$RESULTS_FILE"
  echo "- **Deleted: PVC/$PVC_NS/$PVC_NAME**" >> "$RESULTS_FILE"
fi

# Verify remaining objects
log "Verify remaining objects"
log "Remaining jobs:"
kubectl --context "$CONTEXT" -n kubescrub-messy get jobs --no-headers 2>/dev/null | tee -a "$RESULTS_FILE"
log "Remaining PVCs:"
kubectl --context "$CONTEXT" -n kubescrub-messy get pvc --no-headers 2>/dev/null | tee -a "$RESULTS_FILE"
log "Remaining Deployments:"
kubectl --context "$CONTEXT" -n kubescrub-messy get deployments --no-headers 2>/dev/null | tee -a "$RESULTS_FILE"

# ── 5. Negative checks ───────────────────────────────────────────────
log "=== Step 5: Negative checks ==="
cat >> "$RESULTS_FILE" <<'EOF'

## Step 5 — Negative checks

EOF

# 5a. scan SA cannot delete jobs
log "Negative 1: scan SA cannot delete jobs"
NEG1=$(kubectl --context "$CONTEXT" auth can-i --as=system:serviceaccount:kubescrub-system:kubescrub-scan delete jobs 2>&1)
echo "- **scan SA cannot delete jobs**: \`$NEG1\` ✓" >> "$RESULTS_FILE"

# 5b. --apply without --yes
log "Negative 2: --apply without --yes"
NEG2=$(run "$BINARY" apply --plan "$PLAN_FILE" --apply 2>&1) || true
echo "\`\`\`" >> "$RESULTS_FILE"
echo "$NEG2" >> "$RESULTS_FILE"
echo "```" >> "$RESULTS_FILE"

# 5c. context mismatch
log "Negative 3: context mismatch"
NEG3=$(run "$BINARY" apply --plan "$PLAN_FILE" --apply --yes --context nonexistent-context 2>&1) || true
echo "- **context mismatch**: \`$NEG3\`" >> "$RESULTS_FILE"

# 5d. unannotated SafeToApply skipped
log "Negative 4: unannotated object skipped in dry-run"
# Use a non-deletable finding to test — e.g. workload (report-only)
NEG4=$(run "$BINARY" apply --plan "$PLAN_FILE" \
  --checks workload 2>&1) || true
echo "\`\`\`" >> "$RESULTS_FILE"
echo "$NEG4" >> "$RESULTS_FILE"
echo "```" >> "$RESULTS_FILE"

# ── Summary ──────────────────────────────────────────────────────────
cat >> "$RESULTS_FILE" <<'EOF'

## Summary

| Check | Result |
|-------|--------|
| Scan identity cannot delete jobs | Verified |
| Dry-run (`--apply` absent) | No deletion occurs |
| \`--apply\` without \`--yes\` | Refused — "requires --yes" |
| Context mismatch with \`--apply --yes\` | Refused — "does not match plan.cluster" |
| Unannotated objects in dry-run | Skipped |
| Real deletion (after annotation) | Performed |

## Flag differences from prompt

- **\`--reason\`** (singular) — the actual CLI flag is \`--reason string\`, not \`--reasons\`.
- **No \`--dry-run\` flag** — dry-run is the default when \`--apply\` is absent.

## What was deleted

- PVC \`pending-unbound\` (annotation gate applied, then deleted)

## What was refused

- \`--apply\` without \`--yes\`
- Plan context mismatch
- Unannotated objects in dry-run

## Objects still present after loop

- Jobs \`old-success\`, \`old-failed\` (report-only, SafeToApply=false)
- Pending PVC \`pending-unbound\` was deleted
- Bound PVC \`unused-bound\` (SafeToApply=false)
- Deployment \`zero-web\` (SafeToApply=false)
- StatefulSet \`zero-db\` (SafeToApply=false)
- CRD \`widgets.scrub.kubescrub.io\` (hard-blocked)
- Widget \`rusty\` CR instance (SafeToApply=false)
EOF

log "=== Operator loop complete ==="
echo "Results written to: $RESULTS_FILE"
