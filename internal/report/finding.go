package report

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type Action string

const (
	ActionNone   Action = "none"
	ActionReport Action = "report"
	ActionPatch  Action = "patch"
	ActionDelete Action = "delete"
)

type Finding struct {
	ID          string            `json:"id"`
	Check       string            `json:"check"`
	Severity    Severity          `json:"severity"`
	Cluster     string            `json:"cluster"`
	Namespace   string            `json:"namespace,omitempty"`
	Group       string            `json:"group,omitempty"`
	Version     string            `json:"version"`
	Kind        string            `json:"kind"`
	Name        string            `json:"name"`
	Message     string            `json:"message"`
	Reason      string            `json:"reason"`
	Suggested   Action            `json:"suggestedAction"`
	Replacement string            `json:"replacement,omitempty"`
	Age         string            `json:"age,omitempty"`
	Risk        string            `json:"risk"`
	Labels      map[string]string `json:"labels,omitempty"`
	OwnerRefs   []string          `json:"ownerRefs,omitempty"`
	Evidence    map[string]any    `json:"evidence,omitempty"`
	SafeToApply bool              `json:"safeToApply"`
}
