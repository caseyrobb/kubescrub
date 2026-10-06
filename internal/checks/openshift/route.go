package openshift

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"

	"github.com/caseyrobb/kubescrub/internal/checks"
	"github.com/caseyrobb/kubescrub/internal/report"
)

const routeGroup = "route.openshift.io"
const routeVersion = "v1"
const routeKind = "Route"

type route struct{}

// NewRoute creates a checker that flags routes pointing to missing or misconfigured Services.
func NewRoute() checks.Check { return &route{} }

func (c *route) Name() string        { return "route" }
func (c *route) Description() string { return "Checks OpenShift Routes for missing or misconfigured backends" }

func (c *route) Run(ctx context.Context, rt checks.Runtime) ([]report.Finding, error) {
	// Skip if route.openshift.io is not available.
	if !isAPIAvailable(rt.Discovery, routeGroup, routeVersion) {
		return nil, nil
	}

	var findings []report.Finding

	// Discover the Routes GVR.
	routeGVR, err := discoverGVR(rt.Discovery, routeGroup, routeVersion, "routes")
	if err != nil {
		return nil, nil
	}

	// List Routes via dynamic client.
	routeList, err := rt.Dynamic.Resource(routeGVR).List(ctx, metav1.ListOptions{})
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

	// Collect Services and EndpointSlices per namespace.
	services, err := c.listServices(ctx, rt, excludedNs)
	if err != nil {
		return nil, err
	}
	endpoints, err := c.listEndpointSlices(ctx, rt, excludedNs)
	if err != nil {
		return nil, err
	}

	// Evaluate each Route.
	for _, item := range routeList.Items {
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

		spec, ok := item.Object["spec"].(map[string]interface{})
		if !ok {
			continue
		}

		// Get the target Service name and kind.
		toKind, _ := spec["to"].(map[string]interface{})
		toKindValue := getStringVal(toKind, "kind")
		toName := getStringVal(toKind, "name")

		// Default to "Service" if kind is empty.
		if toKindValue == "" {
			toKindValue = "Service"
		}

		// Only flag routes that target a Service.
		if toKindValue != "Service" {
			continue
		}

		// Check if the Service exists.
		if toName != "" && !serviceExists(services, ns, toName) {
			findings = append(findings, report.Finding{
				ID:          fmt.Sprintf("route-missing-service-%s-%s", ns, name),
				Check:       "route",
				Severity:    report.SeverityHigh,
				Cluster:     rt.Cluster,
				Namespace:   ns,
				Group:       routeGroup,
				Version:     routeVersion,
				Kind:        routeKind,
				Name:        name,
				Message:     fmt.Sprintf("Route %s/%s targets Service %q which does not exist", ns, name, toName),
				Reason:      "route-missing-service",
				Suggested:   report.ActionReport,
				SafeToApply: false,
				Evidence: map[string]any{
					"targetService": toName,
				},
			})
			continue
		}

		// Check target port mismatch.
		port, hasPort := spec["port"].(map[string]interface{})
		if hasPort {
			targetPort := getStringVal(port, "targetPort")
			if targetPort != "" && toName != "" {
				if !servicePortExists(services, ns, toName, targetPort) {
					findings = append(findings, report.Finding{
						ID:          fmt.Sprintf("route-port-mismatch-%s-%s", ns, name),
						Check:       "route",
						Severity:    report.SeverityMedium,
						Cluster:     rt.Cluster,
						Namespace:   ns,
						Group:       routeGroup,
						Version:     routeVersion,
						Kind:        routeKind,
						Name:        name,
						Message:     fmt.Sprintf("Route %s/%s targetPort %q is not a port on Service %s/%s", ns, name, targetPort, ns, toName),
						Reason:      "route-port-mismatch",
						Suggested:   report.ActionReport,
						SafeToApply: false,
						Evidence: map[string]any{
							"targetPort":    targetPort,
							"targetService": toName,
						},
					})
				}
			}
		}

		// Check for zero ready endpoints.
		if toName != "" {
			epKey := ns + "/" + toName
			if ep, ok := endpoints[epKey]; ok && len(ep) == 0 {
				findings = append(findings, report.Finding{
					ID:          fmt.Sprintf("route-no-endpoints-%s-%s", ns, name),
					Check:       "route",
					Severity:    report.SeverityMedium,
					Cluster:     rt.Cluster,
					Namespace:   ns,
					Group:       routeGroup,
					Version:     routeVersion,
					Kind:        routeKind,
					Name:        name,
					Message:     fmt.Sprintf("Route %s/%s targets Service %s/%s which has no ready endpoints", ns, name, ns, toName),
					Reason:      "route-no-endpoints",
					Suggested:   report.ActionReport,
					SafeToApply: false,
					Evidence: map[string]any{
						"targetService": toName,
					},
				})
			}
		}
	}

	return findings, nil
}

