package store

import (
	"testing"
	"time"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/services/controlplane/internal/config"
)

func TestMemoryStoreHeartbeatRotatesPublicKey(t *testing.T) {
	dataStore := NewMemoryStore(config.Config{
		AdminEmail:        "admin@example.com",
		AdminPassword:     "dev-password",
		AdminToken:        "dev-admin-token",
		RegistrationToken: "dev-register-token",
		DNSDomain:         "internal.net",
		RelayAddresses:    []string{"relay-ap-1.example.net:3478"},
	})

	regResp, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "memory-node",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-before",
		RegistrationToken: "dev-register-token",
	})
	if err != nil {
		t.Fatalf("register device: %v", err)
	}

	hbResp, err := dataStore.UpdateHeartbeat(regResp.Node.ID, regResp.NodeToken, api.HeartbeatRequest{
		Status:    "online",
		PublicKey: "pubkey-after",
	})
	if err != nil {
		t.Fatalf("update heartbeat: %v", err)
	}
	if hbResp.Node.PublicKey != "pubkey-after" {
		t.Fatalf("expected rotated public key, got %q", hbResp.Node.PublicKey)
	}
	if hbResp.BootstrapVersion < 2 {
		t.Fatalf("expected bootstrap version increment after key rotation, got %d", hbResp.BootstrapVersion)
	}

	bootstrap, err := dataStore.GetBootstrap(regResp.Node.ID, regResp.NodeToken)
	if err != nil {
		t.Fatalf("get bootstrap: %v", err)
	}
	if bootstrap.Node.PublicKey != "pubkey-after" {
		t.Fatalf("expected bootstrap node public key to be rotated, got %q", bootstrap.Node.PublicKey)
	}
}

func TestMemoryStoreHeartbeatPersistsEndpointRecords(t *testing.T) {
	dataStore := NewMemoryStore(config.Config{
		AdminEmail:        "admin@example.com",
		AdminPassword:     "dev-password",
		AdminToken:        "dev-admin-token",
		RegistrationToken: "dev-register-token",
		DNSDomain:         "internal.net",
		RelayAddresses:    []string{"relay-ap-1.example.net:3478"},
	})

	regResp, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "memory-endpoints",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-before",
		RegistrationToken: "dev-register-token",
	})
	if err != nil {
		t.Fatalf("register device: %v", err)
	}

	hbResp, err := dataStore.UpdateHeartbeat(regResp.Node.ID, regResp.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{
				Address: "198.51.100.10:51820",
				Source:  "static",
			},
			{
				Address: "203.0.113.10:51820",
				Source:  "stun",
			},
		},
	})
	if err != nil {
		t.Fatalf("update heartbeat: %v", err)
	}
	if hbResp.BootstrapVersion < 2 {
		t.Fatalf("expected bootstrap version increment after endpoint update, got %d", hbResp.BootstrapVersion)
	}
	if len(hbResp.Node.EndpointRecords) != 2 {
		t.Fatalf("expected endpoint records in heartbeat response, got %#v", hbResp.Node.EndpointRecords)
	}

	bootstrap, err := dataStore.GetBootstrap(regResp.Node.ID, regResp.NodeToken)
	if err != nil {
		t.Fatalf("get bootstrap: %v", err)
	}
	if len(bootstrap.Node.EndpointRecords) != 2 {
		t.Fatalf("expected endpoint records on bootstrap node, got %#v", bootstrap.Node.EndpointRecords)
	}
	if bootstrap.Node.EndpointRecords[0].Source != "stun" {
		t.Fatalf("expected freshest/highest-priority stun endpoint first, got %#v", bootstrap.Node.EndpointRecords)
	}
}

func TestMemoryStoreHeartbeatTimestampOnlyEndpointRefreshDoesNotBumpBootstrapVersion(t *testing.T) {
	dataStore := NewMemoryStore(config.Config{
		AdminEmail:        "admin@example.com",
		AdminPassword:     "dev-password",
		AdminToken:        "dev-admin-token",
		RegistrationToken: "dev-register-token",
		DNSDomain:         "internal.net",
		RelayAddresses:    []string{"relay-ap-1.example.net:3478"},
	})

	regResp, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "memory-endpoint-refresh",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-before",
		RegistrationToken: "dev-register-token",
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

func TestMemoryStoreHeartbeatPersistsNATSummaryAndSchedulesDirectAttempt(t *testing.T) {
	dataStore := NewMemoryStore(config.Config{
		AdminEmail:        "admin@example.com",
		AdminPassword:     "dev-password",
		AdminToken:        "dev-admin-token",
		RegistrationToken: "dev-register-token",
		DNSDomain:         "internal.net",
		RelayAddresses:    []string{"relay-ap-1.example.net:3478"},
	})

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

	if _, err := dataStore.UpdateHeartbeat(nodeA.Node.ID, nodeA.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "198.51.100.10:51820", Source: "stun"},
		},
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
}

