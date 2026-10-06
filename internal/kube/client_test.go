package kube

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
)

func TestResolveKubeconfigFlagWins(t *testing.T) {
	// Flag takes precedence over KUBECONFIG env.
	t.Setenv("KUBECONFIG", "/env/path")
	got := ResolveKubeconfigPath("/flag/path")
	if got != "/flag/path" {
		t.Errorf("ResolveKubeconfigPath(\"/flag/path\") = %q, want \"/flag/path\"", got)
	}
}

func TestResolveKubeconfigEnvWinsOverDefault(t *testing.T) {
	// KUBECONFIG env wins when no flag is set.
	t.Setenv("KUBECONFIG", "/env/path")
	got := ResolveKubeconfigPath("")
	if got != "/env/path" {
		t.Errorf("ResolveKubeconfigPath(\"\") = %q, want \"/env/path\"", got)
	}
}

func TestResolveKubeconfigDefaultWhenUnset(t *testing.T) {
	// When neither flag nor env is set, falls back to default.
	t.Setenv("KUBECONFIG", "")
	got := ResolveKubeconfigPath("")
	if got != clientcmd.RecommendedHomeFile {
		t.Errorf("ResolveKubeconfigPath(\"\") = %q, want %q", got, clientcmd.RecommendedHomeFile)
	}
}

func TestResolveCurrentContextError(t *testing.T) {
	// KUBECONFIG pointing to a nonexistent file should fail.
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "nonexistent")
	t.Setenv("KUBECONFIG", missing)

	_, err := ResolveCurrentContext("")
	if err == nil {
		t.Fatal("expected error for nonexistent KUBECONFIG")
	}
	// Error should mention the path.
	if want := missing; err != nil && !containsStr(err.Error(), want) {
		t.Errorf("error = %q, want to contain %q", err.Error(), want)
	}
}

func TestResolveCurrentContextWithExistingFile(t *testing.T) {
	// KUBECONFIG pointing to a valid file should succeed.
	tmp := t.TempDir()
	kubeconfigPath := filepath.Join(tmp, "kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte(`apiVersion: v1
kind: Config
clusters:
- name: test-cluster
  cluster:
    server: https://test:6443
contexts:
- name: from-env
  context:
    cluster: test-cluster
    user: test-user
current-context: from-env
users:
- name: test-user
  user:
    token: test-token
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kubeconfigPath)

	ctx, err := ResolveCurrentContext("")
	if err != nil {
		t.Fatalf("ResolveCurrentContext: %v", err)
	}
	if ctx != "from-env" {
		t.Errorf("current-context = %q, want %q", ctx, "from-env")
	}
}

func TestResolveCurrentContextFlagOverridesEnv(t *testing.T) {
	// --kubeconfig flag takes precedence over KUBECONFIG env.
	tmp := t.TempDir()
	envFile := filepath.Join(tmp, "env-kubeconfig")
	flagFile := filepath.Join(tmp, "flag-kubeconfig")

	for _, f := range []struct {
		path string
		ctx  string
	}{
		{envFile, "from-env"},
		{flagFile, "from-flag"},
	} {
		if err := os.WriteFile(f.path, []byte(`apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: https://test:6443
contexts:
- name: `+f.ctx+`
  context:
    cluster: test
    user: test
current-context: `+f.ctx+`
users:
- name: test
  user:
    token: test
`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("KUBECONFIG", envFile)

	// --kubeconfig flag should override KUBECONFIG env.
	ctx, err := ResolveCurrentContext(flagFile)
	if err != nil {
		t.Fatalf("ResolveCurrentContext: %v", err)
	}
	if ctx != "from-flag" {
		t.Errorf("current-context = %q, want %q", ctx, "from-flag")
	}
}

func TestResolveCurrentContextContextOverride(t *testing.T) {
	tmp := t.TempDir()
	kubeconfigPath := filepath.Join(tmp, "kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte(`apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: https://test:6443
contexts:
- name: from-env
  context:
    cluster: test
    user: test
- name: other
  context:
    cluster: test
    user: test
current-context: from-env
users:
- name: test
  user:
    token: test
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set KUBECONFIG so the temp file is used, not the system default.
	t.Setenv("KUBECONFIG", kubeconfigPath)

	// No --context → uses file's current-context.
	ctx, err := ResolveCurrentContext("")
	if err != nil {
		t.Fatalf("ResolveCurrentContext: %v", err)
	}
	if ctx != "from-env" {
		t.Errorf("current-context without override = %q, want %q", ctx, "from-env")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStrHelper(s, substr))
}

func containsStrHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
