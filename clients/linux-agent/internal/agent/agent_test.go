package agent

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nodeweave/clients/linux-agent/internal/config"
	"nodeweave/clients/linux-agent/internal/state"
	"nodeweave/packages/contracts/go/api"
	contractsclient "nodeweave/packages/contracts/go/client"
	"nodeweave/packages/runtime/go/dataplane"
	"nodeweave/packages/runtime/go/secureudp"
	"nodeweave/packages/runtime/go/session"
)

func TestReloadDataplaneKeepsRuntimeWhenSignatureUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		DataplaneMode: "udp",
		DataplanePath: filepath.Join(tmpDir, "dataplane.json"),
		TunnelMode:    "off",
	}

	if err := state.SaveDataplane(cfg.DataplanePath, dataplane.Spec{
		NodeID:        "node-a",
		ListenAddress: "127.0.0.1:0",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.11/32",
				PrefixBits:       32,
				PeerNodeID:       "node-b",
				CandidateAddress: "127.0.0.1:18080",
			},
		},
	}); err != nil {
		t.Fatalf("save dataplane spec: %v", err)
	}

	svc := &Service{cfg: cfg}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer svc.stopDataplane()

	if err := svc.reloadDataplane(ctx); err != nil {
		t.Fatalf("reload dataplane first time: %v", err)
	}
	first := svc.dataplaneRuntime
	if first == nil || first.signature == "" {
		t.Fatalf("expected active dataplane runtime, got %#v", first)
	}

	if err := svc.reloadDataplane(ctx); err != nil {
		t.Fatalf("reload dataplane second time: %v", err)
	}
	if svc.dataplaneRuntime != first {
		t.Fatalf("expected dataplane runtime to be reused when signature is unchanged")
	}
}

func TestReloadDataplaneRestartsSecureUDPWhenSessionChanges(t *testing.T) {
	tmpDir := t.TempDir()
	privateKey, _, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate node key pair: %v", err)
	}
	_, peerPublicKeyA, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate peer key pair A: %v", err)
	}
	_, peerPublicKeyB, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate peer key pair B: %v", err)
	}

	cfg := config.Config{
		DataplaneMode:  "secure-udp",
		DataplanePath:  filepath.Join(tmpDir, "dataplane.json"),
		SessionPath:    filepath.Join(tmpDir, "session.json"),
		PrivateKeyPath: filepath.Join(tmpDir, "node.key"),
		TunnelMode:     "off",
	}
	if err := os.WriteFile(cfg.PrivateKeyPath, []byte(privateKey), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	if err := state.SaveDataplane(cfg.DataplanePath, dataplane.Spec{
		NodeID:        "node-a",
		ListenAddress: "127.0.0.1:0",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.11/32",
				PrefixBits:       32,
				PeerNodeID:       "node-b",
				CandidateAddress: "127.0.0.1:19080",
				CandidateKind:    "direct",
			},
		},
	}); err != nil {
		t.Fatalf("save dataplane spec: %v", err)
	}

	writeSession := func(publicKey string) {
		t.Helper()
		if err := state.SaveSession(cfg.SessionPath, session.Spec{
			NodeID:        "node-a",
			ListenAddress: "127.0.0.1:0",
			Peers: []session.Peer{
				{
					NodeID:             "node-b",
					PublicKey:          publicKey,
					PreferredCandidate: "127.0.0.1:19080",
					Candidates: []session.Candidate{
						{Kind: "direct", Address: "127.0.0.1:19080", Priority: 1000},
					},
				},
			},
		}); err != nil {
			t.Fatalf("save session spec: %v", err)
		}
	}

	writeSession(peerPublicKeyA)

	svc := &Service{cfg: cfg}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer svc.stopDataplane()

	if err := svc.reloadDataplane(ctx); err != nil {
		t.Fatalf("reload secure dataplane first time: %v", err)
	}
	first := svc.dataplaneRuntime
	if first == nil || first.signature == "" {
		t.Fatalf("expected active secure dataplane runtime, got %#v", first)
	}
	firstSignature := first.signature

	writeSession(peerPublicKeyB)

	if err := svc.reloadDataplane(ctx); err != nil {
		t.Fatalf("reload secure dataplane second time: %v", err)
	}
	second := svc.dataplaneRuntime
	if second == nil {
		t.Fatal("expected secure dataplane runtime after reload")
	}
	if second == first {
		t.Fatal("expected dataplane runtime to restart when secure session material changes")
	}
	if second.signature == firstSignature {
		t.Fatal("expected dataplane signature to change when secure session material changes")
	}
}

func TestSendHeartbeatIncludesResolvedPublicKey(t *testing.T) {
	tmpDir := t.TempDir()
	privateKey, publicKey, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate node key pair: %v", err)
	}

	var received api.HeartbeatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/nodes/node-1/heartbeat" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode heartbeat request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(api.HeartbeatResponse{
			Node: api.Node{
				ID:        "node-1",
				PublicKey: received.PublicKey,
				Status:    "online",
			},
			BootstrapVersion: 2,
		}); err != nil {
			t.Fatalf("encode heartbeat response: %v", err)
		}
	}))
	defer server.Close()

	cfg := config.Config{
		ServerURL:      server.URL,
		PrivateKeyPath: filepath.Join(tmpDir, "node.key"),
		StatePath:      filepath.Join(tmpDir, "state.json"),
		RelayRegion:    "ap",
	}
	if err := os.WriteFile(cfg.PrivateKeyPath, []byte(privateKey), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	svc := &Service{
		cfg:    cfg,
		client: contractsclient.New(server.URL),
		state: state.File{
			Node: api.Node{
				ID:        "node-1",
				PublicKey: "stale-public-key",
			},
			NodeToken: "node-token",
		},
	}

	if err := svc.SendHeartbeat(context.Background()); err != nil {
		t.Fatalf("send heartbeat: %v", err)
	}
	if received.PublicKey != publicKey {
		t.Fatalf("expected heartbeat public key %q, got %q", publicKey, received.PublicKey)
	}
	if svc.state.Node.PublicKey != publicKey {
		t.Fatalf("expected state node public key to update to %q, got %q", publicKey, svc.state.Node.PublicKey)
	}
}

