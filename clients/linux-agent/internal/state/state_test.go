package state

import (
	"path/filepath"
	"testing"
	"time"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/packages/runtime/go/dataplane"
	"nodeweave/packages/runtime/go/driver"
	"nodeweave/packages/runtime/go/overlay"
	linuxplan "nodeweave/packages/runtime/go/plan/linux"
	"nodeweave/packages/runtime/go/secureudp"
	"nodeweave/packages/runtime/go/session"
	"nodeweave/packages/runtime/go/stun"
)

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")

	want := File{
		ServerURL: "http://127.0.0.1:8080",
		Device: api.Device{
			ID:   "dev_1",
			Name: "agent",
		},
		Node: api.Node{
			ID:        "node_1",
			OverlayIP: "100.64.0.10",
		},
		NodeToken: "node_token",
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("save state: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	if got.ServerURL != want.ServerURL || got.Node.ID != want.Node.ID || got.NodeToken != want.NodeToken {
		t.Fatalf("unexpected roundtrip state: %#v", got)
	}
}

func TestSaveAndLoadPlan(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "plan.json")

	want := linuxplan.Plan{
		NodeID:    "node_1",
		Interface: "nw0",
		Operations: []linuxplan.Operation{
			{
				Description: "ensure TUN interface exists",
				Command:     []string{"ip", "tuntap", "add", "dev", "nw0", "mode", "tun"},
			},
		},
	}

	if err := SavePlan(path, want); err != nil {
		t.Fatalf("save plan: %v", err)
	}

	got, err := LoadPlan(path)
	if err != nil {
		t.Fatalf("load plan: %v", err)
	}
	if got.NodeID != want.NodeID || got.Interface != want.Interface || len(got.Operations) != 1 {
		t.Fatalf("unexpected plan roundtrip: %#v", got)
	}
}

func TestSaveAndLoadRuntime(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "runtime.json")

	want := overlay.Snapshot{
		NodeID: "node_1",
		Interface: overlay.InterfaceState{
			Name:        "nw0",
			AddressCIDR: "100.64.0.10/10",
			MTU:         1380,
		},
	}

	if err := SaveRuntime(path, want); err != nil {
		t.Fatalf("save runtime: %v", err)
	}

	got, err := LoadRuntime(path)
	if err != nil {
		t.Fatalf("load runtime: %v", err)
	}
	if got.NodeID != want.NodeID || got.Interface.AddressCIDR != want.Interface.AddressCIDR {
		t.Fatalf("unexpected runtime roundtrip: %#v", got)
	}
}

func TestSaveAndLoadApplyReport(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "apply-report.json")

	want := driver.Report{
		Backend: "linux-plan",
		Success: true,
		Operations: []driver.OperationResult{
			{
				Description: "noop",
				Command:     []string{"true"},
				ExitCode:    0,
			},
		},
	}

	if err := SaveApplyReport(path, want); err != nil {
		t.Fatalf("save apply report: %v", err)
	}

	got, err := LoadApplyReport(path)
	if err != nil {
		t.Fatalf("load apply report: %v", err)
	}
	if got.Backend != want.Backend || len(got.Operations) != 1 {
		t.Fatalf("unexpected apply report roundtrip: %#v", got)
	}
}

func TestSaveAndLoadSession(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "session.json")

	want := session.Spec{
		NodeID:        "node_1",
		ListenAddress: "0.0.0.0:51820",
		Peers: []session.Peer{
			{
				NodeID:             "node_2",
				PreferredCandidate: "198.51.100.10:51820",
				Candidates: []session.Candidate{
					{Kind: "direct", Address: "198.51.100.10:51820", Priority: 1000},
				},
			},
		},
	}

	if err := SaveSession(path, want); err != nil {
		t.Fatalf("save session: %v", err)
	}

	got, err := LoadSession(path)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if got.NodeID != want.NodeID || len(got.Peers) != 1 {
		t.Fatalf("unexpected session roundtrip: %#v", got)
	}
}

func TestSaveAndLoadSessionReport(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "session-report.json")

	want := session.Report{
		NodeID: "node_1",
		Mode:   "off",
		Peers: []session.PeerReport{
			{
				NodeID:             "node_2",
				PreferredCandidate: "198.51.100.10:51820",
				SelectedCandidate:  "198.51.100.10:51820",
				Candidates: []session.CandidateResult{
					{Kind: "direct", Address: "198.51.100.10:51820", Status: "disabled"},
				},
			},
		},
	}

	if err := SaveSessionReport(path, want); err != nil {
		t.Fatalf("save session report: %v", err)
	}

	got, err := LoadSessionReport(path)
	if err != nil {
		t.Fatalf("load session report: %v", err)
	}
	if got.Mode != want.Mode || len(got.Peers) != 1 {
		t.Fatalf("unexpected session report roundtrip: %#v", got)
	}
}

func TestSaveAndLoadDataplane(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "dataplane.json")

	want := dataplane.Spec{
		NodeID:        "node_1",
		ListenAddress: "127.0.0.1:51820",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.2/32",
				PeerNodeID:       "node_2",
				CandidateAddress: "198.51.100.10:51820",
				CandidateKind:    "direct",
				RoutePriority:    1000,
			},
		},
	}

	if err := SaveDataplane(path, want); err != nil {
		t.Fatalf("save dataplane: %v", err)
	}

	got, err := LoadDataplane(path)
	if err != nil {
		t.Fatalf("load dataplane: %v", err)
	}
	if got.NodeID != want.NodeID || len(got.Routes) != 1 {
		t.Fatalf("unexpected dataplane roundtrip: %#v", got)
	}
}

