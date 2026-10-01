package apply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"unicode"

	"github.com/caseyrobb/kubescrub/internal/policy"
	"github.com/caseyrobb/kubescrub/internal/report"
)

const (
	// PlanAPIVersion is the API version of the Plan wrapper.
	PlanAPIVersion = "kubescrub.io/v1"
	// PlanKind is the kind value for the Plan wrapper.
	PlanKind = "Plan"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// PlanResult holds findings after plan loading and filtering.
type PlanResult struct {
	// Candidates are findings that passed SafeToApply + allow-list checks
	// and are eligible for deletion (subject to live-object gates at execution).
	Candidates []report.Finding
	// SkippedReasons documents why findings were filtered out.
	SkippedReasons []SkipReason
}

// SkipReason records why a finding was excluded from the apply plan.
type SkipReason struct {
	FindingID string
	Reason    string
}

// HardBlockKinds lists resource kinds that are never eligible for apply-mode
// deletion, regardless of SafeToApply or SuggestedAction.
var HardBlockKinds = map[string]struct{}{
	"CustomResourceDefinition": {},
	"ClusterRole":              {},
	"ClusterRoleBinding":       {},
	"Role":                     {},
	"RoleBinding":              {},
}

// Plan is the versioned wrapper that serialises all findings for a single
// cluster scan.  It is produced by the scan command and consumed by
// apply --plan.
type Plan struct {
	APIVersion    string            `json:"apiVersion"`
	Kind          string            `json:"kind"`
	PlanVersion   int               `json:"planVersion"`
	Cluster       string            `json:"cluster,omitempty"`
	GeneratedAt   string            `json:"generatedAt"`
	PolicyPath    string            `json:"policyPath,omitempty"`
	KubeScrubVer  string            `json:"kubescrubVersion,omitempty"`
	Findings      []report.Finding  `json:"findings"`
}

// NewPlan creates a fresh Plan wrapper ready for findings to be appended.
func NewPlan(cluster, generatedAt string) *Plan {
	return &Plan{
		APIVersion:   PlanAPIVersion,
		Kind:         PlanKind,
		PlanVersion:  1,
		Cluster:      cluster,
		GeneratedAt:  generatedAt,
		KubeScrubVer: Version,
		Findings:     []report.Finding{},
	}
}

// EncodeTo marshals the Plan wrapper to JSON and writes it to the given path.
// If path is empty the JSON bytes are returned instead.
func (p *Plan) EncodeTo(path string) ([]byte, error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal plan: %w", err)
	}
	if path != "" {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return nil, fmt.Errorf("write plan file %s: %w", path, err)
		}
	}
	return data, nil
}

// DecodeFrom reads JSON from data and populates the Plan.  It validates the
// apiVersion and kind fields.  On unknown apiVersion or wrong kind it
// returns a descriptive error.
func (p *Plan) DecodeFrom(data []byte) error {
	// Check if this is a legacy bare array (starts with '[').
	trimmed := bytes.TrimLeftFunc(data, unicode.IsSpace)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var findings []report.Finding
		if err := json.Unmarshal(data, &findings); err != nil {
			return fmt.Errorf("unmarshal legacy array: %w", err)
		}
		*p = Plan{
			APIVersion:  PlanAPIVersion,
			Kind:        PlanKind,
			PlanVersion: 1,
			Findings:    findings,
		}
		return nil
	}

	// Fast path: check if this looks like a wrapper by probing apiVersion.
	var probe struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("unmarshal plan JSON: %w", err)
	}

	if probe.APIVersion != "" || probe.Kind != "" {
		// We have a wrapper – decode it properly.
		type planAlias Plan // avoid recursion
		var wrapped planAlias
		if err := json.Unmarshal(data, &wrapped); err != nil {
			return fmt.Errorf("unmarshal plan wrapper: %w", err)
		}
		if wrapped.APIVersion != PlanAPIVersion {
			return fmt.Errorf("unsupported apiVersion %q (expected %q)", wrapped.APIVersion, PlanAPIVersion)
		}
		if wrapped.Kind != PlanKind {
			return fmt.Errorf("unsupported kind %q (expected %q)", wrapped.Kind, PlanKind)
		}
		*p = Plan(wrapped)
		return nil
	}

	// Fallback: legacy bare []Finding array.
	var findings []report.Finding
	if err := json.Unmarshal(data, &findings); err != nil {
		return fmt.Errorf("unmarshal plan wrapper or legacy array: %w", err)
	}
	// Wrap into a Plan with defaults.
	*p = Plan{
		APIVersion:  PlanAPIVersion,
		Kind:        PlanKind,
		PlanVersion: 1,
		Findings:    findings,
	}
	return nil
}

