---
name: add-checker
description: Add a KubeScrub checker with Finding contract, registry entry, and fixture tests
---

Implement a new internal/checks/<name> package for KubeScrub.

Required:
- Name() matches CLI --checks value
- Returns []report.Finding from internal/report/finding.go only
- Evidence includes the raw fields that justified the finding
- Test loads testdata/fixtures/<name>/ YAML
- Does not mutate the cluster
- Register the checker in internal/app/registry.go

After implementation, run go test ./internal/checks/<name>/...
