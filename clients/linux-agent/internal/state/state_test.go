package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/packages/runtime/go/dataplane"
	"nodeweave/packages/runtime/go/driver"
	"nodeweave/packages/runtime/go/forwarding/serial"
	"nodeweave/packages/runtime/go/forwarding/usb"
	"nodeweave/packages/runtime/go/overlay"
	linuxplan "nodeweave/packages/runtime/go/plan/linux"
	"nodeweave/packages/runtime/go/secureudp"
	"nodeweave/packages/runtime/go/session"
	"nodeweave/packages/runtime/go/stun"
)

func stateDirectAttemptCandidate(address string) api.DirectAttemptCandidate {
	return api.DirectAttemptCandidate{
		Address:  address,
		Source:   "heartbeat",
		Priority: 1000,
		Phase:    api.DirectAttemptPhasePrimary,
	}
}

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
			Candidates:    []api.DirectAttemptCandidate{stateDirectAttemptCandidate("203.0.113.10:51820")},
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
				Profile:        "primary_upgrade",
				Status:         "waiting_transport",
				Result:         "queued",
				WaitReason:     "transport_unavailable",
				QueuedAt:       time.Now().UTC(),
				LastUpdatedAt:  time.Now().UTC(),
				Candidates:     []api.DirectAttemptCandidate{stateDirectAttemptCandidate("203.0.113.10:51820")},
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
	if len(got.Entries) != 1 || got.Entries[0].AttemptID != want.Entries[0].AttemptID || got.Entries[0].WaitReason != want.Entries[0].WaitReason || got.Entries[0].Profile != want.Entries[0].Profile {
		t.Fatalf("unexpected direct attempt report roundtrip: %#v", got)
	}
}

func TestLoadDirectAttemptsMigratesLegacyCandidates(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "direct-attempts-legacy.json")
	raw := []byte(`[
  {
    "attempt_id": "attempt-legacy",
    "peer_node_id": "node_2",
    "issued_at": "2026-03-12T10:00:00Z",
    "execute_at": "2026-03-12T10:00:05Z",
    "window": 600,
    "burst_interval": 80,
    "candidates": ["203.0.113.10:51820"],
    "profile": "primary_upgrade",
    "reason": "manual_recover"
  }
]`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write legacy direct attempts: %v", err)
	}

	got, err := LoadDirectAttempts(path)
	if err != nil {
		t.Fatalf("load legacy direct attempts: %v", err)
	}
	if len(got) != 1 || len(got[0].Candidates) != 1 {
		t.Fatalf("unexpected migrated direct attempts: %#v", got)
	}
	candidate := got[0].Candidates[0]
	if candidate.Address != "203.0.113.10:51820" || candidate.Source != "heartbeat" || candidate.Phase != api.DirectAttemptPhasePrimary {
		t.Fatalf("unexpected migrated candidate: %#v", candidate)
	}
	if got[0].Profile != "primary_upgrade" {
		t.Fatalf("expected direct attempt profile to roundtrip, got %#v", got)
	}

	if err := SaveDirectAttempts(path, got); err != nil {
		t.Fatalf("save migrated direct attempts: %v", err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated direct attempts: %v", err)
	}
	if !strings.Contains(string(saved), "\"address\"") {
		t.Fatalf("expected migrated direct attempts to persist object candidates, got %s", string(saved))
	}
}

func TestLoadDirectAttemptReportMigratesLegacyCandidates(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "direct-attempt-report-legacy.json")
	raw := []byte(`{
  "generated_at": "2026-03-12T10:00:00Z",
  "entries": [
    {
      "attempt_id": "attempt-legacy",
      "peer_node_id": "node_2",
      "issued_at": "2026-03-12T10:00:00Z",
      "execute_at": "2026-03-12T10:00:05Z",
      "candidates": ["203.0.113.10:51820"],
      "status": "queued",
      "result": "queued"
    }
  ]
}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write legacy direct attempt report: %v", err)
	}

	got, err := LoadDirectAttemptReport(path)
	if err != nil {
		t.Fatalf("load legacy direct attempt report: %v", err)
	}
	if len(got.Entries) != 1 || len(got.Entries[0].Candidates) != 1 {
		t.Fatalf("unexpected migrated direct attempt report: %#v", got)
	}
	candidate := got.Entries[0].Candidates[0]
	if candidate.Address != "203.0.113.10:51820" || candidate.Source != "heartbeat" || candidate.Phase != api.DirectAttemptPhasePrimary {
		t.Fatalf("unexpected migrated report candidate: %#v", candidate)
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

func TestSaveAndLoadSerialForwards(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "serial-forwards.json")
	reportPath := filepath.Join(tmpDir, "serial-forward-report.json")
	specs := []serial.SessionSpec{
		{
			NodeID:     "node-a",
			PeerNodeID: "node-b",
			Local:      serial.PortConfig{Name: "/dev/ttyUSB0", BaudRate: 9600},
			Remote:     serial.PortConfig{Name: "COM7", BaudRate: 9600},
		},
	}

	if err := SaveSerialForwards(path, specs); err != nil {
		t.Fatalf("save serial forwards: %v", err)
	}
	if err := SaveSerialForwardReport(reportPath, []serial.SessionReport{serial.ConfiguredReport(specs[0], "linux-agent")}); err != nil {
		t.Fatalf("save serial forward report: %v", err)
	}

	gotSpecs, err := LoadSerialForwards(path)
	if err != nil {
		t.Fatalf("load serial forwards: %v", err)
	}
	if len(gotSpecs) != 1 || gotSpecs[0].Local.Name != "/dev/ttyUSB0" {
		t.Fatalf("unexpected serial forwarding state: %#v", gotSpecs)
	}

	gotReports, err := LoadSerialForwardReport(reportPath)
	if err != nil {
		t.Fatalf("load serial forward report: %v", err)
	}
	if len(gotReports) != 1 || gotReports[0].ClosedBy != "linux-agent" {
		t.Fatalf("unexpected serial forwarding report: %#v", gotReports)
	}
}

func TestSaveAndLoadUSBForwards(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "usb-forwards.json")
	reportPath := filepath.Join(tmpDir, "usb-forward-report.json")
	specs := []usb.SessionSpec{
		{
			NodeID:     "node-a",
			PeerNodeID: "node-b",
			Local:      usb.DeviceDescriptor{BusID: "1", DeviceID: "3", VendorID: "1d6b", ProductID: "0002"},
			Remote:     usb.DeviceDescriptor{VendorID: "1d6b", ProductID: "0002", Interface: "0"},
		},
	}

	if err := SaveUSBForwards(path, specs); err != nil {
		t.Fatalf("save usb forwards: %v", err)
	}
	if err := SaveUSBForwardReport(reportPath, []usb.SessionReport{usb.ConfiguredReport(specs[0], "linux-agent")}); err != nil {
		t.Fatalf("save usb forward report: %v", err)
	}

	gotSpecs, err := LoadUSBForwards(path)
	if err != nil {
		t.Fatalf("load usb forwards: %v", err)
	}
	if len(gotSpecs) != 1 || gotSpecs[0].Local.BusID != "1" || gotSpecs[0].Remote.Interface != "0" {
		t.Fatalf("unexpected usb forwarding state: %#v", gotSpecs)
	}

	gotReports, err := LoadUSBForwardReport(reportPath)
	if err != nil {
		t.Fatalf("load usb forward report: %v", err)
	}
	if len(gotReports) != 1 || gotReports[0].ClaimedBy != "linux-agent" {
		t.Fatalf("unexpected usb forwarding report: %#v", gotReports)
	}
}
