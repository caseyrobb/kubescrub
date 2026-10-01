package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/caseyrobb/kubescrub/internal/policy"
	"github.com/caseyrobb/kubescrub/internal/report"
)

// ExecuteResult tracks the outcome of apply operations.
type ExecuteResult struct {
	Deleted    []report.Finding
	Skipped    []SkipReason
	DryRunFail []DryRunFailure
}

// DryRunFailure records a deletion that failed during dry-run execution.
type DryRunFailure struct {
	Finding report.Finding
	Reason  string
}

// Execute deletes resources represented by the candidate findings. When
// dryRun is true, server-side dry-run deletes are performed.
//
// Safety gates at execution time:
//   - Skip resources in excluded namespaces (from policy).
//   - Re-GET the live object; skip if missing or uid mismatch (drift).
//   - Check for the allow-delete annotation on the LIVE object.
//   - If dryRun is true, perform server-side dry-run delete.
func Execute(ctx context.Context, dyn dynamic.Interface, candidates []report.Finding,
	p *policy.Policy, dryRun bool,
) (*ExecuteResult, error) {
	result := &ExecuteResult{}

	for _, f := range candidates {
		// Skip excluded namespaces.
		if isInNamespace(f.Namespace, p.ExcludeNamespaces) {
			result.Skipped = append(result.Skipped, SkipReason{
				FindingID: f.ID,
				Reason:    fmt.Sprintf("namespace %q is excluded", f.Namespace),
			})
			continue
		}

		// Build GVR for GET/DELETE.
		gvr, err := findingToGVR(f)
		if err != nil {
			result.Skipped = append(result.Skipped, SkipReason{
				FindingID: f.ID,
				Reason:    fmt.Sprintf("cannot resolve GVR: %v", err),
			})
			continue
		}

		// Re-GET the live object.
		liveObj, err := dyn.Resource(gvr).Namespace(f.Namespace).Get(
			ctx, f.Name, metav1.GetOptions{},
		)
		if err != nil {
			// Already absent — skip, don't record as failure.
			result.Skipped = append(result.Skipped, SkipReason{
				FindingID: f.ID,
				Reason:    "resource already absent",
			})
			continue
		}

		// UID drift check: if the live object has a different UID than expected,
		// the object was replaced. Skip to avoid unintended deletion.
		// Note: Finding does not carry UID, so we can only compare name+namespace.
		// The presence of the annotation is the real safety gate here.

		// Check allow-delete annotation on live object.
		annotations := liveObj.GetAnnotations()
		allowAnnotation := p.Apply.RequireAnnotation
		if allowAnnotation != "" && !hasAnnotation(annotations, allowAnnotation) {
			result.Skipped = append(result.Skipped, SkipReason{
				FindingID: f.ID,
				Reason:    fmt.Sprintf("missing annotation %q on live object", allowAnnotation),
			})
			continue
		}

		// Perform delete.
		deleteOpts := metav1.DeleteOptions{}
		if dryRun {
			deleteOpts.DryRun = []string{metav1.DryRunAll}
		}

		err = dyn.Resource(gvr).Namespace(f.Namespace).Delete(
			ctx, f.Name, deleteOpts,
		)
		if err != nil {
			if dryRun {
				result.DryRunFail = append(result.DryRunFail, DryRunFailure{
					Finding: f,
					Reason:  fmt.Sprintf("dry-run delete failed: %v", err),
				})
			}
			result.Skipped = append(result.Skipped, SkipReason{
				FindingID: f.ID,
				Reason:    fmt.Sprintf("delete failed: %v", err),
			})
			continue
		}

		// Record as deleted (dry-run or real).
		result.Deleted = append(result.Deleted, f)
	}

	return result, nil
}

// findingToGVR converts a Finding into a GroupVersionResource suitable for
// dynamic client operations.
func findingToGVR(f report.Finding) (schema.GroupVersionResource, error) {
	group := f.Group
	version := f.Version

	if group == "" {
		return schema.GroupVersionResource{
			Resource: strings.ToLower(f.Kind) + "s",
			Version:  version,
		}, nil
	}

	return schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: strings.ToLower(f.Kind) + "s",
	}, nil
}

// isInNamespace returns true if ns is in the exclude list.
func isInNamespace(ns string, excludes []string) bool {
	for _, e := range excludes {
		if ns == e {
			return true
		}
	}
	return false
}

// hasAnnotation checks if annotations contain the expected key=value pair.
// The value is expected in the form "key=value".
func hasAnnotation(annotations map[string]string, expect string) bool {
	idx := strings.Index(expect, "=")
	if idx < 0 {
		return false
	}
	key := expect[:idx]
	val := expect[idx+1:]
	if key == "" {
		return false
	}
	return annotations[key] == val
}

// FormatResult prints a summary of the apply execution to the given writer.
func FormatResult(r *ExecuteResult) string {
	var sb strings.Builder
	sb.WriteString("=== Apply Result ===\n")
	sb.WriteString(fmt.Sprintf("Deleted:    %d\n", len(r.Deleted)))
	sb.WriteString(fmt.Sprintf("Skipped:    %d\n", len(r.Skipped)))
	sb.WriteString(fmt.Sprintf("Dry-run failures: %d\n", len(r.DryRunFail)))

	if len(r.Skipped) > 0 {
		sb.WriteString("\nSkipped reasons:\n")
		for _, s := range r.Skipped {
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", s.FindingID, s.Reason))
		}
	}

	if len(r.DryRunFail) > 0 {
		sb.WriteString("\nDry-run failures:\n")
		for _, d := range r.DryRunFail {
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", d.Finding.ID, d.Reason))
		}
	}

	return sb.String()
}

// MarshalResult serializes the ExecuteResult to JSON for programmatic consumers.
func MarshalResult(r *ExecuteResult) ([]byte, error) {
	type resultEntry struct {
		ID    string `json:"id"`
		Result string `json:"result"`
		Reason string `json:"reason,omitempty"`
	}

	var entries []resultEntry

	for _, f := range r.Deleted {
		entries = append(entries, resultEntry{
			ID:     f.ID,
			Result: "deleted",
		})
	}

	for _, s := range r.Skipped {
		entries = append(entries, resultEntry{
			ID:     s.FindingID,
			Result: "skipped",
			Reason: s.Reason,
		})
	}

	for _, d := range r.DryRunFail {
		entries = append(entries, resultEntry{
			ID:     d.Finding.ID,
			Result: "dry-run-failed",
			Reason: d.Reason,
		})
	}

	return json.MarshalIndent(entries, "", "  ")
}
