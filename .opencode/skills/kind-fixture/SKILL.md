---
name: kind-fixture
description: Maintain testdata/clusters/messy.yaml and Kind cluster kubescrub-dev on Podman
---

# Kind fixture (Podman)

This repo uses Kind with Podman, not Docker.

Always prefix Kind commands with:

```bash
export KIND_EXPERIMENTAL_PROVIDER=podman