func TestSaveAndLoadTransportReport(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "transport-report.json")

	want := secureudp.Report{
		NodeID:           "node_1",
		ListenAddress:    "127.0.0.1:51820",
		DirectRetryAfter: "15s",
		Peers: []secureudp.PeerStatus{
			{
				NodeID:        "node_2",
				ActiveAddress: "relay.example.net:3478",
				ActiveKind:    "relay",
				Candidates: []secureudp.CandidateStatus{
					{Kind: "direct", Address: "198.51.100.10:51820", Priority: 1000},
					{Kind: "relay", Address: "relay.example.net:3478", Priority: 500, Active: true},
				},
			},
		},
	}

	if err := SaveTransportReport(path, want); err != nil {
		t.Fatalf("save transport report: %v", err)
	}

	got, err := LoadTransportReport(path)
	if err != nil {
		t.Fatalf("load transport report: %v", err)
	}
	if got.NodeID != want.NodeID || len(got.Peers) != 1 {
		t.Fatalf("unexpected transport report roundtrip: %#v", got)
	}
}

func TestSaveAndLoadRecoveryStates(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "recovery-state.json")

	want := []api.PeerRecoveryState{
		{
			PeerNodeID:     "node_2",
			Blocked:        true,
			BlockReason:    "suppressed_timeout_budget",
			BlockedUntil:   time.Now().UTC().Add(30 * time.Second),
			DecisionStatus: "blocked",
			DecisionReason: "suppressed_timeout_budget",
			DecisionAt:     time.Now().UTC(),
			DecisionNextAt: time.Now().UTC().Add(15 * time.Second),
		},
	}

	if err := SaveRecoveryStates(path, want); err != nil {
		t.Fatalf("save recovery states: %v", err)
	}

	got, err := LoadRecoveryStates(path)
	if err != nil {
		t.Fatalf("load recovery states: %v", err)
	}
	if len(got) != 1 || got[0].PeerNodeID != want[0].PeerNodeID || !got[0].Blocked {
		t.Fatalf("unexpected recovery state roundtrip: %#v", got)
	}
	if got[0].DecisionStatus != want[0].DecisionStatus || got[0].DecisionReason != want[0].DecisionReason || got[0].DecisionAt.IsZero() || got[0].DecisionNextAt.IsZero() {
		t.Fatalf("expected decision metadata roundtrip, got %#v", got)
	}
}

func TestSaveAndLoadDirectAttempts(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "direct-attempts.json")

	want := []api.DirectAttemptInstruction{
		{
			AttemptID:     "attempt-1",
			PeerNodeID:    "node_2",
			ExecuteAt:     time.Now().UTC().Add(5 * time.Second),
			Window:        600,
			BurstInterval: 80,
			Candidates:    []string{"203.0.113.10:51820"},
			Reason:        "manual_recover",
		},
	}

	if err := SaveDirectAttempts(path, want); err != nil {
		t.Fatalf("save direct attempts: %v", err)
	}

	got, err := LoadDirectAttempts(path)
	if err != nil {
		t.Fatalf("load direct attempts: %v", err)
	}
	if len(got) != 1 || got[0].AttemptID != want[0].AttemptID || got[0].PeerNodeID != want[0].PeerNodeID {
		t.Fatalf("unexpected direct attempt roundtrip: %#v", got)
	}
}

func TestSaveAndLoadDirectAttemptReport(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "direct-attempt-report.json")

	want := DirectAttemptReport{
		GeneratedAt: time.Now().UTC(),
		Entries: []DirectAttemptReportEntry{
			{
				AttemptID:      "attempt-1",
				PeerNodeID:     "node_2",
				IssuedAt:       time.Now().UTC().Add(-1 * time.Second),
				ExecuteAt:      time.Now().UTC().Add(5 * time.Second),
				Status:         "waiting_transport",
				Result:         "queued",
				WaitReason:     "transport_unavailable",
				QueuedAt:       time.Now().UTC(),
				LastUpdatedAt:  time.Now().UTC(),
				Candidates:     []string{"203.0.113.10:51820"},
				ReachedAddress: "",
				ActiveAddress:  "",
			},
		},
	}

	if err := SaveDirectAttemptReport(path, want); err != nil {
		t.Fatalf("save direct attempt report: %v", err)
	}

	got, err := LoadDirectAttemptReport(path)
	if err != nil {
		t.Fatalf("load direct attempt report: %v", err)
	}
	if len(got.Entries) != 1 || got.Entries[0].AttemptID != want.Entries[0].AttemptID || got.Entries[0].WaitReason != want.Entries[0].WaitReason {
		t.Fatalf("unexpected direct attempt report roundtrip: %#v", got)
	}
}

func TestSaveAndLoadSTUNReport(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "stun-report.json")

	want := stun.Report{
		Reachable:       true,
		SelectedAddress: "198.51.100.20:54321",
		Servers: []stun.Result{
			{
				Server:           "stun.example.net:3478",
				Status:           "reachable",
				RTTMillis:        12,
				ReflexiveAddress: "198.51.100.20:54321",
			},
		},
	}

	if err := SaveSTUNReport(path, want); err != nil {
		t.Fatalf("save stun report: %v", err)
	}

	got, err := LoadSTUNReport(path)
	if err != nil {
		t.Fatalf("load stun report: %v", err)
	}
	if !got.Reachable || got.SelectedAddress != want.SelectedAddress || len(got.Servers) != 1 {
		t.Fatalf("unexpected stun report roundtrip: %#v", got)
	}
}
