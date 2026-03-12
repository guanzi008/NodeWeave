package store

import (
	"path/filepath"
	"testing"
	"time"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/services/controlplane/internal/config"
)

func TestSQLiteStorePersistsRoutesAcrossRestart(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		StorageDriver:     "sqlite",
		SQLitePath:        filepath.Join(tmpDir, "controlplane.db"),
		AdminEmail:        "admin@example.com",
		AdminPassword:     "dev-password",
		AdminToken:        "dev-admin-token",
		RegistrationToken: "dev-register-token",
		DNSDomain:         "internal.net",
		RelayAddresses:    []string{"relay-ap-1.example.net:3478"},
	}

	firstStore, err := NewSQLiteStore(cfg)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}

	regResp, err := firstStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "persist-node",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-persist",
		RegistrationToken: cfg.RegistrationToken,
	})
	if err != nil {
		t.Fatalf("register device: %v", err)
	}

	if _, err := firstStore.CreateRoute(api.CreateRouteRequest{
		NetworkCIDR: "10.33.0.0/16",
		ViaNodeID:   regResp.Node.ID,
		Priority:    100,
	}); err != nil {
		t.Fatalf("create route: %v", err)
	}
	if err := firstStore.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	secondStore, err := NewSQLiteStore(cfg)
	if err != nil {
		t.Fatalf("reopen sqlite store: %v", err)
	}
	defer func() {
		_ = secondStore.Close()
	}()

	routes := secondStore.ListRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route after reopen, got %d", len(routes))
	}
	if routes[0].NetworkCIDR != "10.33.0.0/16" {
		t.Fatalf("unexpected route after reopen: %s", routes[0].NetworkCIDR)
	}
}

func TestSQLiteStorePersistsHeartbeatPublicKeyRotationAcrossRestart(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		StorageDriver:     "sqlite",
		SQLitePath:        filepath.Join(tmpDir, "controlplane.db"),
		AdminEmail:        "admin@example.com",
		AdminPassword:     "dev-password",
		AdminToken:        "dev-admin-token",
		RegistrationToken: "dev-register-token",
		DNSDomain:         "internal.net",
		RelayAddresses:    []string{"relay-ap-1.example.net:3478"},
	}

	firstStore, err := NewSQLiteStore(cfg)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}

	regResp, err := firstStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "rotate-node",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-before",
		RegistrationToken: cfg.RegistrationToken,
	})
	if err != nil {
		t.Fatalf("register device: %v", err)
	}

	hbResp, err := firstStore.UpdateHeartbeat(regResp.Node.ID, regResp.NodeToken, api.HeartbeatRequest{
		Status:    "online",
		PublicKey: "pubkey-after",
	})
	if err != nil {
		t.Fatalf("update heartbeat: %v", err)
	}
	if hbResp.Node.PublicKey != "pubkey-after" {
		t.Fatalf("expected rotated public key in heartbeat response, got %q", hbResp.Node.PublicKey)
	}
	if hbResp.BootstrapVersion < 2 {
		t.Fatalf("expected bootstrap version increment after rotation, got %d", hbResp.BootstrapVersion)
	}

	if err := firstStore.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	secondStore, err := NewSQLiteStore(cfg)
	if err != nil {
		t.Fatalf("reopen sqlite store: %v", err)
	}
	defer func() {
		_ = secondStore.Close()
	}()

	bootstrap, err := secondStore.GetBootstrap(regResp.Node.ID, regResp.NodeToken)
	if err != nil {
		t.Fatalf("get bootstrap after reopen: %v", err)
	}
	if bootstrap.Node.PublicKey != "pubkey-after" {
		t.Fatalf("expected rotated public key after reopen, got %q", bootstrap.Node.PublicKey)
	}
}

