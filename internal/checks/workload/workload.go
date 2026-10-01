package workload

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/caseyrobb/kubescrub/internal/checks"
	"github.com/caseyrobb/kubescrub/internal/report"
)

const appsV1 = "apps/v1"

type workload struct{}

func New() checks.Check { return &workload{} }

func (c *workload) Name() string { return "workload" }

func (c *workload) Description() string {
	return "Checks for stale Deployments, Jobs, and ReplicaSets"
}

func (c *workload) Run(ctx context.Context, rt checks.Runtime) ([]report.Finding, error) {
	var findings []report.Finding

	// Collect findings from each check.
	f, err := c.checkZeroReplicas(ctx, rt)
	if err != nil {
		return nil, err
	}
	findings = append(findings, f...)

	f, err = c.checkMissingImageTags(ctx, rt)
	if err != nil {
		return nil, err
	}
	findings = append(findings, f...)

	f, err = c.checkDeprecatedAPIs(ctx, rt)
	if err != nil {
		return nil, err
	}
	findings = append(findings, f...)

	f, err = c.checkOrphanReplicaSets(ctx, rt)
	if err != nil {
		return nil, err
	}
	findings = append(findings, f...)

	return findings, nil
}

// listDeploymentsWithPagination lists all Deployments in a namespace with Continue pagination.
func listDeploymentsWithPagination(ctx context.Context, client checks.Runtime, ns string) ([]appsv1.Deployment, error) {
	page, err := client.Client.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{Limit: 500})
	if err != nil {
		return nil, err
	}
	allItems := page.Items
	for page.Continue != "" {
		page, err = client.Client.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{
			Limit:    500,
			Continue: page.Continue,
		})
		if err != nil {
			return nil, err
		}
		allItems = append(allItems, page.Items...)
	}
	return allItems, nil
}

// listStatefulSetsWithPagination lists all StatefulSets in a namespace with Continue pagination.
func listStatefulSetsWithPagination(ctx context.Context, client checks.Runtime, ns string) ([]appsv1.StatefulSet, error) {
	page, err := client.Client.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{Limit: 500})
	if err != nil {
		return nil, err
	}
	allItems := page.Items
	for page.Continue != "" {
		page, err = client.Client.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{
			Limit:    500,
			Continue: page.Continue,
		})
		if err != nil {
			return nil, err
		}
		allItems = append(allItems, page.Items...)
	}
	return allItems, nil
}

// listJobsWithPagination lists all Jobs in a namespace with Continue pagination.
func listJobsWithPagination(ctx context.Context, client checks.Runtime, ns string) ([]batchv1.Job, error) {
	page, err := client.Client.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{Limit: 500})
	if err != nil {
		return nil, err
	}
	allItems := page.Items
	for page.Continue != "" {
		page, err = client.Client.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{
			Limit:    500,
			Continue: page.Continue,
		})
		if err != nil {
			return nil, err
		}
		allItems = append(allItems, page.Items...)
	}
	return allItems, nil
}

// listReplicaSetsWithPagination lists all ReplicaSets in a namespace with Continue pagination.
func listReplicaSetsWithPagination(ctx context.Context, client checks.Runtime, ns string) ([]appsv1.ReplicaSet, error) {
	page, err := client.Client.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{Limit: 500})
	if err != nil {
		return nil, err
	}
	allItems := page.Items
	for page.Continue != "" {
		page, err = client.Client.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{
			Limit:    500,
			Continue: page.Continue,
		})
		if err != nil {
			return nil, err
		}
		allItems = append(allItems, page.Items...)
	}
	return allItems, nil
}

// listWithDynamicPagination lists all unstructured items for a GVR with Continue pagination.
func listWithDynamicPagination(ctx context.Context, rt checks.Runtime, gvr schema.GroupVersionResource) ([]unstructured.Unstructured, error) {
	page, err := rt.Dynamic.Resource(gvr).List(ctx, metav1.ListOptions{Limit: 500})
	if err != nil {
		return nil, err
	}
	allItems := page.Items
	for page.GetContinue() != "" {
		page, err = rt.Dynamic.Resource(gvr).List(ctx, metav1.ListOptions{
			Limit:    500,
			Continue: page.GetContinue(),
		})
		if err != nil {
			return nil, err
		}
		allItems = append(allItems, page.Items...)
	}
	return allItems, nil
}

