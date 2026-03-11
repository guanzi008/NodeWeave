package linuxexec

import (
	"context"
	"testing"
	"time"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/packages/runtime/go/driver"
	"nodeweave/packages/runtime/go/overlay"
)

type fakeRunner struct {
	results []driver.OperationResult
	calls   [][]string
}

func (f *fakeRunner) Run(_ context.Context, command []string, _ time.Duration) driver.OperationResult {
	f.calls = append(f.calls, append([]string(nil), command...))
	if len(f.results) == 0 {
		return driver.OperationResult{
			Command:     append([]string(nil), command...),
			ExitCode:    0,
			StartedAt:   time.Now().UTC(),
			CompletedAt: time.Now().UTC(),
		}
	}
	result := f.results[0]
	f.results = f.results[1:]
	if len(result.Command) == 0 {
		result.Command = append([]string(nil), command...)
	}
	if result.StartedAt.IsZero() {
		result.StartedAt = time.Now().UTC()
	}
	if result.CompletedAt.IsZero() {
		result.CompletedAt = time.Now().UTC()
	}
	return result
}

type fakeInspector struct {
	interfaceStates map[string]InterfaceState
	addresses       map[string]bool
	routes          map[string]RouteState
	nameservers     map[string][]string
	domains         map[string][]string
}

func (f fakeInspector) InterfaceState(_ context.Context, ifName string, _ time.Duration) (InterfaceState, error) {
	return f.interfaceStates[ifName], nil
}

func (f fakeInspector) AddressAssigned(_ context.Context, ifName, cidr string, _ time.Duration) (bool, error) {
	return f.addresses[ifName+"|"+cidr], nil
}

func (f fakeInspector) RouteState(_ context.Context, cidr string, _ time.Duration) (RouteState, error) {
	return f.routes[cidr], nil
}

func (f fakeInspector) LinkNameservers(_ context.Context, ifName string, _ time.Duration) ([]string, error) {
	return append([]string(nil), f.nameservers[ifName]...), nil
}

func (f fakeInspector) LinkDomains(_ context.Context, ifName string, _ time.Duration) ([]string, error) {
	return append([]string(nil), f.domains[ifName]...), nil
}

func TestApplyExecutesLinuxPlan(t *testing.T) {
	runner := &fakeRunner{}
	inspector := fakeInspector{
		interfaceStates: map[string]InterfaceState{},
		addresses:       map[string]bool{},
		routes:          map[string]RouteState{},
		nameservers:     map[string][]string{},
		domains:         map[string][]string{},
	}
	driver := NewWithDeps(Config{RequireRoot: false, CommandTimeout: time.Second}, runner, inspector)

	snapshot := overlay.Snapshot{
		NodeID: "node-1",
		Interface: overlay.InterfaceState{
			Name:        "nw0",
			AddressCIDR: "100.64.0.10/10",
			MTU:         1380,
		},
		DNS: api.DNSConfig{
			Domain:      "internal.net",
			Nameservers: []string{"100.64.0.53"},
		},
	}

	report, err := driver.Apply(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !report.Success {
		t.Fatalf("expected success report")
	}
	if len(runner.calls) < 3 {
		t.Fatalf("expected at least 3 commands, got %d", len(runner.calls))
	}
}

func TestApplyStopsOnCommandFailure(t *testing.T) {
	runner := &fakeRunner{
		results: []driver.OperationResult{
			{ExitCode: 0, Status: "applied"},
			{ExitCode: 1, Error: "boom", Status: "failed"},
		},
	}
	driver := NewWithDeps(Config{RequireRoot: false, CommandTimeout: time.Second}, runner, fakeInspector{})

	snapshot := overlay.Snapshot{
		NodeID: "node-1",
		Interface: overlay.InterfaceState{
			Name:        "nw0",
			AddressCIDR: "100.64.0.10/10",
			MTU:         1380,
		},
	}

	report, err := driver.Apply(context.Background(), snapshot)
	if err == nil {
		t.Fatal("expected apply to fail")
	}
	if report.Success {
		t.Fatal("expected failure report")
	}
	if len(report.Operations) != 2 {
		t.Fatalf("expected 2 operations before stop, got %d", len(report.Operations))
	}
}

func TestApplySkipsSatisfiedOperations(t *testing.T) {
	runner := &fakeRunner{}
	driver := NewWithDeps(Config{RequireRoot: false, CommandTimeout: time.Second}, runner, fakeInspector{
		interfaceStates: map[string]InterfaceState{
			"nw0": {Exists: true, Up: true, MTU: 1380},
		},
		addresses: map[string]bool{
			"nw0|100.64.0.10/10": true,
		},
		routes: map[string]RouteState{
			"100.64.0.11/32": {Exists: true, Device: "nw0"},
			"10.20.0.0/16":   {Exists: true, Device: "nw0"},
		},
		nameservers: map[string][]string{
			"nw0": {"100.64.0.53"},
		},
		domains: map[string][]string{
			"nw0": {"internal.net"},
		},
	})

	snapshot := overlay.Snapshot{
		NodeID: "node-1",
		Interface: overlay.InterfaceState{
			Name:        "nw0",
			AddressCIDR: "100.64.0.10/10",
			MTU:         1380,
		},
		Peers: []overlay.PeerState{
			{NodeID: "peer-1", OverlayIP: "100.64.0.11", AllowedIPs: []string{"100.64.0.11/32", "10.20.0.0/16"}},
		},
		Routes: []overlay.RouteState{
			{NetworkCIDR: "10.20.0.0/16", ViaNodeID: "peer-1"},
		},
		DNS: api.DNSConfig{
			Domain:      "internal.net",
			Nameservers: []string{"100.64.0.53"},
		},
	}

	report, err := driver.Apply(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected all operations to be skipped, got %d calls", len(runner.calls))
	}
	if report.Operations[0].Status != "skipped" {
		t.Fatalf("expected first operation to be skipped, got %s", report.Operations[0].Status)
	}
}

func TestApplyAddsExitNodeDefaultRoute(t *testing.T) {
	runner := &fakeRunner{}
	driver := NewWithDeps(Config{RequireRoot: false, CommandTimeout: time.Second}, runner, fakeInspector{
		interfaceStates: map[string]InterfaceState{},
		addresses:       map[string]bool{},
		routes:          map[string]RouteState{},
		nameservers:     map[string][]string{},
		domains:         map[string][]string{},
	})

	snapshot := overlay.Snapshot{
		NodeID: "node-1",
		Interface: overlay.InterfaceState{
			Name:        "nw0",
			AddressCIDR: "100.64.0.10/10",
			MTU:         1380,
		},
		Peers: []overlay.PeerState{
			{NodeID: "node-exit", OverlayIP: "100.64.0.11", AllowedIPs: []string{"100.64.0.11/32"}},
		},
		ExitNode: &api.ExitNodeConfig{
			NodeID:        "node-exit",
			AllowInternet: true,
		},
	}

	report, err := driver.Apply(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !report.Success {
		t.Fatalf("expected success report")
	}
	foundDefault := false
	for _, call := range runner.calls {
		if len(call) >= 4 && call[0] == "ip" && call[1] == "route" && call[2] == "replace" && call[3] == "0.0.0.0/0" {
			foundDefault = true
			break
		}
	}
	if !foundDefault {
		t.Fatalf("expected default route command in %#v", runner.calls)
	}
}
