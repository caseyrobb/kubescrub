package kube

import (
	"testing"
	"time"

	"k8s.io/client-go/rest"
)

func TestNewClientFromConfigTimeout(t *testing.T) {
	cfg := &rest.Config{}
	c, err := NewClientFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewClientFromConfig: %v", err)
	}
	if c.Config.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", c.Config.Timeout)
	}
	if c.Kubernetes == nil {
		t.Error("Kubernetes client should not be nil")
	}
	if c.Dynamic == nil {
		t.Error("Dynamic client should not be nil")
	}
}

func TestNewClientFromConfigPreservesExistingTimeout(t *testing.T) {
	cfg := &rest.Config{Timeout: 10 * time.Second}
	c, err := NewClientFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewClientFromConfig: %v", err)
	}
	// The config should keep its existing timeout, not overwrite.
	if c.Config.Timeout != 10*time.Second {
		t.Errorf("timeout = %v, want 10s (preserved)", c.Config.Timeout)
	}
}

func TestNewClientFromConfigCreatesValidClients(t *testing.T) {
	cfg := &rest.Config{
		Host: "https://test.example.com:6443",
	}
	c, err := NewClientFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewClientFromConfig: %v", err)
	}
	if c.Config.Host != cfg.Host {
		t.Errorf("Config.Host = %q, want %q", c.Config.Host, cfg.Host)
	}
}
