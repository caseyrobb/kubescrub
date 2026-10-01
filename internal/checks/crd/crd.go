package crd

import (
	"context"
	"fmt"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/caseyrobb/kubescrub/internal/checks"
	"github.com/caseyrobb/kubescrub/internal/report"
)

type crd struct{}

func New() checks.Check { return &crd{} }

func (c *crd) Name() string                          { return "crd" }
func (c *crd) Description() string                   { return "Checks for deprecated CRD versions and CR objects not on the storage version" }

func (c *crd) Run(ctx context.Context, rt checks.Runtime) ([]report.Finding, error) {
	var findings []report.Finding

	// Build excluded namespace set.
	excludedNs := make(map[string]struct{})
	for _, ns := range rt.Policy.ExcludeNamespaces {
		excludedNs[ns] = struct{}{}
	}
	for _, ns := range checks.SystemNamespaces() {
		excludedNs[ns] = struct{}{}
	}

	// List CRDs via dynamic client (ApiextensionsV1 not available on kubernetes.Interface).
	gvr := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}
	unstructuredList, err := rt.Dynamic.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return findings, nil
	}

	// Convert unstructured CRDs to typed objects.
	var crds []apiextensionsv1.CustomResourceDefinition
	for _, item := range unstructuredList.Items {
		var crd apiextensionsv1.CustomResourceDefinition
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(
			item.UnstructuredContent(), &crd); err != nil {
			continue
		}
		crds = append(crds, crd)
	}

	// Dedup key: check|namespace|group|kind|name|reason|version
	seen := make(map[string]struct{})

	addFinding := func(f report.Finding) {
		key := dedupKey(f)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		findings = append(findings, f)
	}

	// --- Class A: CRD version findings ---
	for _, crd := range crds {
		hasDeprecated := false
		var deprecatedVersions []string

		for _, v := range crd.Spec.Versions {
			if v.Deprecated {
				hasDeprecated = true
				deprecatedVersions = append(deprecatedVersions, v.Name)

				classA := report.Finding{
					Check:       "crd",
					Severity:    reportSeverity(deprecatedVersions),
					Cluster:     rt.Cluster,
					Group:       crd.Spec.Group,
					Version:     v.Name,
					Kind:        "CustomResourceDefinition",
					Name:        crd.Name,
					Message:     fmt.Sprintf("CRD %s has deprecated version %s", crd.Name, v.Name),
					Reason:      "crd-version-deprecated",
					Suggested:   report.ActionReport,
					SafeToApply: false,
					Evidence:    map[string]any{"deprecationWarning": crd.Spec.Versions[0].DeprecationWarning},
				}
				addFinding(classA)
			}
		}

		// Check if storage version is deprecated.
		for _, v := range crd.Spec.Versions {
			if v.Storage && v.Deprecated {
				classAStorage := report.Finding{
					Check:       "crd",
					Severity:    reportSeverity(deprecatedVersions),
					Cluster:     rt.Cluster,
					Group:       crd.Spec.Group,
					Version:     v.Name,
					Kind:        "CustomResourceDefinition",
					Name:        crd.Name,
					Message:     fmt.Sprintf("CRD %s storage version %s is deprecated", crd.Name, v.Name),
					Reason:      "crd-storage-deprecated",
					Suggested:   report.ActionReport,
					SafeToApply: false,
					Evidence:    map[string]any{},
				}
				addFinding(classAStorage)
			}
		}

		// Check deprecated versions that are still served.
		for _, v := range crd.Spec.Versions {
			if v.Deprecated && v.Served {
				classAServed := report.Finding{
					Check:       "crd",
					Severity:    reportSeverity(deprecatedVersions),
					Cluster:     rt.Cluster,
					Group:       crd.Spec.Group,
					Version:     v.Name,
					Kind:        "CustomResourceDefinition",
					Name:        crd.Name,
					Message:     fmt.Sprintf("CRD %s deprecated version %s is still served", crd.Name, v.Name),
					Reason:      "crd-deprecated-still-served",
					Suggested:   report.ActionReport,
					SafeToApply: false,
					Evidence:    map[string]any{},
				}
				addFinding(classAServed)
			}
		}

		// --- Class B: CR objects not on storage version ---
		if !hasDeprecated {
			continue
		}

		// Find the storage version.
		var storageVersion string
		for _, v := range crd.Spec.Versions {
			if v.Storage {
				storageVersion = v.Name
				break
			}
		}
		if storageVersion == "" {
			continue
		}

		plural := crd.Spec.Names.Plural
		kind := crd.Spec.Names.Kind
		group := crd.Spec.Group
		_ = crd.Spec.Scope

		// List on ALL served versions (storage + deprecated served) to find
		// CR instances that exist on deprecated versions too.
		servedVersions := []string{storageVersion}
		for _, v := range crd.Spec.Versions {
			if v.Served && v.Name != storageVersion {
				servedVersions = append(servedVersions, v.Name)
			}
		}

		for _, ver := range servedVersions {
			listGvr := schema.GroupVersionResource{
				Group:    group,
				Version:  ver,
				Resource: plural,
			}

			instances, err := listAllInstances(ctx, rt, rt.Dynamic, listGvr, excludedNs)
			if err != nil {
				continue
			}

			for _, instance := range instances {
				item := instance.(*unstructured.Unstructured)
				apiVersion := item.GetAPIVersion()
				objVersion := parseAPIVersion(apiVersion)
				if objVersion == "" {
					continue
				}
				if objVersion == storageVersion {
					continue
				}

				classB := report.Finding{
					Check:       "crd",
					Severity:    reportSeverity(nil),
					Cluster:     rt.Cluster,
					Namespace:   item.GetNamespace(),
					Group:       group,
					Version:     objVersion,
					Kind:        kind,
					Name:        item.GetName(),
					Message:     fmt.Sprintf("CR %s/%s in group %s uses version %s instead of storage version %s",
						item.GetNamespace(), item.GetName(), group, objVersion, storageVersion),
					Reason:      "cr-not-storage-version",
					Suggested:   report.ActionPatch,
					SafeToApply: false,
					Evidence: map[string]any{
						"apiVersion":     apiVersion,
						"storageVersion": storageVersion,
					},
				}
				addFinding(classB)
			}
		}
	}

	return findings, nil
}

