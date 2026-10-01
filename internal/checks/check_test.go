package checks

import (
	"context"
	"testing"

	"github.com/caseyrobb/kubescrub/internal/report"
)

// fakeCheck is a test double that implements the Check interface.
type fakeCheck struct {
	name        string
	description string
	findings    []report.Finding
	err         error
}

func (f *fakeCheck) Name() string        { return f.name }
func (f *fakeCheck) Description() string { return f.description }
func (f *fakeCheck) Run(ctx context.Context, rt Runtime) ([]report.Finding, error) {
	return f.findings, f.err
}

func TestCheckInterface(t *testing.T) {
	c := &fakeCheck{
		name:        "test-check",
		description: "A test check",
		findings:    []report.Finding{{Check: "test-check", Name: "test"}},
	}

	if c.Name() != "test-check" {
		t.Errorf("Name() = %q, want %q", c.Name(), "test-check")
	}
	if c.Description() != "A test check" {
		t.Errorf("Description() = %q, want %q", c.Description(), "A test check")
	}
}

func TestCheckRunReturnsFindings(t *testing.T) {
	c := &fakeCheck{
		name:     "test-check",
		findings: []report.Finding{{Check: "test", Name: "f1"}},
	}
	result, err := c.Run(context.Background(), Runtime{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 finding, got %d", len(result))
	}
}

func TestCheckRunReturnsError(t *testing.T) {
	c := &fakeCheck{
		name:  "failing-check",
		err:   context.DeadlineExceeded,
	}
	_, err := c.Run(context.Background(), Runtime{})
	if err == nil {
		t.Fatal("expected error from Run()")
	}
}

func TestCheckRunReturnsNilFindings(t *testing.T) {
	c := &fakeCheck{name: "empty-check"}
	result, err := c.Run(context.Background(), Runtime{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil findings, got %d", len(result))
	}
}
