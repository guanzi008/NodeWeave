package session

import (
	"context"
	"testing"
	"time"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/packages/runtime/go/overlay"
)

func TestBuildPrefersDirectEndpoints(t *testing.T) {
	spec := Build(overlay.Snapshot{
		NodeID: "node-a",
		Peers: []overlay.PeerState{
			{
				NodeID:      "node-b",
				OverlayIP:   "100.64.0.11",
				PublicKey:   "pubkey-b",
				Endpoints:   []string{"198.51.100.10:51820"},
				RelayRegion: "ap",
			},
			{
				NodeID:      "node-c",
				OverlayIP:   "100.64.0.12",
				PublicKey:   "pubkey-c",
				RelayRegion: "us",
			},
		},
		Relays: []api.RelayNode{
			{Region: "ap", Address: "relay-ap.example.net:3478"},
			{Region: "us", Address: "relay-us.example.net:3478"},
		},
	}, Config{ListenAddress: "0.0.0.0:51820"})

	if len(spec.Peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(spec.Peers))
	}
	if spec.ListenAddress != "0.0.0.0:51820" {
		t.Fatalf("unexpected listen address %q", spec.ListenAddress)
	}
	if spec.Peers[0].PreferredCandidate != "198.51.100.10:51820" {
		t.Fatalf("expected direct endpoint preferred, got %q", spec.Peers[0].PreferredCandidate)
	}
	if spec.Peers[1].PreferredCandidate != "relay-us.example.net:3478" {
		t.Fatalf("expected same-region relay preferred, got %q", spec.Peers[1].PreferredCandidate)
	}
}

func TestBuildPrefersRecentSTUNEndpointRecords(t *testing.T) {
	now := time.Now().UTC()
	spec := Build(overlay.Snapshot{
		NodeID: "node-a",
		Peers: []overlay.PeerState{
			{
				NodeID:    "node-b",
				OverlayIP: "100.64.0.11",
				PublicKey: "pubkey-b",
				Endpoints: []string{
					"198.51.100.10:51820",
					"203.0.113.10:51820",
				},
				EndpointRecords: []api.EndpointObservation{
					{
						Address:    "198.51.100.10:51820",
						Source:     "static",
						ObservedAt: now.Add(-5 * time.Minute),
					},
					{
						Address:    "203.0.113.10:51820",
						Source:     "stun",
						ObservedAt: now,
					},
				},
			},
		},
	}, Config{})

	if len(spec.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(spec.Peers))
	}
	if spec.Peers[0].PreferredCandidate != "203.0.113.10:51820" {
		t.Fatalf("expected recent stun endpoint preferred, got %q", spec.Peers[0].PreferredCandidate)
	}
	if len(spec.Peers[0].Candidates) < 2 || spec.Peers[0].Candidates[1].Address != "198.51.100.10:51820" {
		t.Fatalf("expected static endpoint to stay as lower-priority fallback, got %#v", spec.Peers[0].Candidates)
	}
}

func TestFreshDirectCandidatesFiltersStaleAndKeepsSourceOrdering(t *testing.T) {
	now := time.Now().UTC()
	candidates := FreshDirectCandidates(now, nil, []api.EndpointObservation{
		{
			Address:    "198.51.100.10:51820",
			Source:     "static",
			ObservedAt: now.Add(-10 * time.Second),
		},
		{
			Address:    "203.0.113.10:51820",
			Source:     "stun",
			ObservedAt: now,
		},
		{
			Address:    "203.0.113.20:51820",
			Source:     "stun",
			ObservedAt: now.Add(-2 * time.Minute),
		},
	}, 45*time.Second)

	if len(candidates) != 2 {
		t.Fatalf("expected 2 fresh direct candidates, got %#v", candidates)
	}
	if candidates[0].Address != "203.0.113.10:51820" {
		t.Fatalf("expected freshest stun candidate first, got %#v", candidates)
	}
	if candidates[1].Address != "198.51.100.10:51820" {
		t.Fatalf("expected fresh static candidate second, got %#v", candidates)
	}
}

func TestProbeUDPResponder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	responder, err := NewResponder("127.0.0.1:0", "node-b")
	if err != nil {
		t.Fatalf("new responder: %v", err)
	}
	defer func() {
		_ = responder.Close()
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- responder.Serve(ctx)
	}()

	spec := Spec{
		NodeID:        "node-a",
		ListenAddress: "127.0.0.1:0",
		Peers: []Peer{
			{
				NodeID:             "node-b",
				PreferredCandidate: responder.Address(),
				Candidates: []Candidate{
					{
						Kind:     "direct",
						Address:  responder.Address(),
						Priority: 1000,
					},
				},
			},
		},
	}

	report, err := Probe(context.Background(), spec, ProbeConfig{
		Mode:    "udp",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if len(report.Peers) != 1 || len(report.Peers[0].Candidates) != 1 {
		t.Fatalf("unexpected probe report: %#v", report)
	}
	if !report.Peers[0].Reachable {
		t.Fatalf("expected peer to be reachable: %#v", report.Peers[0])
	}
	if report.Peers[0].Candidates[0].Status != "reachable" {
		t.Fatalf("expected reachable status, got %q", report.Peers[0].Candidates[0].Status)
	}
	if report.Peers[0].Candidates[0].RespondedBy != "node-b" {
		t.Fatalf("expected responder node-b, got %q", report.Peers[0].Candidates[0].RespondedBy)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("responder serve: %v", err)
	}
}

func TestProbeOffMarksCandidatesDisabled(t *testing.T) {
	report, err := Probe(context.Background(), Spec{
		NodeID: "node-a",
		Peers: []Peer{
			{
				NodeID:             "node-b",
				PreferredCandidate: "198.51.100.10:51820",
				Candidates: []Candidate{
					{Kind: "direct", Address: "198.51.100.10:51820", Priority: 1000},
				},
			},
		},
	}, ProbeConfig{Mode: "off"})
	if err != nil {
		t.Fatalf("probe off: %v", err)
	}
	if report.Peers[0].Candidates[0].Status != "disabled" {
		t.Fatalf("expected disabled candidate, got %q", report.Peers[0].Candidates[0].Status)
	}
	if report.Peers[0].SelectedCandidate != "198.51.100.10:51820" {
		t.Fatalf("expected preferred candidate to stay selected, got %q", report.Peers[0].SelectedCandidate)
	}
}