// listServices collects Services keyed by "namespace/name", returning ports per service.
// Returns: ns/name -> list of port names and numbers.
func (c *route) listServices(ctx context.Context, rt checks.Runtime, excludedNs map[string]struct{}) (map[string][]string, error) {
	if rt.Client == nil {
		return nil, nil
	}
	result := make(map[string][]string)

	var namespaces []string
	if len(rt.Namespaces) > 0 {
		namespaces = rt.Namespaces
	} else {
		nsList, err := rt.Client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, nil
		}
		for _, ns := range nsList.Items {
			if _, excluded := excludedNs[ns.Name]; !excluded {
				namespaces = append(namespaces, ns.Name)
			}
		}
	}

	for _, ns := range namespaces {
		svcList, err := rt.Client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for _, svc := range svcList.Items {
			key := ns + "/" + svc.Name
			var ports []string
			for _, p := range svc.Spec.Ports {
				if p.Name != "" {
					ports = append(ports, p.Name)
				}
				ports = append(ports, fmt.Sprintf("%d", p.Port))
			}
			result[key] = ports
		}
	}

	return result, nil
}

// listEndpointSlices collects ready endpoint addresses per service key "namespace/serviceName".
func (c *route) listEndpointSlices(ctx context.Context, rt checks.Runtime, excludedNs map[string]struct{}) (map[string][]string, error) {
	if rt.Client == nil {
		return nil, nil
	}
	result := make(map[string][]string)

	var namespaces []string
	if len(rt.Namespaces) > 0 {
		namespaces = rt.Namespaces
	} else {
		nsList, err := rt.Client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, nil
		}
		for _, ns := range nsList.Items {
			if _, excluded := excludedNs[ns.Name]; !excluded {
				namespaces = append(namespaces, ns.Name)
			}
		}
	}

	for _, ns := range namespaces {
		slList, err := rt.Client.DiscoveryV1().EndpointSlices(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for _, sl := range slList.Items {
			svcKey := sl.Labels["kubernetes.io/service-name"]
			if svcKey == "" {
				continue
			}
			key := ns + "/" + svcKey
			if _, ok := result[key]; !ok {
				result[key] = nil
			}
			for _, ep := range sl.Endpoints {
				if ep.Conditions.Ready != nil && *ep.Conditions.Ready {
					for _, addr := range ep.Addresses {
						result[key] = append(result[key], addr)
					}
				}
			}
		}
	}

	return result, nil
}

func serviceExists(services map[string][]string, ns, name string) bool {
	key := ns + "/" + name
	_, exists := services[key]
	return exists
}

func servicePortExists(services map[string][]string, ns, name, targetPort string) bool {
	key := ns + "/" + name
	ports, ok := services[key]
	if !ok {
		return false
	}
	for _, p := range ports {
		if p == targetPort {
			return true
		}
	}
	return false
}

// discoverGVR returns the GVR for a known resource.
func discoverGVR(_ discovery.DiscoveryInterface, group, version, resource string) (schema.GroupVersionResource, error) {
	return schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}, nil
}

// isAPIAvailable checks if a group/version is served by the API server.
func isAPIAvailable(discovery discovery.DiscoveryInterface, group, version string) bool {
	serverGroups, err := discovery.ServerGroups()
	if err != nil {
		return false
	}
	for _, groupEntry := range serverGroups.Groups {
		if groupEntry.Name == group {
			for _, v := range groupEntry.Versions {
				if v.GroupVersion == group+"/"+version {
					return true
				}
			}
		}
	}
	return false
}

// getStringVal extracts a string value from a map.
func getStringVal(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// splitAtSlash splits a string on the first '/' character.
func splitAtSlash(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}
