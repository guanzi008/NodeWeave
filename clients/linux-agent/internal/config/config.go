package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	ServerURL                string        `json:"server_url"`
	RegistrationToken        string        `json:"registration_token"`
	DeviceName               string        `json:"device_name"`
	Platform                 string        `json:"platform"`
	Version                  string        `json:"version"`
	PublicKey                string        `json:"public_key"`
	PrivateKeyPath           string        `json:"private_key_path"`
	StatePath                string        `json:"state_path"`
	BootstrapPath            string        `json:"bootstrap_path"`
	RuntimePath              string        `json:"runtime_path"`
	PlanPath                 string        `json:"plan_path"`
	ApplyReportPath          string        `json:"apply_report_path"`
	SessionPath              string        `json:"session_path"`
	SessionReportPath        string        `json:"session_report_path"`
	DataplanePath            string        `json:"dataplane_path"`
	DirectAttemptPath        string        `json:"direct_attempt_path"`
	DirectAttemptReportPath  string        `json:"direct_attempt_report_path"`
	TransportReportPath      string        `json:"transport_report_path"`
	RecoveryStatePath        string        `json:"recovery_state_path"`
	STUNServers              []string      `json:"stun_servers"`
	STUNReportPath           string        `json:"stun_report_path"`
	AdvertiseEndpoints       []string      `json:"advertise_endpoints"`
	RelayRegion              string        `json:"relay_region"`
	AutoEnroll               bool          `json:"auto_enroll"`
	ApplyMode                string        `json:"apply_mode"`
	DataplaneMode            string        `json:"dataplane_mode"`
	DataplaneListenAddress   string        `json:"dataplane_listen_address"`
	TunnelMode               string        `json:"tunnel_mode"`
	TunnelName               string        `json:"tunnel_name"`
	InterfaceName            string        `json:"interface_name"`
	InterfaceMTU             int           `json:"interface_mtu"`
	ExecRequireRoot          bool          `json:"exec_require_root"`
	ExecCommandTimeout       time.Duration `json:"-"`
	ExecCommandTimeoutText   string        `json:"exec_command_timeout"`
	SessionProbeMode         string        `json:"session_probe_mode"`
	SessionListenAddress     string        `json:"session_listen_address"`
	SessionProbeTimeout      time.Duration `json:"-"`
	SessionProbeTimeoutText  string        `json:"session_probe_timeout"`
	STUNTimeout              time.Duration `json:"-"`
	STUNTimeoutText          string        `json:"stun_timeout"`
	DirectWarmupInterval     time.Duration `json:"-"`
	DirectWarmupIntervalText string        `json:"direct_warmup_interval"`
	HeartbeatInterval        time.Duration `json:"-"`
	BootstrapInterval        time.Duration `json:"-"`
	HeartbeatIntervalText    string        `json:"heartbeat_interval"`
	BootstrapIntervalText    string        `json:"bootstrap_interval"`
}

func DefaultPath() string {
	baseDir, err := os.UserConfigDir()
	if err != nil {
		return ".nodeweave-linux-agent.json"
	}
	return filepath.Join(baseDir, "nodeweave", "linux-agent.json")
}

