package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultPolicy(t *testing.T) {
	p := DefaultPolicy()

	if len(p.ExcludeNamespaces) == 0 {
		t.Fatal("expected non-empty ExcludeNamespaces")
	}
	if len(p.ExcludeNamespaces) != 4 {
		t.Errorf("expected 4 excluded namespaces, got %d", len(p.ExcludeNamespaces))
	}
	for _, ns := range []string{"kube-system", "kube-public", "kube-node-lease", "local-path-storage"} {
		found := false
		for _, e := range p.ExcludeNamespaces {
			if e == ns {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("excluded namespace %q not found", ns)
		}
	}
	if p.Thresholds.CompletedJobAge != 1*time.Second {
		t.Errorf("CompletedJobAge = %v, want 1s", p.Thresholds.CompletedJobAge)
	}
	if !p.RBAC.FlagWildcards {
		t.Error("FlagWildcards should be true by default")
	}
	if !p.RBAC.FlagClusterAdmin {
		t.Error("FlagClusterAdmin should be true by default")
	}
	if p.Apply.RequireAnnotation != "kubescrub.io/allow-delete=true" {
		t.Errorf("RequireAnnotation = %q, want %q", p.Apply.RequireAnnotation,
			"kubescrub.io/allow-delete=true")
	}
}

func TestDefaultPolicyThresholds(t *testing.T) {
	p := DefaultPolicy()
	tests := []struct {
		name    string
		value   time.Duration
		wantMin time.Duration
	}{
		{"CompletedJobAge", p.Thresholds.CompletedJobAge, 1 * time.Second},
		{"FailedJobAge", p.Thresholds.FailedJobAge, 1 * time.Second},
		{"PendingPVCAge", p.Thresholds.PendingPVCAge, 1 * time.Second},
		{"UnusedPVCAge", p.Thresholds.UnusedPVCAge, 1 * time.Second},
		{"UnusedReplicaSetAge", p.Thresholds.UnusedReplicaSetAge, 1 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value < tt.wantMin {
				t.Errorf("%s = %v, want >= %v", tt.name, tt.value, tt.wantMin)
			}
		})
	}
}

func TestDefaultPolicyRBAC(t *testing.T) {
	p := DefaultPolicy()
	if !p.RBAC.FlagWildcards {
		t.Error("default RBAC should flag wildcards")
	}
	if !p.RBAC.FlagClusterAdmin {
		t.Error("default RBAC should flag cluster-admin")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("nonexistent-policy.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "no such file") && !strings.Contains(err.Error(), "no such") {
		t.Errorf("expected file-not-found error, got %q", err.Error())
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.yaml")
	if err := os.WriteFile(path, []byte("{{{invalid yaml:::"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadDefaultsMerge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "minimal.yaml")
	yaml := `thresholds:
  completedJobAge: 2s
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if p.Thresholds.CompletedJobAge != 2*time.Second {
		t.Errorf("CompletedJobAge = %v, want 2s", p.Thresholds.CompletedJobAge)
	}
	// Missing fields should get defaults.
	if p.Apply.RequireAnnotation != "kubescrub.io/allow-delete=true" {
		t.Errorf("RequireAnnotation = %q, want default", p.Apply.RequireAnnotation)
	}
	if len(p.ExcludeNamespaces) == 0 {
		t.Error("ExcludeNamespaces should have defaults when not set")
	}
}

func TestLoadFullPolicy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "full.yaml")
	yaml := `excludeNamespaces:
  - custom-ns
thresholds:
  completedJobAge: 5s
  failedJobAge: 10s
  pendingPVCAge: 30s
  unusedPVCAge: 60s
  unusedReplicaSetAge: 120s
rbac:
  flagWildcards: false
  flagClusterAdmin: false
  ignoreSubjects:
    - my-sa
apply:
  requireAnnotation: custom.io/allow=yes
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(p.ExcludeNamespaces) != 1 || p.ExcludeNamespaces[0] != "custom-ns" {
		t.Errorf("ExcludeNamespaces = %v, want [custom-ns]", p.ExcludeNamespaces)
	}
	if p.Thresholds.CompletedJobAge != 5*time.Second {
		t.Errorf("CompletedJobAge = %v, want 5s", p.Thresholds.CompletedJobAge)
	}
	if p.Thresholds.FailedJobAge != 10*time.Second {
		t.Errorf("FailedJobAge = %v, want 10s", p.Thresholds.FailedJobAge)
	}
	if p.Thresholds.PendingPVCAge != 30*time.Second {
		t.Errorf("PendingPVCAge = %v, want 30s", p.Thresholds.PendingPVCAge)
	}
	if p.Thresholds.UnusedPVCAge != 60*time.Second {
		t.Errorf("UnusedPVCAge = %v, want 60s", p.Thresholds.UnusedPVCAge)
	}
	if p.Thresholds.UnusedReplicaSetAge != 120*time.Second {
		t.Errorf("UnusedReplicaSetAge = %v, want 120s", p.Thresholds.UnusedReplicaSetAge)
	}
	if p.RBAC.FlagWildcards {
		t.Error("FlagWildcards should be false")
	}
	if p.RBAC.FlagClusterAdmin {
		t.Error("FlagClusterAdmin should be false")
	}
	if len(p.RBAC.IgnoreSubjects) != 1 || p.RBAC.IgnoreSubjects[0] != "my-sa" {
		t.Errorf("IgnoreSubjects = %v, want [my-sa]", p.RBAC.IgnoreSubjects)
	}
	if p.Apply.RequireAnnotation != "custom.io/allow=yes" {
		t.Errorf("RequireAnnotation = %q, want %q", p.Apply.RequireAnnotation,
			"custom.io/allow=yes")
	}
}

func TestLoadEmptyFileUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	def := DefaultPolicy()
	if len(p.ExcludeNamespaces) != len(def.ExcludeNamespaces) {
		t.Errorf("ExcludeNamespaces length = %d, want %d",
			len(p.ExcludeNamespaces), len(def.ExcludeNamespaces))
	}
	if p.Thresholds.CompletedJobAge != def.Thresholds.CompletedJobAge {
		t.Errorf("CompletedJobAge = %v, want %v",
			p.Thresholds.CompletedJobAge, def.Thresholds.CompletedJobAge)
	}
}