func TestMemoryStoreSkipsDirectAttemptWhenPeerOffline(t *testing.T) {
	dataStore := NewMemoryStore(config.Config{
		AdminEmail:        "admin@example.com",
		AdminPassword:     "dev-password",
		AdminToken:        "dev-admin-token",
		RegistrationToken: "dev-register-token",
		DNSDomain:         "internal.net",
	})

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
	if _, err := dataStore.CreateDeviceAndNode(api.DeviceRegistrationRequest{
		DeviceName:        "node-b",
		Platform:          "linux",
		Version:           "0.1.0",
		PublicKey:         "pubkey-b",
		RegistrationToken: "dev-register-token",
	}); err != nil {
		t.Fatalf("register node b: %v", err)
	}

	if _, err := dataStore.UpdateHeartbeat(nodeA.Node.ID, nodeA.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "198.51.100.10:51820", Source: "stun"},
		},
	}); err != nil {
		t.Fatalf("heartbeat node a: %v", err)
	}

	hbResp, err := dataStore.UpdateHeartbeat(nodeA.Node.ID, nodeA.NodeToken, api.HeartbeatRequest{
		Status: "online",
		EndpointRecords: []api.EndpointObservation{
			{Address: "198.51.100.10:51820", Source: "stun"},
		},
	})
	if err != nil {
		t.Fatalf("second heartbeat node a: %v", err)
	}
	if len(hbResp.DirectAttempts) != 0 {
		t.Fatalf("expected no direct attempts while peer stays offline, got %#v", hbResp.DirectAttempts)
	}
}

func TestMemoryStoreDoesNotDuplicateActiveDirectAttempt(t *testing.T) {
	dataStore := NewMemoryStore(config.Config{
		AdminEmail:        "admin@example.com",
		AdminPassword:     "dev-password",
		AdminToken:        "dev-admin-token",
		RegistrationToken: "dev-register-token",
		DNSDomain:         "internal.net",
	})

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

	if _, err := dataStore.UpdateHeartbeat(nodeA.Node.ID, nodeA.NodeToken, api.HeartbeatRequest{
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "198.51.100.10:51820", Source: "stun"}},
	}); err != nil {
		t.Fatalf("heartbeat node a: %v", err)
	}
	first, err := dataStore.UpdateHeartbeat(nodeB.Node.ID, nodeB.NodeToken, api.HeartbeatRequest{
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "203.0.113.10:51820", Source: "stun"}},
	})
	if err != nil {
		t.Fatalf("first heartbeat node b: %v", err)
	}
	second, err := dataStore.UpdateHeartbeat(nodeB.Node.ID, nodeB.NodeToken, api.HeartbeatRequest{
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "203.0.113.10:51820", Source: "stun"}},
	})
	if err != nil {
		t.Fatalf("second heartbeat node b: %v", err)
	}
	if len(first.DirectAttempts) != 1 || len(second.DirectAttempts) != 1 {
		t.Fatalf("expected a single direct attempt on each response, got first=%#v second=%#v", first.DirectAttempts, second.DirectAttempts)
	}
	if first.DirectAttempts[0].AttemptID != second.DirectAttempts[0].AttemptID {
		t.Fatalf("expected active direct attempt to be reused, got %q then %q", first.DirectAttempts[0].AttemptID, second.DirectAttempts[0].AttemptID)
	}
}

