package rbac

import (
	"context"
	"fmt"
	"sort"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/caseyrobb/kubescrub/internal/checks"
	"github.com/caseyrobb/kubescrub/internal/report"
)

type rbacChecker struct{}

func New() checks.Check                          { return &rbacChecker{} }
func (c *rbacChecker) Name() string              { return "rbac" }
func (c *rbacChecker) Description() string       { return "Checks for overly permissive RBAC configurations" }

func (c *rbacChecker) Run(ctx context.Context, rt checks.Runtime) ([]report.Finding, error) {
	var findings []report.Finding
	cfg := rt.Policy.RBAC

	// ── helpers ────────────────────────────────────────────────────────
	excludedNs := map[string]struct{}{}
	for _, ns := range rt.Policy.ExcludeNamespaces {
		excludedNs[ns] = struct{}{}
	}

	findingKey := func(kind, ns, name, reason string) string {
		return fmt.Sprintf("%s|%s|%s|%s", kind, ns, name, reason)
	}

	// ── list all RBAC / SA objects (paginated) ────────────────────────
	roleList, err := rt.Client.RbacV1().Roles("").List(ctx, paginate(""))
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	clusterRoleList, err := rt.Client.RbacV1().ClusterRoles().List(ctx, paginate(""))
	if err != nil {
		return nil, fmt.Errorf("list cluster roles: %w", err)
	}
	roleBindingList, err := rt.Client.RbacV1().RoleBindings("").List(ctx, paginate(""))
	if err != nil {
		return nil, fmt.Errorf("list role bindings: %w", err)
	}
	crbList, err := rt.Client.RbacV1().ClusterRoleBindings().List(ctx, paginate(""))
	if err != nil {
		return nil, fmt.Errorf("list cluster role bindings: %w", err)
	}
	saList, err := rt.Client.CoreV1().ServiceAccounts("").List(ctx, paginate(""))
	if err != nil {
		return nil, fmt.Errorf("list service accounts: %w", err)
	}

	// ── indexes ────────────────────────────────────────────────────────
	// SA set: key = namespace/name
	saSet := make(map[string]struct{}, len(saList.Items))
	for _, sa := range saList.Items {
		saSet[sa.Namespace+"/"+sa.Name] = struct{}{}
	}

	// Bindings per binding key: kind|ns/name → []rbacv1.Subject
	type bindingRef struct {
		roleRefName string
		namespace   string // empty for CRoleBinding
		subjects    []rbacv1.Subject
		kind        string // "RoleBinding" or "ClusterRoleBinding"
	}
	bindingRefs := make([]bindingRef, 0, len(roleBindingList.Items)+len(crbList.Items))
	refRoleToBinding := make(map[string]map[string]struct{}) // roleName → set of bindingKeys
	refClusterRoleToBinding := make(map[string]map[string]struct{})

	for _, rb := range roleBindingList.Items {
		if _, excluded := excludedNs[rb.Namespace]; excluded {
			continue
		}
		bk := "RoleBinding|" + rb.Namespace + "/" + rb.Name
		bindingRefs = append(bindingRefs, bindingRef{rb.RoleRef.Name, rb.Namespace, rb.Subjects, "RoleBinding"})
		if refSet, ok := refRoleToBinding[rb.RoleRef.Name]; ok {
			refSet[bk] = struct{}{}
		} else {
			refRoleToBinding[rb.RoleRef.Name] = map[string]struct{}{bk: {}}
		}
	}

	for _, crb := range crbList.Items {
		bk := "ClusterRoleBinding|/" + crb.Name
		bindingRefs = append(bindingRefs, bindingRef{crb.RoleRef.Name, "", crb.Subjects, "ClusterRoleBinding"})
		if refSet, ok := refClusterRoleToBinding[crb.RoleRef.Name]; ok {
			refSet[bk] = struct{}{}
		} else {
			refClusterRoleToBinding[crb.RoleRef.Name] = map[string]struct{}{bk: {}}
		}
	}

	// Build role name → namespace map
	roleNameToNs := make(map[string]string, len(roleList.Items))
	for _, r := range roleList.Items {
		roleNameToNs[r.Name] = r.Namespace
	}

	// ── helper: add if not excluded ────────────────────────────────────
	addFinding := func(kind, ns, name, reason, msg, risk string) {
		key := findingKey(kind, ns, name, reason)
		for _, f := range findings {
			if findingKey(f.Kind, f.Namespace, f.Name, f.Reason) == key {
				return
			}
		}
		findings = append(findings, report.Finding{
			Check:       c.Name(),
			Severity:    severityFor(reason),
			Cluster:     rt.Cluster,
			Namespace:   ns,
			Kind:        kind,
			Name:        name,
			Message:     msg,
			Reason:      reason,
			Suggested:   report.ActionReport,
			SafeToApply: false,
			Risk:        risk,
		})
	}

	// ── 1. unused Roles ────────────────────────────────────────────────
	for _, r := range roleList.Items {
		if _, excluded := excludedNs[r.Namespace]; excluded {
			continue
		}
		if _, referenced := refRoleToBinding[r.Name]; !referenced {
			addFinding("Role", r.Namespace, r.Name, "rbac-unused-role",
				fmt.Sprintf("Role %q in namespace %q is not referenced by any RoleBinding", r.Name, r.Namespace),
				"Unused roles accumulate over time and increase the attack surface")
		}
	}

	// ── 2. unused ClusterRoles ─────────────────────────────────────────
	for _, cr := range clusterRoleList.Items {
		if isBuiltinClusterRole(cr.Name) {
			continue
		}
		if _, referenced := refClusterRoleToBinding[cr.Name]; !referenced {
			addFinding("ClusterRole", "", cr.Name, "rbac-unused-clusterrole",
				fmt.Sprintf("ClusterRole %q is not referenced by any ClusterRoleBinding", cr.Name),
				"Unused cluster roles accumulate over time and increase the attack surface")
		}
	}

	// ── 3. dangling ServiceAccount subjects ────────────────────────────
	// Build ignore set from policy.
	ignoreSet := make(map[string]struct{}, len(cfg.IgnoreSubjects))
	for _, s := range cfg.IgnoreSubjects {
		ignoreSet[s] = struct{}{}
	}
	for _, br := range bindingRefs {
		for _, subj := range br.subjects {
			if subj.Kind != "ServiceAccount" {
				continue
			}
			if subj.Namespace == "" {
				continue
			}
			if _, excluded := excludedNs[subj.Namespace]; excluded {
				continue
			}
			key := subj.Namespace + "/" + subj.Name
			if _, ignored := ignoreSet[key]; ignored {
				continue
			}
			if _, exists := saSet[key]; !exists {
				addFinding("ServiceAccount", subj.Namespace, subj.Name, "rbac-dangling-subject",
					fmt.Sprintf("ServiceAccount %q in namespace %q is referenced by %s %q but does not exist",
						subj.Name, subj.Namespace, br.kind, br.roleRefName),
					"Bindings to non-existent subjects are dangling and waste audit time")
			}
		}
	}

	// ── 4. wildcard permissions ────────────────────────────────────────
	if cfg.FlagWildcards {
		for _, r := range roleList.Items {
			for _, rule := range r.Rules {
				checkWildcard(rule, r.Namespace, "Role", r.Name, func(risk string) {
					addFinding("Role", r.Namespace, r.Name, "rbac-wildcard",
						fmt.Sprintf("Role %q has wildcard permissions (apiGroups/resources=%v verbs=%v)", r.Name, rule.APIGroups, rule.Resources),
						risk)
				})
			}
		}
		for _, cr := range clusterRoleList.Items {
			if isBuiltinClusterRole(cr.Name) {
				continue
			}
			for _, rule := range cr.Rules {
				checkWildcard(rule, "", "ClusterRole", cr.Name, func(risk string) {
					addFinding("ClusterRole", "", cr.Name, "rbac-wildcard",
						fmt.Sprintf("ClusterRole %q has wildcard permissions (apiGroups/resources=%v verbs=%v)", cr.Name, rule.APIGroups, rule.Resources),
						risk)
				})
			}
		}
	}

	// ── 5. privilege escalation verbs ──────────────────────────────────
	for _, r := range roleList.Items {
		for _, rule := range r.Rules {
			for _, verb := range rule.Verbs {
				if isEscalationVerb(verb) {
					addFinding("Role", r.Namespace, r.Name, "rbac-privilege-verb",
						fmt.Sprintf("Role %q grants escalation verb %q on resources %v", r.Name, verb, rule.Resources),
						fmt.Sprintf("Verb %q allows privilege escalation and must be audited", verb))
					break // one finding per rule
				}
			}
		}
	}
	for _, cr := range clusterRoleList.Items {
		if isBuiltinClusterRole(cr.Name) {
			continue
		}
		for _, rule := range cr.Rules {
			for _, verb := range rule.Verbs {
				if isEscalationVerb(verb) {
					addFinding("ClusterRole", "", cr.Name, "rbac-privilege-verb",
						fmt.Sprintf("ClusterRole %q grants escalation verb %q on resources %v", cr.Name, verb, rule.Resources),
						fmt.Sprintf("Verb %q allows privilege escalation and must be audited", verb))
					break
				}
			}
		}
	}

	// ── 6. cluster-admin bindings ──────────────────────────────────────
	if cfg.FlagClusterAdmin {
		for _, crb := range crbList.Items {
			if crb.RoleRef.Name != "cluster-admin" {
				continue
			}
			for _, subj := range crb.Subjects {
				if subj.Kind == "Group" {
					addFinding("ClusterRoleBinding", "", crb.Name, "rbac-cluster-admin",
						fmt.Sprintf("ClusterRoleBinding %q grants cluster-admin to group %q", crb.Name, subj.Name),
						fmt.Sprintf("Binding cluster-admin to group %q grants full cluster control", subj.Name))
				} else {
					// User or SA
					ns := subj.Namespace
					if subj.Kind == "ServiceAccount" {
						addFinding("ClusterRoleBinding", ns, crb.Name, "rbac-cluster-admin",
							fmt.Sprintf("ClusterRoleBinding %q grants cluster-admin to %s %q in namespace %q", crb.Name, subj.Kind, subj.Name, ns),
							fmt.Sprintf("Binding cluster-admin to %s %q grants full cluster control", subj.Kind, subj.Name))
					} else {
						addFinding("ClusterRoleBinding", "", crb.Name, "rbac-cluster-admin",
							fmt.Sprintf("ClusterRoleBinding %q grants cluster-admin to %s %q", crb.Name, subj.Kind, subj.Name),
							fmt.Sprintf("Binding cluster-admin to %s %q grants full cluster control", subj.Kind, subj.Name))
					}
				}
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return severityOrder(findings[i].Severity) < severityOrder(findings[j].Severity)
		}
		return findings[i].Name < findings[j].Name
	})

	return findings, nil
}

