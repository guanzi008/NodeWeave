package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"nodeweave/packages/runtime/go/forwarding/serial"
	"nodeweave/packages/runtime/go/forwarding/usb"
)

type Config struct {
	ServerURL               string               `json:"server_url"`
	RegistrationToken       string               `json:"registration_token"`
	DeviceName              string               `json:"device_name"`
	Platform                string               `json:"platform"`
	Version                 string               `json:"version"`
	PublicKey               string               `json:"public_key"`
	StatePath               string               `json:"state_path"`
	BootstrapPath           string               `json:"bootstrap_path"`
	RuntimePath             string               `json:"runtime_path"`
	SerialForwardPath       string               `json:"serial_forward_path"`
	SerialForwardReportPath string               `json:"serial_forward_report_path"`
	SerialForwards          []serial.SessionSpec `json:"serial_forwards,omitempty"`
	USBForwardPath          string               `json:"usb_forward_path"`
	USBForwardReportPath    string               `json:"usb_forward_report_path"`
	USBForwards             []usb.SessionSpec    `json:"usb_forwards,omitempty"`
	AdvertiseEndpoints      []string             `json:"advertise_endpoints"`
	RelayRegion             string               `json:"relay_region"`
	AutoEnroll              bool                 `json:"auto_enroll"`
	InterfaceName           string               `json:"interface_name"`
	InterfaceMTU            int                  `json:"interface_mtu"`
	HeartbeatInterval       time.Duration        `json:"-"`
	BootstrapInterval       time.Duration        `json:"-"`
	HeartbeatIntervalText   string               `json:"heartbeat_interval"`
	BootstrapIntervalText   string               `json:"bootstrap_interval"`
}

func DefaultPath() string {
	baseDir, err := os.UserConfigDir()
	if err != nil {
		return ".nodeweave-windows-agent.json"
	}
	return filepath.Join(baseDir, "nodeweave", "windows-agent.json")
}

func Default() Config {
	baseDir, err := os.UserConfigDir()
	if err != nil {
		baseDir = "."
	}

	deviceName, err := os.Hostname()
	if err != nil || deviceName == "" {
		deviceName = "windows-agent"
	}

	return Config{
		ServerURL:               "http://127.0.0.1:8080",
		RegistrationToken:       "dev-register-token",
		DeviceName:              deviceName,
		Platform:                "windows-agent",
		Version:                 "0.1.0",
		PublicKey:               "",
		StatePath:               filepath.Join(baseDir, "nodeweave", "windows-agent-state.json"),
		BootstrapPath:           filepath.Join(baseDir, "nodeweave", "windows-agent-bootstrap.json"),
		RuntimePath:             filepath.Join(baseDir, "nodeweave", "windows-agent-runtime.json"),
		SerialForwardPath:       filepath.Join(baseDir, "nodeweave", "windows-agent-serial-forwards.json"),
		SerialForwardReportPath: filepath.Join(baseDir, "nodeweave", "windows-agent-serial-forward-report.json"),
		SerialForwards:          []serial.SessionSpec{},
		USBForwardPath:          filepath.Join(baseDir, "nodeweave", "windows-agent-usb-forwards.json"),
		USBForwardReportPath:    filepath.Join(baseDir, "nodeweave", "windows-agent-usb-forward-report.json"),
		USBForwards:             []usb.SessionSpec{},
		AdvertiseEndpoints:      []string{},
		RelayRegion:             "",
		AutoEnroll:              true,
		InterfaceName:           "NodeWeave",
		InterfaceMTU:            1380,
		HeartbeatInterval:       10 * time.Second,
		BootstrapInterval:       30 * time.Second,
		HeartbeatIntervalText:   "10s",
		BootstrapIntervalText:   "30s",
	}
}

func Load(path string) (Config, error) {
	cfg := Default()

	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config file: %w", err)
	}

	if cfg.HeartbeatIntervalText == "" {
		cfg.HeartbeatIntervalText = "10s"
	}
	if cfg.BootstrapIntervalText == "" {
		cfg.BootstrapIntervalText = "30s"
	}

	heartbeatInterval, err := time.ParseDuration(cfg.HeartbeatIntervalText)
	if err != nil {
		return Config{}, fmt.Errorf("parse heartbeat_interval: %w", err)
	}
	bootstrapInterval, err := time.ParseDuration(cfg.BootstrapIntervalText)
	if err != nil {
		return Config{}, fmt.Errorf("parse bootstrap_interval: %w", err)
	}

	cfg.HeartbeatInterval = heartbeatInterval
	cfg.BootstrapInterval = bootstrapInterval
	for idx, spec := range cfg.SerialForwards {
		cfg.SerialForwards[idx] = serial.NormalizeSessionSpec(spec)
	}
	for idx, spec := range cfg.USBForwards {
		cfg.USBForwards[idx] = usb.NormalizeSessionSpec(spec)
	}
	return cfg, nil
}

func WriteExample(path string) error {
	cfg := Default()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}