// AddFinding appends a finding with a stable, non-empty ID.
// The ID format is check|namespace|kind|name|reason|version (empty
// namespace is allowed).
func (p *Plan) AddFinding(f report.Finding) {
	if f.ID == "" {
		f.ID = findID(f)
	}
	p.Findings = append(p.Findings, f)
}

// findID produces a stable Finding ID from its constituent fields.
func findID(f report.Finding) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s",
		f.Check, f.Namespace, f.Kind, f.Name, f.Reason, f.Version,
	)
}

// ApplyFilters controls post-load filtering of candidates.
type ApplyFilters struct {
	// Checks limits to these check names (empty = all).
	Checks []string
	// Reasons limits to these finding reasons (empty = all).
	Reasons []string
	// Namespace keeps only findings in this namespace AND cluster-scoped
	// findings (empty namespace). If empty, all namespaces are kept.
	Namespace string
}

// FilterCandidates applies post-load filters to a PlanResult.
// Filtered-out findings are added to SkippedReasons with reason "filtered".
// The original plan metadata (GeneratedAt, Cluster) is preserved.
func FilterCandidates(result *PlanResult, plan *Plan, filters ApplyFilters) *PlanResult {
	if len(filters.Checks) == 0 && len(filters.Reasons) == 0 && filters.Namespace == "" {
		return result
	}

	filtered := &PlanResult{
		SkippedReasons: append([]SkipReason{}, result.SkippedReasons...),
	}

	for _, f := range result.Candidates {
		// Check filter.
		if len(filters.Checks) > 0 {
			matched := false
			for _, c := range filters.Checks {
				if f.Check == c {
					matched = true
					break
				}
			}
			if !matched {
				filtered.SkippedReasons = append(filtered.SkippedReasons, SkipReason{
					FindingID: f.ID,
					Reason:    "filtered: check not in --checks",
				})
				continue
			}
		}

		// Reason filter.
		if len(filters.Reasons) > 0 {
			matched := false
			for _, r := range filters.Reasons {
				if f.Reason == r {
					matched = true
					break
				}
			}
			if !matched {
				filtered.SkippedReasons = append(filtered.SkippedReasons, SkipReason{
					FindingID: f.ID,
					Reason:    "filtered: reason not in --reason",
				})
				continue
			}
		}

		// Namespace filter: keep exact match AND cluster-scoped (empty namespace).
		if filters.Namespace != "" {
			if f.Namespace != filters.Namespace && f.Namespace != "" {
				filtered.SkippedReasons = append(filtered.SkippedReasons, SkipReason{
					FindingID: f.ID,
					Reason:    "filtered: namespace not in --namespace",
				})
				continue
			}
		}

		filtered.Candidates = append(filtered.Candidates, f)
	}

	return filtered
}

// Load reads a JSON plan file and filters findings eligible for apply-mode
// deletion. It enforces:
//
//   - SuggestedAction must be "delete"
//   - SafeToApply must be true
//   - Kind must not be in the hard-block list
//
// The RequireAnnotation policy config value is NOT enforced here; that is
// checked against the LIVE object at execution time.
//
// Load accepts both the versioned Plan wrapper (apiVersion/kind) and the
// legacy bare []Finding array for backward compatibility.
// Returns both the Plan (for metadata checks) and the PlanResult (for candidates).
func Load(path string, p policy.Policy) (*Plan, *PlanResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read plan file: %w", err)
	}

	var plan Plan
	if err := plan.DecodeFrom(data); err != nil {
		return nil, nil, fmt.Errorf("decode plan: %w", err)
	}

	result := &PlanResult{}

	for _, f := range plan.Findings {
		// Must suggest deletion.
		if f.Suggested != report.ActionDelete {
			result.SkippedReasons = append(result.SkippedReasons, SkipReason{
				FindingID: f.ID,
				Reason:    "suggestedAction is not delete",
			})
			continue
		}

		// Must be safe to apply.
		if !f.SafeToApply {
			result.SkippedReasons = append(result.SkippedReasons, SkipReason{
				FindingID: f.ID,
				Reason:    "safeToApply is false",
			})
			continue
		}

		// Hard-block sensitive kinds.
		if _, blocked := HardBlockKinds[f.Kind]; blocked {
			result.SkippedReasons = append(result.SkippedReasons, SkipReason{
				FindingID: f.ID,
				Reason:    fmt.Sprintf("kind %q is hard-blocked", f.Kind),
			})
			continue
		}

		result.Candidates = append(result.Candidates, f)
	}

	return &plan, result, nil
}
