package kube

import (
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client encapsulates all Kubernetes clients.
type Client struct {
	Config    *rest.Config
	Kubernetes kubernetes.Interface
	Dynamic    dynamic.Interface
}

// NewClient creates a Kubernetes client from the current kubeconfig context.
func NewClient() (*Client, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		return nil, err
	}
	cfg.Timeout = 30 * time.Second
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
