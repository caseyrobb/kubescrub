package apply

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/caseyrobb/kubescrub/internal/report"
)

// TestEncodeDecodeWrapper exercises round-trip: build a Plan, encode to JSON,
// decode it back, and verify all fields survived unchanged.
func TestEncodeDecodeWrapper(t *testing.T) {
	plan := NewPlan("my-cluster", "2026-01-01T00:00:00Z")
	plan.PolicyPath = "policy.yaml"
	plan.AddFinding(report.Finding{
		Check:    "workload",
		Severity: "high",
		Kind:     "Deployment",
		Name:     "old-dep",
		Reason:   "zero replicas",
		Version:  "apps/v1",
	})
	plan.AddFinding(report.Finding{
		Check:    "pvc",
		Severity: "medium",
		Kind:     "PersistentVolumeClaim",
		Name:     "stale-pvc",
		Reason:   "unused",
		Version:  "v1",
	})

	// Encode → decode.
	data, err := plan.EncodeTo("")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	var decoded Plan
	if err := decoded.DecodeFrom(data); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.APIVersion != PlanAPIVersion {
		t.Errorf("APIVersion = %q, want %q", decoded.APIVersion, PlanAPIVersion)
	}
	if decoded.Kind != PlanKind {
		t.Errorf("Kind = %q, want %q", decoded.Kind, PlanKind)
	}
	if decoded.Cluster != "my-cluster" {
		t.Errorf("Cluster = %q, want %q", decoded.Cluster, "my-cluster")
	}
	if decoded.GeneratedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("GeneratedAt = %q", decoded.GeneratedAt)
	}
	if decoded.PolicyPath != "policy.yaml" {
		t.Errorf("PolicyPath = %q", decoded.PolicyPath)
	}
	if len(decoded.Findings) != 2 {
		t.Fatalf("Findings count = %d, want 2", len(decoded.Findings))
	}
	if decoded.Findings[0].ID == "" {
		t.Error("first finding has empty ID after round-trip")
	}
}

// TestEncodeToFile verifies that EncodeTo writes to a file when a path is given.
func TestEncodeToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")

	plan := NewPlan("x", "2026-01-01T00:00:00Z")
	plan.AddFinding(report.Finding{
		Check: "w", Kind: "Deployment", Name: "d",
		Reason:  "r",
		Version: "v1",
	})

	data, err := plan.EncodeTo(path)
	if err != nil {
		t.Fatalf("EncodeTo: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("EncodeTo returned empty data for file write")
	}

	// Verify file exists and round-trips.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	var p Plan
	if err := p.DecodeFrom(b); err != nil {
		t.Fatalf("decode written file: %v", err)
	}
	if len(p.Findings) != 1 {
		t.Errorf("decoded findings = %d, want 1", len(p.Findings))
	}
}

// TestDecodeLegacyArray ensures a bare []Finding JSON array is accepted and
// wrapped into a Plan with defaults.
func TestDecodeLegacyArray(t *testing.T) {
	findings := []report.Finding{
		{Check: "workload", Severity: "high", Kind: "Deployment",
			Name: "old", Reason: "stale", Version: "apps/v1"},
	}
	data, err := json.Marshal(findings)
	if err != nil {
		t.Fatalf("marshal findings: %v", err)
	}

	var plan Plan
	if err := plan.DecodeFrom(data); err != nil {
		t.Fatalf("decode legacy array: %v", err)
	}

	if plan.APIVersion != PlanAPIVersion {
		t.Errorf("APIVersion = %q, want %q", plan.APIVersion, PlanAPIVersion)
	}
	if plan.Kind != PlanKind {
		t.Errorf("Kind = %q, want %q", plan.Kind, PlanKind)
	}
	if len(plan.Findings) != 1 {
		t.Fatalf("Findings count = %d, want 1", len(plan.Findings))
	}
	if plan.Findings[0].Check != "workload" {
		t.Errorf("legacy finding check = %q", plan.Findings[0].Check)
	}
}

