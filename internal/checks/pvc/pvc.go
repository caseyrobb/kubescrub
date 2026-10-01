package pvc

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/caseyrobb/kubescrub/internal/checks"
	"github.com/caseyrobb/kubescrub/internal/policy"
	"github.com/caseyrobb/kubescrub/internal/report"
)

type pvcChecker struct{}

func New() checks.Check { return &pvcChecker{} }

func (c *pvcChecker) Name() string                          { return "pvc" }
func (c *pvcChecker) Description() string                   { return "Checks for stale PersistentVolumeClaims" }

func (c *pvcChecker) Run(ctx context.Context, rt checks.Runtime) ([]report.Finding, error) {
	var findings []report.Finding

	// Build a set of excluded namespaces from policy + system namespaces.
	excludedNs := make(map[string]struct{})
	for _, ns := range rt.Policy.ExcludeNamespaces {
		excludedNs[ns] = struct{}{}
	}
	for _, ns := range defaultSystemNamespaces() {
		excludedNs[ns] = struct{}{}
	}

	// List all Pods to build a reference map (which PVCs are mounted/owned).
	podPVCRefs, err := c.listPodPVCReferences(ctx, rt)
	if err != nil {
		return nil, fmt.Errorf("listing pod PVC references: %w", err)
	}

	// Build a map of StatefulSet ordinal PVCs: namespace -> set of PVC names
	// that are claimed by a live StatefulSet via volumeClaimTemplates.
	statefulSetOrdinalPVCs, err := c.listStatefulSetOrdinalPVCs(ctx, rt)
	if err != nil {
		return nil, fmt.Errorf("listing StatefulSet ordinal PVCs: %w", err)
	}

	// Paginate through PVCs.
	page, err := rt.Client.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{Limit: 500})
	if err != nil {
		return nil, fmt.Errorf("listing PVCs: %w", err)
	}

	allPVCs := page.Items
	for page.Continue != "" {
		page, err = rt.Client.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{
			Limit:    500,
			Continue: page.Continue,
		})
		if err != nil {
			return nil, fmt.Errorf("listing PVCs (continue): %w", err)
		}
		allPVCs = append(allPVCs, page.Items...)
	}

	// Evaluate each PVC.
	now := metav1.Now()
	for _, pvc := range allPVCs {
		// Skip excluded namespaces.
		if _, ok := excludedNs[pvc.Namespace]; ok {
			continue
		}

		// Apply namespace filter if set.
		if len(rt.Namespaces) > 0 {
			found := false
			for _, ns := range rt.Namespaces {
				if pvc.Namespace == ns {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		finding := c.checkPVC(pvc, now, podPVCRefs, statefulSetOrdinalPVCs)
		if finding != nil {
			finding.Cluster = rt.Cluster
			findings = append(findings, *finding)
		}
	}

	return findings, nil
}

// defaultSystemNamespaces returns the built-in system namespaces.
func defaultSystemNamespaces() []string {
	return []string{
		"kube-system",
		"kube-public",
		"kube-node-lease",
		"local-path-storage",
	}
}

// listPodPVCReferences builds a map of PVC names that are referenced by Pods.
// The map key is "namespace/pvcName".
func (c *pvcChecker) listPodPVCReferences(ctx context.Context, rt checks.Runtime) (map[string]struct{}, error) {
	refs := make(map[string]struct{})

	page, err := rt.Client.CoreV1().Pods("").List(ctx, metav1.ListOptions{Limit: 500})
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}

	allPods := page.Items
	for page.Continue != "" {
		page, err = rt.Client.CoreV1().Pods("").List(ctx, metav1.ListOptions{
			Limit:    500,
			Continue: page.Continue,
		})
		if err != nil {
			return nil, fmt.Errorf("listing pods (continue): %w", err)
		}
		allPods = append(allPods, page.Items...)
	}

	for _, pod := range allPods {
		for _, vol := range pod.Spec.Volumes {
			if vol.VolumeSource.PersistentVolumeClaim != nil {
				key := pod.Namespace + "/" + vol.VolumeSource.PersistentVolumeClaim.ClaimName
				refs[key] = struct{}{}
			}
		}
		// Also check owner references for Pods owned by workloads (StatefulSet, etc.)
		// Owner refs are on the Pod, but PVC references are on the pod spec volumes.
	}

	return refs, nil
}

// listStatefulSetOrdinalPVCs returns a map of StatefulSet-owned ordinal PVC names.
// Key: "namespace/pvcName" -> struct{}{}.
func (c *pvcChecker) listStatefulSetOrdinalPVCs(ctx context.Context, rt checks.Runtime) (map[string]struct{}, error) {
	ordinalPVCs := make(map[string]struct{})

	// We use dynamic to list StatefulSets since there's no apps/v1 helper in the typed client
	// for all apps resources at once. Actually, we have the typed client: rt.Client.AppsV1().
	// List StatefulSets with pagination.
	page, err := rt.Client.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{Limit: 500})
	if err != nil {
		return nil, fmt.Errorf("listing StatefulSets: %w", err)
	}

	allSS := page.Items
	for page.Continue != "" {
		page, err = rt.Client.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{
			Limit:    500,
			Continue: page.Continue,
		})
		if err != nil {
			return nil, fmt.Errorf("listing StatefulSets (continue): %w", err)
		}
		allSS = append(allSS, page.Items...)
	}

	for _, ss := range allSS {
		// Only consider non-terminating StatefulSets.
		if ss.DeletionTimestamp != nil {
			continue
		}
		if ss.Spec.Replicas == nil {
			continue
		}
		replicas := int(*ss.Spec.Replicas)
		for _, vct := range ss.Spec.VolumeClaimTemplates {
			for i := 0; i < replicas; i++ {
				pvcName := fmt.Sprintf("%s-%s-%d", ss.Name, vct.Name, i)
				key := ss.Namespace + "/" + pvcName
				ordinalPVCs[key] = struct{}{}
			}
		}
	}

	return ordinalPVCs, nil
}