func TestSQLiteStorePersistsEndpointRecordsAcrossRestart(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		StorageDriver:     "sqlite",
		SQLitePath:        filepath.Join(tmpDir, "controlplane.db"),
		AdminEmail:        "admin@example.com",
		AdminPassword:     "dev-password",
		AdminToken:        "dev-admin-token",
		RegistrationToken: "dev-register-token",
		DNSDomain:         "internal.net",
		RelayAddresses:    []string{"relay-ap-1.example.net:3478"},
	}

	firstStore, err := NewSQLiteStore(cfg)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}

	regResp, err := firstStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "endpoint-node",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-before",
		RegistrationToken: cfg.RegistrationToken,
	})
	if err != nil {
		t.Fatalf("register device: %v", err)
	}

	hbResp, err := firstStore.UpdateHeartbeat(regResp.Node.ID, regResp.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "198.51.100.10:51820", Source: "static"},
			{Address: "203.0.113.10:51820", Source: "stun"},
		},
	})
	if err != nil {
		t.Fatalf("update heartbeat: %v", err)
	}
	if hbResp.BootstrapVersion < 2 {
		t.Fatalf("expected bootstrap version increment after endpoint update, got %d", hbResp.BootstrapVersion)
	}

	if err := firstStore.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	secondStore, err := NewSQLiteStore(cfg)
	if err != nil {
		t.Fatalf("reopen sqlite store: %v", err)
	}
	defer func() {
		_ = secondStore.Close()
	}()

	bootstrap, err := secondStore.GetBootstrap(regResp.Node.ID, regResp.NodeToken)
	if err != nil {
		t.Fatalf("get bootstrap after reopen: %v", err)
	}
	if len(bootstrap.Node.EndpointRecords) != 2 {
		t.Fatalf("expected endpoint records after reopen, got %#v", bootstrap.Node.EndpointRecords)
	}
	if bootstrap.Node.EndpointRecords[0].Source != "stun" {
		t.Fatalf("expected stun endpoint to stay first after reopen, got %#v", bootstrap.Node.EndpointRecords)
	}
}

func TestSQLiteStoreTimestampOnlyEndpointRefreshDoesNotBumpBootstrapVersion(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		StorageDriver:     "sqlite",
		SQLitePath:        filepath.Join(tmpDir, "controlplane.db"),
		AdminEmail:        "admin@example.com",
		AdminPassword:     "dev-password",
		AdminToken:        "dev-admin-token",
		RegistrationToken: "dev-register-token",
		DNSDomain:         "internal.net",
		RelayAddresses:    []string{"relay-ap-1.example.net:3478"},
	}

	dataStore, err := NewSQLiteStore(cfg)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() {
		_ = dataStore.Close()
	}()

	regResp, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "endpoint-refresh-node",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-before",
		RegistrationToken: cfg.RegistrationToken,
	})
	if err != nil {
		t.Fatalf("register device: %v", err)
	}

	firstResp, err := dataStore.UpdateHeartbeat(regResp.Node.ID, regResp.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "203.0.113.10:51820", Source: "stun"},
		},
	})
	if err != nil {
		t.Fatalf("first heartbeat: %v", err)
	}
	secondResp, err := dataStore.UpdateHeartbeat(regResp.Node.ID, regResp.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{
				Address:    "203.0.113.10:51820",
				Source:     "stun",
				ObservedAt: time.Now().UTC().Add(5 * time.Second),
			},
		},
	})
	if err != nil {
		t.Fatalf("second heartbeat: %v", err)
	}
	if secondResp.BootstrapVersion != firstResp.BootstrapVersion {
		t.Fatalf("expected timestamp-only endpoint refresh to keep bootstrap version %d, got %d", firstResp.BootstrapVersion, secondResp.BootstrapVersion)
	}
}

