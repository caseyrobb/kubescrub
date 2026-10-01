package policy

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Policy holds cluster-scan configuration loaded from a YAML file.
type Policy struct {
	ExcludeNamespaces []string       `yaml:"excludeNamespaces"`
	Thresholds        Thresholds     `yaml:"thresholds"`
	RBAC              RBACConfig     `yaml:"rbac"`
	Apply             ApplyConfig    `yaml:"apply"`
}

// Thresholds control age-based detection for various resource states.
type Thresholds struct {
	CompletedJobAge     time.Duration `yaml:"completedJobAge"`
	FailedJobAge        time.Duration `yaml:"failedJobAge"`
	PendingPVCAge       time.Duration `yaml:"pendingPVCAge"`
	UnusedPVCAge        time.Duration `yaml:"unusedPVCAge"`
	UnusedReplicaSetAge time.Duration `yaml:"unusedReplicaSetAge"`
}

// RBACConfig controls RBAC-related scan behavior.
type RBACConfig struct {
	FlagWildcards   bool     `yaml:"flagWildcards"`
	FlagClusterAdmin bool    `yaml:"flagClusterAdmin"`
	IgnoreSubjects  []string `yaml:"ignoreSubjects"`
}

// ApplyConfig controls apply-mode behavior.
type ApplyConfig struct {
	RequireAnnotation string `yaml:"requireAnnotation"`
}

// DefaultPolicy returns a Policy with sensible built-in defaults.
func DefaultPolicy() Policy {
	return Policy{
		ExcludeNamespaces: []string{
			"kube-system",
			"kube-public",
			"kube-node-lease",
			"local-path-storage",
		},
		Thresholds: Thresholds{
			CompletedJobAge:     1 * time.Second,
			FailedJobAge:        1 * time.Second,
			PendingPVCAge:       1 * time.Second,
			UnusedPVCAge:        1 * time.Second,
			UnusedReplicaSetAge: 1 * time.Second,
		},
		RBAC: RBACConfig{
			FlagWildcards:    true,
			FlagClusterAdmin: true,
		},
		Apply: ApplyConfig{
			RequireAnnotation: "kubescrub.io/allow-delete=true",
		},
	}
}

// Load reads a Policy from the given file path. Returns an error on invalid YAML.
func Load(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Policy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	// Merge with defaults for any zero-value fields.
	def := DefaultPolicy()
	if len(p.ExcludeNamespaces) == 0 {
		p.ExcludeNamespaces = def.ExcludeNamespaces
	}
	if p.Thresholds.CompletedJobAge == 0 {
		p.Thresholds.CompletedJobAge = def.Thresholds.CompletedJobAge
	}
	if p.Thresholds.FailedJobAge == 0 {
		p.Thresholds.FailedJobAge = def.Thresholds.FailedJobAge
	}
	if p.Thresholds.PendingPVCAge == 0 {
		p.Thresholds.PendingPVCAge = def.Thresholds.PendingPVCAge
	}
	if p.Thresholds.UnusedPVCAge == 0 {
		p.Thresholds.UnusedPVCAge = def.Thresholds.UnusedPVCAge
	}
	if p.Thresholds.UnusedReplicaSetAge == 0 {
		p.Thresholds.UnusedReplicaSetAge = def.Thresholds.UnusedReplicaSetAge
	}
	return &p, nil
}
