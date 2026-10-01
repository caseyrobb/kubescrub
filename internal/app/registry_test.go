package app

import (
	"testing"

	"github.com/caseyrobb/kubescrub/internal/checks"
)

func TestAllChecksNonEmpty(t *testing.T) {
	checks := allChecks()
	if len(checks) == 0 {
		t.Fatal("expected at least one registered check")
	}
}

func TestAllChecksHaveNames(t *testing.T) {
	names := make(map[string]bool)
	for _, c := range allChecks() {
		name := c.Name()
		if name == "" {
			t.Error("check name should not be empty")
		}
		if names[name] {
			t.Errorf("duplicate check name %q", name)
		}
		names[name] = true
	}
}

func TestAllChecksHaveDescriptions(t *testing.T) {
	for _, c := range allChecks() {
		if c.Description() == "" {
			t.Errorf("check %q has empty description", c.Name())
		}
	}
}

// TestAllChecksUniqueNames ensures no two registered checks share a name.
func TestAllChecksUniqueNames(t *testing.T) {
	names := make(map[string]checks.Check)
	for _, c := range allChecks() {
		if existing, ok := names[c.Name()]; ok {
			t.Errorf("duplicate check name %q: %v and %v", c.Name(), existing, c)
		}
		names[c.Name()] = c
	}
}