// checkZeroReplicas finds Deployments, Jobs, and StatefulSets with zero available replicas.
func (c *workload) checkZeroReplicas(ctx context.Context, rt checks.Runtime) ([]report.Finding, error) {
	var findings []report.Finding
	ns := filterNamespaces(rt.Namespaces)

	// Check Deployments with zero available replicas.
	deployments, err := listDeploymentsWithPagination(ctx, rt, ns)
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	for _, d := range deployments {
		desired := d.Spec.Replicas
		if desired != nil && d.Status.AvailableReplicas == 0 {
			findings = append(findings, report.Finding{
				ID:          fmt.Sprintf("workload-zero-replicas-%s-%s", d.Namespace, d.Name),
				Check:       "workload",
				Severity:    report.SeverityHigh,
				Cluster:     rt.Cluster,
				Namespace:   d.Namespace,
				Version:     appsV1,
				Kind:        "Deployment",
				Name:        d.Name,
				Message:     fmt.Sprintf("Deployment %s/%s has 0 available replicas (desired: %d)", d.Namespace, d.Name, *desired),
				Reason:      "zero-replicas",
				Suggested:   report.ActionReport,
				Evidence: map[string]any{
					"availableReplicas": d.Status.AvailableReplicas,
					"desiredReplicas":   *desired,
				},
			})
		}
	}

	// Check StatefulSets with zero available replicas.
	ss, err := listStatefulSetsWithPagination(ctx, rt, ns)
	if err != nil {
		return nil, fmt.Errorf("list statefulsets: %w", err)
	}
	for _, s := range ss {
		desired := s.Spec.Replicas
		if desired != nil && s.Status.AvailableReplicas == 0 {
			findings = append(findings, report.Finding{
				ID:          fmt.Sprintf("workload-zero-replicas-%s-%s", s.Namespace, s.Name),
				Check:       "workload",
				Severity:    report.SeverityHigh,
				Cluster:     rt.Cluster,
				Namespace:   s.Namespace,
				Version:     appsV1,
				Kind:        "StatefulSet",
				Name:        s.Name,
				Message:     fmt.Sprintf("StatefulSet %s/%s has 0 available replicas (desired: %d)", s.Namespace, s.Name, *desired),
				Reason:      "zero-replicas",
				Suggested:   report.ActionReport,
				Evidence: map[string]any{
					"availableReplicas": s.Status.AvailableReplicas,
					"desiredReplicas":   *desired,
				},
			})
		}
	}

	// Check Jobs with no active pods.
	jobs, err := listJobsWithPagination(ctx, rt, ns)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	for _, j := range jobs {
		if j.Status.Active == 0 {
			// Only report completed/failed jobs that are still "active" (i.e. not cleaned up).
			// We flag jobs that have completed but not been cleaned up per policy.
			if j.Status.Succeeded > 0 || j.Status.Failed > 0 {
				findings = append(findings, report.Finding{
					ID:          fmt.Sprintf("workload-zero-active-%s-%s", j.Namespace, j.Name),
					Check:       "workload",
					Severity:    report.SeverityInfo,
					Cluster:     rt.Cluster,
					Namespace:   j.Namespace,
					Version:     "batch/v1",
					Kind:        "Job",
					Name:        j.Name,
					Message:     fmt.Sprintf("Job %s/%s has completed with no active pods", j.Namespace, j.Name),
					Reason:      "completed-job",
					Suggested:   report.ActionReport,
					Evidence: map[string]any{
						"active":    j.Status.Active,
						"succeeded": j.Status.Succeeded,
						"failed":    j.Status.Failed,
					},
				})
			}
		}
	}

	return findings, nil
}