// listAllInstances lists all CR instances for a GVR with pagination, skipping excluded namespaces.
func listAllInstances(ctx context.Context, rt checks.Runtime, dyn dynamic.Interface, gvr schema.GroupVersionResource, excludedNs map[string]struct{}) ([]any, error) {
	var instances []any

	// Determine namespaces to list.
	if len(excludedNs) == 0 {
		// List cluster-wide.
		return listClusterWide(ctx, dyn, gvr, nil)
	}

	// For namespaced resources, list in each non-excluded namespace.
	var namespaces []string
	if len(rt.Namespaces) > 0 {
		namespaces = rt.Namespaces
	} else {
		// We can't list all namespaces without more info.
		// Just try listing cluster-wide.
		return listClusterWide(ctx, dyn, gvr, nil)
	}

	for _, ns := range namespaces {
		if _, excluded := excludedNs[ns]; excluded {
			continue
		}
		items, err := listInNamespace(ctx, dyn, gvr, ns)
		if err != nil {
			continue
		}
		for _, item := range items {
			instances = append(instances, &item)
		}
	}

	return instances, nil
}

func listClusterWide(ctx context.Context, dyn dynamic.Interface, gvr schema.GroupVersionResource, excludedNs map[string]struct{}) ([]any, error) {
	list, err := dyn.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var instances []any
	for _, item := range list.Items {
		if excludedNs != nil {
			if _, excluded := excludedNs[item.GetNamespace()]; excluded {
				continue
			}
		}
		instances = append(instances, &item)
	}
	return instances, nil
}

func listInNamespace(ctx context.Context, dyn dynamic.Interface, gvr schema.GroupVersionResource, ns string) ([]unstructured.Unstructured, error) {
	list, err := dyn.Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// dedupKey produces a deduplication key from a Finding.
func dedupKey(f report.Finding) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s",
		f.Check, f.Namespace, f.Group, f.Kind, f.Name, f.Reason, f.Version)
}

// reportSeverity maps CRD findings to severity.
func reportSeverity(deprecatedVersions []string) report.Severity {
	if len(deprecatedVersions) > 0 {
		return report.SeverityMedium
	}
	return report.SeverityInfo
}

// parseAPIVersion extracts the version from a "group/version" string.
func parseAPIVersion(apiVersion string) string {
	parts := strings.Split(apiVersion, "/")
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}
