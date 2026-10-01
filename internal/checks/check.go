package checks

import (
	"context"

	"github.com/caseyrobb/kubescrub/internal/report"
)

type Check interface {
	Name() string
	Description() string
	Run(ctx context.Context, rt Runtime) ([]report.Finding, error)
}
