package relay

import (
	"context"
	"net"
	"testing"
	"time"

	"nodeweave/packages/runtime/go/dataplane"
	"nodeweave/packages/runtime/go/secureudp"
	"nodeweave/packages/runtime/go/session"
)

type sink struct {
	received chan dataplane.InboundPacket
}

func (s sink) HandleInbound(_ context.Context, packet dataplane.InboundPacket) error {
	s.received <- packet
	return nil
}

func TestRelayForwardsSecureUDPPackets(t *testing.T) {
	relayService, err := Listen(Config{
		ListenAddress: "127.0.0.1:0",
		MappingTTL:    time.Minute,
	})
	if err != nil {
		t.Fatalf("listen relay: %v", err)
	}
	defer func() {
		_ = relayService.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	relayErrCh := make(chan error, 1)
	go func() { relayErrCh <- relayService.Serve(ctx) }()

	privateKeyA, publicKeyA, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	privateKeyB, publicKeyB, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair B: %v", err)
	}

	transportA, err := secureudp.Listen(secureudp.Config{
		NodeID:         "node-a",
		ListenAddress:  "127.0.0.1:0",
		PrivateKey:     privateKeyA,
		Peers:          []session.Peer{{NodeID: "node-b", PublicKey: publicKeyB}},
		RelayAddresses: []string{relayService.Address()},
	})
	if err != nil {
		t.Fatalf("listen secure transport A: %v", err)
	}
	defer func() {
		_ = transportA.Close()
	}()

	transportB, err := secureudp.Listen(secureudp.Config{
		NodeID:         "node-b",
		ListenAddress:  "127.0.0.1:0",
		PrivateKey:     privateKeyB,
		Peers:          []session.Peer{{NodeID: "node-a", PublicKey: publicKeyA}},
		RelayAddresses: []string{relayService.Address()},
	})
	if err != nil {
		t.Fatalf("listen secure transport B: %v", err)
	}
	defer func() {
		_ = transportB.Close()
	}()

	if err := transportA.Announce(context.Background(), relayService.Address()); err != nil {
		t.Fatalf("announce A: %v", err)
	}
	if err := transportB.Announce(context.Background(), relayService.Address()); err != nil {
		t.Fatalf("announce B: %v", err)
	}

	engineA := dataplane.NewEngine(dataplane.Spec{
		NodeID: "node-a",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.11/32",
				PrefixBits:       32,
				PeerNodeID:       "node-b",
				CandidateAddress: relayService.Address(),
				CandidateKind:    "relay",
			},
		},
	}, transportA, sink{received: make(chan dataplane.InboundPacket, 1)})

	sinkB := sink{received: make(chan dataplane.InboundPacket, 1)}
	engineB := dataplane.NewEngine(dataplane.Spec{
		NodeID: "node-b",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.10/32",
				PrefixBits:       32,
				PeerNodeID:       "node-a",
				CandidateAddress: relayService.Address(),
				CandidateKind:    "relay",
			},
		},
	}, transportB, sinkB)

	engineErrCh := make(chan error, 2)
	go func() { engineErrCh <- engineA.Serve(ctx) }()
	go func() { engineErrCh <- engineB.Serve(ctx) }()

	if err := engineA.SendPacket(context.Background(), "100.64.0.11", []byte("hello-over-relay")); err != nil {
		t.Fatalf("send packet via relay: %v", err)
	}

	select {
	case packet := <-sinkB.received:
		if packet.SourceNodeID != "node-a" {
			t.Fatalf("unexpected source node %q", packet.SourceNodeID)
		}
		if string(packet.Payload) != "hello-over-relay" {
			t.Fatalf("unexpected payload %q", string(packet.Payload))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for relay forwarded packet")
	}

	cancel()
	if err := <-relayErrCh; err != nil {
		t.Fatalf("relay serve: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := <-engineErrCh; err != nil {
			t.Fatalf("engine serve: %v", err)
		}
	}
}

