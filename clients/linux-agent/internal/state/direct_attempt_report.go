package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type DirectAttemptReport struct {
	GeneratedAt time.Time                  `json:"generated_at"`
	Entries     []DirectAttemptReportEntry `json:"entries"`
}

type DirectAttemptReportEntry struct {
	AttemptID      string    `json:"attempt_id"`
	PeerNodeID     string    `json:"peer_node_id"`
	ExecuteAt      time.Time `json:"execute_at,omitempty"`
	Window         int64     `json:"window,omitempty"`
	BurstInterval  int64     `json:"burst_interval,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	Candidates     []string  `json:"candidates,omitempty"`
	Status         string    `json:"status,omitempty"`
	Result         string    `json:"result,omitempty"`
	WaitReason     string    `json:"wait_reason,omitempty"`
	LastError      string    `json:"last_error,omitempty"`
	QueuedAt       time.Time `json:"queued_at,omitempty"`
	ScheduledAt    time.Time `json:"scheduled_at,omitempty"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	CompletedAt    time.Time `json:"completed_at,omitempty"`
	LastUpdatedAt  time.Time `json:"last_updated_at,omitempty"`
	ReachedAddress string    `json:"reached_address,omitempty"`
	ActiveAddress  string    `json:"active_address,omitempty"`
}

func LoadDirectAttemptReport(path string) (DirectAttemptReport, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return DirectAttemptReport{}, fmt.Errorf("read direct attempt report file: %w", err)
	}
	var report DirectAttemptReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return DirectAttemptReport{}, fmt.Errorf("parse direct attempt report file: %w", err)
	}
	return report, nil
}

func SaveDirectAttemptReport(path string, report DirectAttemptReport) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create direct attempt report dir: %w", err)
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal direct attempt report file: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write direct attempt report file: %w", err)
	}
	return nil
}