// TestRejectUnsupportedAPIVersion ensures DecodeFrom returns a clear error
// when the apiVersion is not kubescrub.io/v1.
func TestRejectUnsupportedAPIVersion(t *testing.T) {
	jsonData := []byte(`{"apiVersion":"old.io/v1","kind":"Plan","findings":[]}`)
	var plan Plan
	err := plan.DecodeFrom(jsonData)
	if err == nil {
		t.Fatal("expected error for unsupported apiVersion")
	}
	if !containsStr(err.Error(), "unsupported apiVersion") {
		t.Errorf("error = %q, want 'unsupported apiVersion'", err.Error())
	}
}

// TestRejectWrongKind ensures DecodeFrom returns a clear error when kind != Plan.
func TestRejectWrongKind(t *testing.T) {
	jsonData := []byte(`{"apiVersion":"kubescrub.io/v1","kind":"Report","findings":[]}`)
	var plan Plan
	err := plan.DecodeFrom(jsonData)
	if err == nil {
		t.Fatal("expected error for wrong kind")
	}
	if !containsStr(err.Error(), "unsupported kind") {
		t.Errorf("error = %q, want 'unsupported kind'", err.Error())
	}
}

// TestStableFindingIDs verifies that findID produces consistent, stable IDs
// with the expected format: check|namespace|kind|name|reason|version.
func TestStableFindingIDs(t *testing.T) {
	f1 := report.Finding{
		Check: "workload", Namespace: "default", Kind: "Deployment",
		Name: "web", Reason: "zero replicas", Version: "apps/v1",
	}
	f2 := report.Finding{
		Check: "workload", Namespace: "default", Kind: "Deployment",
		Name: "web", Reason: "zero replicas", Version: "apps/v1",
	}
	f3 := report.Finding{
		Check: "workload", Namespace: "", Kind: "Deployment",
		Name: "web", Reason: "zero replicas", Version: "apps/v1",
	}

	id1 := findID(f1)
	id2 := findID(f2)
	id3 := findID(f3)

	if id1 == "" {
		t.Error("stable ID must not be empty")
	}
	if id1 != id2 {
		t.Errorf("same finding produced different IDs: %q vs %q", id1, id2)
	}
	if id1 == id3 {
		t.Error("different namespace (empty vs non-empty) should produce different IDs")
	}

	// Verify format contains expected pipe-separated segments.
	expected := "workload|default|Deployment|web|zero replicas|apps/v1"
	if id1 != expected {
		t.Errorf("ID = %q, want %q", id1, expected)
	}
}

// TestAddFindingGeneratesID verifies that Plan.AddFinding assigns a stable
// non-empty ID when the finding has none.
func TestAddFindingGeneratesID(t *testing.T) {
	plan := NewPlan("c", "2026-01-01T00:00:00Z")
	plan.AddFinding(report.Finding{
		Check: "workload", Kind: "Deployment", Name: "d",
		Reason:  "stale",
		Version: "apps/v1",
	})
	if plan.Findings[0].ID == "" {
		t.Error("AddFinding should generate a stable non-empty ID")
	}
}

// TestAddFindingPreservesExistingID verifies that Plan.AddFinding does not
// overwrite an ID that is already set.
func TestAddFindingPreservesExistingID(t *testing.T) {
	plan := NewPlan("c", "2026-01-01T00:00:00Z")
	plan.AddFinding(report.Finding{
		ID:    "custom-id-123",
		Check: "workload", Kind: "Deployment",
		Name: "d", Reason: "stale", Version: "apps/v1",
	})
	if plan.Findings[0].ID != "custom-id-123" {
		t.Errorf("expected ID to be preserved, got %q", plan.Findings[0].ID)
	}
}

// containsStr is a tiny helper to avoid importing strings.
func containsStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