// checkMissingImageTags finds containers using :latest or no tag.
func (c *workload) checkMissingImageTags(ctx context.Context, rt checks.Runtime) ([]report.Finding, error) {
	ns := filterNamespaces(rt.Namespaces)
	var findings []report.Finding

	// Scan Deployments.
	deployments, err := listDeploymentsWithPagination(ctx, rt, ns)
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	for _, d := range deployments {
		for _, container := range d.Spec.Template.Spec.Containers {
			if hasMissingTag(container.Image) {
				findings = append(findings, report.Finding{
					ID:          fmt.Sprintf("workload-no-tag-%s-%s-%s", d.Namespace, d.Name, container.Name),
					Check:       "workload",
					Severity:    report.SeverityMedium,
					Cluster:     rt.Cluster,
					Namespace:   d.Namespace,
					Version:     appsV1,
					Kind:        "Deployment",
					Name:        d.Name,
					Message:     fmt.Sprintf("Container %s in Deployment %s/%s uses image without tag: %s", container.Name, d.Namespace, d.Name, container.Image),
					Reason:      "missing-image-tag",
					Suggested:   report.ActionReport,
					Evidence: map[string]any{
						"image": container.Image,
					},
				})
			}
		}
		// Also check init containers.
		for _, container := range d.Spec.Template.Spec.InitContainers {
			if hasMissingTag(container.Image) {
				findings = append(findings, report.Finding{
					ID:          fmt.Sprintf("workload-no-tag-%s-%s-init-%s", d.Namespace, d.Name, container.Name),
					Check:       "workload",
					Severity:    report.SeverityMedium,
					Cluster:     rt.Cluster,
					Namespace:   d.Namespace,
					Version:     appsV1,
					Kind:        "Deployment",
					Name:        d.Name,
					Message:     fmt.Sprintf("Init container %s in Deployment %s/%s uses image without tag: %s", container.Name, d.Namespace, d.Name, container.Image),
					Reason:      "missing-image-tag",
					Suggested:   report.ActionReport,
					Evidence: map[string]any{
						"image": container.Image,
					},
				})
			}
		}
	}

	// Scan StatefulSets.
	ss, err := listStatefulSetsWithPagination(ctx, rt, ns)
	if err != nil {
		return nil, fmt.Errorf("list statefulsets: %w", err)
	}
	for _, s := range ss {
		for _, container := range s.Spec.Template.Spec.Containers {
			if hasMissingTag(container.Image) {
				findings = append(findings, report.Finding{
					ID:          fmt.Sprintf("workload-no-tag-%s-%s-%s", s.Namespace, s.Name, container.Name),
					Check:       "workload",
					Severity:    report.SeverityMedium,
					Cluster:     rt.Cluster,
					Namespace:   s.Namespace,
					Version:     appsV1,
					Kind:        "StatefulSet",
					Name:        s.Name,
					Message:     fmt.Sprintf("Container %s in StatefulSet %s/%s uses image without tag: %s", container.Name, s.Namespace, s.Name, container.Image),
					Reason:      "missing-image-tag",
					Suggested:   report.ActionReport,
					Evidence: map[string]any{
						"image": container.Image,
					},
				})
			}
		}
	}

	return findings, nil
}

// checkDeprecatedAPIs finds resources using deprecated API versions.
func (c *workload) checkDeprecatedAPIs(ctx context.Context, rt checks.Runtime) ([]report.Finding, error) {
	// List known deprecated GVRs.
	deprecatedGVs := map[string]bool{
		"extensions/v1beta1":                   true,
		"apps/v1beta1":                         true,
		"apps/v1beta2":                         true,
		"networking.k8s.io/v1beta1":            true,
		"policy/v1beta1":                       true,
		"apiregistration.k8s.io/v1beta1":       true,
	}

	var findings []report.Finding

	// Discover available API versions and look for deprecated resources.
	// We check common workload kinds that used to be in extensions.
	kinds := []schema.GroupVersionKind{
		{Group: "extensions", Version: "v1beta1", Kind: "Deployment"},
		{Group: "extensions", Version: "v1beta1", Kind: "StatefulSet"},
		{Group: "extensions", Version: "v1beta1", Kind: "DaemonSet"},
		{Group: "extensions", Version: "v1beta1", Kind: "Ingress"},
		{Group: "networking.k8s.io", Version: "v1beta1", Kind: "Ingress"},
		{Group: "policy", Version: "v1beta1", Kind: "PodSecurityPolicy"},
	}

	for _, gvk := range kinds {
		if !deprecatedGVs[gvk.GroupVersion().String()] {
			continue
		}
		// Try to list; if the API is gone, skip silently.
		gvr := schema.GroupVersionResource{
			Group:    gvk.Group,
			Version:  gvk.Version,
			Resource: toResource(gvk.Kind),
		}
		allItems, err := listWithDynamicPagination(ctx, rt, gvr)
		if err != nil {
			continue // API version no longer available
		}
		for _, item := range allItems {
			findings = append(findings, report.Finding{
				ID:          fmt.Sprintf("workload-deprecated-api-%s-%s-%s", item.GetNamespace(), item.GetKind(), item.GetName()),
				Check:       "workload",
				Severity:    report.SeverityHigh,
				Cluster:     rt.Cluster,
				Namespace:   item.GetNamespace(),
				Version:     gvk.Version,
				Group:       gvk.Group,
				Kind:        gvk.Kind,
				Name:        item.GetName(),
				Message:     fmt.Sprintf("Resource %s/%s uses deprecated API %s/%s", item.GetNamespace(), item.GetName(), gvk.Group, gvk.Version),
				Reason:      "deprecated-api",
				Suggested:   report.ActionReport,
				Evidence: map[string]any{
					"apiVersion": fmt.Sprintf("%s/%s", gvk.Group, gvk.Version),
					"kind":       gvk.Kind,
				},
			})
		}
	}

	return findings, nil
}

