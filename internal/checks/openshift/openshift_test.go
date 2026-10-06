package openshift

import (
	"context"
	"time"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	discoveryfake "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/scheme"
	ktesting "k8s.io/client-go/testing"

	"github.com/caseyrobb/kubescrub/internal/checks"
	"github.com/caseyrobb/kubescrub/internal/policy"
	"github.com/caseyrobb/kubescrub/internal/report"
)

// --- helpers ---

func newFakeDynamic(objects ...any) *dynamicfake.FakeDynamicClient {
	s := runtime.NewScheme()
	_ = scheme.AddToScheme(s)
	var runtimeObjs []runtime.Object
	for _, o := range objects {
		if u, ok := o.(*unstructured.Unstructured); ok {
			runtimeObjs = append(runtimeObjs, u)
		}
	}
	// Register List kinds for OpenShift resources.
	listKinds := map[schema.GroupVersionResource]string{
		{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "clusterserviceversions"}: "ClusterServiceVersionList",
		{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "subscriptions"}:            "SubscriptionList",
		{Group: "route.openshift.io", Version: "v1", Resource: "routes"}:                           "RouteList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(s, listKinds, runtimeObjs...)
}

// newFakeDiscovery sets up a fake discovery client that reports the given
// group versions as available. Each group version should be "group/version".
func newFakeDiscovery(groupVersions ...string) *discoveryfake.FakeDiscovery {
	fd := &discoveryfake.FakeDiscovery{
		Fake: &ktesting.Fake{},
	}
	var resources []*metav1.APIResourceList
	for _, gv := range groupVersions {
		resources = append(resources, &metav1.APIResourceList{
			GroupVersion: gv,
		})
	}
	fd.Resources = resources
	return fd
}

func makeRoute(ns, name, toKind, toName, targetPort string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: make(map[string]any)}
	obj.SetAPIVersion("route.openshift.io/v1")
	obj.SetKind("Route")
	obj.SetNamespace(ns)
	obj.SetName(name)
	spec := map[string]any{
		"to": map[string]any{
			"kind": toKind,
			"name": toName,
		},
	}
	if targetPort != "" {
		spec["port"] = map[string]any{"targetPort": targetPort}
	}
	obj.Object["spec"] = spec
	return obj
}

func makeCSV(ns, name, phase string, creationTime metav1.Time) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: make(map[string]any)}
	obj.SetAPIVersion("operators.coreos.com/v1alpha1")
	obj.SetKind("ClusterServiceVersion")
	obj.SetNamespace(ns)
	obj.SetName(name)
	obj.SetCreationTimestamp(creationTime)
	obj.Object["status"] = map[string]any{"phase": phase}
	return obj
}

func makeSubscription(ns, name, currentCSV string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: make(map[string]any)}
	obj.SetAPIVersion("operators.coreos.com/v1alpha1")
	obj.SetKind("Subscription")
	obj.SetNamespace(ns)
	obj.SetName(name)
	obj.Object["status"] = map[string]any{"currentCSV": currentCSV}
	return obj
}

// --- route tests ---

