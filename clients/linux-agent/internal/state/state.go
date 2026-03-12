package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

type File struct {
	ServerURL       string              `json:"server_url"`
	Device          api.Device          `json:"device"`
	Node            api.Node            `json:"node"`
	NodeToken       string              `json:"node_token"`
	Bootstrap       api.BootstrapConfig `json:"bootstrap"`
	LastHeartbeatAt time.Time           `json:"last_heartbeat_at,omitempty"`
	LastBootstrapAt time.Time           `json:"last_bootstrap_at,omitempty"`
}

func LoadRuntime(path string) (overlay.Snapshot, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return overlay.Snapshot{}, fmt.Errorf("read runtime file: %w", err)
	}
	var snapshot overlay.Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return overlay.Snapshot{}, fmt.Errorf("parse runtime file: %w", err)
	}
	return snapshot, nil
}

func LoadPlan(path string) (linuxplan.Plan, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return linuxplan.Plan{}, fmt.Errorf("read plan file: %w", err)
	}
	var plan linuxplan.Plan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return linuxplan.Plan{}, fmt.Errorf("parse plan file: %w", err)
	}
	return plan, nil
}

func LoadApplyReport(path string) (driver.Report, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return driver.Report{}, fmt.Errorf("read apply report file: %w", err)
	}
	var report driver.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return driver.Report{}, fmt.Errorf("parse apply report file: %w", err)
	}
	return report, nil
}

func LoadSession(path string) (session.Spec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return session.Spec{}, fmt.Errorf("read session file: %w", err)
	}
	var spec session.Spec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return session.Spec{}, fmt.Errorf("parse session file: %w", err)
	}
	return spec, nil
}

func LoadSessionReport(path string) (session.Report, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return session.Report{}, fmt.Errorf("read session report file: %w", err)
	}
	var report session.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return session.Report{}, fmt.Errorf("parse session report file: %w", err)
	}
	return report, nil
}

func LoadDataplane(path string) (dataplane.Spec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return dataplane.Spec{}, fmt.Errorf("read dataplane file: %w", err)
	}
	var spec dataplane.Spec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return dataplane.Spec{}, fmt.Errorf("parse dataplane file: %w", err)
	}
	return spec, nil
}

func LoadTransportReport(path string) (secureudp.Report, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return secureudp.Report{}, fmt.Errorf("read transport report file: %w", err)
	}
	var report secureudp.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return secureudp.Report{}, fmt.Errorf("parse transport report file: %w", err)
	}
	return report, nil
}

func LoadRecoveryStates(path string) ([]api.PeerRecoveryState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read recovery state file: %w", err)
	}
	var states []api.PeerRecoveryState
	if err := json.Unmarshal(raw, &states); err != nil {
		return nil, fmt.Errorf("parse recovery state file: %w", err)
	}
	return states, nil
}

func LoadDirectAttempts(path string) ([]api.DirectAttemptInstruction, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read direct attempt file: %w", err)
	}
	var attempts []api.DirectAttemptInstruction
	if err := json.Unmarshal(raw, &attempts); err != nil {
		return nil, fmt.Errorf("parse direct attempt file: %w", err)
	}
	return attempts, nil
}

func LoadSTUNReport(path string) (stun.Report, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return stun.Report{}, fmt.Errorf("read stun report file: %w", err)
	}
	var report stun.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return stun.Report{}, fmt.Errorf("parse stun report file: %w", err)
	}
	return report, nil
}

func Load(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, fmt.Errorf("read state file: %w", err)
	}
	var file File
	if err := json.Unmarshal(raw, &file); err != nil {
		return File{}, fmt.Errorf("parse state file: %w", err)
	}
	return file, nil
}

func Save(path string, file File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	return nil
}

func SaveRuntime(path string, snapshot overlay.Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create runtime dir: %w", err)
	}
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal runtime file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write runtime file: %w", err)
	}
	return nil
}

func SavePlan(path string, plan linuxplan.Plan) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create plan dir: %w", err)
	}
	raw, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plan file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write plan file: %w", err)
	}
	return nil
}

func SaveApplyReport(path string, report driver.Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create apply report dir: %w", err)
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal apply report file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write apply report file: %w", err)
	}
	return nil
}

func SaveSession(path string, spec session.Spec) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	raw, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}
	return nil
}

func SaveSessionReport(path string, report session.Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create session report dir: %w", err)
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session report file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write session report file: %w", err)
	}
	return nil
}

func SaveDataplane(path string, spec dataplane.Spec) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dataplane dir: %w", err)
	}
	raw, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal dataplane file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write dataplane file: %w", err)
	}
	return nil
}

func SaveTransportReport(path string, report secureudp.Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create transport report dir: %w", err)
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal transport report file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write transport report file: %w", err)
	}
	return nil
}

func SaveRecoveryStates(path string, states []api.PeerRecoveryState) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create recovery state dir: %w", err)
	}
	raw, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal recovery state file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write recovery state file: %w", err)
	}
	return nil
}

func SaveDirectAttempts(path string, attempts []api.DirectAttemptInstruction) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create direct attempt dir: %w", err)
	}
	raw, err := json.MarshalIndent(attempts, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal direct attempt file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write direct attempt file: %w", err)
	}
	return nil
}

func SaveSTUNReport(path string, report stun.Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create stun report dir: %w", err)
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal stun report file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write stun report file: %w", err)
	}
	return nil
}
