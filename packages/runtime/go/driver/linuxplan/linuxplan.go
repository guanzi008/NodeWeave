package linuxplan

import (
	"context"
	"time"

	"nodeweave/packages/runtime/go/driver"
	"nodeweave/packages/runtime/go/overlay"
	linuxplan "nodeweave/packages/runtime/go/plan/linux"
)

type Driver struct{}

func New() Driver {
	return Driver{}
}

func (Driver) Name() string {
	return "linux-plan"
}

func (Driver) Apply(_ context.Context, snapshot overlay.Snapshot) (driver.Report, error) {
	plan := linuxplan.Build(snapshot)
	results := make([]driver.OperationResult, 0, len(plan.Operations))
	for _, operation := range plan.Operations {
		now := time.Now().UTC()
		results = append(results, driver.OperationResult{
			Description: operation.Description,
			Command:     append([]string(nil), operation.Command...),
			Status:      "planned",
			StartedAt:   now,
			CompletedAt: now,
			ExitCode:    0,
		})
	}
	return driver.Report{
		Backend:    "linux-plan",
		AppliedAt:  time.Now().UTC(),
		Success:    true,
		Operations: results,
	}, nil
}
