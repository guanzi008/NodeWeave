package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/packages/runtime/go/forwarding/serial"
	"nodeweave/packages/runtime/go/forwarding/usb"
	"nodeweave/packages/runtime/go/overlay"
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

func LoadBootstrap(path string) (api.BootstrapConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return api.BootstrapConfig{}, fmt.Errorf("read bootstrap file: %w", err)
	}
	var bootstrap api.BootstrapConfig
	if err := json.Unmarshal(raw, &bootstrap); err != nil {
		return api.BootstrapConfig{}, fmt.Errorf("parse bootstrap file: %w", err)
	}
	return bootstrap, nil
}

func SaveBootstrap(path string, bootstrap api.BootstrapConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create bootstrap dir: %w", err)
	}
	raw, err := json.MarshalIndent(bootstrap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal bootstrap file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write bootstrap file: %w", err)
	}
	return nil
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
