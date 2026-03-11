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

	"nodeweave/clients/linux-cli/internal/state"
	"nodeweave/packages/contracts/go/api"
	contractsclient "nodeweave/packages/contracts/go/client"
	"nodeweave/packages/runtime/go/secureudp"
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
	case "routes":
		exitIfErr(runRoutes(os.Args[2:]))
	case "route-create":
		exitIfErr(runRouteCreate(os.Args[2:]))
	case "dns-zones":
		exitIfErr(runDNSZones(os.Args[2:]))
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
	deviceName := fs.String("device-name", hostOr("linux-cli"), "device name")
	platform := fs.String("platform", "linux", "platform")
	version := fs.String("version", "0.1.0", "client version")
	publicKey := fs.String("public-key", "", "node public key")
	capabilities := fs.String("capabilities", "cli", "comma-separated capabilities")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*publicKey) == "" {
		_, generatedPublicKey, err := secureudp.GenerateKeyPair()
		if err != nil {
			return fmt.Errorf("generate default node public key: %w", err)
		}
		*publicKey = generatedPublicKey
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

func runRoutes(args []string) error {
	fs := flag.NewFlagSet("routes", flag.ContinueOnError)
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

	resp, err := client.ListRoutes(ctx)
	if err != nil {
		return err
	}
	return printJSON(resp)
}

func runRouteCreate(args []string) error {
	fs := flag.NewFlagSet("route-create", flag.ContinueOnError)
	serverURL := fs.String("server", defaultServerURL, "control plane base url")
	token := fs.String("token", envOr("NODEWEAVE_ADMIN_TOKEN", ""), "admin token")
	network := fs.String("network", "", "route network CIDR")
	viaNode := fs.String("via-node", "", "via node id")
	priority := fs.Int("priority", 100, "route priority")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*token) == "" {
		return fmt.Errorf("admin token is required")
	}
	if strings.TrimSpace(*network) == "" || strings.TrimSpace(*viaNode) == "" {
		return fmt.Errorf("network and via-node are required")
	}

	client := contractsclient.New(*serverURL).WithToken(*token)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.CreateRoute(ctx, api.CreateRouteRequest{
		NetworkCIDR: *network,
		ViaNodeID:   *viaNode,
		Priority:    *priority,
	})
	if err != nil {
		return err
	}
	return printJSON(resp)
}

func runDNSZones(args []string) error {
	fs := flag.NewFlagSet("dns-zones", flag.ContinueOnError)
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

	resp, err := client.ListDNSZones(ctx)
	if err != nil {
		return err
	}
	return printJSON(resp)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: linux-cli <enroll|status|heartbeat|bootstrap|login|nodes|routes|route-create|dns-zones> [flags]")
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

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
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

func envOr(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func hostOr(fallback string) string {
	if host, err := os.Hostname(); err == nil && host != "" {
		return host
	}
	return fallback
}
