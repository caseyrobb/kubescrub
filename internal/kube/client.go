package kube

import (
	"fmt"
	"os"
	"strings"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client encapsulates all Kubernetes clients.
type Client struct {
	Config     *rest.Config
	Kubernetes kubernetes.Interface
	Dynamic    dynamic.Interface
}

// resolveKubeconfig returns the kubeconfig file path using the following
// precedence (same as kubectl):
//
//  1. --kubeconfig flag (if non-empty).
//  2. KUBECONFIG environment variable (if set and non-empty).
//  3. Default path (~/.kube/config).
func resolveKubeconfig(kubeconfigFlag string) string {
	if kubeconfigFlag != "" {
		return kubeconfigFlag
	}
	if env := os.Getenv("KUBECONFIG"); env != "" {
		return env
	}
	return clientcmd.RecommendedHomeFile
}

// newLoadingRules returns a clientcmd loading rules that honors the kubeconfig
// precedence: flag > KUBECONFIG env > default path.
// When the flag is set, only that file is loaded.
// When KUBECONFIG env is set (but no flag), it supports colon-separated paths.
// When neither is set, only the default path is loaded.
func newLoadingRules(kubeconfigFlag string) *clientcmd.ClientConfigLoadingRules {
	if kubeconfigFlag != "" {
		// Flag takes precedence: load only the specified file.
		return &clientcmd.ClientConfigLoadingRules{Precedence: []string{kubeconfigFlag}}
	}
	if env := os.Getenv("KUBECONFIG"); env != "" {
		// KUBECONFIG env set: support colon-separated list.
		return &clientcmd.ClientConfigLoadingRules{Precedence: strings.Split(env, ":")}
	}
	// Default: load from ~/.kube/config.
	return clientcmd.NewDefaultClientConfigLoadingRules()
}

// NewClient creates a Kubernetes client from the selected kubeconfig context.
// The kubeconfigFlag parameter should be the --kubeconfig flag value (empty string if not set).
// If context is non-empty, that named context is used; otherwise the file's
// current-context is loaded.
func NewClient(kubeconfigFlag string, context string) (*Client, error) {
	loadingRules := newLoadingRules(kubeconfigFlag)
	configOverrides := &clientcmd.ConfigOverrides{
		CurrentContext: context,
	}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	cfg, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	return NewClientFromConfig(cfg)
}

// NewClientFromConfig creates clients from an explicit rest.Config.
func NewClientFromConfig(cfg *rest.Config) (*Client, error) {
	// If no timeout set, default to 30s.
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	k8s, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{
		Config:     cfg,
		Kubernetes: k8s,
		Dynamic:    dyn,
	}, nil
}

// ResolveKubeconfigPath returns the kubeconfig path resolved from flag and env.
// This is exported for use by commands that need the raw path (e.g. to extract
// current-context via RawConfig).
func ResolveKubeconfigPath(kubeconfigFlag string) string {
	return resolveKubeconfig(kubeconfigFlag)
}

// ResolveCurrentContext reads the current-context from the kubeconfig file
// determined by the given flag. It returns an error if the file does not exist.
func ResolveCurrentContext(kubeconfigFlag string) (string, error) {
	loadingRules := newLoadingRules(kubeconfigFlag)

	// Check existence if KUBECONFIG was explicitly set (env or flag).
	if kubeconfigFlag != "" || os.Getenv("KUBECONFIG") != "" {
		// Validate that at least one of the files exists.
		for _, path := range loadingRules.Precedence {
			if _, err := os.Stat(path); err == nil {
				// Found a valid file.
				break
			}
			return "", fmt.Errorf("kubeconfig %q does not exist", path)
		}
	}

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	rawConfig, err := kubeConfig.RawConfig()
	if err != nil {
		return "", err
	}
	return rawConfig.CurrentContext, nil
}
