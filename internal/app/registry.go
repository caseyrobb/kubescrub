package app

import (
	"github.com/caseyrobb/kubescrub/internal/checks"
	"github.com/caseyrobb/kubescrub/internal/checks/crd"
	"github.com/caseyrobb/kubescrub/internal/checks/openshift"
	"github.com/caseyrobb/kubescrub/internal/checks/pvc"
	"github.com/caseyrobb/kubescrub/internal/checks/rbac"
	"github.com/caseyrobb/kubescrub/internal/checks/workload"
)

func allChecks() []checks.Check {
	return []checks.Check{
		workload.New(),
		pvc.New(),
		crd.New(),
		rbac.New(),
		openshift.NewRoute(),
		openshift.NewCSV(),
	}
}
