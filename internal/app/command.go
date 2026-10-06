package app

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/caseyrobb/kubescrub/internal/apply"
	"github.com/caseyrobb/kubescrub/internal/checks"
	"github.com/caseyrobb/kubescrub/internal/kube"
	"github.com/caseyrobb/kubescrub/internal/policy"
	"github.com/caseyrobb/kubescrub/internal/report"
)

func NewRoot() *cobra.Command {
	var policyPath string
	var kubeconfig string
	var context string

	cmd := &cobra.Command{
		Use:   "kubescrub",
		Short: "Cluster hygiene scanner",
		Long:  "KubeScrub scans Kubernetes clusters for hygiene issues and generates reports.",
	}

	cmd.PersistentFlags().StringVar(&policyPath, "policy", "policy.yaml", "Path to policy configuration file")
	cmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to the kubeconfig file to use")
	cmd.PersistentFlags().StringVar(&context, "context", "", "Kube context to use (overrides the file's current-context)")

	cmd.AddCommand(newScanCmd(&policyPath, &kubeconfig, &context))
	cmd.AddCommand(newApplyCmd(&policyPath, &kubeconfig, &context))

	return cmd
}

func newScanCmd(policyPath, kubeconfig, context *string) *cobra.Command {
	var outputFormat string
	var outPath string
	var namespaces string

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan the cluster for hygiene issues",
		Long: `Scan the current Kubernetes context for hygiene issues and produce a versioned
Plan document (kubescrub.io/v1).

The scan command runs all registered checkers, collects findings, and writes
a JSON Plan document.  With --out it writes to a file; without --out it prints
to stdout.

Examples:
  kubescrub scan                          # print JSON plan to stdout
  kubescrub scan --out plan.json          # write plan to file
  kubescrub scan --format json --out plan.json  # explicit JSON to file
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate format.
			if outputFormat == "" {
				outputFormat = "json"
			}
			switch outputFormat {
			case "json":
			default:
				return fmt.Errorf("unsupported format %q (only \"json\" is supported)", outputFormat)
			}

			// Load policy.
			p, err := policy.Load(*policyPath)
			if err != nil {
				return fmt.Errorf("load policy: %w", err)
			}

			// Resolve kubeconfig path (flag > KUBECONFIG > default).
			kubeconfigPath := kube.ResolveKubeconfigPath(*kubeconfig)

			// Resolve the current context from the selected kubeconfig.
			currentContext, err := kube.ResolveCurrentContext(*kubeconfig)
			if err != nil {
				return fmt.Errorf("resolve kubeconfig context: %w", err)
			}

			// Build Kubernetes client using the resolved config.
			k8sClient, err := kube.NewClient(kubeconfigPath, *context)
			if err != nil {
				return fmt.Errorf("build kubernetes client: %w", err)
			}

			// Determine namespace filter.
			var nsFilter []string
			if namespaces != "" {
				nsFilter = strings.Split(namespaces, ",")
			}

			// Use the resolved current-context as the cluster identifier.
			clusterID := currentContext
			if clusterID == "" {
				clusterID = k8sClient.Config.Host // fallback to API server URL
			}

			rt := checks.Runtime{
				Cluster:    clusterID,
				Client:     k8sClient.Kubernetes,
				Dynamic:    k8sClient.Dynamic,
				Discovery:  k8sClient.Kubernetes.Discovery(),
				Policy:     *p,
				Namespaces: nsFilter,
			}

			// Run all checks and collect findings. Errors per-checker are logged but
			// do not abort the scan — partial results are still useful.
			var findings []report.Finding
			for _, chk := range allChecks() {
				chkFindings, err := chk.Run(cmd.Context(), rt)
				if err != nil {
					fmt.Fprintf(os.Stderr, "WARNING: check %s: %v\n", chk.Name(), err)
					continue
				}
				findings = append(findings, chkFindings...)
			}

			// Deduplicate findings by ID (cross-checker dedup).
			seen := make(map[string]struct{}, len(findings))
			deduped := findings[:0]
			for _, f := range findings {
				if f.ID == "" {
					deduped = append(deduped, f)
					continue
				}
				if _, exists := seen[f.ID]; exists {
					continue
				}
				seen[f.ID] = struct{}{}
				deduped = append(deduped, f)
			}
			findings = deduped

			// Build versioned Plan wrapper.
			plan := apply.NewPlan(rt.Cluster, time.Now().UTC().Format(time.RFC3339))
			for _, f := range findings {
				plan.AddFinding(f)
			}
			plan.PolicyPath = *policyPath

			// Encode to JSON.
			var data []byte
			if outPath != "" {
				encoded, err := plan.EncodeTo(outPath)
				if err != nil {
					return fmt.Errorf("encode plan: %w", err)
				}
				data = encoded
				fmt.Fprintf(os.Stderr, "Plan written to %s (%d findings)\n", outPath, len(plan.Findings))
			} else {
				encoded, err := plan.EncodeTo("")
				if err != nil {
					return fmt.Errorf("encode plan: %w", err)
				}
				data = encoded
			}

			// Write to stdout.
			fmt.Println(string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&outputFormat, "format", "", "Output format (default \"json\")")
	cmd.Flags().StringVar(&outPath, "out", "", "Write plan document to FILE instead of stdout")
	cmd.Flags().StringVar(&namespaces, "namespaces", "", "Comma-separated list of namespaces to scan (empty means all)")

	return cmd
}
