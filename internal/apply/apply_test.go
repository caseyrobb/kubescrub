package apply

import (
	"testing"

	"github.com/caseyrobb/kubescrub/internal/report"
)

func makePlan(cluster, generatedAt string, findings ...report.Finding) *Plan {
	p := &Plan{
		APIVersion:  PlanAPIVersion,
		Kind:        PlanKind,
		PlanVersion: 1,
		Cluster:     cluster,
		GeneratedAt: generatedAt,
	}
	for _, f := range findings {
		p.AddFinding(f)
	}
	return p
}

func TestFilterByCheck(t *testing.T) {
	findings := []report.Finding{
		{ID: "1", Check: "workload", Reason: "zero-replicas", Namespace: "ns1", Kind: "Deployment", Name: "dep1", Suggested: report.ActionDelete, SafeToApply: true},
		{ID: "2", Check: "pvc", Reason: "unused-pvc", Namespace: "ns1", Kind: "persistentvolumeclaims", Name: "pvc1", Suggested: report.ActionDelete, SafeToApply: true},
		{ID: "3", Check: "workload", Reason: "completed-job", Namespace: "ns2", Kind: "Job", Name: "job1", Suggested: report.ActionDelete, SafeToApply: true},
	}
	plan := makePlan("ctx", "2025-01-01T00:00:00Z", findings...)
	result := &PlanResult{
		Candidates:     findings,
		SkippedReasons: nil,
	}

	filtered := FilterCandidates(result, plan, ApplyFilters{Checks: []string{"workload"}})

	if len(filtered.Candidates) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(filtered.Candidates))
	}
	for _, f := range filtered.Candidates {
		if f.Check != "workload" {
			t.Errorf("unexpected check %q in candidates", f.Check)
		}
	}
	if len(filtered.SkippedReasons) != 1 {
		t.Errorf("expected 1 skipped reason, got %d", len(filtered.SkippedReasons))
	}
	if filtered.SkippedReasons[0].Reason != "filtered: check not in --checks" {
		t.Errorf("expected filtered reason, got %q", filtered.SkippedReasons[0].Reason)
	}
}

func TestFilterByReason(t *testing.T) {
	findings := []report.Finding{
		{ID: "1", Check: "pvc", Reason: "unused-pvc", Namespace: "ns1", Kind: "persistentvolumeclaims", Name: "pvc1", Suggested: report.ActionDelete, SafeToApply: true},
		{ID: "2", Check: "pvc", Reason: "unbound-pvc", Namespace: "ns1", Kind: "persistentvolumeclaims", Name: "pvc2", Suggested: report.ActionDelete, SafeToApply: true},
		{ID: "3", Check: "workload", Reason: "completed-job", Namespace: "ns1", Kind: "Job", Name: "job1", Suggested: report.ActionDelete, SafeToApply: true},
	}
	plan := makePlan("ctx", "2025-01-01T00:00:00Z", findings...)
	result := &PlanResult{Candidates: findings, SkippedReasons: nil}

	filtered := FilterCandidates(result, plan, ApplyFilters{Reasons: []string{"unbound-pvc"}})

	if len(filtered.Candidates) != 1 {
		t.Errorf("expected 1 candidate, got %d", len(filtered.Candidates))
	}
	if filtered.Candidates[0].Reason != "unbound-pvc" {
		t.Errorf("expected reason unbound-pvc, got %q", filtered.Candidates[0].Reason)
	}
	if len(filtered.SkippedReasons) != 2 {
		t.Errorf("expected 2 skipped reasons, got %d", len(filtered.SkippedReasons))
	}
}

func TestFilterByNamespace(t *testing.T) {
	findings := []report.Finding{
		{ID: "1", Check: "workload", Reason: "zero-replicas", Namespace: "kubescrub-messy", Kind: "Deployment", Name: "dep1", Suggested: report.ActionDelete, SafeToApply: true},
		{ID: "2", Check: "rbac", Reason: "rbac-wildcard", Namespace: "", Kind: "ClusterRole", Name: "admin", Suggested: report.ActionDelete, SafeToApply: true},
		{ID: "3", Check: "workload", Reason: "completed-job", Namespace: "default", Kind: "Job", Name: "job1", Suggested: report.ActionDelete, SafeToApply: true},
	}
	plan := makePlan("ctx", "2025-01-01T00:00:00Z", findings...)
	result := &PlanResult{Candidates: findings, SkippedReasons: nil}

	filtered := FilterCandidates(result, plan, ApplyFilters{Namespace: "kubescrub-messy"})

	// Should keep ns=kubescrub-messy AND cluster-scoped (empty namespace).
	if len(filtered.Candidates) != 2 {
		t.Errorf("expected 2 candidates (ns match + cluster-scoped), got %d", len(filtered.Candidates))
	}
	for _, f := range filtered.Candidates {
		if f.Namespace != "kubescrub-messy" && f.Namespace != "" {
			t.Errorf("unexpected namespace %q in filtered candidates", f.Namespace)
		}
	}
	// Skipped: default namespace.
	if len(filtered.SkippedReasons) != 1 {
		t.Errorf("expected 1 skipped reason, got %d", len(filtered.SkippedReasons))
	}
}