func TestSecureUDPTransportFallsBackToRelayWhenDirectHandshakeFails(t *testing.T) {
	relayService, err := Listen(Config{
		ListenAddress: "127.0.0.1:0",
		MappingTTL:    time.Minute,
	})
	if err != nil {
		t.Fatalf("listen relay: %v", err)
	}
	defer func() {
		_ = relayService.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	relayErrCh := make(chan error, 1)
	go func() { relayErrCh <- relayService.Serve(ctx) }()

	privateKeyA, publicKeyA, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	privateKeyB, publicKeyB, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair B: %v", err)
	}

	deadDirect := "127.0.0.1:1"

	transportA, err := secureudp.Listen(secureudp.Config{
		NodeID:           "node-a",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyA,
		HandshakeTimeout: 150 * time.Millisecond,
		Peers: []session.Peer{
			{
				NodeID:    "node-b",
				PublicKey: publicKeyB,
				Candidates: []session.Candidate{
					{Kind: "direct", Address: deadDirect, Priority: 1000},
					{Kind: "relay", Address: relayService.Address(), Priority: 500},
				},
			},
		},
		RelayAddresses: []string{relayService.Address()},
	})
	if err != nil {
		t.Fatalf("listen secure transport A: %v", err)
	}
	defer func() {
		_ = transportA.Close()
	}()

	transportB, err := secureudp.Listen(secureudp.Config{
		NodeID:           "node-b",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyB,
		HandshakeTimeout: 150 * time.Millisecond,
		Peers: []session.Peer{
			{
				NodeID:    "node-a",
				PublicKey: publicKeyA,
				Candidates: []session.Candidate{
					{Kind: "relay", Address: relayService.Address(), Priority: 500},
				},
			},
		},
		RelayAddresses: []string{relayService.Address()},
	})
	if err != nil {
		t.Fatalf("listen secure transport B: %v", err)
	}
	defer func() {
		_ = transportB.Close()
	}()

	if err := transportB.Announce(context.Background(), relayService.Address()); err != nil {
		t.Fatalf("announce B: %v", err)
	}

	engineA := dataplane.NewEngine(dataplane.Spec{
		NodeID: "node-a",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.11/32",
				PrefixBits:       32,
				PeerNodeID:       "node-b",
				CandidateAddress: deadDirect,
				CandidateKind:    "direct",
			},
		},
	}, transportA, sink{received: make(chan dataplane.InboundPacket, 1)})

	sinkB := sink{received: make(chan dataplane.InboundPacket, 2)}
	engineB := dataplane.NewEngine(dataplane.Spec{
		NodeID: "node-b",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.10/32",
				PrefixBits:       32,
				PeerNodeID:       "node-a",
				CandidateAddress: relayService.Address(),
				CandidateKind:    "relay",
			},
		},
	}, transportB, sinkB)

	engineErrCh := make(chan error, 2)
	go func() { engineErrCh <- engineA.Serve(ctx) }()
	go func() { engineErrCh <- engineB.Serve(ctx) }()

	if err := engineA.SendPacket(context.Background(), "100.64.0.11", []byte("fallback-over-relay")); err != nil {
		t.Fatalf("send packet with relay fallback: %v", err)
	}

	select {
	case packet := <-sinkB.received:
		if packet.SourceNodeID != "node-a" {
			t.Fatalf("unexpected source node %q", packet.SourceNodeID)
		}
		if string(packet.Payload) != "fallback-over-relay" {
			t.Fatalf("unexpected payload %q", string(packet.Payload))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for relay fallback packet")
	}

	cancel()
	if err := <-relayErrCh; err != nil {
		t.Fatalf("relay serve: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := <-engineErrCh; err != nil {
			t.Fatalf("engine serve: %v", err)
		}
	}
}

