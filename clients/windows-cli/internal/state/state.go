package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"nodeweave/packages/contracts/go/api"
)

type File struct {
	ServerURL string     `json:"server_url"`
	Device    api.Device `json:"device"`
	Node      api.Node   `json:"node"`
	NodeToken string     `json:"node_token"`
}

func DefaultPath() string {
	baseDir, err := os.UserConfigDir()
	if err != nil {
		return ".nodeweave-windows-cli.json"
	}
	return filepath.Join(baseDir, "nodeweave", "windows-cli.json")
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
