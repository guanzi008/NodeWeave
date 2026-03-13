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
	"nodeweave/packages/runtime/go/forwarding/serial"
	"nodeweave/packages/runtime/go/forwarding/usb"
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
	var compat []struct {
		AttemptID     string          `json:"attempt_id"`
		PeerNodeID    string          `json:"peer_node_id"`
		IssuedAt      time.Time       `json:"issued_at,omitempty"`
		ExecuteAt     time.Time       `json:"execute_at"`
		Window        int64           `json:"window,omitempty"`
		BurstInterval int64           `json:"burst_interval,omitempty"`
		Candidates    json.RawMessage `json:"candidates,omitempty"`
		Profile       string          `json:"profile,omitempty"`
		Reason        string          `json:"reason,omitempty"`
	}
	if err := json.Unmarshal(raw, &compat); err != nil {
		return nil, fmt.Errorf("parse direct attempt file: %w", err)
	}
	attempts := make([]api.DirectAttemptInstruction, 0, len(compat))
	for _, instruction := range compat {
		candidates, err := api.UnmarshalDirectAttemptCandidatesJSON(instruction.Candidates, instruction.IssuedAt, instruction.ExecuteAt)
		if err != nil {
			return nil, fmt.Errorf("parse direct attempt candidates: %w", err)
		}
		attempts = append(attempts, api.DirectAttemptInstruction{
			AttemptID:     instruction.AttemptID,
			PeerNodeID:    instruction.PeerNodeID,
			IssuedAt:      instruction.IssuedAt,
			ExecuteAt:     instruction.ExecuteAt,
			Window:        instruction.Window,
			BurstInterval: instruction.BurstInterval,
			Candidates:    candidates,
			Profile:       instruction.Profile,
			Reason:        instruction.Reason,
		})
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

func LoadSerialForwards(path string) ([]serial.SessionSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read serial forward file: %w", err)
	}
	var specs []serial.SessionSpec
	if err := json.Unmarshal(raw, &specs); err != nil {
		return nil, fmt.Errorf("parse serial forward file: %w", err)
	}
	for idx, spec := range specs {
		specs[idx] = serial.NormalizeSessionSpec(spec)
	}
	return specs, nil
}

func LoadSerialForwardReport(path string) ([]serial.SessionReport, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read serial forward report file: %w", err)
	}
	var reports []serial.SessionReport
	if err := json.Unmarshal(raw, &reports); err != nil {
		return nil, fmt.Errorf("parse serial forward report file: %w", err)
	}
	return reports, nil
}

func LoadUSBForwards(path string) ([]usb.SessionSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read usb forward file: %w", err)
	}
	var specs []usb.SessionSpec
	if err := json.Unmarshal(raw, &specs); err != nil {
		return nil, fmt.Errorf("parse usb forward file: %w", err)
	}
	for idx, spec := range specs {
		specs[idx] = usb.NormalizeSessionSpec(spec)
	}
	return specs, nil
}

func LoadUSBForwardReport(path string) ([]usb.SessionReport, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read usb forward report file: %w", err)
	}
	var reports []usb.SessionReport
	if err := json.Unmarshal(raw, &reports); err != nil {
		return nil, fmt.Errorf("parse usb forward report file: %w", err)
	}
	return reports, nil
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

func SaveSerialForwards(path string, specs []serial.SessionSpec) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create serial forward dir: %w", err)
	}
	raw, err := json.MarshalIndent(specs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal serial forward file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write serial forward file: %w", err)
	}
	return nil
}

func SaveSerialForwardReport(path string, reports []serial.SessionReport) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create serial forward report dir: %w", err)
	}
	raw, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal serial forward report file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write serial forward report file: %w", err)
	}
	return nil
}

func SaveUSBForwards(path string, specs []usb.SessionSpec) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create usb forward dir: %w", err)
	}
	raw, err := json.MarshalIndent(specs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal usb forward file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write usb forward file: %w", err)
	}
	return nil
}

func SaveUSBForwardReport(path string, reports []usb.SessionReport) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create usb forward report dir: %w", err)
	}
	raw, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal usb forward report file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write usb forward report file: %w", err)
	}
	return nil
}