func TestSecureUDPTransportRecoversDirectAfterRelayFallback(t *testing.T) {
	relayService, err := Listen(Config{
		ListenAddress: "127.0.0.1:0",
		MappingTTL:    time.Minute,
	})
	if err != nil {
		t.Fatalf("listen relay: %v", err)
	}
	defer func() {
		_ = relayService.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	relayErrCh := make(chan error, 1)
	go func() { relayErrCh <- relayService.Serve(ctx) }()

	privateKeyA, publicKeyA, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	privateKeyB, publicKeyB, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair B: %v", err)
	}

	blackhole, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen blackhole direct socket: %v", err)
	}
	directAddress := blackhole.LocalAddr().String()

	transportA, err := secureudp.Listen(secureudp.Config{
		NodeID:           "node-a",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyA,
		HandshakeTimeout: 150 * time.Millisecond,
		DirectRetryAfter: 50 * time.Millisecond,
		Peers: []session.Peer{
			{
				NodeID:    "node-b",
				PublicKey: publicKeyB,
				Candidates: []session.Candidate{
					{Kind: "direct", Address: directAddress, Priority: 1000},
					{Kind: "relay", Address: relayService.Address(), Priority: 500},
				},
			},
		},
		RelayAddresses: []string{relayService.Address()},
	})
	if err != nil {
		t.Fatalf("listen secure transport A: %v", err)
	}
	defer func() {
		_ = transportA.Close()
	}()

	transportBRelay, err := secureudp.Listen(secureudp.Config{
		NodeID:           "node-b",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyB,
		HandshakeTimeout: 150 * time.Millisecond,
		Peers: []session.Peer{
			{
				NodeID:    "node-a",
				PublicKey: publicKeyA,
				Candidates: []session.Candidate{
					{Kind: "relay", Address: relayService.Address(), Priority: 500},
				},
			},
		},
		RelayAddresses: []string{relayService.Address()},
	})
	if err != nil {
		t.Fatalf("listen secure relay transport B: %v", err)
	}
	defer func() {
		_ = transportBRelay.Close()
	}()

	if err := transportBRelay.Announce(context.Background(), relayService.Address()); err != nil {
		t.Fatalf("announce B relay endpoint: %v", err)
	}

	sinkRelay := sink{received: make(chan dataplane.InboundPacket, 2)}
	engineA := dataplane.NewEngine(dataplane.Spec{
		NodeID: "node-a",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.11/32",
				PrefixBits:       32,
				PeerNodeID:       "node-b",
				CandidateAddress: directAddress,
				CandidateKind:    "direct",
			},
		},
	}, transportA, sink{received: make(chan dataplane.InboundPacket, 1)})
	engineBRelay := dataplane.NewEngine(dataplane.Spec{
		NodeID: "node-b",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.10/32",
				PrefixBits:       32,
				PeerNodeID:       "node-a",
				CandidateAddress: relayService.Address(),
				CandidateKind:    "relay",
			},
		},
	}, transportBRelay, sinkRelay)

	engineErrCh := make(chan error, 3)
	go func() { engineErrCh <- engineA.Serve(ctx) }()
	go func() { engineErrCh <- engineBRelay.Serve(ctx) }()

	if err := engineA.SendPacket(context.Background(), "100.64.0.11", []byte("first-over-relay")); err != nil {
		t.Fatalf("send first packet with relay fallback: %v", err)
	}

	select {
	case packet := <-sinkRelay.received:
		if string(packet.Payload) != "first-over-relay" {
			t.Fatalf("unexpected relay payload %q", string(packet.Payload))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial relay fallback packet")
	}

	if err := blackhole.Close(); err != nil {
		t.Fatalf("close blackhole direct socket: %v", err)
	}

	transportBDirect, err := secureudp.Listen(secureudp.Config{
		NodeID:           "node-b",
		ListenAddress:    directAddress,
		PrivateKey:       privateKeyB,
		HandshakeTimeout: 150 * time.Millisecond,
		Peers: []session.Peer{
			{
				NodeID:    "node-a",
				PublicKey: publicKeyA,
				Candidates: []session.Candidate{
					{Kind: "direct", Address: transportA.Address(), Priority: 1000},
					{Kind: "relay", Address: relayService.Address(), Priority: 500},
				},
			},
		},
		RelayAddresses: []string{relayService.Address()},
	})
	if err != nil {
		t.Fatalf("listen secure direct transport B: %v", err)
	}
	defer func() {
		_ = transportBDirect.Close()
	}()

	sinkDirect := sink{received: make(chan dataplane.InboundPacket, 2)}
	engineBDirect := dataplane.NewEngine(dataplane.Spec{
		NodeID: "node-b",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.10/32",
				PrefixBits:       32,
				PeerNodeID:       "node-a",
				CandidateAddress: transportA.Address(),
				CandidateKind:    "direct",
			},
		},
	}, transportBDirect, sinkDirect)
	go func() { engineErrCh <- engineBDirect.Serve(ctx) }()

	time.Sleep(70 * time.Millisecond)

	if err := engineA.SendPacket(context.Background(), "100.64.0.11", []byte("second-over-direct")); err != nil {
		t.Fatalf("send second packet with direct recovery: %v", err)
	}

	select {
	case packet := <-sinkDirect.received:
		if string(packet.Payload) != "second-over-direct" {
			t.Fatalf("unexpected direct payload %q", string(packet.Payload))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for direct recovery packet")
	}

	select {
	case packet := <-sinkRelay.received:
		t.Fatalf("expected recovered direct path, got extra relay packet %#v", packet)
	case <-time.After(300 * time.Millisecond):
	}

	cancel()
	if err := <-relayErrCh; err != nil {
		t.Fatalf("relay serve: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := <-engineErrCh; err != nil {
			t.Fatalf("engine serve: %v", err)
		}
	}
}
