package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"nodeweave/clients/windows-agent/internal/agent"
	"nodeweave/clients/windows-agent/internal/config"
	"nodeweave/clients/windows-agent/internal/state"
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
	case "bootstrap-status":
		exitIfErr(runBootstrapStatus(os.Args[2:]))
	case "runtime-status":
		exitIfErr(runRuntimeStatus(os.Args[2:]))
	case "heartbeat":
		exitIfErr(runHeartbeat(os.Args[2:]))
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

func runBootstrapStatus(args []string) error {
	fs := flag.NewFlagSet("bootstrap-status", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultPath(), "path to agent config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	bootstrap, err := state.LoadBootstrap(cfg.BootstrapPath)
	if err != nil {
		return err
	}
	return printJSON(bootstrap)
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

func runHeartbeat(args []string) error {
	fs := flag.NewFlagSet("heartbeat", flag.ContinueOnError)
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
	resp, err := svc.Heartbeat(ctx)
	if err != nil {
		return err
	}
	return printJSON(resp)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: windows-agent <init-config|enroll|run|status|bootstrap-status|runtime-status|heartbeat>")
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func exitIfErr(err error) {
	if err == nil {
		return
	}
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
