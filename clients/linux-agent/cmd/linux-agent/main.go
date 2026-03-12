package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"nodeweave/clients/linux-agent/internal/agent"
	"nodeweave/clients/linux-agent/internal/config"
	"nodeweave/clients/linux-agent/internal/state"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "init-config":
		exitIfErr(runInitConfig(os.Args[2:]))
	case "enroll":
		exitIfErr(runEnroll(os.Args[2:]))
	case "run":
		exitIfErr(runRun(os.Args[2:]))
	case "status":
		exitIfErr(runStatus(os.Args[2:]))
	case "runtime-status":
		exitIfErr(runRuntimeStatus(os.Args[2:]))
	case "plan-status":
		exitIfErr(runPlanStatus(os.Args[2:]))
	case "apply-status":
		exitIfErr(runApplyStatus(os.Args[2:]))
	case "session-status":
		exitIfErr(runSessionStatus(os.Args[2:]))
	case "session-report":
		exitIfErr(runSessionReport(os.Args[2:]))
	case "dataplane-status":
		exitIfErr(runDataplaneStatus(os.Args[2:]))
	case "transport-status":
		exitIfErr(runTransportStatus(os.Args[2:]))
	case "recovery-status":
		exitIfErr(runRecoveryStatus(os.Args[2:]))
	case "stun-status":
		exitIfErr(runSTUNStatus(os.Args[2:]))
	default:
		printUsage()
		os.Exit(2)
	}
}

func runInitConfig(args []string) error {
	fs := flag.NewFlagSet("init-config", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "path to agent config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := config.WriteExample(*configPath); err != nil {
		return err
	}
	fmt.Println(*configPath)
	return nil
}

func runEnroll(args []string) error {
	fs := flag.NewFlagSet("enroll", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "path to agent config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	svc, err := agent.New(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := svc.EnsureEnrolled(ctx); err != nil {
		return err
	}
	if err := svc.SyncBootstrap(ctx); err != nil {
		return err
	}
	if err := svc.ApplyRuntime(ctx); err != nil {
		return err
	}

	return printJSON(svc.CurrentState())
}

func runRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "path to agent config")
	duration := fs.Duration("duration", 0, "optional run duration")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	svc, err := agent.New(cfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *duration)
		defer cancel()
	}

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	return svc.Run(ctx)
}

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "path to agent config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	currentState, err := state.Load(cfg.StatePath)
	if err != nil {
		return err
	}
	return printJSON(currentState)
}

func runRuntimeStatus(args []string) error {
	fs := flag.NewFlagSet("runtime-status", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "path to agent config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	snapshot, err := state.LoadRuntime(cfg.RuntimePath)
	if err != nil {
		return err
	}
	return printJSON(snapshot)
}

func runPlanStatus(args []string) error {
	fs := flag.NewFlagSet("plan-status", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "path to agent config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	plan, err := state.LoadPlan(cfg.PlanPath)
	if err != nil {
		return err
	}
	return printJSON(plan)
}

func runApplyStatus(args []string) error {
	fs := flag.NewFlagSet("apply-status", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "path to agent config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	report, err := state.LoadApplyReport(cfg.ApplyReportPath)
	if err != nil {
		return err
	}
	return printJSON(report)
}

func runSessionStatus(args []string) error {
	fs := flag.NewFlagSet("session-status", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "path to agent config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	spec, err := state.LoadSession(cfg.SessionPath)
	if err != nil {
		return err
	}
	return printJSON(spec)
}

func runSessionReport(args []string) error {
	fs := flag.NewFlagSet("session-report", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "path to agent config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	report, err := state.LoadSessionReport(cfg.SessionReportPath)
	if err != nil {
		return err
	}
	return printJSON(report)
}

func runDataplaneStatus(args []string) error {
	fs := flag.NewFlagSet("dataplane-status", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "path to agent config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	spec, err := state.LoadDataplane(cfg.DataplanePath)
	if err != nil {
		return err
	}
	return printJSON(spec)
}

func runTransportStatus(args []string) error {
	fs := flag.NewFlagSet("transport-status", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "path to agent config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	report, err := state.LoadTransportReport(cfg.TransportReportPath)
	if err != nil {
		return err
	}
	return printJSON(report)
}

func runRecoveryStatus(args []string) error {
	fs := flag.NewFlagSet("recovery-status", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "path to agent config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	states, err := state.LoadRecoveryStates(cfg.RecoveryStatePath)
	if err != nil {
		return err
	}
	return printJSON(states)
}

func runSTUNStatus(args []string) error {
	fs := flag.NewFlagSet("stun-status", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "path to agent config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	report, err := state.LoadSTUNReport(cfg.STUNReportPath)
	if err != nil {
		return err
	}
	return printJSON(report)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: linux-agent <init-config|enroll|run|status|runtime-status|plan-status|apply-status|session-status|session-report|dataplane-status|transport-status|recovery-status|stun-status> [flags]")
}

func exitIfErr(err error) {
	if err == nil {
		return
	}
	if !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(1)
}

func printJSON(v any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}
