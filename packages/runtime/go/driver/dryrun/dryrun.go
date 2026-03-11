package dryrun

import (
	"context"
	"time"

	"nodeweave/packages/runtime/go/driver"
	"nodeweave/packages/runtime/go/overlay"
)

type Driver struct{}

func New() Driver {
	return Driver{}
}

func (Driver) Name() string {
	return "dry-run"
}

func (Driver) Apply(_ context.Context, _ overlay.Snapshot) (driver.Report, error) {
	return driver.Report{
		Backend:    "dry-run",
		AppliedAt:  time.Now().UTC(),
		Success:    true,
		Operations: []driver.OperationResult{},
	}, nil
}