func Default() Config {
	baseDir, err := os.UserConfigDir()
	if err != nil {
		baseDir = "."
	}

	deviceName, err := os.Hostname()
	if err != nil || deviceName == "" {
		deviceName = "linux-agent"
	}

	return Config{
		ServerURL:                "http://127.0.0.1:8080",
		RegistrationToken:        "dev-register-token",
		DeviceName:               deviceName,
		Platform:                 "linux-agent",
		Version:                  "0.1.0",
		PublicKey:                "",
		PrivateKeyPath:           filepath.Join(baseDir, "nodeweave", "linux-agent-private.key"),
		StatePath:                filepath.Join(baseDir, "nodeweave", "linux-agent-state.json"),
		BootstrapPath:            filepath.Join(baseDir, "nodeweave", "linux-agent-bootstrap.json"),
		RuntimePath:              filepath.Join(baseDir, "nodeweave", "linux-agent-runtime.json"),
		PlanPath:                 filepath.Join(baseDir, "nodeweave", "linux-agent-plan.json"),
		ApplyReportPath:          filepath.Join(baseDir, "nodeweave", "linux-agent-apply-report.json"),
		SessionPath:              filepath.Join(baseDir, "nodeweave", "linux-agent-session.json"),
		SessionReportPath:        filepath.Join(baseDir, "nodeweave", "linux-agent-session-report.json"),
		DataplanePath:            filepath.Join(baseDir, "nodeweave", "linux-agent-dataplane.json"),
		DirectAttemptPath:        filepath.Join(baseDir, "nodeweave", "linux-agent-direct-attempts.json"),
		DirectAttemptReportPath:  filepath.Join(baseDir, "nodeweave", "linux-agent-direct-attempt-report.json"),
		TransportReportPath:      filepath.Join(baseDir, "nodeweave", "linux-agent-transport-report.json"),
		RecoveryStatePath:        filepath.Join(baseDir, "nodeweave", "linux-agent-recovery-state.json"),
		STUNServers:              []string{},
		STUNReportPath:           filepath.Join(baseDir, "nodeweave", "linux-agent-stun-report.json"),
		AdvertiseEndpoints:       []string{},
		RelayRegion:              "",
		AutoEnroll:               true,
		ApplyMode:                "dry-run",
		DataplaneMode:            "off",
		DataplaneListenAddress:   "",
		TunnelMode:               "off",
		TunnelName:               "nw0",
		InterfaceName:            "nw0",
		InterfaceMTU:             1380,
		ExecRequireRoot:          true,
		ExecCommandTimeout:       5 * time.Second,
		ExecCommandTimeoutText:   "5s",
		SessionProbeMode:         "off",
		SessionListenAddress:     "",
		SessionProbeTimeout:      1500 * time.Millisecond,
		SessionProbeTimeoutText:  "1500ms",
		STUNTimeout:              2 * time.Second,
		STUNTimeoutText:          "2s",
		DirectWarmupInterval:     5 * time.Second,
		DirectWarmupIntervalText: "5s",
		HeartbeatInterval:        10 * time.Second,
		BootstrapInterval:        30 * time.Second,
		HeartbeatIntervalText:    "10s",
		BootstrapIntervalText:    "30s",
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
	if cfg.ExecCommandTimeoutText == "" {
		cfg.ExecCommandTimeoutText = "5s"
	}
	if cfg.SessionProbeTimeoutText == "" {
		cfg.SessionProbeTimeoutText = "1500ms"
	}
	if cfg.STUNTimeoutText == "" {
		cfg.STUNTimeoutText = "2s"
	}
	if cfg.DirectWarmupIntervalText == "" {
		cfg.DirectWarmupIntervalText = "5s"
	}

	heartbeatInterval, err := time.ParseDuration(cfg.HeartbeatIntervalText)
	if err != nil {
		return Config{}, fmt.Errorf("parse heartbeat_interval: %w", err)
	}
	bootstrapInterval, err := time.ParseDuration(cfg.BootstrapIntervalText)
	if err != nil {
		return Config{}, fmt.Errorf("parse bootstrap_interval: %w", err)
	}
	execCommandTimeout, err := time.ParseDuration(cfg.ExecCommandTimeoutText)
	if err != nil {
		return Config{}, fmt.Errorf("parse exec_command_timeout: %w", err)
	}
	sessionProbeTimeout, err := time.ParseDuration(cfg.SessionProbeTimeoutText)
	if err != nil {
		return Config{}, fmt.Errorf("parse session_probe_timeout: %w", err)
	}
	stunTimeout, err := time.ParseDuration(cfg.STUNTimeoutText)
	if err != nil {
		return Config{}, fmt.Errorf("parse stun_timeout: %w", err)
	}
	directWarmupInterval, err := time.ParseDuration(cfg.DirectWarmupIntervalText)
	if err != nil {
		return Config{}, fmt.Errorf("parse direct_warmup_interval: %w", err)
	}

	cfg.HeartbeatInterval = heartbeatInterval
	cfg.BootstrapInterval = bootstrapInterval
	cfg.ExecCommandTimeout = execCommandTimeout
	cfg.SessionProbeTimeout = sessionProbeTimeout
	cfg.STUNTimeout = stunTimeout
	cfg.DirectWarmupInterval = directWarmupInterval
	return cfg, nil
}

func WriteExample(path string) error {
	cfg := Default()
	if cfg.PublicKey == "" && cfg.PrivateKeyPath == "" {
		cfg.PublicKey = "devpub-change-me"
	}

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