func TestMemoryStoreUsesRelayActiveReasonAndSkipsWhenBothDirect(t *testing.T) {
	dataStore := NewMemoryStore(config.Config{
		AdminEmail:        "admin@example.com",
		AdminPassword:     "dev-password",
		AdminToken:        "dev-admin-token",
		RegistrationToken: "dev-register-token",
		DNSDomain:         "internal.net",
	})

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

	if _, err := dataStore.UpdateHeartbeat(nodeA.Node.ID, nodeA.NodeToken, api.HeartbeatRequest{
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "198.51.100.10:51820", Source: "stun"}},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID: nodeB.Node.ID,
			ActiveKind: "relay",
			ReportedAt: time.Now().UTC(),
		}},
	}); err != nil {
		t.Fatalf("heartbeat node a: %v", err)
	}
	resp, err := dataStore.UpdateHeartbeat(nodeB.Node.ID, nodeB.NodeToken, api.HeartbeatRequest{
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "203.0.113.10:51820", Source: "stun"}},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID: nodeA.Node.ID,
			ActiveKind: "direct",
			ReportedAt: time.Now().UTC(),
		}},
	})
	if err != nil {
		t.Fatalf("heartbeat node b: %v", err)
	}
	if len(resp.DirectAttempts) != 1 || resp.DirectAttempts[0].Reason != "relay_active" {
		t.Fatalf("expected relay_active reason, got %#v", resp.DirectAttempts)
	}

	if _, err := dataStore.UpdateHeartbeat(nodeA.Node.ID, nodeA.NodeToken, api.HeartbeatRequest{
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "198.51.100.10:51820", Source: "stun"}},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID: nodeB.Node.ID,
			ActiveKind: "direct",
			ReportedAt: time.Now().UTC(),
		}},
	}); err != nil {
		t.Fatalf("heartbeat node a direct: %v", err)
	}
	dataStore.mu.Lock()
	dataStore.directAttempts = map[string]directAttemptPair{}
	dataStore.mu.Unlock()
	resp, err = dataStore.UpdateHeartbeat(nodeB.Node.ID, nodeB.NodeToken, api.HeartbeatRequest{
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "203.0.113.10:51820", Source: "stun"}},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID: nodeA.Node.ID,
			ActiveKind: "direct",
			ReportedAt: time.Now().UTC(),
		}},
	})
	if err != nil {
		t.Fatalf("heartbeat node b direct: %v", err)
	}
	if len(resp.DirectAttempts) != 0 {
		t.Fatalf("expected no direct attempts when both peers report direct, got %#v", resp.DirectAttempts)
	}

	bootstrap, err := dataStore.GetBootstrap(nodeA.Node.ID, nodeA.NodeToken)
	if err != nil {
		t.Fatalf("get bootstrap for node a: %v", err)
	}
	if len(bootstrap.Peers) != 1 || bootstrap.Peers[0].ObservedTransportKind != "direct" {
		t.Fatalf("expected observed transport summary in bootstrap peer, got %#v", bootstrap.Peers)
	}
}

func TestMemoryStoreCooldownSkipsImmediateRetryAfterTimeout(t *testing.T) {
	dataStore := NewMemoryStore(config.Config{
		AdminEmail:        "admin@example.com",
		AdminPassword:     "dev-password",
		AdminToken:        "dev-admin-token",
		RegistrationToken: "dev-register-token",
		DNSDomain:         "internal.net",
	})

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

	if _, err := dataStore.UpdateHeartbeat(nodeA.Node.ID, nodeA.NodeToken, api.HeartbeatRequest{
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "198.51.100.10:51820", Source: "stun"}},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID:              nodeB.Node.ID,
			ActiveKind:              "relay",
			ReportedAt:              time.Now().UTC(),
			LastDirectAttemptResult: "timeout",
		}},
	}); err != nil {
		t.Fatalf("heartbeat node a: %v", err)
	}
	resp, err := dataStore.UpdateHeartbeat(nodeB.Node.ID, nodeB.NodeToken, api.HeartbeatRequest{
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "203.0.113.10:51820", Source: "stun"}},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID: nodeA.Node.ID,
			ActiveKind: "relay",
			ReportedAt: time.Now().UTC(),
		}},
	})
	if err != nil {
		t.Fatalf("heartbeat node b: %v", err)
	}
	if len(resp.DirectAttempts) != 0 {
		t.Fatalf("expected timeout cooldown to suppress retry, got %#v", resp.DirectAttempts)
	}
}

