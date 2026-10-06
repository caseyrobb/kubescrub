package openshift

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/caseyrobb/kubescrub/internal/checks"
	"github.com/caseyrobb/kubescrub/internal/report"
)

const csvGroup = "operators.coreos.com"
const csvVersion = "v1alpha1"
const csvKind = "ClusterServiceVersion"

type csv struct{}

// NewCSV creates a checker for ClusterServiceVersion lifecycle.
func NewCSV() checks.Check { return &csv{} }

func (c *csv) Name() string        { return "csv" }
func (c *csv) Description() string { return "Checks OpenShift ClusterServiceVersions for stuck or outdated operators" }

func (c *csv) Run(ctx context.Context, rt checks.Runtime) ([]report.Finding, error) {
	// Skip if operators.coreos.com is not available.
	if !isAPIAvailable(rt.Discovery, csvGroup, csvVersion) {
		return nil, nil
	}

	var findings []report.Finding

	// Discover GVRs.
	csvGVR, err := discoverGVR(rt.Discovery, csvGroup, csvVersion, "clusterserviceversions")
	if err != nil {
		return nil, nil
	}
	subGVR, err := discoverGVR(rt.Discovery, csvGroup, csvVersion, "subscriptions")
	if err != nil {
		return nil, nil
	}

	// Build excluded namespace set.
	excludedNs := make(map[string]struct{})
	for _, ns := range rt.Policy.ExcludeNamespaces {
		excludedNs[ns] = struct{}{}
	}
	for _, ns := range checks.SystemNamespaces() {
		excludedNs[ns] = struct{}{}
	}

	// List Subscriptions to find current CSVs.
	subList, err := rt.Dynamic.Resource(subGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil
	}

	// Build set of "namespace/CSV name" that are current.
	currentCSVs := make(map[string]struct{})
	for _, item := range subList.Items {
		ns := item.GetNamespace()

		// Skip excluded namespaces.
		if _, ok := excludedNs[ns]; ok {
			continue
		}

		// Apply namespace filter.
		if len(rt.Namespaces) > 0 {
			found := false
			for _, filterNS := range rt.Namespaces {
				if ns == filterNS {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		status, ok := item.Object["status"].(map[string]interface{})
		if !ok {
			continue
		}
		currentCSV, ok := status["currentCSV"].(string)
		if !ok || currentCSV == "" {
			continue
		}

		key := ns + "/" + currentCSV
		currentCSVs[key] = struct{}{}
	}

	// List ClusterServiceVersions.
	csvList, err := rt.Dynamic.Resource(csvGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil
	}

	// Policy threshold for stuck CSVs.
	stuckThreshold := 1 * time.Hour // default

	// Evaluate each CSV.
	now := time.Now()
	for _, item := range csvList.Items {
		ns := item.GetNamespace()
		name := item.GetName()

		// Skip excluded namespaces.
		if _, ok := excludedNs[ns]; ok {
			continue
		}

		// Apply namespace filter.
		if len(rt.Namespaces) > 0 {
			found := false
			for _, filterNS := range rt.Namespaces {
				if ns == filterNS {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		key := ns + "/" + name

		// Check phase.
		status, ok := item.Object["status"].(map[string]interface{})
		if !ok {
			continue
		}
		phase, _ := status["phase"].(string)

		// Check creation timestamp for age.
		creationTimestamp := item.GetCreationTimestamp()
		age := now.Sub(creationTimestamp.Time)

		// If CSV is named by a Subscription's currentCSV, it's current — skip.
		if _, isCurrent := currentCSVs[key]; isCurrent {
			// Still check for stuck phase.
			if (phase == "Failed" || phase == "Pending") && age >= stuckThreshold {
				findings = append(findings, report.Finding{
					ID:          fmt.Sprintf("csv-stuck-%s-%s", ns, name),
					Check:       "csv",
					Severity:    report.SeverityHigh,
					Cluster:     rt.Cluster,
					Namespace:   ns,
					Group:       csvGroup,
					Version:     csvVersion,
					Kind:        csvKind,
					Name:        name,
					Message:     fmt.Sprintf("CSV %s/%s is %s for %s", ns, name, phase, age.Round(time.Second)),
					Reason:      "csv-stuck",
					Suggested:   report.ActionReport,
					SafeToApply: false,
					Evidence: map[string]any{
						"phase":     phase,
						"age":       age.String(),
						"isCurrent": true,
					},
				})
			}
			continue
		}

		// CSV is not current — flag for potential pruning.
		findings = append(findings, report.Finding{
			ID:          fmt.Sprintf("csv-not-current-%s-%s", ns, name),
			Check:       "csv",
			Severity:    report.SeverityLow,
			Cluster:     rt.Cluster,
			Namespace:   ns,
			Group:       csvGroup,
			Version:     csvVersion,
			Kind:        csvKind,
			Name:        name,
			Message:     fmt.Sprintf("CSV %s/%s is not the current version for any Subscription", ns, name),
			Reason:      "csv-not-current",
			Suggested:   report.ActionReport,
			SafeToApply: false,
			Evidence:    map[string]any{},
		})

		// Also check for stuck non-current CSVs.
		if (phase == "Failed" || phase == "Pending") && age >= stuckThreshold {
			findings = append(findings, report.Finding{
				ID:          fmt.Sprintf("csv-stuck-%s-%s", ns, name),
				Check:       "csv",
				Severity:    report.SeverityHigh,
				Cluster:     rt.Cluster,
				Namespace:   ns,
				Group:       csvGroup,
				Version:     csvVersion,
				Kind:        csvKind,
				Name:        name,
				Message:     fmt.Sprintf("CSV %s/%s is %s for %s (not current)", ns, name, phase, age.Round(time.Second)),
				Reason:      "csv-stuck",
				Suggested:   report.ActionReport,
				SafeToApply: false,
				Evidence: map[string]any{
					"phase":     phase,
					"age":       age.String(),
					"isCurrent": false,
				},
			})
		}
	}

	return findings, nil
}