func TestSQLiteBootstrapIncludesPeers(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		StorageDriver:     "sqlite",
		SQLitePath:        filepath.Join(tmpDir, "controlplane.db"),
		AdminEmail:        "admin@example.com",
		AdminPassword:     "dev-password",
		AdminToken:        "dev-admin-token",
		RegistrationToken: "dev-register-token",
		DNSDomain:         "internal.net",
		RelayAddresses:    []string{"relay-ap-1.example.net:3478"},
	}

	dataStore, err := NewSQLiteStore(cfg)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() {
		_ = dataStore.Close()
	}()

	firstResp, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "node-a",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-a",
		RegistrationToken: cfg.RegistrationToken,
	})
	if err != nil {
		t.Fatalf("register first node: %v", err)
	}

	secondResp, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "node-b",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-b",
		RegistrationToken: cfg.RegistrationToken,
	})
	if err != nil {
		t.Fatalf("register second node: %v", err)
	}
	if _, err := dataStore.UpdateHeartbeat(secondResp.Node.ID, secondResp.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "203.0.113.10:51820", Source: "stun"},
		},
	}); err != nil {
		t.Fatalf("heartbeat second node: %v", err)
	}

	if _, err := dataStore.CreateRoute(api.CreateRouteRequest{
		NetworkCIDR: "10.44.0.0/16",
		ViaNodeID:   secondResp.Node.ID,
		Priority:    100,
	}); err != nil {
		t.Fatalf("create route: %v", err)
	}

	bootstrap, err := dataStore.GetBootstrap(firstResp.Node.ID, firstResp.NodeToken)
	if err != nil {
		t.Fatalf("get bootstrap: %v", err)
	}

	if bootstrap.Node.ID != firstResp.Node.ID {
		t.Fatalf("expected self node %s, got %s", firstResp.Node.ID, bootstrap.Node.ID)
	}
	if len(bootstrap.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(bootstrap.Peers))
	}
	if bootstrap.Peers[0].NodeID != secondResp.Node.ID {
		t.Fatalf("expected peer %s, got %s", secondResp.Node.ID, bootstrap.Peers[0].NodeID)
	}
	if len(bootstrap.Peers[0].EndpointRecords) != 1 || bootstrap.Peers[0].EndpointRecords[0].Source != "stun" {
		t.Fatalf("expected peer endpoint records in bootstrap, got %#v", bootstrap.Peers[0].EndpointRecords)
	}
	if len(bootstrap.Peers[0].AllowedIPs) != 2 {
		t.Fatalf("expected peer allowed ips to include overlay and route, got %#v", bootstrap.Peers[0].AllowedIPs)
	}
}

func TestSQLiteStorePersistsNATSummaryAndSchedulesDirectAttempt(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		StorageDriver:     "sqlite",
		SQLitePath:        filepath.Join(tmpDir, "controlplane.db"),
		AdminEmail:        "admin@example.com",
		AdminPassword:     "dev-password",
		AdminToken:        "dev-admin-token",
		RegistrationToken: "dev-register-token",
		DNSDomain:         "internal.net",
		RelayAddresses:    []string{"relay-ap-1.example.net:3478"},
	}

	dataStore, err := NewSQLiteStore(cfg)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() { _ = dataStore.Close() }()

	nodeA, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "node-a",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-a",
		RegistrationToken: cfg.RegistrationToken,
	})
	if err != nil {
		t.Fatalf("register node a: %v", err)
	}
	nodeB, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "node-b",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-b",
		RegistrationToken: cfg.RegistrationToken,
	})
	if err != nil {
		t.Fatalf("register node b: %v", err)
	}

	if _, err := dataStore.UpdateHeartbeat(nodeA.Node.ID, nodeA.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "198.51.100.10:51820", Source: "stun"},
		},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID: nodeB.Node.ID,
			ActiveKind: "relay",
			ReportedAt: time.Now().UTC(),
		}},
		NATReport: api.NATReport{
			GeneratedAt:              time.Now().UTC(),
			MappingBehavior:          "stable_port",
			SelectedReflexiveAddress: "198.51.100.10:51820",
			Reachable:                true,
			Samples: []api.NATSample{
				{Server: "stun-a", Status: "reachable", ReflexiveAddress: "198.51.100.10:51820"},
			},
		},
	}); err != nil {
		t.Fatalf("heartbeat node a: %v", err)
	}

	hbResp, err := dataStore.UpdateHeartbeat(nodeB.Node.ID, nodeB.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "203.0.113.10:51820", Source: "stun"},
		},
		NATReport: api.NATReport{
			GeneratedAt:              time.Now().UTC(),
			MappingBehavior:          "varying_port",
			SelectedReflexiveAddress: "203.0.113.10:51820",
			Reachable:                true,
			Samples: []api.NATSample{
				{Server: "stun-b", Status: "reachable", ReflexiveAddress: "203.0.113.10:51820"},
			},
		},
	})
	if err != nil {
		t.Fatalf("heartbeat node b: %v", err)
	}
	if len(hbResp.DirectAttempts) == 0 {
		t.Fatal("expected coordinated direct attempt for node b")
	}

	bootstrap, err := dataStore.GetBootstrap(nodeA.Node.ID, nodeA.NodeToken)
	if err != nil {
		t.Fatalf("get bootstrap for node a: %v", err)
	}
	if len(bootstrap.Peers) != 1 {
		t.Fatalf("expected one peer, got %#v", bootstrap.Peers)
	}
	if bootstrap.Peers[0].NATMappingBehavior != "varying_port" || !bootstrap.Peers[0].NATReachable || bootstrap.Peers[0].NATReportedAt.IsZero() {
		t.Fatalf("expected NAT summary on bootstrap peer, got %#v", bootstrap.Peers[0])
	}
	if bootstrap.Peers[0].ObservedTransportKind != "" {
		t.Fatalf("expected no observed transport summary from peer without report, got %#v", bootstrap.Peers[0])
	}
}

