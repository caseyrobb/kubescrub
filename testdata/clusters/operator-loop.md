# Operator Loop Results

## Overview

- Cluster: `kind-kubescrub-dev` (Podman Kind)
- Namespace: `kubescrub-messy`
- Plan file: `/tmp/kubescrub-plan.json`
- Policy: `testdata/clusters/policy-kind.yaml`

## Step 1 — Install RBAC

namespace/kubescrub-system created
serviceaccount/kubescrub-scan created
clusterrole.rbac.authorization.k8s.io/kubescrub-scan created
clusterrole.rbac.authorization.k8s.io/kubescrub-scan-cr created
clusterrole.rbac.authorization.k8s.io/kubescrub-scan-helm created
namespace/kubescrub-system configured
serviceaccount/kubescrub-apply created
clusterrole.rbac.authorization.k8s.io/kubescrub-apply created
error: unknown flag: --overwrite
See 'kubectl create clusterrolebinding --help' for usage.
# Operator Loop Results

## Overview

- Cluster: `kind-kubescrub-dev` (Podman Kind)
- Namespace: `kubescrub-messy`
- Plan file: `/tmp/kubescrub-plan.json`
- Policy: `testdata/clusters/policy-kind.yaml`

## Step 1 — Install RBAC

namespace/kubescrub-system created
serviceaccount/kubescrub-scan created
clusterrole.rbac.authorization.k8s.io/kubescrub-scan created
clusterrole.rbac.authorization.k8s.io/kubescrub-scan-cr created
clusterrole.rbac.authorization.k8s.io/kubescrub-scan-helm created
namespace/kubescrub-system configured
serviceaccount/kubescrub-apply created
clusterrole.rbac.authorization.k8s.io/kubescrub-apply created
clusterrolebinding.rbac.authorization.k8s.io/kubescrub-scan-binding created
clusterrolebinding.rbac.authorization.k8s.io/kubescrub-scan-cr-binding created
clusterrolebinding.rbac.authorization.k8s.io/kubescrub-apply-binding created
