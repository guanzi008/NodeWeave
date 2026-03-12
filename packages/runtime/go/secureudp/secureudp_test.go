package secureudp

import (
	"context"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"nodeweave/packages/runtime/go/dataplane"
	"nodeweave/packages/runtime/go/session"
)

type testSink struct {
	received chan dataplane.InboundPacket
}

func (s testSink) HandleInbound(_ context.Context, packet dataplane.InboundPacket) error {
	s.received <- packet
	return nil
}

func TestTransportHandshakeAndEncryptedFrameDelivery(t *testing.T) {
	privateKeyA, publicKeyA, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	privateKeyB, publicKeyB, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair B: %v", err)
	}

	transportA, err := Listen(Config{
		NodeID:           "node-a",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyA,
		HandshakeTimeout: 500 * time.Millisecond,
		Peers: []session.Peer{
			{NodeID: "node-b", PublicKey: publicKeyB},
		},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() {
		_ = transportA.Close()
	}()

	transportB, err := Listen(Config{
		NodeID:           "node-b",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyB,
		HandshakeTimeout: 500 * time.Millisecond,
		Peers: []session.Peer{
			{NodeID: "node-a", PublicKey: publicKeyA},
		},
	})
	if err != nil {
		t.Fatalf("listen transport B: %v", err)
	}
	defer func() {
		_ = transportB.Close()
	}()

	sinkA := testSink{received: make(chan dataplane.InboundPacket, 1)}
	sinkB := testSink{received: make(chan dataplane.InboundPacket, 1)}

	engineA := dataplane.NewEngine(dataplane.Spec{
		NodeID: "node-a",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.11/32",
				PrefixBits:       32,
				PeerNodeID:       "node-b",
				CandidateAddress: transportB.Address(),
			},
		},
	}, transportA, sinkA)
	engineB := dataplane.NewEngine(dataplane.Spec{
		NodeID: "node-b",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.10/32",
				PrefixBits:       32,
				PeerNodeID:       "node-a",
				CandidateAddress: transportA.Address(),
			},
		},
	}, transportB, sinkB)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 2)
	go func() { errCh <- engineA.Serve(ctx) }()
	go func() { errCh <- engineB.Serve(ctx) }()

	if err := engineA.SendPacket(context.Background(), "100.64.0.11", []byte("hello-secure-node-b")); err != nil {
		t.Fatalf("send secure packet: %v", err)
	}

	select {
	case packet := <-sinkB.received:
		if packet.SourceNodeID != "node-a" {
			t.Fatalf("unexpected source node %q", packet.SourceNodeID)
		}
		if packet.DestinationIP != "100.64.0.11" {
			t.Fatalf("unexpected destination ip %q", packet.DestinationIP)
		}
		if string(packet.Payload) != "hello-secure-node-b" {
			t.Fatalf("unexpected payload %q", string(packet.Payload))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for secure packet")
	}

	reportA := transportA.Snapshot()
	if len(reportA.Peers) != 1 {
		t.Fatalf("expected one peer in report A, got %#v", reportA.Peers)
	}
	if reportA.Peers[0].SentPackets != 1 || reportA.Peers[0].SentBytes != int64(len("hello-secure-node-b")) {
		t.Fatalf("unexpected send counters in report A: %#v", reportA.Peers[0])
	}
	if reportA.Peers[0].SessionsEstablished == 0 {
		t.Fatalf("expected session establishment to be counted in report A: %#v", reportA.Peers[0])
	}
	if reportA.Peers[0].ActiveAddress != transportB.Address() {
		t.Fatalf("unexpected active path in report A: %#v", reportA.Peers[0])
	}
	if reportA.Peers[0].ActiveKind == "relay" {
		t.Fatalf("expected non-relay active path in report A: %#v", reportA.Peers[0])
	}

	reportB := transportB.Snapshot()
	if len(reportB.Peers) != 1 {
		t.Fatalf("expected one peer in report B, got %#v", reportB.Peers)
	}
	if reportB.Peers[0].ReceivedPackets != 1 || reportB.Peers[0].ReceivedBytes != int64(len("hello-secure-node-b")) {
		t.Fatalf("unexpected receive counters in report B: %#v", reportB.Peers[0])
	}
	if reportB.Peers[0].LastReceiveAt.IsZero() {
		t.Fatalf("expected last receive timestamp in report B: %#v", reportB.Peers[0])
	}

	cancel()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("engine exited with error: %v", err)
		}
	}
}

