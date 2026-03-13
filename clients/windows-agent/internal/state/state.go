package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"nodeweave/packages/contracts/go/api"
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
