package checks

import (
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/caseyrobb/kubescrub/internal/policy"
)

type Runtime struct {
	Cluster    string
	Client     kubernetes.Interface
	Dynamic    dynamic.Interface
	Discovery  discovery.DiscoveryInterface
	Policy     policy.Policy
	Namespaces []string // empty means all
}