func TestRecordDirectAttemptResultTracksFailureBudget(t *testing.T) {
	transport := &Transport{
		peerStats: map[string]peerMetrics{},
	}
	attemptedAt := time.Now().UTC()

	transport.recordDirectAttemptResult("node-b", "attempt-1", "relay_active", attemptedAt, "timeout")
	transport.recordDirectAttemptResult("node-b", "attempt-2", "manual_recover", attemptedAt.Add(1*time.Second), "relay_kept")

	report := transport.Snapshot()
	if len(report.Peers) != 1 {
		t.Fatalf("expected one peer in snapshot, got %#v", report.Peers)
	}
	if report.Peers[0].ConsecutiveDirectFailures != 2 {
		t.Fatalf("expected consecutive direct failures to reach 2, got %#v", report.Peers[0])
	}

	successAt := attemptedAt.Add(2 * time.Second)
	transport.recordDirectAttemptResult("node-b", "attempt-3", "manual_recover", successAt, "success")

	report = transport.Snapshot()
	if report.Peers[0].ConsecutiveDirectFailures != 0 {
		t.Fatalf("expected success to reset consecutive failures, got %#v", report.Peers[0])
	}
	if !report.Peers[0].LastDirectSuccessAt.Equal(successAt) {
		t.Fatalf("expected last direct success time %s, got %#v", successAt, report.Peers[0])
	}
}

func TestTransportSendFailsWhenPeerKeyMismatch(t *testing.T) {
	privateKeyA, publicKeyA, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	privateKeyB, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair B: %v", err)
	}
	_, wrongPublicKeyB, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate wrong key pair B: %v", err)
	}

	transportA, err := Listen(Config{
		NodeID:           "node-a",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyA,
		HandshakeTimeout: 150 * time.Millisecond,
		Peers: []session.Peer{
			{NodeID: "node-b", PublicKey: wrongPublicKeyB},
		},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() {
		_ = transportA.Close()
	}()

	transportB, err := Listen(Config{
		NodeID:           "node-b",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyB,
		HandshakeTimeout: 150 * time.Millisecond,
		Peers: []session.Peer{
			{NodeID: "node-a", PublicKey: publicKeyA},
		},
	})
	if err != nil {
		t.Fatalf("listen transport B: %v", err)
	}
	defer func() {
		_ = transportB.Close()
	}()

	sinkA := testSink{received: make(chan dataplane.InboundPacket, 1)}
	sinkB := testSink{received: make(chan dataplane.InboundPacket, 1)}

	engineA := dataplane.NewEngine(dataplane.Spec{
		NodeID: "node-a",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.11/32",
				PrefixBits:       32,
				PeerNodeID:       "node-b",
				CandidateAddress: transportB.Address(),
			},
		},
	}, transportA, sinkA)
	engineB := dataplane.NewEngine(dataplane.Spec{
		NodeID: "node-b",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.10/32",
				PrefixBits:       32,
				PeerNodeID:       "node-a",
				CandidateAddress: transportA.Address(),
			},
		},
	}, transportB, sinkB)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 2)
	go func() { errCh <- engineA.Serve(ctx) }()
	go func() { errCh <- engineB.Serve(ctx) }()

	err = engineA.SendPacket(context.Background(), "100.64.0.11", []byte("should-timeout"))
	if err == nil || !strings.Contains(err.Error(), "handshake timeout") {
		t.Fatalf("expected handshake timeout, got %v", err)
	}

	reportA := transportA.Snapshot()
	if len(reportA.Peers) != 1 {
		t.Fatalf("expected one peer in mismatch report, got %#v", reportA.Peers)
	}
	if reportA.Peers[0].HandshakeTimeouts != 1 {
		t.Fatalf("expected a single handshake timeout, got %#v", reportA.Peers[0])
	}
	if !strings.Contains(reportA.Peers[0].LastSendError, "handshake timeout") {
		t.Fatalf("expected last send error to record timeout, got %#v", reportA.Peers[0])
	}
	if reportA.Peers[0].SentPackets != 0 {
		t.Fatalf("expected no successful sends after mismatch, got %#v", reportA.Peers[0])
	}

	cancel()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("engine exited with error: %v", err)
		}
	}
}

