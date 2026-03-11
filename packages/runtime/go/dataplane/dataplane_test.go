package dataplane

import (
	"context"
	"testing"
	"time"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/packages/runtime/go/overlay"
	"nodeweave/packages/runtime/go/session"
)

type memorySink struct {
	received chan InboundPacket
}

func (m memorySink) HandleInbound(_ context.Context, packet InboundPacket) error {
	m.received <- packet
	return nil
}

func TestBuildAndResolve(t *testing.T) {
	spec, err := Build(overlay.Snapshot{
		NodeID: "node-a",
		Peers: []overlay.PeerState{
			{
				NodeID:    "node-b",
				OverlayIP: "100.64.0.11",
			},
		},
		Routes: []overlay.RouteState{
			{
				NetworkCIDR: "10.20.0.0/16",
				ViaNodeID:   "node-b",
				Priority:    100,
			},
		},
		ExitNode: &api.ExitNodeConfig{
			NodeID:        "node-b",
			AllowInternet: true,
		},
	}, session.Spec{
		NodeID: "node-a",
		Peers: []session.Peer{
			{
				NodeID:             "node-b",
				PreferredCandidate: "198.51.100.10:51820",
				Candidates: []session.Candidate{
					{Kind: "direct", Address: "198.51.100.10:51820", Priority: 1000},
				},
			},
		},
	}, Config{ListenAddress: "0.0.0.0:51820"})
	if err != nil {
		t.Fatalf("build dataplane spec: %v", err)
	}

	if spec.ListenAddress != "0.0.0.0:51820" {
		t.Fatalf("unexpected listen address %q", spec.ListenAddress)
	}
	if len(spec.Routes) < 3 {
		t.Fatalf("expected host, static, and default routes, got %#v", spec.Routes)
	}

	route, err := spec.Resolve("10.20.1.1")
	if err != nil {
		t.Fatalf("resolve route: %v", err)
	}
	if route.NetworkCIDR != "10.20.0.0/16" {
		t.Fatalf("expected static route match, got %#v", route)
	}

	defaultRoute, err := spec.Resolve("8.8.8.8")
	if err != nil {
		t.Fatalf("resolve default route: %v", err)
	}
	if defaultRoute.NetworkCIDR != "0.0.0.0/0" {
		t.Fatalf("expected default route, got %#v", defaultRoute)
	}
}

func TestBuildPrefersProbeSelectedCandidate(t *testing.T) {
	spec, err := Build(overlay.Snapshot{
		NodeID: "node-a",
		Peers: []overlay.PeerState{
			{
				NodeID:    "node-b",
				OverlayIP: "100.64.0.11",
			},
		},
	}, session.Spec{
		NodeID: "node-a",
		Peers: []session.Peer{
			{
				NodeID:             "node-b",
				PreferredCandidate: "198.51.100.10:51820",
				Candidates: []session.Candidate{
					{Kind: "direct", Address: "198.51.100.10:51820", Priority: 1000},
					{Kind: "relay", Address: "relay-us.example.net:3478", Priority: 500},
				},
			},
		},
	}, Config{
		ListenAddress: "0.0.0.0:51820",
		SessionReport: session.Report{
			NodeID: "node-a",
			Peers: []session.PeerReport{
				{
					NodeID:            "node-b",
					SelectedCandidate: "relay-us.example.net:3478",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("build dataplane spec: %v", err)
	}
	if len(spec.Routes) != 1 {
		t.Fatalf("expected a single host route, got %#v", spec.Routes)
	}
	if spec.Routes[0].CandidateAddress != "relay-us.example.net:3478" {
		t.Fatalf("expected probe-selected relay candidate, got %#v", spec.Routes[0])
	}
	if spec.Routes[0].CandidateKind != "relay" {
		t.Fatalf("expected relay candidate kind, got %#v", spec.Routes[0])
	}
}

func TestBuildFallsBackToPreferredCandidateWhenProbeSelectionIsMissing(t *testing.T) {
	spec, err := Build(overlay.Snapshot{
		NodeID: "node-a",
		Peers: []overlay.PeerState{
			{
				NodeID:    "node-b",
				OverlayIP: "100.64.0.11",
			},
		},
	}, session.Spec{
		NodeID: "node-a",
		Peers: []session.Peer{
			{
				NodeID:             "node-b",
				PreferredCandidate: "198.51.100.10:51820",
				Candidates: []session.Candidate{
					{Kind: "direct", Address: "198.51.100.10:51820", Priority: 1000},
					{Kind: "relay", Address: "relay-us.example.net:3478", Priority: 500},
				},
			},
		},
	}, Config{
		SessionReport: session.Report{
			NodeID: "node-a",
			Peers: []session.PeerReport{
				{
					NodeID:            "node-b",
					SelectedCandidate: "203.0.113.10:51820",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("build dataplane spec: %v", err)
	}
	if len(spec.Routes) != 1 {
		t.Fatalf("expected a single host route, got %#v", spec.Routes)
	}
	if spec.Routes[0].CandidateAddress != "198.51.100.10:51820" {
		t.Fatalf("expected fallback to preferred direct candidate, got %#v", spec.Routes[0])
	}
	if spec.Routes[0].CandidateKind != "direct" {
		t.Fatalf("expected direct candidate kind after fallback, got %#v", spec.Routes[0])
	}
}

func TestEngineSendAndReceiveOverUDP(t *testing.T) {
	transportA, err := ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() {
		_ = transportA.Close()
	}()

	transportB, err := ListenUDP("127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen transport B: %v", err)
	}
	defer func() {
		_ = transportB.Close()
	}()

	engineA := NewEngine(Spec{
		NodeID: "node-a",
		Routes: []Route{
			{
				NetworkCIDR:      "100.64.0.11/32",
				PrefixBits:       32,
				PeerNodeID:       "node-b",
				CandidateAddress: transportB.Address(),
				CandidateKind:    "direct",
				RoutePriority:    1000,
			},
		},
	}, transportA, memorySink{received: make(chan InboundPacket, 1)})

	sinkB := memorySink{received: make(chan InboundPacket, 1)}
	engineB := NewEngine(Spec{
		NodeID: "node-b",
		Routes: []Route{
			{
				NetworkCIDR:      "100.64.0.10/32",
				PrefixBits:       32,
				PeerNodeID:       "node-a",
				CandidateAddress: transportA.Address(),
				CandidateKind:    "direct",
				RoutePriority:    1000,
			},
		},
	}, transportB, sinkB)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- engineB.Serve(ctx)
	}()

	if err := engineA.SendPacket(context.Background(), "100.64.0.11", []byte("hello-node-b")); err != nil {
		t.Fatalf("send packet: %v", err)
	}

	select {
	case packet := <-sinkB.received:
		if packet.SourceNodeID != "node-a" {
			t.Fatalf("unexpected source node %q", packet.SourceNodeID)
		}
		if packet.DestinationIP != "100.64.0.11" {
			t.Fatalf("unexpected destination ip %q", packet.DestinationIP)
		}
		if string(packet.Payload) != "hello-node-b" {
			t.Fatalf("unexpected payload %q", string(packet.Payload))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for inbound packet")
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("engine serve: %v", err)
	}
}
