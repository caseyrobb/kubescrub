package checks

import (
	"testing"

	"github.com/caseyrobb/kubescrub/internal/report"
)

func TestSystemNamespaces(t *testing.T) {
	sns := SystemNamespaces()
	if len(sns) != 3 {
		t.Fatalf("expected 3 system namespaces, got %d", len(sns))
	}
	expected := map[string]bool{
		"kube-system":      true,
		"kube-public":      true,
		"kube-node-lease":  true,
	}
	for _, ns := range sns {
		if !expected[ns] {
			t.Errorf("unexpected system namespace %q", ns)
		}
		delete(expected, ns)
	}
	if len(expected) != 0 {
		t.Errorf("missing system namespaces: %v", expected)
	}
}

func TestReportSeverityCritical(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   report.Severity
	}{
		{"critical finding", "critical issue", report.SeverityCritical},
		{"deny access", "deny access to resource", report.SeverityCritical},
		{"forbidden", "operation is forbidden", report.SeverityCritical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReportSeverity(tt.reason)
			if got != tt.want {
				t.Errorf("ReportSeverity(%q) = %q, want %q", tt.reason, got, tt.want)
			}
		})
	}
}

func TestReportSeverityHigh(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   report.Severity
	}{
		{"high severity", "high risk operation", report.SeverityHigh},
		{"excessive permissions", "excessive RBAC permissions", report.SeverityHigh},
		{"wide permissions", "wide permissions granted", report.SeverityHigh},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReportSeverity(tt.reason)
			if got != tt.want {
				t.Errorf("ReportSeverity(%q) = %q, want %q", tt.reason, got, tt.want)
			}
		})
	}
}

func TestReportSeverityMedium(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   report.Severity
	}{
		{"medium issue", "medium concern", report.SeverityMedium},
		{"deprecated API", "deprecated API version", report.SeverityMedium},
		{"unused resource", "unused PVC", report.SeverityMedium},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReportSeverity(tt.reason)
			if got != tt.want {
				t.Errorf("ReportSeverity(%q) = %q, want %q", tt.reason, got, tt.want)
			}
		})
	}
}

func TestReportSeverityLow(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   report.Severity
	}{
		{"low concern", "low priority issue", report.SeverityLow},
		{"minor warning", "minor cleanup needed", report.SeverityLow},
		{"warning", "warning about config", report.SeverityLow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReportSeverity(tt.reason)
			if got != tt.want {
				t.Errorf("ReportSeverity(%q) = %q, want %q", tt.reason, got, tt.want)
			}
		})
	}
}

func TestReportSeverityDefault(t *testing.T) {
	got := ReportSeverity("unknown reason")
	if got != report.SeverityInfo {
		t.Errorf("ReportSeverity(%q) = %q, want %q", "unknown", got, report.SeverityInfo)
	}
}
