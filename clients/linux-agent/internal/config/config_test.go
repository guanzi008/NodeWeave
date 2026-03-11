package config

import (
	"path/filepath"
	"testing"
)

func TestWriteExampleAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "linux-agent.json")

	if err := WriteExample(configPath); err != nil {
		t.Fatalf("write example config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.ServerURL == "" || cfg.StatePath == "" || cfg.BootstrapInterval <= 0 || cfg.HeartbeatInterval <= 0 || cfg.SessionProbeTimeout <= 0 {
		t.Fatalf("unexpected config after load: %#v", cfg)
	}
	if cfg.PrivateKeyPath == "" {
		t.Fatalf("expected private_key_path to be populated, got %#v", cfg)
	}
}
