package app

import (
	"os"
	"testing"
	"time"

	"github.com/caseyrobb/kubescrub/internal/apply"
	"github.com/caseyrobb/kubescrub/internal/report"
)

func makeFakePlan(cluster, generatedAt string, findings ...report.Finding) *apply.Plan {
	p := &apply.Plan{
		APIVersion:  apply.PlanAPIVersion,
		Kind:        apply.PlanKind,
		PlanVersion: 1,
		Cluster:     cluster,
		GeneratedAt: generatedAt,
	}
	for _, f := range findings {
		p.AddFinding(f)
	}
	return p
}

func TestValidateApplyNoYes(t *testing.T) {
	plan := makeFakePlan("ctx", time.Now().UTC().Format(time.RFC3339))
	err := validateApply(plan, true, false, 24*time.Hour, "")
	if err == nil {
		t.Fatal("expected error for --apply without --yes")
	}
	if got := err.Error(); got != "--apply requires --yes to perform real deletions" {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestValidateApplyNoYesDryRun(t *testing.T) {
	plan := makeFakePlan("ctx", time.Now().UTC().Format(time.RFC3339))
	// realApply=false, yes=false → dry-run, no error.
	err := validateApply(plan, false, false, 24*time.Hour, "")
	if err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}
}

func TestValidateApplyNoYesApplyButNoYes(t *testing.T) {
	plan := makeFakePlan("ctx", time.Now().UTC().Format(time.RFC3339))
	err := validateApply(plan, true, false, 24*time.Hour, "")
	if err == nil {
		t.Fatal("expected error when --apply is set but --yes is not")
	}
}

func TestValidateApplyContextMismatchReal(t *testing.T) {
	plan := makeFakePlan("scan-context", time.Now().UTC().Format(time.RFC3339))
	// --apply --yes with mismatched context → error.
	err := validateApply(plan, true, true, 24*time.Hour, "other-context")
	if err == nil {
		t.Fatal("expected error for context mismatch with --apply --yes")
	}
}

func TestValidateApplyContextMismatchDryRun(t *testing.T) {
	plan := makeFakePlan("scan-context", time.Now().UTC().Format(time.RFC3339))
	// Dry-run with mismatched context → warn but no error.
	err := validateApply(plan, false, false, 24*time.Hour, "other-context")
	if err != nil {
		t.Fatalf("dry-run should not error on context mismatch: %v", err)
	}
}

func TestValidateApplyContextMatch(t *testing.T) {
	plan := makeFakePlan("ctx", time.Now().UTC().Format(time.RFC3339))
	err := validateApply(plan, true, true, 24*time.Hour, "ctx")
	if err != nil {
		t.Fatalf("expected no error for matching context: %v", err)
	}
}

func TestValidateApplyContextEmptyPlanCluster(t *testing.T) {
	// Legacy plan with empty cluster → skip context check.
	plan := makeFakePlan("", time.Now().UTC().Format(time.RFC3339))
	err := validateApply(plan, true, true, 24*time.Hour, "some-context")
	if err != nil {
		t.Fatalf("expected no error when plan.cluster is empty: %v", err)
	}
}

func TestValidateApplyPlanAgeExceededReal(t *testing.T) {
	oldTime := time.Now().Add(-48 * time.Hour)
	plan := makeFakePlan("ctx", oldTime.Format(time.RFC3339))
	// --apply --yes with old plan → error.
	err := validateApply(plan, true, true, 24*time.Hour, "")
	if err == nil {
		t.Fatal("expected error for plan age exceeded with --apply --yes")
	}
}

func TestValidateApplyPlanAgeExceededDryRun(t *testing.T) {
	oldTime := time.Now().Add(-48 * time.Hour)
	plan := makeFakePlan("ctx", oldTime.Format(time.RFC3339))
	// Dry-run with old plan → warn but no error.
	err := validateApply(plan, false, false, 24*time.Hour, "")
	if err != nil {
		t.Fatalf("dry-run should not error on plan age: %v", err)
	}
}

func TestValidateApplyPlanAgeWithin(t *testing.T) {
	freshTime := time.Now().Add(-1 * time.Hour)
	plan := makeFakePlan("ctx", freshTime.Format(time.RFC3339))
	err := validateApply(plan, true, true, 24*time.Hour, "")
	if err != nil {
		t.Fatalf("expected no error for fresh plan: %v", err)
	}
}

func TestValidateApplyContextAndAgeBothViolations(t *testing.T) {
	oldTime := time.Now().Add(-48 * time.Hour)
	plan := makeFakePlan("scan-ctx", oldTime.Format(time.RFC3339))
	// --apply --yes with both violations → error on first check (age).
	err := validateApply(plan, true, true, 24*time.Hour, "other-ctx")
	if err == nil {
		t.Fatal("expected error for plan age violation with --apply --yes")
	}
}

func TestValidateApplyFreshMatchingPlan(t *testing.T) {
	freshTime := time.Now().Add(-1 * time.Hour)
	plan := makeFakePlan("ctx", freshTime.Format(time.RFC3339))
	err := validateApply(plan, true, true, 24*time.Hour, "ctx")
	if err != nil {
		t.Fatalf("fresh matching plan should pass: %v", err)
	}
}

func TestValidateApplyAgeWithCustomMax(t *testing.T) {
	// Plan is 30 min old, max is 20 min → error.
	oldTime := time.Now().Add(-30 * time.Minute)
	plan := makeFakePlan("ctx", oldTime.Format(time.RFC3339))
	err := validateApply(plan, true, true, 20*time.Minute, "")
	if err == nil {
		t.Fatal("expected error for plan exceeding custom max-plan-age")
	}
}

func TestValidateApplyStderrOnWarning(t *testing.T) {
	// Redirect stderr to capture warnings.
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	plan := makeFakePlan("other-ctx", time.Now().Add(-10*time.Hour).Format(time.RFC3339))
	// Dry-run → should produce a warning (written to stderr), not error.
	validateApply(plan, false, false, 24*time.Hour, "ctx")

	w.Close()
	os.Stderr = old

	buf := make([]byte, 1024)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("expected warning output: %v", err)
	}
	if n == 0 {
		t.Fatal("expected warning on stderr for context mismatch in dry-run")
	}
}