func TestTransportDropsReplayEnvelope(t *testing.T) {
	privateKeyA, publicKeyA, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	privateKeyB, publicKeyB, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair B: %v", err)
	}

	transportA, err := Listen(Config{
		NodeID:        "node-a",
		ListenAddress: "127.0.0.1:0",
		PrivateKey:    privateKeyA,
		Peers: []session.Peer{
			{NodeID: "node-b", PublicKey: publicKeyB},
		},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() {
		_ = transportA.Close()
	}()

	transportB, err := Listen(Config{
		NodeID:        "node-b",
		ListenAddress: "127.0.0.1:0",
		PrivateKey:    privateKeyB,
		Peers: []session.Peer{
			{NodeID: "node-a", PublicKey: publicKeyA},
		},
	})
	if err != nil {
		t.Fatalf("listen transport B: %v", err)
	}
	defer func() {
		_ = transportB.Close()
	}()

	frame := dataplane.Frame{
		Type:          "packet",
		SourceNodeID:  "node-a",
		TargetNodeID:  "node-b",
		DestinationIP: "100.64.0.11",
		Payload:       []byte("deliver-once"),
		SentAt:        time.Now().UTC(),
	}
	env, err := transportA.encryptEnvelope(envelopeTypeData, "node-b", frame)
	if err != nil {
		t.Fatalf("encrypt envelope: %v", err)
	}

	received := make(chan dataplane.Frame, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- transportB.Serve(ctx, func(_ context.Context, frame dataplane.Frame, _ net.Addr) error {
			received <- frame
			return nil
		})
	}()

	if err := transportA.writeEnvelope(context.Background(), transportB.Address(), env); err != nil {
		t.Fatalf("write envelope first time: %v", err)
	}
	if err := transportA.writeEnvelope(context.Background(), transportB.Address(), env); err != nil {
		t.Fatalf("write envelope second time: %v", err)
	}

	select {
	case frame := <-received:
		if string(frame.Payload) != "deliver-once" {
			t.Fatalf("unexpected payload %q", string(frame.Payload))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for replay test frame")
	}

	select {
	case frame := <-received:
		t.Fatalf("expected replay to be dropped, got second frame %#v", frame)
	case <-time.After(300 * time.Millisecond):
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("transport serve: %v", err)
	}
}

func TestWarmupPeerEstablishesDirectSession(t *testing.T) {
	privateKeyA, publicKeyA, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	privateKeyB, publicKeyB, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair B: %v", err)
	}

	transportA, err := Listen(Config{
		NodeID:           "node-a",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyA,
		HandshakeTimeout: 500 * time.Millisecond,
		Peers: []session.Peer{
			{
				NodeID:    "node-b",
				PublicKey: publicKeyB,
				Candidates: []session.Candidate{
					{Kind: "direct", Address: "127.0.0.1:1", Priority: 1000},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() { _ = transportA.Close() }()

	transportB, err := Listen(Config{
		NodeID:           "node-b",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyB,
		HandshakeTimeout: 500 * time.Millisecond,
		Peers: []session.Peer{
			{
				NodeID:    "node-a",
				PublicKey: publicKeyA,
				Candidates: []session.Candidate{
					{Kind: "direct", Address: transportA.Address(), Priority: 1000},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("listen transport B: %v", err)
	}
	defer func() { _ = transportB.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 2)
	go func() {
		errCh <- transportA.Serve(ctx, func(context.Context, dataplane.Frame, net.Addr) error { return nil })
	}()
	go func() {
		errCh <- transportB.Serve(ctx, func(context.Context, dataplane.Frame, net.Addr) error { return nil })
	}()

	report := transportA.WarmupPeer(context.Background(), "node-b", []string{transportB.Address()})
	if !report.Reachable || len(report.Results) != 1 || report.Results[0].Status != "reachable" {
		t.Fatalf("unexpected warmup report %#v", report)
	}

	snapshot := transportA.Snapshot()
	if len(snapshot.Peers) != 1 {
		t.Fatalf("expected one peer in snapshot, got %#v", snapshot.Peers)
	}
	if snapshot.Peers[0].ActiveAddress != transportB.Address() {
		t.Fatalf("expected active direct address %q, got %#v", transportB.Address(), snapshot.Peers[0])
	}
	if snapshot.Peers[0].SessionsEstablished == 0 {
		t.Fatalf("expected session establishment count after warmup, got %#v", snapshot.Peers[0])
	}

	cancel()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("transport serve: %v", err)
		}
	}
}

func TestWarmupPeerRetriesHelloBurstBeforeTimeout(t *testing.T) {
	privateKeyA, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	_, fakePublicKeyB, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate fake key pair B: %v", err)
	}

	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen raw udp server: %v", err)
	}
	defer func() { _ = listener.Close() }()

	transportA, err := Listen(Config{
		NodeID:                 "node-a",
		ListenAddress:          "127.0.0.1:0",
		PrivateKey:             privateKeyA,
		HandshakeTimeout:       220 * time.Millisecond,
		HandshakeRetryInterval: 40 * time.Millisecond,
		Peers: []session.Peer{
			{
				NodeID:    "node-b",
				PublicKey: fakePublicKeyB,
				Candidates: []session.Candidate{
					{Kind: "direct", Address: listener.LocalAddr().String(), Priority: 1000},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() { _ = transportA.Close() }()

	packetCount := 0
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buffer := make([]byte, 2048)
		_ = listener.SetDeadline(time.Now().Add(400 * time.Millisecond))
		for {
			if _, _, err := listener.ReadFrom(buffer); err != nil {
				return
			}
			packetCount++
		}
	}()

	report := transportA.WarmupPeer(context.Background(), "node-b", []string{listener.LocalAddr().String()})
	if report.Reachable {
		t.Fatalf("expected warmup to fail without responder, got %#v", report)
	}
	if len(report.Results) != 1 || !strings.Contains(report.Results[0].Error, "handshake timeout") {
		t.Fatalf("expected handshake timeout report, got %#v", report)
	}

	<-readDone
	if packetCount < 2 {
		t.Fatalf("expected multiple hello attempts before timeout, got %d packet(s)", packetCount)
	}
}

func TestWarmupPeerFailureUpdatesNextDirectRetryAt(t *testing.T) {
	privateKeyA, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	_, fakePublicKeyB, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate fake key pair B: %v", err)
	}

	transportA, err := Listen(Config{
		NodeID:                 "node-a",
		ListenAddress:          "127.0.0.1:0",
		PrivateKey:             privateKeyA,
		HandshakeTimeout:       120 * time.Millisecond,
		HandshakeRetryInterval: 40 * time.Millisecond,
		DirectRetryAfter:       500 * time.Millisecond,
		Peers: []session.Peer{
			{
				NodeID:    "node-b",
				PublicKey: fakePublicKeyB,
				Candidates: []session.Candidate{
					{Kind: "direct", Address: "127.0.0.1:1", Priority: 1000},
					{Kind: "relay", Address: "relay.example.net:3478", Priority: 500},
				},
			},
		},
		RelayAddresses: []string{"relay.example.net:3478"},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() { _ = transportA.Close() }()

	transportA.markEstablished("node-b", "relay.example.net:3478")

	report := transportA.WarmupPeer(context.Background(), "node-b", []string{"127.0.0.1:1"})
	if report.Reachable {
		t.Fatalf("expected warmup to fail without responder, got %#v", report)
	}

	snapshot := transportA.Snapshot()
	if len(snapshot.Peers) != 1 {
		t.Fatalf("expected one peer in snapshot, got %#v", snapshot.Peers)
	}
	peer := snapshot.Peers[0]
	if peer.ActiveKind != "relay" {
		t.Fatalf("expected relay to remain active, got %#v", peer)
	}
	if peer.LastDirectTryAt.IsZero() {
		t.Fatalf("expected last direct try timestamp after failed warmup, got %#v", peer)
	}
	if peer.NextDirectRetryAt.IsZero() || !peer.NextDirectRetryAt.After(snapshot.GeneratedAt) {
		t.Fatalf("expected future next direct retry timestamp after failed warmup, got %#v", peer)
	}
}

func TestExecuteDirectAttemptEstablishesDirectPath(t *testing.T) {
	privateKeyA, publicKeyA, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	privateKeyB, publicKeyB, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair B: %v", err)
	}

	transportA, err := Listen(Config{
		NodeID:           "node-a",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyA,
		HandshakeTimeout: 400 * time.Millisecond,
		Peers: []session.Peer{
			{NodeID: "node-b", PublicKey: publicKeyB, Candidates: []session.Candidate{{Kind: "direct", Address: "127.0.0.1:1", Priority: 1000}}},
		},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() { _ = transportA.Close() }()

	transportB, err := Listen(Config{
		NodeID:           "node-b",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyB,
		HandshakeTimeout: 400 * time.Millisecond,
		Peers: []session.Peer{
			{NodeID: "node-a", PublicKey: publicKeyA, Candidates: []session.Candidate{{Kind: "direct", Address: transportA.Address(), Priority: 1000}}},
		},
	})
	if err != nil {
		t.Fatalf("listen transport B: %v", err)
	}
	defer func() { _ = transportB.Close() }()
	transportA.peerCandidates["node-b"][0].Address = transportB.Address()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 2)
	go func() {
		errCh <- transportA.Serve(ctx, func(context.Context, dataplane.Frame, net.Addr) error { return nil })
	}()
	go func() {
		errCh <- transportB.Serve(ctx, func(context.Context, dataplane.Frame, net.Addr) error { return nil })
	}()

	result, err := transportA.ExecuteDirectAttempt(context.Background(), DirectAttempt{
		AttemptID:     "attempt-success",
		PeerNodeID:    "node-b",
		Candidates:    []string{transportB.Address()},
		Window:        400 * time.Millisecond,
		BurstInterval: 50 * time.Millisecond,
		Reason:        "fresh_endpoints",
	})
	if err != nil {
		t.Fatalf("execute direct attempt: %v", err)
	}
	if result.Result != "success" || result.ActiveAddress != transportB.Address() {
		t.Fatalf("expected success over direct address, got %#v", result)
	}

	snapshot := transportA.Snapshot()
	if snapshot.Peers[0].LastDirectAttemptID != "attempt-success" || snapshot.Peers[0].LastDirectAttemptResult != "success" {
		t.Fatalf("expected attempt result in snapshot, got %#v", snapshot.Peers[0])
	}

	cancel()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("transport serve: %v", err)
		}
	}
}

func TestExecuteDirectAttemptKeepsRelayOnTimeout(t *testing.T) {
	privateKeyA, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	_, fakePublicKeyB, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate fake key pair B: %v", err)
	}

	transportA, err := Listen(Config{
		NodeID:                 "node-a",
		ListenAddress:          "127.0.0.1:0",
		PrivateKey:             privateKeyA,
		HandshakeTimeout:       120 * time.Millisecond,
		HandshakeRetryInterval: 40 * time.Millisecond,
		Peers: []session.Peer{
			{
				NodeID:    "node-b",
				PublicKey: fakePublicKeyB,
				Candidates: []session.Candidate{
					{Kind: "direct", Address: "127.0.0.1:1", Priority: 1000},
					{Kind: "relay", Address: "relay.example.net:3478", Priority: 500},
				},
			},
		},
		RelayAddresses: []string{"relay.example.net:3478"},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() { _ = transportA.Close() }()

	transportA.markEstablished("node-b", "relay.example.net:3478")

	result, err := transportA.ExecuteDirectAttempt(context.Background(), DirectAttempt{
		AttemptID:     "attempt-relay-kept",
		PeerNodeID:    "node-b",
		Candidates:    []string{"127.0.0.1:1"},
		Window:        120 * time.Millisecond,
		BurstInterval: 40 * time.Millisecond,
		Reason:        "relay_active",
	})
	if err == nil {
		t.Fatal("expected handshake timeout error")
	}
	if result.Result != "relay_kept" || result.ActiveAddress != "relay.example.net:3478" {
		t.Fatalf("expected relay_kept with relay still active, got %#v", result)
	}
}

func TestExecuteDirectAttemptCancelledBeforeExecution(t *testing.T) {
	privateKeyA, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	_, fakePublicKeyB, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate fake key pair B: %v", err)
	}

	transportA, err := Listen(Config{
		NodeID:        "node-a",
		ListenAddress: "127.0.0.1:0",
		PrivateKey:    privateKeyA,
		Peers: []session.Peer{
			{NodeID: "node-b", PublicKey: fakePublicKeyB, Candidates: []session.Candidate{{Kind: "direct", Address: "127.0.0.1:1", Priority: 1000}}},
		},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() { _ = transportA.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := transportA.ExecuteDirectAttempt(ctx, DirectAttempt{
		AttemptID:  "attempt-cancelled",
		PeerNodeID: "node-b",
		Candidates: []string{"127.0.0.1:1"},
		ExecuteAt:  time.Now().UTC().Add(500 * time.Millisecond),
		Window:     200 * time.Millisecond,
		Reason:     "manual_recover",
	})
	if err == nil {
		t.Fatal("expected cancelled attempt error")
	}
	if result.Result != "cancelled" {
		t.Fatalf("expected cancelled result, got %#v", result)
	}
}

func TestExecuteDirectAttemptStartsShortlyBeforeExecuteAt(t *testing.T) {
	privateKeyA, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	_, fakePublicKeyB, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate fake key pair B: %v", err)
	}

	sniffer, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen sniffer: %v", err)
	}
	defer func() { _ = sniffer.Close() }()

	packetAtCh := make(chan time.Time, 8)
	go func() {
		buffer := make([]byte, 2048)
		for {
			if err := sniffer.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
				return
			}
			if _, _, err := sniffer.ReadFrom(buffer); err != nil {
				return
			}
			select {
			case packetAtCh <- time.Now().UTC():
			default:
			}
		}
	}()

	transportA, err := Listen(Config{
		NodeID:                 "node-a",
		ListenAddress:          "127.0.0.1:0",
		PrivateKey:             privateKeyA,
		HandshakeTimeout:       500 * time.Millisecond,
		HandshakeRetryInterval: 50 * time.Millisecond,
		Peers: []session.Peer{
			{NodeID: "node-b", PublicKey: fakePublicKeyB, Candidates: []session.Candidate{{Kind: "direct", Address: sniffer.LocalAddr().String(), Priority: 1000}}},
		},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() { _ = transportA.Close() }()

	executeAt := time.Now().UTC().Add(220 * time.Millisecond)
	_, err = transportA.ExecuteDirectAttempt(context.Background(), DirectAttempt{
		AttemptID:     "attempt-prewarm-lead",
		PeerNodeID:    "node-b",
		Candidates:    []string{sniffer.LocalAddr().String()},
		ExecuteAt:     executeAt,
		Window:        120 * time.Millisecond,
		BurstInterval: 50 * time.Millisecond,
		Reason:        "manual_recover",
	})
	if err == nil {
		t.Fatal("expected timeout without responder")
	}

	firstPacketAt := time.Time{}
	deadline := time.After(500 * time.Millisecond)
	for firstPacketAt.IsZero() {
		select {
		case firstPacketAt = <-packetAtCh:
		case <-deadline:
			t.Fatal("timed out waiting for prewarm packet")
		}
	}
	if !firstPacketAt.Before(executeAt) {
		t.Fatalf("expected first hello burst before execute_at=%s, got %s", executeAt.Format(time.RFC3339Nano), firstPacketAt.Format(time.RFC3339Nano))
	}
}

func TestTransportSendUsesDirectBurstAcrossCandidates(t *testing.T) {
	privateKeyA, publicKeyA, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	privateKeyB, publicKeyB, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair B: %v", err)
	}

	deadDirect := "127.0.0.1:1"

	transportA, err := Listen(Config{
		NodeID:                 "node-a",
		ListenAddress:          "127.0.0.1:0",
		PrivateKey:             privateKeyA,
		HandshakeTimeout:       350 * time.Millisecond,
		HandshakeRetryInterval: 50 * time.Millisecond,
		Peers: []session.Peer{
			{
				NodeID:    "node-b",
				PublicKey: publicKeyB,
				Candidates: []session.Candidate{
					{Kind: "direct", Address: deadDirect, Priority: 1000},
					{Kind: "direct", Address: "127.0.0.1:1", Priority: 999},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() { _ = transportA.Close() }()

	transportB, err := Listen(Config{
		NodeID:                 "node-b",
		ListenAddress:          "127.0.0.1:0",
		PrivateKey:             privateKeyB,
		HandshakeTimeout:       350 * time.Millisecond,
		HandshakeRetryInterval: 50 * time.Millisecond,
		Peers: []session.Peer{
			{
				NodeID:    "node-a",
				PublicKey: publicKeyA,
				Candidates: []session.Candidate{
					{Kind: "direct", Address: transportA.Address(), Priority: 1000},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("listen transport B: %v", err)
	}
	defer func() { _ = transportB.Close() }()

	// Replace the second direct candidate with the actual live address once B is known.
	transportA.peerCandidates["node-b"][1].Address = transportB.Address()

	sinkA := testSink{received: make(chan dataplane.InboundPacket, 1)}
	sinkB := testSink{received: make(chan dataplane.InboundPacket, 1)}

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
	}, transportA, sinkA)
	engineB := dataplane.NewEngine(dataplane.Spec{
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
	}, transportB, sinkB)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 2)
	go func() { errCh <- engineA.Serve(ctx) }()
	go func() { errCh <- engineB.Serve(ctx) }()

	sendErrCh := make(chan error, 1)
	startedAt := time.Now()
	go func() {
		sendErrCh <- engineA.SendPacket(context.Background(), "100.64.0.11", []byte("burst-direct"))
	}()

	select {
	case packet := <-sinkB.received:
		if string(packet.Payload) != "burst-direct" {
			t.Fatalf("unexpected payload %q", string(packet.Payload))
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for packet over burst direct recovery")
	}

	if err := <-sendErrCh; err != nil {
		t.Fatalf("send packet with direct burst: %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed >= 250*time.Millisecond {
		t.Fatalf("expected burst direct recovery faster than sequential timeout, took %v", elapsed)
	}

	report := transportA.Snapshot()
	if len(report.Peers) != 1 || report.Peers[0].ActiveAddress != transportB.Address() {
		t.Fatalf("expected active direct peer after burst recovery, got %#v", report.Peers)
	}

	cancel()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("engine exited with error: %v", err)
		}
	}
}

func TestTransportDiscoverSTUNUsesSharedSocket(t *testing.T) {
	server := newSecureUDPTestSTUNServer(t, 0)
	defer server.Close()

	privateKeyA, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}

	transportA, err := Listen(Config{
		NodeID:        "node-a",
		ListenAddress: "127.0.0.1:0",
		PrivateKey:    privateKeyA,
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() { _ = transportA.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- transportA.Serve(ctx, func(context.Context, dataplane.Frame, net.Addr) error { return nil })
	}()

	report, err := transportA.DiscoverSTUN(context.Background(), []string{server.Address()}, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("discover stun over secure udp socket: %v", err)
	}
	if !report.Reachable || report.SelectedAddress == "" {
		t.Fatalf("expected reachable stun report, got %#v", report)
	}
	if report.SelectedAddress != transportA.Address() {
		t.Fatalf("expected shared-port stun address %q, got %#v", transportA.Address(), report)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("transport serve: %v", err)
	}
}

func TestSendAddressesRetriesDirectBeforeActiveRelayWhenRecoveryWindowOpens(t *testing.T) {
	transport := &Transport{
		peerCandidates: map[string][]session.Candidate{
			"node-b": {
				{Kind: "direct", Address: "198.51.100.10:51820", Priority: 1000},
				{Kind: "relay", Address: "relay.example.net:3478", Priority: 500},
			},
		},
		relayAddresses: map[string]struct{}{
			"relay.example.net:3478": {},
		},
		directRetryAfter: 50 * time.Millisecond,
		activePeer: map[string]string{
			"node-b": "relay.example.net:3478",
		},
		lastDirectTry: map[string]time.Time{},
	}

	addresses := transport.sendAddresses("node-b", "198.51.100.10:51820")
	want := []string{
		"198.51.100.10:51820",
		"relay.example.net:3478",
	}

	if len(addresses) != len(want) {
		t.Fatalf("expected %d addresses, got %d: %#v", len(want), len(addresses), addresses)
	}
	for idx := range want {
		if addresses[idx] != want[idx] {
			t.Fatalf("unexpected address order at %d: want %q got %q", idx, want[idx], addresses[idx])
		}
	}
}

func TestSendAddressesKeepsActiveRelayFirstWhileRecoveryWindowIsCoolingDown(t *testing.T) {
	transport := &Transport{
		peerCandidates: map[string][]session.Candidate{
			"node-b": {
				{Kind: "direct", Address: "198.51.100.10:51820", Priority: 1000},
				{Kind: "relay", Address: "relay.example.net:3478", Priority: 500},
			},
		},
		relayAddresses: map[string]struct{}{
			"relay.example.net:3478": {},
		},
		directRetryAfter: 10 * time.Second,
		activePeer: map[string]string{
			"node-b": "relay.example.net:3478",
		},
		lastDirectTry: map[string]time.Time{
			"node-b": time.Now().UTC(),
		},
	}

	addresses := transport.sendAddresses("node-b", "198.51.100.10:51820")
	want := []string{
		"relay.example.net:3478",
		"198.51.100.10:51820",
	}

	if len(addresses) != len(want) {
		t.Fatalf("expected %d addresses, got %d: %#v", len(want), len(addresses), addresses)
	}
	for idx := range want {
		if addresses[idx] != want[idx] {
			t.Fatalf("unexpected address order at %d: want %q got %q", idx, want[idx], addresses[idx])
		}
	}
}

func TestMarkEstablishedDoesNotDowngradeDirectBackToRelay(t *testing.T) {
	transport := &Transport{
		relayAddresses: map[string]struct{}{
			"relay.example.net:3478": {},
		},
		activePeer:  map[string]string{},
		established: map[string]time.Time{},
	}

	transport.markEstablished("node-b", "198.51.100.10:51820")
	transport.markEstablished("node-b", "relay.example.net:3478")

	if got := transport.activePeer["node-b"]; got != "198.51.100.10:51820" {
		t.Fatalf("expected direct path to stay active, got %q", got)
	}
}

func TestSnapshotReportsActiveRelayAndRecoveryWindow(t *testing.T) {
	now := time.Now().UTC().Add(-3 * time.Second)
	transport := &Transport{
		nodeID:           "node-a",
		directRetryAfter: 10 * time.Second,
		peerCandidates: map[string][]session.Candidate{
			"node-b": {
				{Kind: "direct", Address: "198.51.100.10:51820", Priority: 1000},
				{Kind: "relay", Address: "relay.example.net:3478", Priority: 500},
			},
		},
		relayAddresses: map[string]struct{}{
			"relay.example.net:3478": {},
		},
		activePeer: map[string]string{
			"node-b": "relay.example.net:3478",
		},
		lastDirectTry: map[string]time.Time{
			"node-b": now,
		},
		established: map[string]time.Time{
			"node-b@relay.example.net:3478": now.Add(500 * time.Millisecond),
		},
	}

	report := transport.Snapshot()
	if report.NodeID != "node-a" {
		t.Fatalf("expected node id node-a, got %q", report.NodeID)
	}
	if report.DirectRetryAfter != "10s" {
		t.Fatalf("expected direct retry after 10s, got %q", report.DirectRetryAfter)
	}
	if len(report.Peers) != 1 {
		t.Fatalf("expected one peer, got %d", len(report.Peers))
	}

	peer := report.Peers[0]
	if peer.NodeID != "node-b" {
		t.Fatalf("expected peer node-b, got %q", peer.NodeID)
	}
	if peer.ActiveAddress != "relay.example.net:3478" || peer.ActiveKind != "relay" {
		t.Fatalf("unexpected active path %#v", peer)
	}
	if peer.NextDirectRetryAt.IsZero() {
		t.Fatal("expected next direct retry time to be reported while relay is active")
	}
	if peer.LastEstablishedAt.IsZero() {
		t.Fatal("expected last established time to be reported")
	}
	if len(peer.Candidates) != 2 {
		t.Fatalf("expected two candidates, got %d", len(peer.Candidates))
	}
	if peer.Candidates[0].Kind != "direct" || peer.Candidates[0].Address != "198.51.100.10:51820" {
		t.Fatalf("unexpected first candidate %#v", peer.Candidates[0])
	}
	if !peer.Candidates[1].Active || peer.Candidates[1].Kind != "relay" {
		t.Fatalf("expected relay candidate to be active, got %#v", peer.Candidates[1])
	}
	if peer.Candidates[1].EstablishedAt.IsZero() {
		t.Fatal("expected relay candidate established time to be reported")
	}
}

func TestSnapshotCountsRelayFallbackAndDirectRecovery(t *testing.T) {
	transport := &Transport{
		peerCandidates: map[string][]session.Candidate{
			"node-b": {
				{Kind: "direct", Address: "198.51.100.10:51820", Priority: 1000},
				{Kind: "relay", Address: "relay.example.net:3478", Priority: 500},
			},
		},
		relayAddresses: map[string]struct{}{
			"relay.example.net:3478": {},
		},
		activePeer:  map[string]string{},
		established: map[string]time.Time{},
		peerStats:   map[string]peerMetrics{},
	}

	transport.markEstablished("node-b", "relay.example.net:3478")
	transport.markEstablished("node-b", "198.51.100.10:51820")

	report := transport.Snapshot()
	if len(report.Peers) != 1 {
		t.Fatalf("expected one peer, got %#v", report.Peers)
	}
	peer := report.Peers[0]
	if peer.RelayFallbacks != 1 || peer.DirectRecoveries != 1 {
		t.Fatalf("expected one fallback and one recovery, got %#v", peer)
	}
	if peer.ActiveAddress != "198.51.100.10:51820" || peer.ActiveKind != "direct" {
		t.Fatalf("expected recovered direct path to stay active, got %#v", peer)
	}
	if peer.ActiveSince.IsZero() || peer.LastPathChangeAt.IsZero() {
		t.Fatalf("expected path transition timestamps to be set, got %#v", peer)
	}
}

type secureUDPTestSTUNServer struct {
	conn  net.PacketConn
	delay time.Duration
}

func newSecureUDPTestSTUNServer(t *testing.T, delay time.Duration) *secureUDPTestSTUNServer {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen test stun server: %v", err)
	}
	server := &secureUDPTestSTUNServer{
		conn:  conn,
		delay: delay,
	}
	go server.serve(t)
	return server
}

func (s *secureUDPTestSTUNServer) Address() string {
	return s.conn.LocalAddr().String()
}

func (s *secureUDPTestSTUNServer) Close() {
	_ = s.conn.Close()
}

func (s *secureUDPTestSTUNServer) serve(t *testing.T) {
	t.Helper()

	buffer := make([]byte, 1024)
	for {
		n, addr, err := s.conn.ReadFrom(buffer)
		if err != nil {
			return
		}
		if n < 20 {
			continue
		}
		transactionID := append([]byte(nil), buffer[8:20]...)
		if s.delay > 0 {
			time.Sleep(s.delay)
		}
		response, err := buildSecureUDPTestBindingSuccess(transactionID, addr)
		if err != nil {
			t.Errorf("build test stun response: %v", err)
			return
		}
		if _, err := s.conn.WriteTo(response, addr); err != nil {
			return
		}
	}
}

func buildSecureUDPTestBindingSuccess(transactionID []byte, addr net.Addr) ([]byte, error) {
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return nil, net.InvalidAddrError("non-udp address")
	}
	ip4 := udpAddr.IP.To4()
	if ip4 == nil {
		return nil, net.InvalidAddrError("non-ipv4 address")
	}

	const (
		testMagicCookie                = 0x2112A442
		testBindingSuccessResponseType = 0x0101
		testAttrXORMappedAddress       = 0x0020
	)

	attr := make([]byte, 12)
	binary.BigEndian.PutUint16(attr[0:2], testAttrXORMappedAddress)
	binary.BigEndian.PutUint16(attr[2:4], 8)
	attr[4] = 0
	attr[5] = 0x01
	binary.BigEndian.PutUint16(attr[6:8], uint16(udpAddr.Port)^uint16(testMagicCookie>>16))
	cookieBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cookieBytes, testMagicCookie)
	for idx := 0; idx < 4; idx++ {
		attr[8+idx] = ip4[idx] ^ cookieBytes[idx]
	}

	message := make([]byte, 20+len(attr))
	binary.BigEndian.PutUint16(message[0:2], testBindingSuccessResponseType)
	binary.BigEndian.PutUint16(message[2:4], uint16(len(attr)))
	binary.BigEndian.PutUint32(message[4:8], testMagicCookie)
	copy(message[8:20], transactionID)
	copy(message[20:], attr)
	return message, nil
}