func TestFilterNamespaceKeepsClusterScoped(t *testing.T) {
	findings := []report.Finding{
		{ID: "1", Check: "rbac", Reason: "rbac-wildcard", Namespace: "", Kind: "ClusterRole", Name: "admin", Suggested: report.ActionDelete, SafeToApply: true},
		{ID: "2", Check: "rbac", Reason: "rbac-cluster-admin", Namespace: "", Kind: "ClusterRoleBinding", Name: "binding1", Suggested: report.ActionDelete, SafeToApply: true},
		{ID: "3", Check: "workload", Reason: "zero-replicas", Namespace: "kubescrub-messy", Kind: "Deployment", Name: "dep1", Suggested: report.ActionDelete, SafeToApply: true},
	}
	plan := makePlan("ctx", "2025-01-01T00:00:00Z", findings...)
	result := &PlanResult{Candidates: findings, SkippedReasons: nil}

	filtered := FilterCandidates(result, plan, ApplyFilters{Namespace: "kubescrub-messy"})

	if len(filtered.Candidates) != 3 {
		t.Errorf("expected 3 candidates (2 cluster-scoped + 1 ns match), got %d", len(filtered.Candidates))
	}
}

func TestFilterNoChangeWhenEmpty(t *testing.T) {
	findings := []report.Finding{
		{ID: "1", Check: "workload", Reason: "zero-replicas", Namespace: "ns1", Kind: "Deployment", Name: "dep1", Suggested: report.ActionDelete, SafeToApply: true},
		{ID: "2", Check: "pvc", Reason: "unused-pvc", Namespace: "ns1", Kind: "persistentvolumeclaims", Name: "pvc1", Suggested: report.ActionDelete, SafeToApply: true},
	}
	plan := makePlan("ctx", "2025-01-01T00:00:00Z", findings...)
	result := &PlanResult{Candidates: findings, SkippedReasons: nil}

	filtered := FilterCandidates(result, plan, ApplyFilters{})

	if len(filtered.Candidates) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(filtered.Candidates))
	}
	if len(filtered.SkippedReasons) != 0 {
		t.Errorf("expected 0 skipped reasons, got %d", len(filtered.SkippedReasons))
	}
}

func TestFilterCombination(t *testing.T) {
	findings := []report.Finding{
		{ID: "1", Check: "workload", Reason: "zero-replicas", Namespace: "kubescrub-messy", Kind: "Deployment", Name: "dep1", Suggested: report.ActionDelete, SafeToApply: true},
		{ID: "2", Check: "workload", Reason: "completed-job", Namespace: "kubescrub-messy", Kind: "Job", Name: "job1", Suggested: report.ActionDelete, SafeToApply: true},
		{ID: "3", Check: "workload", Reason: "completed-job", Namespace: "default", Kind: "Job", Name: "job2", Suggested: report.ActionDelete, SafeToApply: true},
		{ID: "4", Check: "pvc", Reason: "unused-pvc", Namespace: "kubescrub-messy", Kind: "persistentvolumeclaims", Name: "pvc1", Suggested: report.ActionDelete, SafeToApply: true},
	}
	plan := makePlan("ctx", "2025-01-01T00:00:00Z", findings...)
	result := &PlanResult{Candidates: findings, SkippedReasons: nil}

	filtered := FilterCandidates(result, plan, ApplyFilters{
		Checks:    []string{"workload"},
		Reasons:   []string{"completed-job"},
		Namespace: "kubescrub-messy",
	})

	// Only workload+completed-job+kubescrub-messy.
	if len(filtered.Candidates) != 1 {
		t.Errorf("expected 1 candidate, got %d: %+v", len(filtered.Candidates), filtered.Candidates)
	}
	if len(filtered.SkippedReasons) != 3 {
		t.Errorf("expected 3 skipped reasons, got %d", len(filtered.SkippedReasons))
	}
}