func TestRouteNoAPIAvailable(t *testing.T) {
	disc := newFakeDiscovery()
	client := newFakeDynamic()

	rt := checks.Runtime{
		Cluster:   "test",
		Dynamic:   client,
		Discovery: disc,
		Policy:    policy.DefaultPolicy(),
	}

	check := NewRoute()
	findings, err := check.Run(context.Background(), rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

func TestRouteMissingService(t *testing.T) {
	route := makeRoute("my-ns", "my-route", "Service", "missing-svc", "")
	disc := newFakeDiscovery("route.openshift.io/v1")
	client := newFakeDynamic(route)

	rt := checks.Runtime{
		Cluster:   "test",
		Dynamic:   client,
		Discovery: disc,
		Policy:    policy.DefaultPolicy(),
	}

	check := NewRoute()
	findings, err := check.Run(context.Background(), rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Reason != "route-missing-service" {
		t.Errorf("reason = %q, want %q", findings[0].Reason, "route-missing-service")
	}
	if findings[0].SafeToApply {
		t.Error("expected SafeToApply = false")
	}
	if findings[0].Suggested != report.ActionReport {
		t.Errorf("Suggested = %q, want %q", findings[0].Suggested, report.ActionReport)
	}
	if findings[0].Group != "route.openshift.io" {
		t.Errorf("Group = %q, want %q", findings[0].Group, "route.openshift.io")
	}
	if findings[0].Version != "v1" {
		t.Errorf("Version = %q, want %q", findings[0].Version, "v1")
	}
	if findings[0].Kind != "Route" {
		t.Errorf("Kind = %q, want %q", findings[0].Kind, "Route")
	}
}

func TestRouteNonServiceTargetSkipped(t *testing.T) {
	route := makeRoute("my-ns", "pod-route", "Pod", "my-pod", "")
	disc := newFakeDiscovery("route.openshift.io/v1")
	client := newFakeDynamic(route)

	rt := checks.Runtime{
		Cluster:   "test",
		Dynamic:   client,
		Discovery: disc,
		Policy:    policy.DefaultPolicy(),
	}

	check := NewRoute()
	findings, err := check.Run(context.Background(), rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for non-Service target, got %d", len(findings))
	}
}

// --- csv tests ---

func TestCSVNoAPIAvailable(t *testing.T) {
	disc := newFakeDiscovery()
	client := newFakeDynamic()

	rt := checks.Runtime{
		Cluster:   "test",
		Dynamic:   client,
		Discovery: disc,
		Policy:    policy.DefaultPolicy(),
	}

	check := NewCSV()
	findings, err := check.Run(context.Background(), rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

func TestCSVCurrenCSV(t *testing.T) {
	sub := makeSubscription("my-ns", "my-sub", "my-operator.v1.0.0")
	csv := makeCSV("my-ns", "my-operator.v1.0.0", "Succeeded", metav1.Now())

	disc := newFakeDiscovery("operators.coreos.com/v1alpha1")
	client := newFakeDynamic(sub, csv)

	rt := checks.Runtime{
		Cluster:   "test",
		Dynamic:   client,
		Discovery: disc,
		Policy:    policy.DefaultPolicy(),
	}

	check := NewCSV()
	findings, err := check.Run(context.Background(), rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for current CSV, got %d: %v", len(findings), findings)
	}
}

func TestCSVNotCurrent(t *testing.T) {
	csv := makeCSV("my-ns", "old-operator.v0.1.0", "Succeeded", metav1.Now())

	disc := newFakeDiscovery("operators.coreos.com/v1alpha1")
	client := newFakeDynamic(csv)

	rt := checks.Runtime{
		Cluster:   "test",
		Dynamic:   client,
		Discovery: disc,
		Policy:    policy.DefaultPolicy(),
	}

	check := NewCSV()
	findings, err := check.Run(context.Background(), rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Reason != "csv-not-current" {
		t.Errorf("reason = %q, want %q", findings[0].Reason, "csv-not-current")
	}
	if findings[0].SafeToApply {
		t.Error("expected SafeToApply = false")
	}
	if findings[0].Group != "operators.coreos.com" {
		t.Errorf("Group = %q, want %q", findings[0].Group, "operators.coreos.com")
	}
	if findings[0].Version != "v1alpha1" {
		t.Errorf("Version = %q, want %q", findings[0].Version, "v1alpha1")
	}
	if findings[0].Kind != "ClusterServiceVersion" {
		t.Errorf("Kind = %q, want %q", findings[0].Kind, "ClusterServiceVersion")
	}
}

func TestCSVStuckCurrent(t *testing.T) {
	sub := makeSubscription("my-ns", "my-sub", "stuck-operator.v1.0.0")
	csv := makeCSV("my-ns", "stuck-operator.v1.0.0", "Failed", metav1.NewTime(metav1.Now().Time.Add(-2*time.Hour)))

	disc := newFakeDiscovery("operators.coreos.com/v1alpha1")
	client := newFakeDynamic(sub, csv)

	rt := checks.Runtime{
		Cluster:   "test",
		Dynamic:   client,
		Discovery: disc,
		Policy:    policy.DefaultPolicy(),
	}

	check := NewCSV()
	findings, err := check.Run(context.Background(), rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Reason != "csv-stuck" {
		t.Errorf("reason = %q, want %q", findings[0].Reason, "csv-stuck")
	}
	if findings[0].Suggested != report.ActionReport {
		t.Errorf("Suggested = %q, want %q", findings[0].Suggested, report.ActionReport)
	}
	if findings[0].SafeToApply {
		t.Error("expected SafeToApply = false")
	}
}