func TestSendHeartbeatIncludesDiscoveredSTUNEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	privateKey, publicKey, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate node key pair: %v", err)
	}

	stunServer := newAgentTestSTUNServer(t, 0)
	defer stunServer.Close()

	var received api.HeartbeatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/nodes/node-1/heartbeat" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode heartbeat request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(api.HeartbeatResponse{
			Node: api.Node{
				ID:              "node-1",
				PublicKey:       received.PublicKey,
				Status:          "online",
				Endpoints:       received.Endpoints,
				EndpointRecords: received.EndpointRecords,
			},
			BootstrapVersion: 2,
		}); err != nil {
			t.Fatalf("encode heartbeat response: %v", err)
		}
	}))
	defer server.Close()

	cfg := config.Config{
		ServerURL:      server.URL,
		PrivateKeyPath: filepath.Join(tmpDir, "node.key"),
		StatePath:      filepath.Join(tmpDir, "state.json"),
		STUNServers:    []string{stunServer.Address()},
		STUNReportPath: filepath.Join(tmpDir, "stun-report.json"),
		STUNTimeout:    300 * time.Millisecond,
		RelayRegion:    "ap",
	}
	if err := os.WriteFile(cfg.PrivateKeyPath, []byte(privateKey), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	svc := &Service{
		cfg:    cfg,
		client: contractsclient.New(server.URL),
		state: state.File{
			Node: api.Node{
				ID:        "node-1",
				PublicKey: "stale-public-key",
			},
			NodeToken: "node-token",
		},
	}

	if err := svc.SendHeartbeat(context.Background()); err != nil {
		t.Fatalf("send heartbeat with stun discovery: %v", err)
	}
	if received.PublicKey != publicKey {
		t.Fatalf("expected heartbeat public key %q, got %q", publicKey, received.PublicKey)
	}
	if len(received.Endpoints) != 1 {
		t.Fatalf("expected one discovered endpoint, got %#v", received.Endpoints)
	}
	if len(received.EndpointRecords) != 1 {
		t.Fatalf("expected one discovered endpoint record, got %#v", received.EndpointRecords)
	}
	report, err := state.LoadSTUNReport(cfg.STUNReportPath)
	if err != nil {
		t.Fatalf("load stun report: %v", err)
	}
	if !report.Reachable || report.SelectedAddress == "" {
		t.Fatalf("expected reachable stun report, got %#v", report)
	}
	if received.Endpoints[0] != report.SelectedAddress {
		t.Fatalf("expected heartbeat endpoint %q to match stun report %#v", received.Endpoints[0], report)
	}
	if received.NATReport.MappingBehavior == "" || received.NATReport.SelectedReflexiveAddress != report.SelectedReflexiveAddress || !received.NATReport.Reachable {
		t.Fatalf("expected heartbeat NAT report to mirror stun report, got %#v report=%#v", received.NATReport, report)
	}
	if received.EndpointRecords[0].Address != report.SelectedAddress || received.EndpointRecords[0].Source != "stun" {
		t.Fatalf("expected stun endpoint record for %q, got %#v", report.SelectedAddress, received.EndpointRecords)
	}
	if len(svc.state.Node.EndpointRecords) != 1 || svc.state.Node.EndpointRecords[0].Source != "stun" {
		t.Fatalf("expected endpoint records persisted in state, got %#v", svc.state.Node.EndpointRecords)
	}
}

func TestSendHeartbeatIncludesDataplaneListenerEndpoint(t *testing.T) {
	tmpDir := t.TempDir()
	var received api.HeartbeatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/nodes/node-1/heartbeat" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode heartbeat request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(api.HeartbeatResponse{
			Node: api.Node{
				ID:              "node-1",
				PublicKey:       received.PublicKey,
				Status:          "online",
				Endpoints:       received.Endpoints,
				EndpointRecords: received.EndpointRecords,
			},
			BootstrapVersion: 2,
		}); err != nil {
			t.Fatalf("encode heartbeat response: %v", err)
		}
	}))
	defer server.Close()

	svc := &Service{
		cfg: config.Config{
			ServerURL: server.URL,
			StatePath: filepath.Join(tmpDir, "state.json"),
		},
		client: contractsclient.New(server.URL),
		state: state.File{
			Node: api.Node{
				ID:        "node-1",
				PublicKey: "pubkey-1",
			},
			NodeToken: "node-token",
		},
		dataplaneRuntime: &activeDataplane{
			listenAddress: "127.0.0.1:51820",
		},
	}

	if err := svc.SendHeartbeat(context.Background()); err != nil {
		t.Fatalf("send heartbeat with dataplane listener: %v", err)
	}
	if len(received.Endpoints) != 1 || received.Endpoints[0] != "127.0.0.1:51820" {
		t.Fatalf("expected dataplane listener endpoint, got %#v", received.Endpoints)
	}
	if len(received.EndpointRecords) != 1 || received.EndpointRecords[0].Source != "listener" {
		t.Fatalf("expected listener endpoint record, got %#v", received.EndpointRecords)
	}
}

func TestSendHeartbeatUsesSecureUDPSharedSocketForSTUN(t *testing.T) {
	tmpDir := t.TempDir()
	stunServer := newAgentTestSTUNServer(t, 0)
	defer stunServer.Close()

	privateKey, publicKey, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate node key pair: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "node.key"), []byte(privateKey), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	transport, err := secureudp.Listen(secureudp.Config{
		NodeID:        "node-1",
		ListenAddress: "127.0.0.1:0",
		PrivateKey:    privateKey,
	})
	if err != nil {
		t.Fatalf("listen secure udp transport: %v", err)
	}
	defer func() { _ = transport.Close() }()

	transportCtx, transportCancel := context.WithCancel(context.Background())
	defer transportCancel()

	transportErrCh := make(chan error, 1)
	go func() {
		transportErrCh <- transport.Serve(transportCtx, func(context.Context, dataplane.Frame, net.Addr) error { return nil })
	}()

	var received api.HeartbeatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/nodes/node-1/heartbeat" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode heartbeat request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(api.HeartbeatResponse{
			Node: api.Node{
				ID:              "node-1",
				PublicKey:       received.PublicKey,
				Status:          "online",
				Endpoints:       received.Endpoints,
				EndpointRecords: received.EndpointRecords,
			},
			BootstrapVersion: 2,
		}); err != nil {
			t.Fatalf("encode heartbeat response: %v", err)
		}
	}))
	defer server.Close()

	cfg := config.Config{
		ServerURL:      server.URL,
		PrivateKeyPath: filepath.Join(tmpDir, "node.key"),
		StatePath:      filepath.Join(tmpDir, "state.json"),
		STUNServers:    []string{stunServer.Address()},
		STUNReportPath: filepath.Join(tmpDir, "stun-report.json"),
		STUNTimeout:    300 * time.Millisecond,
	}

	svc := &Service{
		cfg:    cfg,
		client: contractsclient.New(server.URL),
		state: state.File{
			Node: api.Node{
				ID:        "node-1",
				PublicKey: publicKey,
			},
			NodeToken: "node-token",
		},
		dataplaneRuntime: &activeDataplane{
			listenAddress: transport.Address(),
			secureUDP:     transport,
		},
	}

	if err := svc.SendHeartbeat(context.Background()); err != nil {
		t.Fatalf("send heartbeat with shared secure udp stun: %v", err)
	}

	report, err := state.LoadSTUNReport(cfg.STUNReportPath)
	if err != nil {
		t.Fatalf("load stun report: %v", err)
	}
	if !report.Reachable || report.SelectedAddress != transport.Address() {
		t.Fatalf("expected shared-port stun report %#v for %q", report, transport.Address())
	}
	if len(received.Endpoints) == 0 || received.Endpoints[0] != transport.Address() {
		t.Fatalf("expected heartbeat endpoint %q, got %#v", transport.Address(), received.Endpoints)
	}

	transportCancel()
	if err := <-transportErrCh; err != nil {
		t.Fatalf("transport serve: %v", err)
	}
}