func TestSQLiteBootstrapIncludesObservedFailureBudget(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		StorageDriver:                                     "sqlite",
		SQLitePath:                                        filepath.Join(tmpDir, "controlplane.db"),
		AdminEmail:                                        "admin@example.com",
		AdminPassword:                                     "dev-password",
		AdminToken:                                        "dev-admin-token",
		RegistrationToken:                                 "dev-register-token",
		DNSDomain:                                         "internal.net",
		RelayAddresses:                                    []string{"relay-ap-1.example.net:3478"},
		DirectAttemptCooldown:                             2 * time.Second,
		DirectAttemptFailureSuppressAfter:                 3,
		DirectAttemptFailureSuppressWindow:                2 * time.Minute,
		DirectAttemptTimeoutSuppressAfter:                 3,
		DirectAttemptTimeoutSuppressWindow:                2 * time.Minute,
		DirectAttemptSuppressedProbeLimit:                 2,
		DirectAttemptTimeoutSuppressedProbeLimit:          2,
		DirectAttemptSuppressedProbeRefillInterval:        30 * time.Second,
		DirectAttemptTimeoutSuppressedProbeRefillInterval: 30 * time.Second,
	}

	dataStore, err := NewSQLiteStore(cfg)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() { _ = dataStore.Close() }()

	nodeA, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "node-a",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-a",
		RegistrationToken: cfg.RegistrationToken,
	})
	if err != nil {
		t.Fatalf("register node a: %v", err)
	}
	nodeB, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "node-b",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-b",
		RegistrationToken: cfg.RegistrationToken,
	})
	if err != nil {
		t.Fatalf("register node b: %v", err)
	}

	attemptAt := time.Now().UTC().Add(-5 * time.Second)
	if _, err := dataStore.UpdateHeartbeat(nodeA.Node.ID, nodeA.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "198.51.100.10:51820", Source: "stun"},
		},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID:                nodeB.Node.ID,
			ActiveKind:                "relay",
			ReportedAt:                time.Now().UTC(),
			LastDirectAttemptAt:       attemptAt,
			LastDirectAttemptResult:   "timeout",
			ConsecutiveDirectFailures: 3,
		}},
	}); err != nil {
		t.Fatalf("heartbeat node a: %v", err)
	}
	resp, err := dataStore.UpdateHeartbeat(nodeB.Node.ID, nodeB.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "203.0.113.10:51820", Source: "stun"},
		},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID:          nodeA.Node.ID,
			ActiveKind:          "relay",
			ReportedAt:          time.Now().UTC(),
			LastDirectAttemptAt: attemptAt,
		}},
	})
	if err != nil {
		t.Fatalf("heartbeat node b: %v", err)
	}
	if len(resp.DirectAttempts) != 0 {
		t.Fatalf("expected failure suppression to skip direct attempts, got %#v", resp.DirectAttempts)
	}
	if len(resp.PeerRecoveryStates) != 1 || resp.PeerRecoveryStates[0].BlockReason != "suppressed_timeout_budget" {
		t.Fatalf("expected suppressed timeout recovery state in heartbeat response, got %#v", resp.PeerRecoveryStates)
	}
	if !resp.PeerRecoveryStates[0].ProbeLimited || resp.PeerRecoveryStates[0].ProbeRemaining != 2 {
		t.Fatalf("expected recovery state to include probe budget details, got %#v", resp.PeerRecoveryStates)
	}
	if !resp.PeerRecoveryStates[0].ProbeRefillAt.IsZero() {
		t.Fatalf("expected untouched budget to omit probe refill time, got %#v", resp.PeerRecoveryStates)
	}

	bootstrap, err := dataStore.GetBootstrap(nodeB.Node.ID, nodeB.NodeToken)
	if err != nil {
		t.Fatalf("get bootstrap for node b: %v", err)
	}
	if len(bootstrap.Peers) != 1 {
		t.Fatalf("expected one peer, got %#v", bootstrap.Peers)
	}
	if bootstrap.Peers[0].ObservedConsecutiveDirectFailures != 3 || bootstrap.Peers[0].ObservedLastDirectAttemptResult != "timeout" {
		t.Fatalf("expected observed failure budget summary in bootstrap peer, got %#v", bootstrap.Peers[0])
	}
	if !bootstrap.Peers[0].ObservedDirectRecoveryBlocked || bootstrap.Peers[0].ObservedDirectRecoveryBlockReason != "suppressed_timeout_budget" {
		t.Fatalf("expected observed recovery block state in bootstrap peer, got %#v", bootstrap.Peers[0])
	}
	if !bootstrap.Peers[0].ObservedDirectRecoveryProbeLimited || bootstrap.Peers[0].ObservedDirectRecoveryProbeRemaining != 2 {
		t.Fatalf("expected observed recovery probe budget in bootstrap peer, got %#v", bootstrap.Peers[0])
	}
	if !bootstrap.Peers[0].ObservedDirectRecoveryProbeRefillAt.IsZero() {
		t.Fatalf("expected untouched probe budget to omit refill time in bootstrap peer, got %#v", bootstrap.Peers[0])
	}
}

