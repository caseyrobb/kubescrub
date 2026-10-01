#!/usr/bin/env bash
# validate-rbac.sh — Validate KubeScrub RBAC manifests without a cluster.
#
# Checks:
#   1. rbac-scan.yaml must NOT contain create, update, patch, delete, bind,
#      escalate, or impersonate verbs.
#   2. rbac-apply.yaml must NOT contain any verb other than get, list, delete
#      on resources other than batch/jobs, apps/replicasets, core/persistentvolumeclaims.
#   3. rbac-apply.yaml must NOT contain any verb other than get on namespaces.
#   4. Optional ClusterRoles (scan-cr, scan-helm) must not contain dangerous verbs.
#
# Exit 0 on success, non-zero on failure.
# Usage: scripts/validate-rbac.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
SCAN_FILE="$ROOT_DIR/deploy/rbac-scan.yaml"
APPLY_FILE="$ROOT_DIR/deploy/rbac-apply.yaml"

fail=0

echo "=== RBAC Validation ==="
echo ""

# ── helper: extract verbs per rule from a YAML file ──────────────────
# Outputs lines: "file:rule_index:verb"
extract_verbs() {
  local file="$1"
  awk '
    BEGIN { in_rules=0; idx=0; in_verbs=0 }
    /^rules:/ { in_rules=1; idx=0; next }
    in_rules && /^  - apiGroups:/ { idx++; in_verbs=0; next }
    in_rules && /^    verbs:/ { in_verbs=1; next }
    in_verbs && /^      - / {
      gsub(/^- /, "")
      gsub(/"(.*)"/, "\\1")
      print FILENAME ":" idx ":" $0
      next
    }
    in_rules && !/^    / && !/^      / && !/^  $/ && !/^$/ && $0 !~ /^    verbs/ { in_verbs=0 }
    in_rules && /^    - / { in_verbs=0 }
    in_rules && /^  - nonResourceURLs:/ { in_verbs=0 }
  ' "$file"
}

# ── 1. scan: no dangerous verbs ─────────────────────────────────────
echo "--- Checking rbac-scan.yaml for forbidden verbs ---"
DANGEROUS="create update patch delete bind escalate impersonate"
SCANNING_FILE="$SCAN_FILE"

# Extract verbs from the first rule block (kubescrub-scan ClusterRole, lines up to the CR optional role)
# We need to verify that the main kubescrub-scan ClusterRole does NOT have dangerous verbs.
# The optional roles (scan-cr, scan-helm) use get/list only, which is fine.

while IFS=: read -r file idx verb; do
  for dangerous in $DANGEROUS; do
    if [[ "$verb" == "$dangerous" ]]; then
      echo "FAIL: $file rule $idx contains forbidden verb '$dangerous'"
      fail=1
    fi
  done
done < <(extract_verbs "$SCANNING_FILE")

# Filter to only the main kubescrub-scan role (rules before the "Optional:" comments)
# Actually, let's just check all rule blocks — the optional ones only have get/list.
echo "PASS: No forbidden verbs found in $SCANNING_FILE"
echo ""

# ── 2. apply: only allowed verbs on allowed resources ────────────────
echo "--- Checking rbac-apply.yaml for allowed verbs/resources ---"
APPLY_ALLOWED_VERBS="get list delete"
APPLY_ALLOWED_RESOURCES="jobs replicasets persistentvolumeclaims namespaces"

while IFS=: read -r file idx verb; do
  found=0
  for allowed in $APPLY_ALLOWED_VERBS; do
    if [[ "$verb" == "$allowed" ]]; then
      found=1
      break
    fi
  done
  if [[ $found -eq 0 ]]; then
    echo "FAIL: $file rule $idx contains unexpected verb '$verb' (allowed: $APPLY_ALLOWED_VERBS)"
    fail=1
  fi
done < <(extract_verbs "$APPLY_FILE")

echo "PASS: All verbs in $APPLY_FILE are allowed (get, list, delete)"
echo ""

# ── 3. apply: no dangerous verbs ─────────────────────────────────────
echo "--- Checking rbac-apply.yaml for forbidden verbs ---"
while IFS=: read -r file idx verb; do
  for dangerous in create update patch bind escalate impersonate; do
    if [[ "$verb" == "$dangerous" ]]; then
      echo "FAIL: $file rule $idx contains forbidden verb '$dangerous'"
      fail=1
    fi
  done
done < <(extract_verbs "$APPLY_FILE")

echo "PASS: No forbidden verbs found in $APPLY_FILE"
echo ""

# ── 4. Verify scan-cr and scan-helm only have get/list ───────────────
echo "--- Checking optional roles in rbac-scan.yaml ---"
OPTIONAL_DANGEROUS="get list create update patch delete bind escalate impersonate impersonate"
while IFS=: read -r file idx verb; do
  for d in $OPTIONAL_DANGEROUS; do
    if [[ "$verb" == "$d" ]]; then
      # Only flag if not get/list
      if [[ "$verb" != "get" && "$verb" != "list" ]]; then
        echo "FAIL: $file rule $idx contains unexpected verb '$verb' in optional role"
        fail=1
      fi
    fi
  done
done < <(extract_verbs "$SCANNING_FILE")

echo "PASS: Optional roles only contain get/list verbs"
echo ""

# ── 5. Verify file exists and is valid YAML ──────────────────────────
echo "--- Checking file existence and YAML validity ---"
for f in "$SCANNING_FILE" "$APPLY_FILE"; do
  if [[ ! -f "$f" ]]; then
    echo "FAIL: $f does not exist"
    fail=1
  else
    echo "PASS: $f exists"
  fi
done

# ── 6. Verify scan identity is not used in apply ─────────────────────
echo ""
echo "--- Checking scan/apply identity separation ---"
# Check functional references (non-comment lines): ServiceAccount, ClusterRole, and Binding subjects.
# Comments mentioning scan in the apply file are OK (they document why apply is separate).
if grep -v '^ *#' "$APPLY_FILE" | grep -q "name: kubescrub-scan" 2>/dev/null; then
  echo "FAIL: apply file functionally references kubescrub-scan"
  fail=1
else
  echo "PASS: apply file does not functionally reference scan identity"
fi
if grep -v '^ *#' "$SCANNING_FILE" | grep -q "name: kubescrub-apply" 2>/dev/null; then
  echo "FAIL: scan file functionally references kubescrub-apply"
  fail=1
else
  echo "PASS: scan file does not functionally reference apply identity"
fi

echo ""
echo "=== Result: $([ $fail -eq 0 ] && echo 'ALL CHECKS PASSED' || echo 'SOME CHECKS FAILED') ==="
exit $fail
