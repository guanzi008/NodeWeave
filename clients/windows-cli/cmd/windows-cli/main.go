package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"nodeweave/clients/windows-cli/internal/state"
	"nodeweave/packages/contracts/go/api"
	contractsclient "nodeweave/packages/contracts/go/client"
)

const defaultServerURL = "http://127.0.0.1:8080"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "enroll":
		exitIfErr(runEnroll(os.Args[2:]))
	case "status":
		exitIfErr(runStatus(os.Args[2:]))
	case "heartbeat":
		exitIfErr(runHeartbeat(os.Args[2:]))
	case "bootstrap":
		exitIfErr(runBootstrap(os.Args[2:]))
	case "login":
		exitIfErr(runLogin(os.Args[2:]))
	case "nodes":
		exitIfErr(runNodes(os.Args[2:]))
	default:
		printUsage()
		os.Exit(2)
	}
}

func runEnroll(args []string) error {
	fs := flag.NewFlagSet("enroll", flag.ContinueOnError)
	serverURL := fs.String("server", defaultServerURL, "control plane base url")
	statePath := fs.String("state", state.DefaultPath(), "path to local client state")
	registrationToken := fs.String("registration-token", envOr("NODEWEAVE_REGISTRATION_TOKEN", "dev-register-token"), "registration token")
	deviceName := fs.String("device-name", hostOr("windows-cli"), "device name")
	platform := fs.String("platform", "windows", "platform")
	version := fs.String("version", "0.1.0", "client version")
	publicKey := fs.String("public-key", "windows-cli-devpub", "node public key")
	capabilities := fs.String("capabilities", "cli,windows", "comma-separated capabilities")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client := contractsclient.New(*serverURL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.RegisterDevice(ctx, api.DeviceRegistrationRequest{
		DeviceName:        *deviceName,
		Platform:          *platform,
		Version:           *version,
		PublicKey:         *publicKey,
		Capabilities:      splitCSV(*capabilities),
		RegistrationToken: *registrationToken,
	})
	if err != nil {
		return err
	}

	currentState := state.File{
		ServerURL: *serverURL,
		Device:    resp.Device,
		Node:      resp.Node,
		NodeToken: resp.NodeToken,
	}
	if err := state.Save(*statePath, currentState); err != nil {
		return err
	}
	return printJSON(currentState)
}

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	statePath := fs.String("state", state.DefaultPath(), "path to local client state")
	if err := fs.Parse(args); err != nil {
		return err
	}

	currentState, err := state.Load(*statePath)
	if err != nil {
		return err
	}

	client := contractsclient.New(currentState.ServerURL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bootstrap, err := client.GetBootstrap(ctx, currentState.Node.ID, currentState.NodeToken)
	if err != nil {
		return err
	}

	return printJSON(map[string]any{
		"state":     currentState,
		"bootstrap": bootstrap,
	})
}

func runHeartbeat(args []string) error {
	fs := flag.NewFlagSet("heartbeat", flag.ContinueOnError)
	statePath := fs.String("state", state.DefaultPath(), "path to local client state")
	endpoints := fs.String("endpoints", "", "comma-separated endpoints")
	relayRegion := fs.String("relay-region", "", "relay region")
	statusValue := fs.String("status", "online", "node status")
	if err := fs.Parse(args); err != nil {
		return err
	}

	currentState, err := state.Load(*statePath)
	if err != nil {
		return err
	}

	client := contractsclient.New(currentState.ServerURL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.Heartbeat(ctx, currentState.Node.ID, currentState.NodeToken, api.HeartbeatRequest{
		Endpoints:   splitCSV(*endpoints),
		RelayRegion: *relayRegion,
		Status:      *statusValue,
	})
	if err != nil {
		return err
	}

	currentState.Node = resp.Node
	if err := state.Save(*statePath, currentState); err != nil {
		return err
	}
	return printJSON(resp)
}

func runBootstrap(args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	statePath := fs.String("state", state.DefaultPath(), "path to local client state")
	if err := fs.Parse(args); err != nil {
		return err
	}

	currentState, err := state.Load(*statePath)
	if err != nil {
		return err
	}

	client := contractsclient.New(currentState.ServerURL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.GetBootstrap(ctx, currentState.Node.ID, currentState.NodeToken)
	if err != nil {
		return err
	}
	return printJSON(resp)
}

func runLogin(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	serverURL := fs.String("server", defaultServerURL, "control plane base url")
	email := fs.String("email", envOr("NODEWEAVE_ADMIN_EMAIL", "admin@example.com"), "admin email")
	password := fs.String("password", envOr("NODEWEAVE_ADMIN_PASSWORD", "dev-password"), "admin password")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client := contractsclient.New(*serverURL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.Login(ctx, api.LoginRequest{
		Email:    *email,
		Password: *password,
	})
	if err != nil {
		return err
	}
	return printJSON(resp)
}

func runNodes(args []string) error {
	fs := flag.NewFlagSet("nodes", flag.ContinueOnError)
	serverURL := fs.String("server", defaultServerURL, "control plane base url")
	token := fs.String("token", envOr("NODEWEAVE_ADMIN_TOKEN", ""), "admin token")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*token) == "" {
		return fmt.Errorf("admin token is required")
	}

	client := contractsclient.New(*serverURL).WithToken(*token)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.ListNodes(ctx)
	if err != nil {
		return err
	}
	return printJSON(resp)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: windows-cli <enroll|status|heartbeat|bootstrap|login|nodes>")
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func envOr(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func hostOr(fallback string) string {
	name, err := os.Hostname()
	if err != nil || strings.TrimSpace(name) == "" {
		return fallback
	}
	return name
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