func TestSQLiteSchedulesSuppressedProbeAfterInterval(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		StorageDriver:                              "sqlite",
		SQLitePath:                                 filepath.Join(tmpDir, "controlplane.db"),
		AdminEmail:                                 "admin@example.com",
		AdminPassword:                              "dev-password",
		AdminToken:                                 "dev-admin-token",
		RegistrationToken:                          "dev-register-token",
		DNSDomain:                                  "internal.net",
		RelayAddresses:                             []string{"relay-ap-1.example.net:3478"},
		DirectAttemptCooldown:                      2 * time.Second,
		DirectAttemptFailureSuppressAfter:          3,
		DirectAttemptFailureSuppressWindow:         90 * time.Second,
		DirectAttemptTimeoutSuppressAfter:          3,
		DirectAttemptTimeoutSuppressWindow:         90 * time.Second,
		DirectAttemptSuppressedProbeInterval:       15 * time.Second,
		DirectAttemptSuppressedProbeLimit:          2,
		DirectAttemptSuppressedProbeRefillInterval: 30 * time.Second,
	}

	dataStore, err := NewSQLiteStore(cfg)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() {
		if err := dataStore.Close(); err != nil {
			t.Fatalf("close sqlite store: %v", err)
		}
	}()

	nodeA, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "node-a",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-a",
		RegistrationToken: "dev-register-token",
	})
	if err != nil {
		t.Fatalf("register node a: %v", err)
	}
	nodeB, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "node-b",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-b",
		RegistrationToken: "dev-register-token",
	})
	if err != nil {
		t.Fatalf("register node b: %v", err)
	}

	attemptAt := time.Now().UTC().Add(-16 * time.Second)
	if _, err := dataStore.UpdateHeartbeat(nodeA.Node.ID, nodeA.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "198.51.100.10:51820", Source: "stun"},
		},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID:                nodeB.Node.ID,
			ActiveKind:                "relay",
			ReportedAt:                time.Now().UTC(),
			LastDirectAttemptAt:       attemptAt,
			LastDirectAttemptResult:   "timeout",
			ConsecutiveDirectFailures: 3,
		}},
	}); err != nil {
		t.Fatalf("heartbeat node a: %v", err)
	}
	resp, err := dataStore.UpdateHeartbeat(nodeB.Node.ID, nodeB.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "203.0.113.10:51820", Source: "stun"},
		},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID:          nodeA.Node.ID,
			ActiveKind:          "relay",
			ReportedAt:          time.Now().UTC(),
			LastDirectAttemptAt: attemptAt,
		}},
	})
	if err != nil {
		t.Fatalf("heartbeat node b: %v", err)
	}
	if len(resp.DirectAttempts) != 1 || resp.DirectAttempts[0].Reason != "manual_recover" {
		t.Fatalf("expected suppressed probe to schedule one manual_recover attempt, got %#v", resp.DirectAttempts)
	}
	if len(resp.PeerRecoveryStates) != 1 || resp.PeerRecoveryStates[0].NextProbeAt.IsZero() {
		t.Fatalf("expected recovery state to include next_probe_at, got %#v", resp.PeerRecoveryStates)
	}
	if resp.PeerRecoveryStates[0].ProbeRemaining != 2 {
		t.Fatalf("expected suppressed probe budget to remain available, got %#v", resp.PeerRecoveryStates)
	}
	if !resp.PeerRecoveryStates[0].ProbeRefillAt.IsZero() {
		t.Fatalf("expected available probe budget to omit refill time, got %#v", resp.PeerRecoveryStates)
	}

	bootstrap, err := dataStore.GetBootstrap(nodeB.Node.ID, nodeB.NodeToken)
	if err != nil {
		t.Fatalf("get bootstrap for node b: %v", err)
	}
	if len(bootstrap.Peers) != 1 || bootstrap.Peers[0].ObservedDirectRecoveryNextProbeAt.IsZero() {
		t.Fatalf("expected bootstrap peer to expose next probe at, got %#v", bootstrap.Peers)
	}
	if bootstrap.Peers[0].ObservedDirectRecoveryProbeRemaining != 2 {
		t.Fatalf("expected bootstrap peer to expose remaining probe budget, got %#v", bootstrap.Peers)
	}
	if !bootstrap.Peers[0].ObservedDirectRecoveryProbeRefillAt.IsZero() {
		t.Fatalf("expected bootstrap peer to omit refill time while budget is available, got %#v", bootstrap.Peers)
	}
}

