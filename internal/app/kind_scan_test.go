//go:build kind

package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/caseyrobb/kubescrub/internal/apply"
	"github.com/caseyrobb/kubescrub/internal/checks"
	"github.com/caseyrobb/kubescrub/internal/policy"
	"github.com/caseyrobb/kubescrub/internal/report"
)

// goldenProjection holds the stable subset of a Finding used for
// deterministic comparison against testdata/clusters/expected-scan.json.
type goldenProjection struct {
	ID              string `json:"id"`
	Check           string `json:"check"`
	Reason          string `json:"reason"`
	Namespace       string `json:"namespace,omitempty"`
	Kind            string `json:"kind"`
	Name            string `json:"name"`
	Version         string `json:"version"`
	Replacement     string `json:"replacement,omitempty"`
	SuggestedAction string `json:"suggestedAction"`
	SafeToApply     bool   `json:"safeToApply"`
}

// findGolden returns the golden-projection fields of a Finding.
func findGolden(f report.Finding) goldenProjection {
	return goldenProjection{
		ID:              f.ID,
		Check:           f.Check,
		Reason:          f.Reason,
		Namespace:       f.Namespace,
		Kind:            f.Kind,
		Name:            f.Name,
		Version:         f.Version,
		Replacement:     f.Replacement,
		SuggestedAction: string(f.Suggested),
		SafeToApply:     f.SafeToApply,
	}
}

// sortGoldenProjects sorts a slice of goldenProjection by a stable key.
func sortGoldenProjects(gs []goldenProjection) {
	sort.Slice(gs, func(i, j int) bool {
		if gs[i].Check != gs[j].Check {
			return gs[i].Check < gs[j].Check
		}
		if gs[i].Namespace != gs[j].Namespace {
			return gs[i].Namespace < gs[j].Namespace
		}
		if gs[i].Kind != gs[j].Kind {
			return gs[i].Kind < gs[j].Kind
		}
		return gs[i].Name < gs[j].Name
	})
}

func TestKindScanGolden(t *testing.T) {
	ctx := context.Background()

	// ── build kubeconfig pointing at kind-kubescrub-dev ──────────────
	kubeconfig := clientcmd.RecommendedHomeFile
	if v := os.Getenv("KUBECONFIG"); v != "" {
		kubeconfig = v
	}
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	overrides := &clientcmd.ConfigOverrides{
		CurrentContext: "kind-kubescrub-dev",
	}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	cfg, err := kubeConfig.ClientConfig()
	if err != nil {
		t.Skipf("kind-kubescrub-dev context not reachable: %v", err)
	}

	// ── verify cluster is up ─────────────────────────────────────────
	k8s, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Skipf("cannot create k8s client: %v", err)
	}
	if _, err := k8s.Discovery().ServerVersion(); err != nil {
		t.Skipf("kind-kubescrub-dev unreachable: %v", err)
	}

	// ── optional auto cluster bootstrap via KUBESCRUB_KIND_AUTO ─────
	if os.Getenv("KUBESCRUB_KIND_AUTO") == "1" {
		t.Log("KUBESCRUB_KIND_AUTO=1 — running kind reset")
		cmd := exec.CommandContext(ctx, "bash", "scripts/kind.sh", "reset")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("kind reset failed: %v", err)
		}
	}

	// ── build the scanner (same code as the CLI scan command) ───────
	p, err := policy.Load("testdata/clusters/policy-kind.yaml")
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}

	rt := checks.Runtime{
		Cluster:    cfg.Host,
		Client:     k8s,
		Dynamic:    mustDynamic(cfg),
		Discovery:  k8s.Discovery(),
		Policy:     *p,
		Namespaces: nil, // all namespaces
	}

	var findings []report.Finding
	for _, chk := range allChecks() {
		chkFindings, err := chk.Run(ctx, rt)
		if err != nil {
			t.Fatalf("check %s: %v", chk.Name(), err)
		}
		findings = append(findings, chkFindings...)
	}

	// Build Plan and encode (same as CLI).
	plan := apply.NewPlan(cfg.Host, "")
	for _, f := range findings {
		plan.AddFinding(f)
	}

	// ── golden comparison ───────────────────────────────────────────
	var projections []goldenProjection
	for _, f := range plan.Findings {
		projections = append(projections, findGolden(f))
	}
	sortGoldenProjects(projections)

	goldenPath := filepath.Join("testdata", "clusters", "expected-scan.json")
	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	var expectedProjs []goldenProjection
	if err := json.Unmarshal(expected, &expectedProjs); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}

	actualJSON, err := json.Marshal(projections)
	if err != nil {
		t.Fatalf("marshal actual: %v", err)
	}

	if string(actualJSON) != string(expected) {
		t.Errorf("scan output differs from golden.\nExpected:\n%s\nActual:\n%s",
			string(expected), string(actualJSON))
	} else {
		t.Logf("golden matched — %d findings", len(projections))
	}
}

func mustDynamic(cfg *rest.Config) dynamic.Interface {
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		panic(err)
	}
	return dyn
}
