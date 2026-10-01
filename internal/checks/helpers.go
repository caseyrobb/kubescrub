package checks

import (
	"strings"

	"github.com/caseyrobb/kubescrub/internal/report"
)

// SystemNamespaces returns a set of Kubernetes system namespaces
// that should be excluded from most hygiene checks by default.
func SystemNamespaces() []string {
	return []string{
		"kube-system",
		"kube-public",
		"kube-node-lease",
	}
}

// ReportSeverity maps a reason string to a FindingSeverity.
func ReportSeverity(reason string) report.Severity {
	switch {
	case strings.Contains(reason, "critical"),
		strings.Contains(reason, "deny"),
		strings.Contains(reason, "forbidden"):
		return report.SeverityCritical
	case strings.Contains(reason, "high"),
		strings.Contains(reason, "excessive"),
		strings.Contains(reason, "wide"):
		return report.SeverityHigh
	case strings.Contains(reason, "medium"),
		strings.Contains(reason, "deprecated"),
		strings.Contains(reason, "unused"):
		return report.SeverityMedium
	case strings.Contains(reason, "low"),
		strings.Contains(reason, "minor"),
		strings.Contains(reason, "warning"):
		return report.SeverityLow
	default:
		return report.SeverityInfo
	}
}