func TestSQLiteStopsSuppressedProbeWhenBudgetExhausted(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		StorageDriver:                              "sqlite",
		SQLitePath:                                 filepath.Join(tmpDir, "controlplane.db"),
		AdminEmail:                                 "admin@example.com",
		AdminPassword:                              "dev-password",
		AdminToken:                                 "dev-admin-token",
		RegistrationToken:                          "dev-register-token",
		DNSDomain:                                  "internal.net",
		RelayAddresses:                             []string{"relay-ap-1.example.net:3478"},
		DirectAttemptCooldown:                      2 * time.Second,
		DirectAttemptFailureSuppressAfter:          3,
		DirectAttemptFailureSuppressWindow:         90 * time.Second,
		DirectAttemptTimeoutSuppressAfter:          3,
		DirectAttemptTimeoutSuppressWindow:         90 * time.Second,
		DirectAttemptSuppressedProbeInterval:       15 * time.Second,
		DirectAttemptSuppressedProbeLimit:          2,
		DirectAttemptSuppressedProbeRefillInterval: 30 * time.Second,
	}

	dataStore, err := NewSQLiteStore(cfg)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() { _ = dataStore.Close() }()

	nodeA, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "node-a",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-a",
		RegistrationToken: "dev-register-token",
	})
	if err != nil {
		t.Fatalf("register node a: %v", err)
	}
	nodeB, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "node-b",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-b",
		RegistrationToken: "dev-register-token",
	})
	if err != nil {
		t.Fatalf("register node b: %v", err)
	}

	attemptAt := time.Now().UTC().Add(-16 * time.Second)
	if _, err := dataStore.UpdateHeartbeat(nodeA.Node.ID, nodeA.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "198.51.100.10:51820", Source: "stun"},
		},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID:                nodeB.Node.ID,
			ActiveKind:                "relay",
			ReportedAt:                time.Now().UTC(),
			LastDirectAttemptAt:       attemptAt,
			LastDirectAttemptResult:   "timeout",
			ConsecutiveDirectFailures: 5,
		}},
	}); err != nil {
		t.Fatalf("heartbeat node a: %v", err)
	}
	resp, err := dataStore.UpdateHeartbeat(nodeB.Node.ID, nodeB.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "203.0.113.10:51820", Source: "stun"},
		},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID:          nodeA.Node.ID,
			ActiveKind:          "relay",
			ReportedAt:          time.Now().UTC(),
			LastDirectAttemptAt: attemptAt,
		}},
	})
	if err != nil {
		t.Fatalf("heartbeat node b: %v", err)
	}
	if len(resp.DirectAttempts) != 0 {
		t.Fatalf("expected exhausted suppressed probe budget to skip direct attempts, got %#v", resp.DirectAttempts)
	}
	if len(resp.PeerRecoveryStates) != 1 || resp.PeerRecoveryStates[0].ProbeRemaining != 0 || resp.PeerRecoveryStates[0].NextProbeAt.IsZero() {
		t.Fatalf("expected recovery state to show exhausted probe budget with refill gate, got %#v", resp.PeerRecoveryStates)
	}
	if resp.PeerRecoveryStates[0].ProbeRefillAt.IsZero() || !resp.PeerRecoveryStates[0].NextProbeAt.Equal(resp.PeerRecoveryStates[0].ProbeRefillAt) {
		t.Fatalf("expected exhausted probe budget to expose refill time, got %#v", resp.PeerRecoveryStates)
	}
}