func TestMemoryStoreUsesRelayKeptCooldownOverride(t *testing.T) {
	dataStore := NewMemoryStore(config.Config{
		AdminEmail:                     "admin@example.com",
		AdminPassword:                  "dev-password",
		AdminToken:                     "dev-admin-token",
		RegistrationToken:              "dev-register-token",
		DNSDomain:                      "internal.net",
		DirectAttemptCooldown:          10 * time.Second,
		DirectAttemptTimeoutCooldown:   12 * time.Second,
		DirectAttemptRelayKeptCooldown: 2 * time.Second,
	})

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

	attemptAt := time.Now().UTC().Add(-5 * time.Second)
	if _, err := dataStore.UpdateHeartbeat(nodeA.Node.ID, nodeA.NodeToken, api.HeartbeatRequest{
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "198.51.100.10:51820", Source: "stun"}},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID:              nodeB.Node.ID,
			ActiveKind:              "relay",
			ReportedAt:              time.Now().UTC(),
			LastDirectAttemptAt:     attemptAt,
			LastDirectAttemptResult: "relay_kept",
		}},
	}); err != nil {
		t.Fatalf("heartbeat node a: %v", err)
	}
	resp, err := dataStore.UpdateHeartbeat(nodeB.Node.ID, nodeB.NodeToken, api.HeartbeatRequest{
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "203.0.113.10:51820", Source: "stun"}},
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
	if len(resp.DirectAttempts) == 0 {
		t.Fatal("expected relay_kept cooldown override to allow a new direct attempt")
	}
	if resp.DirectAttempts[0].Reason != "relay_active" && resp.DirectAttempts[0].Reason != "manual_recover" {
		t.Fatalf("expected relay-driven attempt after relay_kept cooldown expiry, got %#v", resp.DirectAttempts)
	}
}

func TestMemoryStoreUsesRelayKeptManualRecoverThresholdOverride(t *testing.T) {
	dataStore := NewMemoryStore(config.Config{
		AdminEmail:                               "admin@example.com",
		AdminPassword:                            "dev-password",
		AdminToken:                               "dev-admin-token",
		RegistrationToken:                        "dev-register-token",
		DNSDomain:                                "internal.net",
		DirectAttemptCooldown:                    2 * time.Second,
		DirectAttemptTimeoutManualRecoverAfter:   15 * time.Second,
		DirectAttemptRelayKeptManualRecoverAfter: 4 * time.Second,
	})

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

	attemptAt := time.Now().UTC().Add(-6 * time.Second)
	if _, err := dataStore.UpdateHeartbeat(nodeA.Node.ID, nodeA.NodeToken, api.HeartbeatRequest{
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "198.51.100.10:51820", Source: "stun"}},
		PeerTransportStates: []api.PeerTransportState{{
			PeerNodeID:              nodeB.Node.ID,
			ActiveKind:              "relay",
			ReportedAt:              time.Now().UTC(),
			LastDirectAttemptAt:     attemptAt,
			LastDirectAttemptResult: "relay_kept",
		}},
	}); err != nil {
		t.Fatalf("heartbeat node a: %v", err)
	}
	resp, err := dataStore.UpdateHeartbeat(nodeB.Node.ID, nodeB.NodeToken, api.HeartbeatRequest{
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "203.0.113.10:51820", Source: "stun"}},
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
	if len(resp.DirectAttempts) == 0 {
		t.Fatal("expected a direct attempt after relay_kept manual recover threshold expiry")
	}
	if resp.DirectAttempts[0].Reason != "manual_recover" {
		t.Fatalf("expected manual_recover after relay_kept threshold expiry, got %#v", resp.DirectAttempts)
	}
}