func TestSendHeartbeatIncludesPeerTransportStates(t *testing.T) {
	tmpDir := t.TempDir()
	privateKeyA, publicKeyA, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	privateKeyB, publicKeyB, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair B: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "node.key"), []byte(privateKeyA), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	transportB, err := secureudp.Listen(secureudp.Config{
		NodeID:           "node-b",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyB,
		HandshakeTimeout: 300 * time.Millisecond,
		Peers: []session.Peer{
			{NodeID: "node-1", PublicKey: publicKeyA, Candidates: []session.Candidate{{Kind: "direct", Address: "127.0.0.1:1", Priority: 1000}}},
		},
	})
	if err != nil {
		t.Fatalf("listen transport B: %v", err)
	}
	defer func() { _ = transportB.Close() }()

	transportA, err := secureudp.Listen(secureudp.Config{
		NodeID:           "node-1",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyA,
		HandshakeTimeout: 300 * time.Millisecond,
		Peers: []session.Peer{
			{NodeID: "node-b", PublicKey: publicKeyB, Candidates: []session.Candidate{{Kind: "direct", Address: transportB.Address(), Priority: 1000}}},
		},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() { _ = transportA.Close() }()

	transportCtx, transportCancel := context.WithCancel(context.Background())
	defer transportCancel()

	transportErrCh := make(chan error, 2)
	go func() {
		transportErrCh <- transportA.Serve(transportCtx, func(context.Context, dataplane.Frame, net.Addr) error { return nil })
	}()
	go func() {
		transportErrCh <- transportB.Serve(transportCtx, func(context.Context, dataplane.Frame, net.Addr) error { return nil })
	}()

	report := transportA.WarmupPeer(context.Background(), "node-b", []string{transportB.Address()})
	if !report.Reachable {
		t.Fatalf("expected warmup to establish direct session, got %#v", report)
	}
	if _, err := transportA.ExecuteDirectAttempt(context.Background(), secureudp.DirectAttempt{
		AttemptID:     "attempt-heartbeat-success",
		PeerNodeID:    "node-b",
		Candidates:    []string{transportB.Address()},
		Window:        300 * time.Millisecond,
		BurstInterval: 50 * time.Millisecond,
		Reason:        "relay_active",
	}); err != nil {
		t.Fatalf("execute direct attempt for heartbeat summary: %v", err)
	}

	var received api.HeartbeatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/nodes/node-1/heartbeat" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode heartbeat request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(api.HeartbeatResponse{
			Node: api.Node{
				ID:        "node-1",
				PublicKey: publicKeyA,
				Status:    "online",
			},
			BootstrapVersion: 2,
			PeerRecoveryStates: []api.PeerRecoveryState{{
				PeerNodeID:                 "node-b",
				Blocked:                    true,
				BlockReason:                "suppressed_timeout_budget",
				BlockedUntil:               time.Now().UTC().Add(30 * time.Second),
				NextProbeAt:                time.Now().UTC().Add(10 * time.Second),
				ProbeLimited:               true,
				ProbeBudget:                2,
				ProbeFailures:              1,
				ProbeRemaining:             1,
				ProbeRefillAt:              time.Now().UTC().Add(40 * time.Second),
				LastIssuedAttemptID:        "attempt-node-1-node-b-1",
				LastIssuedAttemptReason:    "relay_active",
				LastIssuedAttemptAt:        time.Now().UTC().Add(-2 * time.Second),
				LastIssuedAttemptExecuteAt: time.Now().UTC().Add(-1500 * time.Millisecond),
			}},
		}); err != nil {
			t.Fatalf("encode heartbeat response: %v", err)
		}
	}))
	defer server.Close()

	svc := &Service{
		cfg: config.Config{
			ServerURL:         server.URL,
			PrivateKeyPath:    filepath.Join(tmpDir, "node.key"),
			StatePath:         filepath.Join(tmpDir, "state.json"),
			RecoveryStatePath: filepath.Join(tmpDir, "recovery.json"),
		},
		client: contractsclient.New(server.URL),
		state: state.File{
			Node: api.Node{
				ID:        "node-1",
				PublicKey: publicKeyA,
			},
			NodeToken: "node-token",
		},
		dataplaneRuntime: &activeDataplane{
			secureUDP: transportA,
		},
	}

	if err := svc.SendHeartbeat(context.Background()); err != nil {
		t.Fatalf("send heartbeat with peer transport states: %v", err)
	}
	if len(received.PeerTransportStates) != 1 {
		t.Fatalf("expected one peer transport state, got %#v", received.PeerTransportStates)
	}
	if received.PeerTransportStates[0].PeerNodeID != "node-b" || received.PeerTransportStates[0].ActiveKind != "direct" {
		t.Fatalf("expected direct peer transport summary, got %#v", received.PeerTransportStates)
	}
	if received.PeerTransportStates[0].LastDirectSuccessAt.IsZero() {
		t.Fatalf("expected direct success timestamp in peer transport state, got %#v", received.PeerTransportStates)
	}
	if received.PeerTransportStates[0].ConsecutiveDirectFailures != 0 {
		t.Fatalf("expected direct success to reset failure budget, got %#v", received.PeerTransportStates)
	}
	recoveryStates, err := state.LoadRecoveryStates(filepath.Join(tmpDir, "recovery.json"))
	if err != nil {
		t.Fatalf("load recovery states: %v", err)
	}
	if len(recoveryStates) != 1 || !recoveryStates[0].Blocked || recoveryStates[0].PeerNodeID != "node-b" {
		t.Fatalf("expected persisted recovery state from heartbeat response, got %#v", recoveryStates)
	}
	if recoveryStates[0].NextProbeAt.IsZero() {
		t.Fatalf("expected persisted recovery next probe timestamp, got %#v", recoveryStates)
	}
	if !recoveryStates[0].ProbeLimited || recoveryStates[0].ProbeBudget != 2 || recoveryStates[0].ProbeFailures != 1 || recoveryStates[0].ProbeRemaining != 1 {
		t.Fatalf("expected persisted recovery probe budget details, got %#v", recoveryStates)
	}
	if recoveryStates[0].ProbeRefillAt.IsZero() {
		t.Fatalf("expected persisted recovery probe refill time, got %#v", recoveryStates)
	}
	if recoveryStates[0].LastIssuedAttemptID == "" || recoveryStates[0].LastIssuedAttemptReason == "" || recoveryStates[0].LastIssuedAttemptAt.IsZero() || recoveryStates[0].LastIssuedAttemptExecuteAt.IsZero() {
		t.Fatalf("expected persisted recovery state to include latest issued attempt trace, got %#v", recoveryStates)
	}

	transportCancel()
	for i := 0; i < 2; i++ {
		if err := <-transportErrCh; err != nil {
			t.Fatalf("transport serve: %v", err)
		}
	}
}

