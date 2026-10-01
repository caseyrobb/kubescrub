package checks

import (
	"testing"
)

// TestRuntimeIsZeroable verifies that an empty Runtime can be constructed
// without panicking. This ensures it's safe to pass to checks that
// may not use all its fields.
func TestRuntimeIsZeroable(t *testing.T) {
	rt := Runtime{}
	if rt.Cluster != "" {
		t.Error("Cluster should be empty by default")
	}
	if rt.Namespaces != nil {
		t.Error("Namespaces should be nil by default")
	}
}