func TestSQLiteRefillsSuppressedProbeBudgetAfterQuietPeriod(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		StorageDriver:                              "sqlite",
		SQLitePath:                                 filepath.Join(tmpDir, "controlplane.db"),
		AdminEmail:                                 "admin@example.com",
		AdminPassword:                              "dev-password",
		AdminToken:                                 "dev-admin-token",
		RegistrationToken:                          "dev-register-token",
		DNSDomain:                                  "internal.net",
		RelayAddresses:                             []string{"relay-ap-1.example.net:3478"},
		DirectAttemptCooldown:                      2 * time.Second,
		DirectAttemptFailureSuppressAfter:          3,
		DirectAttemptFailureSuppressWindow:         90 * time.Second,
		DirectAttemptTimeoutSuppressAfter:          3,
		DirectAttemptTimeoutSuppressWindow:         90 * time.Second,
		DirectAttemptSuppressedProbeInterval:       15 * time.Second,
		DirectAttemptSuppressedProbeLimit:          2,
		DirectAttemptSuppressedProbeRefillInterval: 30 * time.Second,
	}

	dataStore, err := NewSQLiteStore(cfg)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() { _ = dataStore.Close() }()

	nodeA, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "node-a",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-a",
		RegistrationToken: "dev-register-token",
	})
	if err != nil {
		t.Fatalf("register node a: %v", err)
	}
	nodeB, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "node-b",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-b",
		RegistrationToken: "dev-register-token",
	})
	if err != nil {
		t.Fatalf("register node b: %v", err)
	}

	attemptAt := time.Now().UTC().Add(-31 * time.Second)
	if _, err := dataStore.UpdateHeartbeat(nodeA.Node.ID, nodeA.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "198.51.100.10:51820", Source: "stun"},
		},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID:                nodeB.Node.ID,
			ActiveKind:                "relay",
			ReportedAt:                time.Now().UTC(),
			LastDirectAttemptAt:       attemptAt,
			LastDirectAttemptResult:   "timeout",
			ConsecutiveDirectFailures: 5,
		}},
	}); err != nil {
		t.Fatalf("heartbeat node a: %v", err)
	}
	resp, err := dataStore.UpdateHeartbeat(nodeB.Node.ID, nodeB.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "203.0.113.10:51820", Source: "stun"},
		},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID:          nodeA.Node.ID,
			ActiveKind:          "relay",
			ReportedAt:          time.Now().UTC(),
			LastDirectAttemptAt: attemptAt,
		}},
	})
	if err != nil {
		t.Fatalf("heartbeat node b: %v", err)
	}
	if len(resp.DirectAttempts) != 1 || resp.DirectAttempts[0].Reason != "manual_recover" {
		t.Fatalf("expected refill window to reopen one manual_recover attempt, got %#v", resp.DirectAttempts)
	}
	if len(resp.PeerRecoveryStates) != 1 || resp.PeerRecoveryStates[0].ProbeRemaining != 1 || resp.PeerRecoveryStates[0].ProbeRefillAt.IsZero() {
		t.Fatalf("expected recovery state to reflect one refilled probe slot, got %#v", resp.PeerRecoveryStates)
	}
}