func TestNormalizeAdvertisableListenerAddressRejectsUnspecified(t *testing.T) {
	tests := []string{"0.0.0.0:51820", "[::]:51820", ":51820"}
	for _, input := range tests {
		if address, ok := normalizeAdvertisableListenerAddress(input); ok {
			t.Fatalf("expected %q to be rejected, got %q", input, address)
		}
	}
	if address, ok := normalizeAdvertisableListenerAddress("127.0.0.1:51820"); !ok || address != "127.0.0.1:51820" {
		t.Fatalf("expected loopback listener address to be preserved, got %q ok=%v", address, ok)
	}
}

func TestHeartbeatRefreshesBootstrapWhenVersionAdvances(t *testing.T) {
	tmpDir := t.TempDir()
	privateKey, publicKey, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate node key pair: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "node.key"), []byte(privateKey), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	bootstrapCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/nodes/node-1/heartbeat":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(api.HeartbeatResponse{
				Node: api.Node{
					ID:        "node-1",
					OverlayIP: "100.64.0.10",
					PublicKey: publicKey,
					Status:    "online",
				},
				BootstrapVersion: 3,
			}); err != nil {
				t.Fatalf("encode heartbeat response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/nodes/node-1/bootstrap":
			bootstrapCalls++
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(api.BootstrapConfig{
				Version:     3,
				OverlayCIDR: "100.64.0.0/10",
				Node: api.Node{
					ID:        "node-1",
					OverlayIP: "100.64.0.10",
					PublicKey: publicKey,
					Status:    "online",
				},
				Peers: []api.Peer{
					{
						NodeID:      "node-2",
						OverlayIP:   "100.64.0.11",
						PublicKey:   "peer-pub",
						Endpoints:   []string{"203.0.113.10:51820"},
						RelayRegion: "ap",
						AllowedIPs:  []string{"100.64.0.11/32"},
						Status:      "online",
					},
				},
				DNS: api.DNSConfig{
					Domain:      "internal.net",
					Nameservers: []string{"100.64.0.53"},
				},
			}); err != nil {
				t.Fatalf("encode bootstrap response: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := config.Config{
		ServerURL:         server.URL,
		PrivateKeyPath:    filepath.Join(tmpDir, "node.key"),
		StatePath:         filepath.Join(tmpDir, "state.json"),
		RuntimePath:       filepath.Join(tmpDir, "runtime.json"),
		PlanPath:          filepath.Join(tmpDir, "plan.json"),
		ApplyReportPath:   filepath.Join(tmpDir, "apply-report.json"),
		SessionPath:       filepath.Join(tmpDir, "session.json"),
		SessionReportPath: filepath.Join(tmpDir, "session-report.json"),
		DataplanePath:     filepath.Join(tmpDir, "dataplane.json"),
		ApplyMode:         "dry-run",
		DataplaneMode:     "off",
		InterfaceName:     "nw0",
		InterfaceMTU:      1380,
	}

	svc := &Service{
		cfg:           cfg,
		client:        contractsclient.New(server.URL),
		runtimeDriver: newRuntimeDriver(cfg),
		state: state.File{
			Node: api.Node{
				ID:        "node-1",
				OverlayIP: "100.64.0.10",
				PublicKey: "stale-public-key",
			},
			NodeToken: "node-token",
			Bootstrap: api.BootstrapConfig{
				Version:     1,
				OverlayCIDR: "100.64.0.0/10",
				Node: api.Node{
					ID:        "node-1",
					OverlayIP: "100.64.0.10",
				},
			},
		},
	}

	if err := svc.heartbeatAndRefreshBootstrap(context.Background()); err != nil {
		t.Fatalf("heartbeat and refresh bootstrap: %v", err)
	}
	if bootstrapCalls != 1 {
		t.Fatalf("expected one bootstrap refresh, got %d", bootstrapCalls)
	}
	if svc.state.Bootstrap.Version != 3 {
		t.Fatalf("expected refreshed bootstrap version 3, got %d", svc.state.Bootstrap.Version)
	}
	runtimeSnapshot, err := state.LoadRuntime(cfg.RuntimePath)
	if err != nil {
		t.Fatalf("load runtime snapshot: %v", err)
	}
	if runtimeSnapshot.Version != 3 {
		t.Fatalf("expected runtime snapshot version 3, got %#v", runtimeSnapshot)
	}
	if len(runtimeSnapshot.Peers) != 1 || runtimeSnapshot.Peers[0].NodeID != "node-2" {
		t.Fatalf("expected refreshed peer in runtime snapshot, got %#v", runtimeSnapshot.Peers)
	}
}

func TestApplyRuntimeUsesProbeSelectedRelayCandidate(t *testing.T) {
	tmpDir := t.TempDir()

	responder, err := session.NewResponder("127.0.0.1:0", "node-b")
	if err != nil {
		t.Fatalf("start relay responder: %v", err)
	}
	defer func() {
		_ = responder.Close()
	}()

	responderCtx, responderCancel := context.WithCancel(context.Background())
	defer responderCancel()
	responderErrCh := make(chan error, 1)
	go func() {
		responderErrCh <- responder.Serve(responderCtx)
	}()

	cfg := config.Config{
		RuntimePath:            filepath.Join(tmpDir, "runtime.json"),
		PlanPath:               filepath.Join(tmpDir, "plan.json"),
		ApplyReportPath:        filepath.Join(tmpDir, "apply-report.json"),
		SessionPath:            filepath.Join(tmpDir, "session.json"),
		SessionReportPath:      filepath.Join(tmpDir, "session-report.json"),
		DataplanePath:          filepath.Join(tmpDir, "dataplane.json"),
		DataplaneListenAddress: "127.0.0.1:0",
		SessionProbeMode:       "udp",
		SessionProbeTimeout:    100 * time.Millisecond,
		InterfaceName:          "nw0",
		InterfaceMTU:           1380,
	}

	svc := &Service{
		cfg:           cfg,
		runtimeDriver: newRuntimeDriver(cfg),
		state: state.File{
			Bootstrap: api.BootstrapConfig{
				Version:     1,
				OverlayCIDR: "100.64.0.0/24",
				Node: api.Node{
					ID:        "node-a",
					OverlayIP: "100.64.0.10",
					PublicKey: "pub-a",
					Status:    "online",
				},
				Peers: []api.Peer{
					{
						NodeID:      "node-b",
						OverlayIP:   "100.64.0.11",
						PublicKey:   "pub-b",
						Endpoints:   []string{"127.0.0.1:1"},
						RelayRegion: "ap",
						AllowedIPs:  []string{"100.64.0.11/32"},
						Status:      "online",
					},
				},
				Relays: []api.RelayNode{
					{
						Region:  "ap",
						Address: responder.Address(),
					},
				},
			},
		},
	}

	if err := svc.ApplyRuntime(context.Background()); err != nil {
		t.Fatalf("apply runtime: %v", err)
	}

	report, err := state.LoadSessionReport(cfg.SessionReportPath)
	if err != nil {
		t.Fatalf("load session report: %v", err)
	}
	if len(report.Peers) != 1 {
		t.Fatalf("expected a single peer report, got %#v", report.Peers)
	}
	if report.Peers[0].SelectedCandidate != responder.Address() {
		t.Fatalf("expected relay responder to be selected, got %#v", report.Peers[0])
	}

	spec, err := state.LoadDataplane(cfg.DataplanePath)
	if err != nil {
		t.Fatalf("load dataplane spec: %v", err)
	}
	if len(spec.Routes) == 0 {
		t.Fatalf("expected dataplane routes, got %#v", spec)
	}
	if spec.Routes[0].CandidateAddress != responder.Address() {
		t.Fatalf("expected dataplane to use probe-selected relay candidate, got %#v", spec.Routes[0])
	}
	if spec.Routes[0].CandidateKind != "relay" {
		t.Fatalf("expected relay candidate kind, got %#v", spec.Routes[0])
	}

	responderCancel()
	if err := <-responderErrCh; err != nil {
		t.Fatalf("responder serve: %v", err)
	}
}

func TestStartDirectWarmupEstablishesDirectPath(t *testing.T) {
	privateKeyA, publicKeyA, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	privateKeyB, publicKeyB, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair B: %v", err)
	}

	transportA, err := secureudp.Listen(secureudp.Config{
		NodeID:           "node-a",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyA,
		HandshakeTimeout: 400 * time.Millisecond,
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

	transportB, err := secureudp.Listen(secureudp.Config{
		NodeID:           "node-b",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyB,
		HandshakeTimeout: 400 * time.Millisecond,
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

	svc := &Service{
		cfg: config.Config{
			DirectWarmupInterval: 20 * time.Millisecond,
			SessionProbeTimeout:  200 * time.Millisecond,
		},
	}
	spec := session.Spec{
		NodeID: "node-a",
		Peers: []session.Peer{
			{
				NodeID:    "node-b",
				PublicKey: publicKeyB,
				Candidates: []session.Candidate{
					{Kind: "direct", Address: transportB.Address(), Priority: 1000},
				},
			},
		},
	}

	svc.startDirectWarmup(ctx, transportA, spec)

	deadline := time.Now().Add(2 * time.Second)
	for {
		snapshot := transportA.Snapshot()
		if len(snapshot.Peers) == 1 && snapshot.Peers[0].ActiveAddress == transportB.Address() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for direct warmup, snapshot=%#v", snapshot)
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("transport serve: %v", err)
		}
	}
}

func TestStartDirectWarmupUsesTransportRetrySchedule(t *testing.T) {
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
		NodeID:                 "node-a",
		ListenAddress:          "127.0.0.1:0",
		PrivateKey:             privateKeyA,
		HandshakeTimeout:       90 * time.Millisecond,
		HandshakeRetryInterval: 30 * time.Millisecond,
		DirectRetryAfter:       50 * time.Millisecond,
		Peers: []session.Peer{
			{
				NodeID:    "node-b",
				PublicKey: publicKeyB,
				Candidates: []session.Candidate{
					{Kind: "direct", Address: directAddress, Priority: 1000},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() { _ = transportA.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 2)
	go func() {
		errCh <- transportA.Serve(ctx, func(context.Context, dataplane.Frame, net.Addr) error { return nil })
	}()

	svc := &Service{
		cfg: config.Config{
			DirectWarmupInterval: time.Hour,
			SessionProbeTimeout:  120 * time.Millisecond,
		},
	}
	spec := session.Spec{
		NodeID: "node-a",
		Peers: []session.Peer{
			{
				NodeID:    "node-b",
				PublicKey: publicKeyB,
				Candidates: []session.Candidate{
					{Kind: "direct", Address: directAddress, Priority: 1000},
				},
			},
		},
	}

	svc.startDirectWarmup(ctx, transportA, spec)

	time.Sleep(140 * time.Millisecond)
	if err := blackhole.Close(); err != nil {
		t.Fatalf("close blackhole direct socket: %v", err)
	}

	transportB, err := secureudp.Listen(secureudp.Config{
		NodeID:                 "node-b",
		ListenAddress:          directAddress,
		PrivateKey:             privateKeyB,
		HandshakeTimeout:       90 * time.Millisecond,
		HandshakeRetryInterval: 30 * time.Millisecond,
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

	go func() {
		errCh <- transportB.Serve(ctx, func(context.Context, dataplane.Frame, net.Addr) error { return nil })
	}()

	deadline := time.Now().Add(600 * time.Millisecond)
	for {
		snapshot := transportA.Snapshot()
		if len(snapshot.Peers) == 1 && snapshot.Peers[0].ActiveAddress == directAddress {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for scheduled direct warmup recovery, snapshot=%#v", snapshot)
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("transport serve: %v", err)
		}
	}
}

func TestStartDirectWarmupRespectsRecoveryNextProbeAt(t *testing.T) {
	privateKeyA, publicKeyA, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	privateKeyB, publicKeyB, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair B: %v", err)
	}

	transportA, err := secureudp.Listen(secureudp.Config{
		NodeID:        "node-a",
		ListenAddress: "127.0.0.1:0",
		PrivateKey:    privateKeyA,
		Peers: []session.Peer{{
			NodeID:    "node-b",
			PublicKey: publicKeyB,
			Candidates: []session.Candidate{
				{Kind: "direct", Address: "127.0.0.1:0", Priority: 1000},
			},
		}},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() { _ = transportA.Close() }()

	transportB, err := secureudp.Listen(secureudp.Config{
		NodeID:        "node-b",
		ListenAddress: "127.0.0.1:0",
		PrivateKey:    privateKeyB,
		Peers: []session.Peer{{
			NodeID:    "node-a",
			PublicKey: publicKeyA,
			Candidates: []session.Candidate{
				{Kind: "direct", Address: transportA.Address(), Priority: 1000},
			},
		}},
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

	svc := &Service{
		cfg: config.Config{
			DirectWarmupInterval: 20 * time.Millisecond,
			SessionProbeTimeout:  200 * time.Millisecond,
		},
		recoveryStates: map[string]api.PeerRecoveryState{},
	}
	svc.setRecoveryStates([]api.PeerRecoveryState{{
		PeerNodeID:   "node-b",
		Blocked:      true,
		BlockReason:  "suppressed_timeout_budget",
		BlockedUntil: time.Now().UTC().Add(400 * time.Millisecond),
		NextProbeAt:  time.Now().UTC().Add(200 * time.Millisecond),
	}})

	spec := session.Spec{
		NodeID: "node-a",
		Peers: []session.Peer{{
			NodeID:    "node-b",
			PublicKey: publicKeyB,
			Candidates: []session.Candidate{
				{Kind: "direct", Address: transportB.Address(), Priority: 1000},
			},
		}},
	}

	svc.startDirectWarmup(ctx, transportA, spec)

	time.Sleep(120 * time.Millisecond)
	snapshot := transportA.Snapshot()
	if len(snapshot.Peers) == 1 && snapshot.Peers[0].ActiveAddress == transportB.Address() {
		t.Fatalf("expected warmup to stay blocked before next_probe_at, snapshot=%#v", snapshot)
	}

	deadline := time.Now().Add(900 * time.Millisecond)
	for {
		snapshot = transportA.Snapshot()
		if len(snapshot.Peers) == 1 && snapshot.Peers[0].ActiveAddress == transportB.Address() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for warmup after recovery gate, snapshot=%#v", snapshot)
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("transport serve: %v", err)
		}
	}
}

func TestStartDirectWarmupUsesBlockedUntilWhenNoNextProbe(t *testing.T) {
	privateKeyA, publicKeyA, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	privateKeyB, publicKeyB, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair B: %v", err)
	}

	transportA, err := secureudp.Listen(secureudp.Config{
		NodeID:        "node-a",
		ListenAddress: "127.0.0.1:0",
		PrivateKey:    privateKeyA,
		Peers: []session.Peer{{
			NodeID:    "node-b",
			PublicKey: publicKeyB,
			Candidates: []session.Candidate{
				{Kind: "direct", Address: "127.0.0.1:0", Priority: 1000},
			},
		}},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() { _ = transportA.Close() }()

	transportB, err := secureudp.Listen(secureudp.Config{
		NodeID:        "node-b",
		ListenAddress: "127.0.0.1:0",
		PrivateKey:    privateKeyB,
		Peers: []session.Peer{{
			NodeID:    "node-a",
			PublicKey: publicKeyA,
			Candidates: []session.Candidate{
				{Kind: "direct", Address: transportA.Address(), Priority: 1000},
			},
		}},
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

	svc := &Service{
		cfg: config.Config{
			DirectWarmupInterval: 20 * time.Millisecond,
			SessionProbeTimeout:  200 * time.Millisecond,
		},
		recoveryStates: map[string]api.PeerRecoveryState{},
	}
	svc.setRecoveryStates([]api.PeerRecoveryState{{
		PeerNodeID:   "node-b",
		Blocked:      true,
		BlockReason:  "suppressed_timeout_budget",
		BlockedUntil: time.Now().UTC().Add(180 * time.Millisecond),
	}})

	spec := session.Spec{
		NodeID: "node-a",
		Peers: []session.Peer{{
			NodeID:    "node-b",
			PublicKey: publicKeyB,
			Candidates: []session.Candidate{
				{Kind: "direct", Address: transportB.Address(), Priority: 1000},
			},
		}},
	}

	svc.startDirectWarmup(ctx, transportA, spec)

	time.Sleep(90 * time.Millisecond)
	snapshot := transportA.Snapshot()
	if len(snapshot.Peers) == 1 && snapshot.Peers[0].ActiveAddress == transportB.Address() {
		t.Fatalf("expected warmup to stay blocked before blocked_until, snapshot=%#v", snapshot)
	}

	deadline := time.Now().Add(700 * time.Millisecond)
	for {
		snapshot = transportA.Snapshot()
		if len(snapshot.Peers) == 1 && snapshot.Peers[0].ActiveAddress == transportB.Address() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for warmup after blocked_until, snapshot=%#v", snapshot)
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("transport serve: %v", err)
		}
	}
}

func TestStartDirectWarmupWakesImmediatelyOnRecoveryUpdate(t *testing.T) {
	privateKeyA, publicKeyA, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	privateKeyB, publicKeyB, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair B: %v", err)
	}

	transportA, err := secureudp.Listen(secureudp.Config{
		NodeID:        "node-a",
		ListenAddress: "127.0.0.1:0",
		PrivateKey:    privateKeyA,
		Peers: []session.Peer{{
			NodeID:    "node-b",
			PublicKey: publicKeyB,
			Candidates: []session.Candidate{
				{Kind: "direct", Address: "127.0.0.1:0", Priority: 1000},
			},
		}},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() { _ = transportA.Close() }()

	transportB, err := secureudp.Listen(secureudp.Config{
		NodeID:        "node-b",
		ListenAddress: "127.0.0.1:0",
		PrivateKey:    privateKeyB,
		Peers: []session.Peer{{
			NodeID:    "node-a",
			PublicKey: publicKeyA,
			Candidates: []session.Candidate{
				{Kind: "direct", Address: transportA.Address(), Priority: 1000},
			},
		}},
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

	svc := &Service{
		cfg: config.Config{
			DirectWarmupInterval: time.Hour,
			SessionProbeTimeout:  200 * time.Millisecond,
		},
		recoveryStates: map[string]api.PeerRecoveryState{},
		warmupWakeCh:   make(chan struct{}, 1),
	}
	svc.setRecoveryStates([]api.PeerRecoveryState{{
		PeerNodeID:   "node-b",
		Blocked:      true,
		BlockReason:  "suppressed_timeout_budget",
		BlockedUntil: time.Now().UTC().Add(time.Hour),
	}})

	spec := session.Spec{
		NodeID: "node-a",
		Peers: []session.Peer{{
			NodeID:    "node-b",
			PublicKey: publicKeyB,
			Candidates: []session.Candidate{
				{Kind: "direct", Address: transportB.Address(), Priority: 1000},
			},
		}},
	}

	svc.startDirectWarmup(ctx, transportA, spec)

	time.Sleep(120 * time.Millisecond)
	snapshot := transportA.Snapshot()
	if len(snapshot.Peers) == 1 && snapshot.Peers[0].ActiveAddress == transportB.Address() {
		t.Fatalf("expected long recovery gate to block warmup before wake, snapshot=%#v", snapshot)
	}

	svc.setRecoveryStates(nil)

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		snapshot = transportA.Snapshot()
		if len(snapshot.Peers) == 1 && snapshot.Peers[0].ActiveAddress == transportB.Address() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for warmup wake after recovery update, snapshot=%#v", snapshot)
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("transport serve: %v", err)
		}
	}
}

func TestScheduleDirectAttemptsExecutesAndPersistsTransportReport(t *testing.T) {
	tmpDir := t.TempDir()
	privateKeyA, publicKeyA, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	privateKeyB, publicKeyB, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair B: %v", err)
	}

	transportB, err := secureudp.Listen(secureudp.Config{
		NodeID:           "node-b",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyB,
		HandshakeTimeout: 400 * time.Millisecond,
		Peers: []session.Peer{
			{NodeID: "node-a", PublicKey: publicKeyA, Candidates: []session.Candidate{{Kind: "direct", Address: "127.0.0.1:1", Priority: 1000}}},
		},
	})
	if err != nil {
		t.Fatalf("listen transport B: %v", err)
	}
	defer func() { _ = transportB.Close() }()

	transportA, err := secureudp.Listen(secureudp.Config{
		NodeID:           "node-a",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyA,
		HandshakeTimeout: 400 * time.Millisecond,
		Peers: []session.Peer{
			{NodeID: "node-b", PublicKey: publicKeyB, Candidates: []session.Candidate{{Kind: "direct", Address: transportB.Address(), Priority: 1000}}},
		},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() { _ = transportA.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 2)
	go func() {
		errCh <- transportA.Serve(ctx, func(context.Context, dataplane.Frame, net.Addr) error { return nil })
	}()
	go func() {
		errCh <- transportB.Serve(ctx, func(context.Context, dataplane.Frame, net.Addr) error { return nil })
	}()

	svc := &Service{
		cfg: config.Config{
			DirectAttemptPath:       filepath.Join(tmpDir, "direct-attempts.json"),
			DirectAttemptReportPath: filepath.Join(tmpDir, "direct-attempt-report.json"),
			TransportReportPath:     filepath.Join(tmpDir, "transport-report.json"),
		},
		dataplaneRuntime: &activeDataplane{
			secureUDP: transportA,
		},
		attemptReports:    map[string]state.DirectAttemptReportEntry{},
		pendingAttempts:   map[string]api.DirectAttemptInstruction{},
		scheduledAttempts: map[string]context.CancelFunc{},
	}

	svc.scheduleDirectAttempts([]api.DirectAttemptInstruction{
		{
			AttemptID:     "attempt-agent-success",
			PeerNodeID:    "node-b",
			IssuedAt:      time.Now().UTC().Add(-50 * time.Millisecond),
			ExecuteAt:     time.Now().UTC(),
			Window:        400,
			BurstInterval: 50,
			Candidates:    []string{transportB.Address()},
			Reason:        "fresh_endpoints",
		},
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		report, err := state.LoadTransportReport(filepath.Join(tmpDir, "transport-report.json"))
		if err == nil && len(report.Peers) == 1 && report.Peers[0].LastDirectAttemptID == "attempt-agent-success" && report.Peers[0].LastDirectAttemptResult == "success" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for persisted direct attempt transport report")
		}
		time.Sleep(20 * time.Millisecond)
	}
	attempts, err := state.LoadDirectAttempts(filepath.Join(tmpDir, "direct-attempts.json"))
	if err != nil {
		t.Fatalf("load persisted direct attempts: %v", err)
	}
	if len(attempts) != 0 {
		t.Fatalf("expected completed direct attempt queue to be empty, got %#v", attempts)
	}
	report, err := state.LoadDirectAttemptReport(filepath.Join(tmpDir, "direct-attempt-report.json"))
	if err != nil {
		t.Fatalf("load direct attempt report: %v", err)
	}
	if len(report.Entries) != 1 || report.Entries[0].AttemptID != "attempt-agent-success" || report.Entries[0].Status != "completed" || report.Entries[0].Result != "success" {
		t.Fatalf("expected completed direct attempt report entry, got %#v", report)
	}
	if report.Entries[0].IssuedAt.IsZero() {
		t.Fatalf("expected direct attempt report entry to retain issued_at, got %#v", report.Entries[0])
	}

	cancel()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("transport serve: %v", err)
		}
	}
}

func TestScheduleDirectAttemptsPersistsWithoutTransportAndRestoresLater(t *testing.T) {
	tmpDir := t.TempDir()
	privateKeyA, publicKeyA, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair A: %v", err)
	}
	privateKeyB, publicKeyB, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair B: %v", err)
	}

	svc := &Service{
		cfg: config.Config{
			DirectAttemptPath:       filepath.Join(tmpDir, "direct-attempts.json"),
			DirectAttemptReportPath: filepath.Join(tmpDir, "direct-attempt-report.json"),
			TransportReportPath:     filepath.Join(tmpDir, "transport-report.json"),
		},
		attemptReports:    map[string]state.DirectAttemptReportEntry{},
		pendingAttempts:   map[string]api.DirectAttemptInstruction{},
		scheduledAttempts: map[string]context.CancelFunc{},
	}

	transportB, err := secureudp.Listen(secureudp.Config{
		NodeID:           "node-b",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyB,
		HandshakeTimeout: 400 * time.Millisecond,
		Peers: []session.Peer{
			{NodeID: "node-a", PublicKey: publicKeyA, Candidates: []session.Candidate{{Kind: "direct", Address: "127.0.0.1:1", Priority: 1000}}},
		},
	})
	if err != nil {
		t.Fatalf("listen transport B: %v", err)
	}
	defer func() { _ = transportB.Close() }()

	transportA, err := secureudp.Listen(secureudp.Config{
		NodeID:           "node-a",
		ListenAddress:    "127.0.0.1:0",
		PrivateKey:       privateKeyA,
		HandshakeTimeout: 400 * time.Millisecond,
		Peers: []session.Peer{
			{NodeID: "node-b", PublicKey: publicKeyB, Candidates: []session.Candidate{{Kind: "direct", Address: transportB.Address(), Priority: 1000}}},
		},
	})
	if err != nil {
		t.Fatalf("listen transport A: %v", err)
	}
	defer func() { _ = transportA.Close() }()

	instruction := api.DirectAttemptInstruction{
		AttemptID:     "attempt-agent-restore",
		PeerNodeID:    "node-b",
		IssuedAt:      time.Now().UTC().Add(-50 * time.Millisecond),
		ExecuteAt:     time.Now().UTC().Add(150 * time.Millisecond),
		Window:        800,
		BurstInterval: 50,
		Candidates:    []string{transportB.Address()},
		Reason:        "manual_recover",
	}
	svc.scheduleDirectAttempts([]api.DirectAttemptInstruction{instruction})

	attempts, err := state.LoadDirectAttempts(svc.cfg.DirectAttemptPath)
	if err != nil {
		t.Fatalf("load persisted direct attempts without transport: %v", err)
	}
	if len(attempts) != 1 || attempts[0].AttemptID != instruction.AttemptID || len(attempts[0].Candidates) != 1 || attempts[0].Candidates[0] != transportB.Address() {
		t.Fatalf("expected pending direct attempt to persist, got %#v", attempts)
	}
	report, err := state.LoadDirectAttemptReport(svc.cfg.DirectAttemptReportPath)
	if err != nil {
		t.Fatalf("load direct attempt report while transport missing: %v", err)
	}
	if len(report.Entries) != 1 || report.Entries[0].Status != "waiting_transport" || report.Entries[0].WaitReason != "transport_unavailable" {
		t.Fatalf("expected waiting_transport direct attempt report, got %#v", report)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 2)
	go func() {
		errCh <- transportA.Serve(ctx, func(context.Context, dataplane.Frame, net.Addr) error { return nil })
	}()
	go func() {
		errCh <- transportB.Serve(ctx, func(context.Context, dataplane.Frame, net.Addr) error { return nil })
	}()

	svc.dataplaneRuntime = &activeDataplane{
		secureUDP: transportA,
	}
	svc.scheduleDirectAttempts(nil)

	deadline := time.Now().Add(3 * time.Second)
	for {
		report, err := state.LoadTransportReport(svc.cfg.TransportReportPath)
		if err == nil && len(report.Peers) == 1 && report.Peers[0].LastDirectAttemptID == instruction.AttemptID && report.Peers[0].LastDirectAttemptResult == "success" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for restored direct attempt to complete")
		}
		time.Sleep(20 * time.Millisecond)
	}

	attempts, err = state.LoadDirectAttempts(svc.cfg.DirectAttemptPath)
	if err != nil {
		t.Fatalf("load persisted direct attempts after restore: %v", err)
	}
	if len(attempts) != 0 {
		t.Fatalf("expected restored direct attempt queue to be empty, got %#v", attempts)
	}
	report, err = state.LoadDirectAttemptReport(svc.cfg.DirectAttemptReportPath)
	if err != nil {
		t.Fatalf("load direct attempt report after restore: %v", err)
	}
	if len(report.Entries) != 1 || report.Entries[0].AttemptID != instruction.AttemptID || report.Entries[0].Status != "completed" || report.Entries[0].Result != "success" {
		t.Fatalf("expected completed restored direct attempt report, got %#v", report)
	}

	cancel()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("transport serve: %v", err)
		}
	}
}

func TestScheduleDirectAttemptsMarksExpiredAttemptsInReport(t *testing.T) {
	tmpDir := t.TempDir()

	svc := &Service{
		cfg: config.Config{
			DirectAttemptPath:       filepath.Join(tmpDir, "direct-attempts.json"),
			DirectAttemptReportPath: filepath.Join(tmpDir, "direct-attempt-report.json"),
		},
		attemptReports:    map[string]state.DirectAttemptReportEntry{},
		pendingAttempts:   map[string]api.DirectAttemptInstruction{},
		scheduledAttempts: map[string]context.CancelFunc{},
	}

	svc.scheduleDirectAttempts([]api.DirectAttemptInstruction{
		{
			AttemptID:     "attempt-expired",
			PeerNodeID:    "node-b",
			ExecuteAt:     time.Now().UTC().Add(-2 * time.Second),
			Window:        200,
			BurstInterval: 50,
			Candidates:    []string{"203.0.113.10:51820"},
			Reason:        "manual_recover",
		},
	})

	attempts, err := state.LoadDirectAttempts(svc.cfg.DirectAttemptPath)
	if err != nil {
		t.Fatalf("load direct attempts after expiration: %v", err)
	}
	if len(attempts) != 0 {
		t.Fatalf("expected expired direct attempt queue to be empty, got %#v", attempts)
	}
	report, err := state.LoadDirectAttemptReport(svc.cfg.DirectAttemptReportPath)
	if err != nil {
		t.Fatalf("load direct attempt report after expiration: %v", err)
	}
	if len(report.Entries) != 1 || report.Entries[0].AttemptID != "attempt-expired" || report.Entries[0].Status != "expired" || report.Entries[0].Result != "expired" {
		t.Fatalf("expected expired direct attempt report entry, got %#v", report)
	}
}

type agentTestSTUNServer struct {
	conn  net.PacketConn
	delay time.Duration
}

func newAgentTestSTUNServer(t *testing.T, delay time.Duration) *agentTestSTUNServer {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen test stun server: %v", err)
	}
	server := &agentTestSTUNServer{
		conn:  conn,
		delay: delay,
	}
	go server.serve(t)
	return server
}

func (s *agentTestSTUNServer) Address() string {
	return s.conn.LocalAddr().String()
}

func (s *agentTestSTUNServer) Close() {
	_ = s.conn.Close()
}

func (s *agentTestSTUNServer) serve(t *testing.T) {
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
		response, err := buildAgentTestSTUNSuccess(transactionID, addr)
		if err != nil {
			t.Errorf("build test stun response: %v", err)
			return
		}
		if _, err := s.conn.WriteTo(response, addr); err != nil {
			return
		}
	}
}

func buildAgentTestSTUNSuccess(transactionID []byte, addr net.Addr) ([]byte, error) {
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return nil, net.InvalidAddrError("non-udp address")
	}
	ip4 := udpAddr.IP.To4()
	if ip4 == nil {
		return nil, net.InvalidAddrError("non-ipv4 address")
	}

	const (
		magicCookie                = 0x2112A442
		bindingSuccessResponseType = 0x0101
		attrXORMappedAddress       = 0x0020
	)

	attr := make([]byte, 12)
	binary.BigEndian.PutUint16(attr[0:2], attrXORMappedAddress)
	binary.BigEndian.PutUint16(attr[2:4], 8)
	attr[4] = 0
	attr[5] = 0x01
	binary.BigEndian.PutUint16(attr[6:8], uint16(udpAddr.Port)^uint16(magicCookie>>16))
	cookieBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cookieBytes, magicCookie)
	for idx := 0; idx < 4; idx++ {
		attr[8+idx] = ip4[idx] ^ cookieBytes[idx]
	}

	message := make([]byte, 20+len(attr))
	binary.BigEndian.PutUint16(message[0:2], bindingSuccessResponseType)
	binary.BigEndian.PutUint16(message[2:4], uint16(len(attr)))
	binary.BigEndian.PutUint32(message[4:8], magicCookie)
	copy(message[8:20], transactionID)
	copy(message[20:], attr)
	return message, nil
}

func TestReloadDataplanePersistsSecureUDPTransportReport(t *testing.T) {
	tmpDir := t.TempDir()
	privateKey, _, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate node key pair: %v", err)
	}
	_, peerPublicKey, err := secureudp.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate peer key pair: %v", err)
	}

	cfg := config.Config{
		DataplaneMode:       "secure-udp",
		DataplanePath:       filepath.Join(tmpDir, "dataplane.json"),
		SessionPath:         filepath.Join(tmpDir, "session.json"),
		TransportReportPath: filepath.Join(tmpDir, "transport-report.json"),
		PrivateKeyPath:      filepath.Join(tmpDir, "node.key"),
		TunnelMode:          "off",
	}
	if err := os.WriteFile(cfg.PrivateKeyPath, []byte(privateKey), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	if err := state.SaveDataplane(cfg.DataplanePath, dataplane.Spec{
		NodeID:        "node-a",
		ListenAddress: "127.0.0.1:0",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.11/32",
				PrefixBits:       32,
				PeerNodeID:       "node-b",
				CandidateAddress: "198.51.100.10:51820",
				CandidateKind:    "direct",
			},
		},
	}); err != nil {
		t.Fatalf("save dataplane spec: %v", err)
	}
	if err := state.SaveSession(cfg.SessionPath, session.Spec{
		NodeID:        "node-a",
		ListenAddress: "127.0.0.1:0",
		Peers: []session.Peer{
			{
				NodeID:             "node-b",
				PublicKey:          peerPublicKey,
				PreferredCandidate: "198.51.100.10:51820",
				Candidates: []session.Candidate{
					{Kind: "direct", Address: "198.51.100.10:51820", Priority: 1000},
					{Kind: "relay", Address: "127.0.0.1:3478", Priority: 500},
				},
			},
		},
	}); err != nil {
		t.Fatalf("save session spec: %v", err)
	}

	svc := &Service{cfg: cfg}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer svc.stopDataplane()

	if err := svc.reloadDataplane(ctx); err != nil {
		t.Fatalf("reload secure dataplane: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		report, err := state.LoadTransportReport(cfg.TransportReportPath)
		if err == nil && report.NodeID == "node-a" && report.ListenAddress != "" && len(report.Peers) == 1 {
			if report.Peers[0].NodeID != "node-b" {
				t.Fatalf("expected transport report peer node-b, got %#v", report.Peers[0])
			}
			if len(report.Peers[0].Candidates) != 2 {
				t.Fatalf("expected transport report candidates, got %#v", report.Peers[0].Candidates)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for transport report, last err=%v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