// checkOrphanReplicaSets finds ReplicaSets not owned by any Deployment, CronJob, or another ReplicaSet.
func (c *workload) checkOrphanReplicaSets(ctx context.Context, rt checks.Runtime) ([]report.Finding, error) {
	var findings []report.Finding
	ns := filterNamespaces(rt.Namespaces)

	// Get all ReplicaSets.
	allReplicaSets, err := listReplicaSetsWithPagination(ctx, rt, ns)
	if err != nil {
		return nil, fmt.Errorf("list replicasets: %w", err)
	}

	// Build a set of ReplicaSet names that have a known owner.
	// Use owner references directly — a ReplicaSet is not orphaned if it has
	// an owner reference to a Deployment, CronJob, or another ReplicaSet.
	ownedRS := make(map[string]struct{})
	for _, rs := range allReplicaSets {
		owned := false
		for _, ref := range rs.OwnerReferences {
			if ref.Kind == "Deployment" && ref.APIVersion == appsV1 {
				owned = true
				break
			}
			if ref.Kind == "CronJob" && ref.APIVersion == "batch/v1" {
				owned = true
				break
			}
			if ref.Kind == "ReplicaSet" && ref.APIVersion == appsV1 {
				owned = true
				break
			}
		}
		if owned {
			ownedRS[rs.Namespace+"/"+rs.Name] = struct{}{}
		}
	}

	for _, rs := range allReplicaSets {
		key := rs.Namespace + "/" + rs.Name
		if _, isOwned := ownedRS[key]; isOwned {
			continue
		}
		// Orphan ReplicaSet
		findings = append(findings, report.Finding{
			ID:          fmt.Sprintf("workload-orphan-rs-%s-%s", rs.Namespace, rs.Name),
			Check:       "workload",
			Severity:    report.SeverityLow,
			Cluster:     rt.Cluster,
			Namespace:   rs.Namespace,
			Version:     appsV1,
			Kind:        "ReplicaSet",
			Name:        rs.Name,
			Message:     fmt.Sprintf("ReplicaSet %s/%s has no Deployment, CronJob, or ReplicaSet owner (orphan)", rs.Namespace, rs.Name),
			Reason:      "orphan-replicaset",
			Suggested:   report.ActionReport,
			Evidence:    map[string]any{"info": "No owner reference to a Deployment, CronJob, or ReplicaSet found"},
		})
	}

	return findings, nil
}

// hasMissingTag checks if an image uses :latest or has no tag at all.
func hasMissingTag(image string) bool {
	if image == "" {
		return true
	}
	// Split on ':' to check for tag.
	// Handle registry:port/image format carefully.
	parts := strings.Split(image, "/")
	last := parts[len(parts)-1]
	colonIdx := strings.Index(last, ":")
	tagPresent := colonIdx >= 0
	slashIdx := strings.Index(last, "/")
	if slashIdx >= 0 && slashIdx < colonIdx {
		// Tag is before a slash, so it's part of a path, not a tag.
		tagPresent = false
	}
	if !tagPresent {
		return true // No tag -> implicit latest
	}
	tag := last[colonIdx+1:]
	return tag == "latest" // Explicit latest tag
}

// toResource converts a Kubernetes Kind to its plural resource name.
func toResource(kind string) string {
	switch kind {
	case "Deployment":
		return "deployments"
	case "StatefulSet":
		return "statefulsets"
	case "DaemonSet":
		return "daemonsets"
	case "Ingress":
		return "ingresses"
	case "PodSecurityPolicy":
		return "podsecuritypolicies"
	default:
		return strings.ToLower(kind) + "s"
	}
}

// zeroReplicaFindings is a placeholder for zero-replica finding logic.
func (c *workload) zeroReplicaFindings(namespace, name, kind string) []report.Finding {
	return nil
}

// orphanRSFindings is a placeholder for orphan RS finding logic.
func (c *workload) orphanRSFindings(ns, namespace, name string) []report.Finding {
	return nil
}

// filterNamespaces returns the effective namespace list for queries.
func filterNamespaces(namespaces []string) string {
	if len(namespaces) == 0 {
		return ""
	}
	return strings.Join(namespaces, ",")
}

// LabelsForDeployment returns the labels for a Deployment resource.
func LabelsForDeployment(d appsv1.Deployment) labels.Set {
	return d.Spec.Selector.MatchLabels
}

// LabelsForStatefulSet returns the labels for a StatefulSet resource.
func LabelsForStatefulSet(s appsv1.StatefulSet) labels.Set {
	return s.Spec.Selector.MatchLabels
}
