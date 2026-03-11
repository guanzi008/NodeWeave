package linux

import (
	"testing"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/packages/runtime/go/overlay"
)

func TestBuild(t *testing.T) {
	snapshot := overlay.Snapshot{
		NodeID: "node-1",
		Interface: overlay.InterfaceState{
			Name:        "nw0",
			AddressCIDR: "100.64.0.10/10",
			MTU:         1380,
		},
		Peers: []overlay.PeerState{
			{
				NodeID:     "node-peer",
				OverlayIP:  "100.64.0.11",
				AllowedIPs: []string{"100.64.0.11/32", "10.20.0.0/16"},
			},
		},
		Routes: []overlay.RouteState{
			{
				NetworkCIDR: "10.20.0.0/16",
				ViaNodeID:   "node-peer",
			},
		},
		DNS: api.DNSConfig{
			Domain:      "internal.net",
			Nameservers: []string{"100.64.0.53"},
		},
		ExitNode: &api.ExitNodeConfig{
			NodeID:        "node-peer",
			AllowInternet: true,
		},
	}

	plan := Build(snapshot)
	if len(plan.Operations) < 7 {
		t.Fatalf("expected plan to have operations, got %d", len(plan.Operations))
	}
	if plan.Operations[0].Command[0] != "ip" {
		t.Fatalf("expected first command to start with ip, got %#v", plan.Operations[0].Command)
	}
	foundStaticRoute := false
	foundDefaultRoute := false
	for _, operation := range plan.Operations {
		if operation.RouteCIDR == "10.20.0.0/16" {
			foundStaticRoute = true
		}
		if operation.RouteCIDR == "0.0.0.0/0" {
			foundDefaultRoute = true
		}
	}
	if !foundStaticRoute {
		t.Fatalf("expected plan to include static route operation")
	}
	if !foundDefaultRoute {
		t.Fatalf("expected plan to include exit node default route")
	}
}