func TestSQLiteBootstrapIncludesConfiguredExitNode(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		StorageDriver:         "sqlite",
		SQLitePath:            filepath.Join(tmpDir, "controlplane.db"),
		AdminEmail:            "admin@example.com",
		AdminPassword:         "dev-password",
		AdminToken:            "dev-admin-token",
		RegistrationToken:     "dev-register-token",
		DNSDomain:             "internal.net",
		RelayAddresses:        []string{"relay-ap-1.example.net:3478"},
		ExitNodeMode:          "enforced",
		ExitNodeAllowLAN:      true,
		ExitNodeAllowInternet: true,
		ExitNodeDNSMode:       "follow_exit",
	}

	dataStore, err := NewSQLiteStore(cfg)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() {
		_ = dataStore.Close()
	}()

	firstResp, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "node-a",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-a",
		RegistrationToken: cfg.RegistrationToken,
	})
	if err != nil {
		t.Fatalf("register first node: %v", err)
	}

	exitResp, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "node-exit",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-exit",
		RegistrationToken: cfg.RegistrationToken,
	})
	if err != nil {
		t.Fatalf("register exit node: %v", err)
	}

	dataStore.cfg.ExitNodeID = exitResp.Node.ID

	bootstrap, err := dataStore.GetBootstrap(firstResp.Node.ID, firstResp.NodeToken)
	if err != nil {
		t.Fatalf("get bootstrap: %v", err)
	}
	if bootstrap.ExitNode == nil {
		t.Fatal("expected exit node config in bootstrap")
	}
	if bootstrap.ExitNode.NodeID != exitResp.Node.ID {
		t.Fatalf("expected exit node %s, got %s", exitResp.Node.ID, bootstrap.ExitNode.NodeID)
	}
	if !bootstrap.ExitNode.AllowInternet {
		t.Fatal("expected exit node internet routing to be enabled")
	}
}