// ── helpers ───────────────────────────────────────────────────────────

// paginate returns a ListOptions with Limit=500 so that List can be called
// repeatedly with Continue to walk all pages.  The first call passes token "".
func paginate(continueToken string) metav1.ListOptions {
	return metav1.ListOptions{Limit: 500, Continue: continueToken}
}

// isBuiltinClusterRole returns true for well-known built-in roles that should
// never be flagged as "unused" even if nothing references them.
func isBuiltinClusterRole(name string) bool {
	switch name {
	case "cluster-admin", "admin", "edit", "view":
		return true
	}
	if len(name) >= len("system:") && name[:7] == "system:" {
		return true
	}
	// Common controller-manager roles
	switch name {
	case "system:controller:node-controller", "system:controller:deployment-controller",
		"system:controller:replicaset-controller", "system:controller:replication-controller",
		"system:controller:service-account-controller", "system:controller:token-cleaner",
		"system:controller:endpoint-controller", "system:controller:route-controller",
		"system:controller:certificate-signing-request":
		return true
	}
	return false
}

// escalationVerbs are verbs that can increase a user's permissions.
var escalationVerbs = map[string]struct{}{
	"escalate": {}, "bind": {}, "impersonate": {},
}

func isEscalationVerb(verb string) bool {
	_, ok := escalationVerbs[verb]
	return ok
}

func checkWildcard(rule rbacv1.PolicyRule, kind, ns, name string, cb func(risk string)) {
	for _, apiGroup := range rule.APIGroups {
		if apiGroup == "*" {
			cb("Wildcard API group grants access to all current and future API groups")
			return
		}
	}
	for _, res := range rule.Resources {
		if res == "*" {
			cb("Wildcard resource grants access to all current and future resources")
			return
		}
	}
}

func severityFor(reason string) report.Severity {
	switch reason {
	case "rbac-dangling-subject":
		return report.SeverityMedium
	case "rbac-wildcard", "rbac-privilege-verb", "rbac-cluster-admin":
		return report.SeverityHigh
	default:
		return report.SeverityLow
	}
}

func severityOrder(s report.Severity) int {
	switch s {
	case report.SeverityHigh:
		return 0
	case report.SeverityMedium:
		return 1
	case report.SeverityLow:
		return 2
	default:
		return 3
	}
}