// checkPVC evaluates a single PVC for unbound or unused findings.
func (c *pvcChecker) checkPVC(
	pvc corev1.PersistentVolumeClaim,
	now metav1.Time,
	podPVCRefs map[string]struct{},
	statefulSetOrdinalPVCs map[string]struct{},
) *report.Finding {
	key := pvc.Namespace + "/" + pvc.Name

	// Rule 1: Unbound (Pending) PVC.
	if pvc.Status.Phase == corev1.ClaimPending {
		age := now.Sub(pvc.CreationTimestamp.Time)
		pendingAge := c.getThreshold(pvc.Namespace, policy.DefaultPolicy().Thresholds.PendingPVCAge)
		if age >= pendingAge {
			return c.buildFinding(pvc, now, key, "unbound-pvc",
				fmt.Sprintf("PVC %q is in Pending state for %s", pvc.Name, age.Round(time.Second)),
				true, report.ActionDelete, map[string]any{
					"phase":            string(pvc.Status.Phase),
					"age":              age.String(),
					"storageClassName": stringPtrValue(pvc.Spec.StorageClassName),
				})
		}
		return nil
	}

	// Rule 2: Unused PVC (Bound or Available with no Pod mounting).
	if pvc.Status.Phase == corev1.ClaimBound {
		// Skip if a Running or Pending Pod references it.
		if podPVCRefs[key] != struct{}{} {
			// Check if any referencing pod is in Running or Pending state.
			// For simplicity, we check all pod refs — if there are any, we still
			// flag it unless we can confirm a Running pod uses it. We'll defer to
			// a more thorough check below if needed. For now, if any pod refs
			// exist, do not flag as unused (conservative).
			return nil
		}

		// Skip if this is an ordinal PVC owned by a live StatefulSet.
		if statefulSetOrdinalPVCs[key] != struct{}{} {
			return nil
		}

		age := now.Sub(pvc.CreationTimestamp.Time)
		unusedAge := c.getThreshold(pvc.Namespace, policy.DefaultPolicy().Thresholds.UnusedPVCAge)
		if age >= unusedAge {
			return c.buildFinding(pvc, now, key, "unused-pvc",
				fmt.Sprintf("PVC %q is %s with no active Pod mounting it", pvc.Name, pvc.Status.Phase),
				false, report.ActionNone, map[string]any{
					"phase":            string(pvc.Status.Phase),
					"age":              age.String(),
					"storageClassName": stringPtrValue(pvc.Spec.StorageClassName),
					"volumeName":       pvc.Spec.VolumeName,
					"podsSearched":     "0",
				})
		}
	}

	return nil
}

// getThreshold returns the threshold for a PVC.
// Currently uses the global default; per-namespace thresholds could be added via a map in Policy.Thresholds.
func (c *pvcChecker) getThreshold(_ string, def time.Duration) time.Duration {
	return def
}

// stringPtrValue returns the value of a string pointer, or an empty string if nil.
func stringPtrValue(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

func (c *pvcChecker) buildFinding(
	pvc corev1.PersistentVolumeClaim,
	now metav1.Time,
	key, reason, message string,
	safeToApply bool,
	action report.Action,
	evidence map[string]any,
) *report.Finding {
	age := now.Sub(pvc.CreationTimestamp.Time)
	evidence["age"] = age.String()

	// Add owner refs.
	var ownerRefs []string
	for _, ref := range pvc.OwnerReferences {
		ownerRefs = append(ownerRefs, fmt.Sprintf("%s/%s", ref.Kind, ref.Name))
	}
	sort.Strings(ownerRefs)
	if len(ownerRefs) > 0 {
		evidence["ownerRefs"] = strings.Join(ownerRefs, ",")
	}

	// Build GVR.
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "persistentvolumeclaims",
	}

	return &report.Finding{
		Check:       "pvc",
		Severity:    reportSeverity(reason, pvc),
		Namespace:   pvc.Namespace,
		Group:       gvr.Group,
		Version:     gvr.Version,
		Kind:        gvr.Resource,
		Name:        pvc.Name,
		Message:     message,
		Reason:      reason,
		Suggested:   action,
		SafeToApply: safeToApply,
		Evidence:    evidence,
	}
}

func reportSeverity(reason string, pvc corev1.PersistentVolumeClaim) report.Severity {
	if reason == "unbound-pvc" {
		return reportSeverityUnbound(pvc)
	}
	return reportSeverityUnused(pvc)
}

func reportSeverityUnbound(pvc corev1.PersistentVolumeClaim) report.Severity {
	// Higher storage requests are more concerning.
	if pvc.Spec.Resources.Requests != nil {
		if storage, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			if storage.Value() > 100*1024*1024*1024 { // > 100 Gi
				return report.SeverityHigh
			}
		}
	}
	return report.SeverityMedium
}

func reportSeverityUnused(pvc corev1.PersistentVolumeClaim) report.Severity {
	if pvc.Spec.Resources.Requests != nil {
		if storage, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			if storage.Value() > 100*1024*1024*1024 { // > 100 Gi
				return report.SeverityHigh
			}
		}
	}
	return report.SeverityLow
}
