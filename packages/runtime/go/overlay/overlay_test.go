package overlay

import (
	"testing"
	"time"

	"nodeweave/packages/contracts/go/api"
)

func TestCompile(t *testing.T) {
	bootstrap := api.BootstrapConfig{
		Version:     2,
		OverlayCIDR: "100.64.0.0/10",
		Node: api.Node{
			ID:        "node-self",
			OverlayIP: "100.64.0.10",
		},
		Peers: []api.Peer{
			{
				NodeID:    "node-peer",
				OverlayIP: "100.64.0.11",
				PublicKey: "peer-key",
				Endpoints: []string{"203.0.113.10:51820"},
				EndpointRecords: []api.EndpointObservation{
					{
						Address:    "203.0.113.10:51820",
						Source:     "stun",
						ObservedAt: time.Now().UTC(),
					},
				},
				RelayRegion: "ap",
				AllowedIPs:  []string{"100.64.0.11/32", "10.20.0.0/16"},
				Status:      "online",
				LastSeenAt:  time.Now().UTC(),
			},
		},
		Routes: []api.Route{
			{
				NetworkCIDR: "10.20.0.0/16",
				ViaNodeID:   "node-peer",
				Priority:    100,
			},
		},
		DNS: api.DNSConfig{
			Domain:      "internal.net",
			Nameservers: []string{"100.64.0.53"},
		},
		Relays: []api.RelayNode{
			{Region: "ap", Address: "relay-ap-1.example.net:3478"},
		},
		ACL: api.ACLSnapshot{
			Version:       2,
			DefaultAction: "deny",
		},
	}

	snapshot, err := Compile(bootstrap, Config{InterfaceName: "nw0", MTU: 1380}, "dry-run")
	if err != nil {
		t.Fatalf("compile snapshot: %v", err)
	}

	if snapshot.Interface.AddressCIDR != "100.64.0.10/10" {
		t.Fatalf("unexpected interface cidr: %s", snapshot.Interface.AddressCIDR)
	}
	if len(snapshot.Peers) != 1 || snapshot.Peers[0].NodeID != "node-peer" {
		t.Fatalf("unexpected peers: %#v", snapshot.Peers)
	}
	if len(snapshot.Peers[0].EndpointRecords) != 1 || snapshot.Peers[0].EndpointRecords[0].Source != "stun" {
		t.Fatalf("expected endpoint records to be preserved, got %#v", snapshot.Peers[0].EndpointRecords)
	}
	if len(snapshot.Routes) != 1 || snapshot.Routes[0].ViaOverlay != "100.64.0.11" {
		t.Fatalf("unexpected routes: %#v", snapshot.Routes)
	}
}
