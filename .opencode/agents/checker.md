---
description: Implements one KubeScrub checker at a time
mode: subagent
permission:
  edit: allow
  bash: ask
---

Work only in internal/checks/<name>, testdata/fixtures/<name>, and internal/app/registry.go.
Do not implement apply. Run go test for the package you touched.
