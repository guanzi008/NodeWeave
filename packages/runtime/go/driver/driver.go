package driver

import (
	"context"
	"time"

	"nodeweave/packages/runtime/go/overlay"
)

type OperationResult struct {
	Description string    `json:"description"`
	Command     []string  `json:"command"`
	Status      string    `json:"status"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	ExitCode    int       `json:"exit_code"`
	Stdout      string    `json:"stdout,omitempty"`
	Stderr      string    `json:"stderr,omitempty"`
	Error       string    `json:"error,omitempty"`
}

type Report struct {
	Backend    string            `json:"backend"`
	AppliedAt  time.Time         `json:"applied_at"`
	Success    bool              `json:"success"`
	Operations []OperationResult `json:"operations"`
}

type Driver interface {
	Name() string
	Apply(context.Context, overlay.Snapshot) (Report, error)
}
