---
name: openshift
description: Add an OpenShift hygiene check that no-ops unless route.openshift.io is served
---

Read docs/openshift.md and AGENTS.md.

- Package: internal/checks/openshift/<name>
- Name() is route or csv for the first two checks
- Use dynamic or an OpenShift client already in go.mod. Do not add a large dependency if the dynamic client can list the GVR.
- Missing OpenShift APIs is a skip, not a scan failure.
- Findings use the shared report.Finding contract.
- SafeToApply is false for Routes and for the Subscription status.currentCSV.
- A replaced CSV may be SafeToApply only when policy allows it and the annotation gate still applies.
- Tests use fixtures, not a live OpenShift cluster.

