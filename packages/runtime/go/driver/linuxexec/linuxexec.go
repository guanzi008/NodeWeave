package linuxexec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"nodeweave/packages/runtime/go/driver"
	"nodeweave/packages/runtime/go/overlay"
	linuxplan "nodeweave/packages/runtime/go/plan/linux"
)

type Config struct {
	RequireRoot    bool
	CommandTimeout time.Duration
}

type Runner interface {
	Run(context.Context, []string, time.Duration) driver.OperationResult
}

type Driver struct {
	config    Config
	runner    Runner
	inspector Inspector
}

func New(config Config) Driver {
	if config.CommandTimeout <= 0 {
		config.CommandTimeout = 5 * time.Second
	}
	return Driver{
		config:    config,
		runner:    OSRunner{},
		inspector: OSInspector{},
	}
}

func NewWithRunner(config Config, runner Runner) Driver {
	if config.CommandTimeout <= 0 {
		config.CommandTimeout = 5 * time.Second
	}
	return Driver{
		config:    config,
		runner:    runner,
		inspector: OSInspector{},
	}
}

func NewWithDeps(config Config, runner Runner, inspector Inspector) Driver {
	if config.CommandTimeout <= 0 {
		config.CommandTimeout = 5 * time.Second
	}
	return Driver{
		config:    config,
		runner:    runner,
		inspector: inspector,
	}
}

func (Driver) Name() string {
	return "linux-exec"
}

func (d Driver) Apply(ctx context.Context, snapshot overlay.Snapshot) (driver.Report, error) {
	report := driver.Report{
		Backend:   d.Name(),
		AppliedAt: time.Now().UTC(),
		Success:   false,
	}

	if d.config.RequireRoot && os.Geteuid() != 0 {
		err := errors.New("linux-exec requires root privileges")
		report.Operations = append(report.Operations, driver.OperationResult{
			Description: "preflight privilege check",
			Error:       err.Error(),
		})
		return report, err
	}

	plan := linuxplan.Build(snapshot)
	for _, operation := range plan.Operations {
		satisfied, err := operationSatisfied(ctx, d.inspector, operation, d.config.CommandTimeout)
		if err != nil {
			report.Operations = append(report.Operations, driver.OperationResult{
				Description: operation.Description,
				Command:     append([]string(nil), operation.Command...),
				Status:      "probe_failed",
				StartedAt:   time.Now().UTC(),
				CompletedAt: time.Now().UTC(),
				ExitCode:    -1,
				Error:       err.Error(),
			})
			return report, err
		}
		if satisfied {
			now := time.Now().UTC()
			report.Operations = append(report.Operations, driver.OperationResult{
				Description: operation.Description,
				Command:     append([]string(nil), operation.Command...),
				Status:      "skipped",
				StartedAt:   now,
				CompletedAt: now,
				ExitCode:    0,
			})
			continue
		}

		result := d.runner.Run(ctx, operation.Command, d.config.CommandTimeout)
		result.Description = operation.Description
		if len(result.Command) == 0 {
			result.Command = append([]string(nil), operation.Command...)
		}
		if result.Status == "" {
			if result.Error != "" || result.ExitCode != 0 {
				result.Status = "failed"
			} else {
				result.Status = "applied"
			}
		}
		report.Operations = append(report.Operations, result)
		if result.Error != "" || result.ExitCode != 0 {
			err := fmt.Errorf("command failed: %s", strings.Join(operation.Command, " "))
			return report, err
		}
	}

	report.Success = true
	return report, nil
}

type OSRunner struct{}

func (OSRunner) Run(ctx context.Context, command []string, timeout time.Duration) (result driver.OperationResult) {
	result = driver.OperationResult{
		Command:   append([]string(nil), command...),
		StartedAt: time.Now().UTC(),
		ExitCode:  -1,
	}

	if len(command) == 0 {
		result.Error = "empty command"
		result.CompletedAt = time.Now().UTC()
		return result
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, command[0], command[1:]...)
	output, err := cmd.CombinedOutput()
	result.Stdout = string(output)
	if err == nil {
		result.ExitCode = 0
		result.CompletedAt = time.Now().UTC()
		return result
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		result.Error = err.Error()
		result.CompletedAt = time.Now().UTC()
		return result
	}

	if runCtx.Err() == context.DeadlineExceeded {
		result.Error = "command timed out"
		result.CompletedAt = time.Now().UTC()
		return result
	}

	result.Error = err.Error()
	result.CompletedAt = time.Now().UTC()
	return result
}
