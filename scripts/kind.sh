#!/usr/bin/env bash
set -euo pipefail

export KIND_EXPERIMENTAL_PROVIDER=podman

KIND_CLUSTER="kubescrub-dev"
KUBE_CONTEXT="kind-${KIND_CLUSTER}"
FIXTURE="testdata/clusters/messy.yaml"
CRD_NAME="widgets.scrub.kubescrub.io"

cmd="${1:-status}"
shift || true

# ── helpers ──────────────────────────────────────────────────────────

cluster_exists() {
  kind get clusters 2>/dev/null | grep -qx "${KIND_CLUSTER}"
}

api_ready() {
  kubectl --context "${KUBE_CONTEXT}" cluster-info &>/dev/null
}

# ── start ────────────────────────────────────────────────────────────

do_start() {
  if cluster_exists && api_ready; then
    echo "Cluster ${KIND_CLUSTER} already running."
    return 0
  fi

  if cluster_exists; then
    echo "Deleting existing cluster ${KIND_CLUSTER} …"
    kind delete cluster --name "${KIND_CLUSTER}" || true
  fi

  echo "Creating cluster ${KIND_CLUSTER} …"
  # First attempt
  kind create cluster --name "${KIND_CLUSTER}" --wait 60s && return 0

  # Retry with systemd-run --scope --user -p Delegate=yes on cgroup errors
  echo "First create failed — retrying with Delegate=yes …"
  systemd-run --scope --user -p Delegate=yes \
    kind create cluster --name "${KIND_CLUSTER}" --wait 60s && return 0

  # Fallback: raw retry (no systemd)
  echo "systemd-run not available — retrying directly …"
  kind create cluster --name "${KIND_CLUSTER}" --wait 60s && return 0

  echo "ERROR: cluster creation failed after retries."
  exit 1
}

# ── stop ─────────────────────────────────────────────────────────────

do_stop() {
  if cluster_exists; then
    kind delete cluster --name "${KIND_CLUSTER}"
    echo "Cluster stopped."
  else
    echo "Cluster ${KIND_CLUSTER} does not exist."
  fi
}

# ── restart ──────────────────────────────────────────────────────────

do_restart() {
  do_stop
  do_start
}

# ── reset ────────────────────────────────────────────────────────────

do_reset() {
  do_restart
  echo "Applying fixture (CRD first, then CR instances) …"
  # First pass: create CRD only (ignore errors on CR instances that need it)
  kubectl --context "${KUBE_CONTEXT}" apply -f "${FIXTURE}" || true
  do_wait_crd
  # Second pass: create CR instances now that CRD is established
  echo "Re-applying fixture (CR instances) …"
  kubectl --context "${KUBE_CONTEXT}" apply -f "${FIXTURE}"
  do_ready
}

# ── status ───────────────────────────────────────────────────────────

do_status() {
  echo "Cluster name : ${KIND_CLUSTER}"
  echo "Kube context : ${KUBE_CONTEXT}"

  if cluster_exists; then
    echo "Exists       : yes"
  else
    echo "Exists       : no"
    exit 0
  fi

  if api_ready; then
    echo "API ready    : yes"
    kubectl --context "${KUBE_CONTEXT}" cluster-info --short 2>/dev/null || true
  else
    echo "API ready    : no"
  fi
}

# ── apply ────────────────────────────────────────────────────────────

do_apply() {
  if ! cluster_exists || ! api_ready; then
    echo "Cluster not running — run ./scripts/kind.sh start first."
    exit 1
  fi
  kubectl --context "${KUBE_CONTEXT}" apply -f "${FIXTURE}" || true
  do_wait_crd
  echo "Re-applying fixture (CR instances) …"
  kubectl --context "${KUBE_CONTEXT}" apply -f "${FIXTURE}"
}

# ── ready ────────────────────────────────────────────────────────────

do_ready() {
  if ! cluster_exists || ! api_ready; then
    echo "Cluster not running."
    exit 1
  fi

  do_wait_crd
  echo "Waiting for old-success …"
  kubectl --context "${KUBE_CONTEXT}" -n kubescrub-messy wait --for=condition=Complete \
    job/old-success --timeout=120s || true
  echo "Waiting for old-failed …"
  kubectl --context "${KUBE_CONTEXT}" -n kubescrub-messy wait --for=condition=Failed \
    job/old-failed --timeout=120s || true
  echo "Waiting for pending-unbound …"
  kubectl --context "${KUBE_CONTEXT}" -n kubescrub-messy wait --for=jsonpath='{.status.phase}'=Pending \
    pvc/pending-unbound --timeout=120s || true
  echo "Waiting for unused-bound (Bound) …"
  kubectl --context "${KUBE_CONTEXT}" -n kubescrub-messy wait --for=jsonpath='{.status.phase}'=Bound \
    pvc/unused-bound --timeout=120s || true
  echo "Cluster is ready."
}

# ── wait helpers ─────────────────────────────────────────────────────

do_wait_crd() {
  echo "Waiting for CRD ${CRD_NAME} …"
  kubectl --context "${KUBE_CONTEXT}" wait --for=condition=Established \
    crd/"${CRD_NAME}" --timeout=60s
}

# ── dispatch ─────────────────────────────────────────────────────────

case "${cmd}" in
  start)    do_start    ;;
  stop)     do_stop     ;;
  restart)  do_restart  ;;
  reset)    do_reset    ;;
  status)   do_status   ;;
  apply)    do_apply    ;;
  ready)    do_ready    ;;
  *)
    echo "Usage: $0 {start|stop|restart|reset|status|apply|ready}"
    exit 1
    ;;
esac