func TestMemoryStoreSuppressesAttemptsAfterFailureBudget(t *testing.T) {
	dataStore := NewMemoryStore(config.Config{
		AdminEmail:                                        "admin@example.com",
		AdminPassword:                                     "dev-password",
		AdminToken:                                        "dev-admin-token",
		RegistrationToken:                                 "dev-register-token",
		DNSDomain:                                         "internal.net",
		DirectAttemptCooldown:                             2 * time.Second,
		DirectAttemptFailureSuppressAfter:                 3,
		DirectAttemptFailureSuppressWindow:                2 * time.Minute,
		DirectAttemptTimeoutSuppressAfter:                 3,
		DirectAttemptTimeoutSuppressWindow:                2 * time.Minute,
		DirectAttemptSuppressedProbeLimit:                 2,
		DirectAttemptTimeoutSuppressedProbeLimit:          2,
		DirectAttemptSuppressedProbeRefillInterval:        30 * time.Second,
		DirectAttemptTimeoutSuppressedProbeRefillInterval: 30 * time.Second,
	})

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

	attemptAt := time.Now().UTC().Add(-5 * time.Second)
	if _, err := dataStore.UpdateHeartbeat(nodeA.Node.ID, nodeA.NodeToken, api.HeartbeatRequest{
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "198.51.100.10:51820", Source: "stun"}},
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
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "203.0.113.10:51820", Source: "stun"}},
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
		t.Fatalf("expected suppression after repeated failures, got %#v", resp.DirectAttempts)
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
	if len(bootstrap.Peers) != 1 || bootstrap.Peers[0].ObservedConsecutiveDirectFailures != 3 {
		t.Fatalf("expected observed failure budget in bootstrap peer, got %#v", bootstrap.Peers)
	}
	if !bootstrap.Peers[0].ObservedDirectRecoveryBlocked || bootstrap.Peers[0].ObservedDirectRecoveryBlockReason != "suppressed_timeout_budget" {
		t.Fatalf("expected observed recovery block state in bootstrap peer, got %#v", bootstrap.Peers)
	}
	if !bootstrap.Peers[0].ObservedDirectRecoveryProbeLimited || bootstrap.Peers[0].ObservedDirectRecoveryProbeRemaining != 2 {
		t.Fatalf("expected observed recovery probe budget in bootstrap peer, got %#v", bootstrap.Peers)
	}
	if !bootstrap.Peers[0].ObservedDirectRecoveryProbeRefillAt.IsZero() {
		t.Fatalf("expected untouched probe budget to omit refill time in bootstrap peer, got %#v", bootstrap.Peers)
	}
}

func TestMemoryStoreSchedulesSuppressedProbeAfterInterval(t *testing.T) {
	cfg := config.Config{
		RegistrationToken:                          "dev-register-token",
		DirectAttemptCooldown:                      2 * time.Second,
		DirectAttemptFailureSuppressAfter:          3,
		DirectAttemptFailureSuppressWindow:         90 * time.Second,
		DirectAttemptTimeoutSuppressAfter:          3,
		DirectAttemptTimeoutSuppressWindow:         90 * time.Second,
		DirectAttemptSuppressedProbeInterval:       15 * time.Second,
		DirectAttemptSuppressedProbeLimit:          2,
		DirectAttemptSuppressedProbeRefillInterval: 30 * time.Second,
	}
	dataStore := NewMemoryStore(cfg)

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
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "198.51.100.10:51820", Source: "stun"}},
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
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "203.0.113.10:51820", Source: "stun"}},
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

func TestMemoryStoreStopsSuppressedProbeWhenBudgetExhausted(t *testing.T) {
	dataStore := NewMemoryStore(config.Config{
		RegistrationToken:                          "dev-register-token",
		DirectAttemptCooldown:                      2 * time.Second,
		DirectAttemptFailureSuppressAfter:          3,
		DirectAttemptFailureSuppressWindow:         90 * time.Second,
		DirectAttemptTimeoutSuppressAfter:          3,
		DirectAttemptTimeoutSuppressWindow:         90 * time.Second,
		DirectAttemptSuppressedProbeInterval:       15 * time.Second,
		DirectAttemptSuppressedProbeLimit:          2,
		DirectAttemptSuppressedProbeRefillInterval: 30 * time.Second,
	})

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
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "198.51.100.10:51820", Source: "stun"}},
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
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "203.0.113.10:51820", Source: "stun"}},
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

func TestMemoryStoreRefillsSuppressedProbeBudgetAfterQuietPeriod(t *testing.T) {
	dataStore := NewMemoryStore(config.Config{
		RegistrationToken:                          "dev-register-token",
		DirectAttemptCooldown:                      2 * time.Second,
		DirectAttemptFailureSuppressAfter:          3,
		DirectAttemptFailureSuppressWindow:         90 * time.Second,
		DirectAttemptTimeoutSuppressAfter:          3,
		DirectAttemptTimeoutSuppressWindow:         90 * time.Second,
		DirectAttemptSuppressedProbeInterval:       15 * time.Second,
		DirectAttemptSuppressedProbeLimit:          2,
		DirectAttemptSuppressedProbeRefillInterval: 30 * time.Second,
	})

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
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "198.51.100.10:51820", Source: "stun"}},
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
		Status:          "online",
		EndpointRecords: []api.EndpointObservation{{Address: "203.0.113.10:51820", Source: "stun"}},
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
