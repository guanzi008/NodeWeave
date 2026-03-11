package store

import (
	"testing"
	"time"

	"nodeweave/packages/contracts/go/api"
)

func TestDirectAttemptReasonUsesManualRecoverAfterCooldown(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow:        30 * time.Second,
		DirectAttemptCooldown:           5 * time.Second,
		DirectAttemptManualRecoverAfter: 20 * time.Second,
	}

	reason, schedule := directAttemptReasonWithPolicy(api.PeerTransportState{
		ActiveKind:              "relay",
		ReportedAt:              now,
		LastDirectAttemptAt:     now.Add(-25 * time.Second),
		LastDirectAttemptResult: "timeout",
	}, api.PeerTransportState{
		ActiveKind:          "relay",
		ReportedAt:          now,
		LastDirectAttemptAt: now.Add(-25 * time.Second),
	}, now, policy)
	if !schedule || reason != "manual_recover" {
		t.Fatalf("expected manual_recover after cooldown window, got reason=%q schedule=%v", reason, schedule)
	}
}

func TestDirectAttemptReasonSkipsDuringCooldown(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow:        30 * time.Second,
		DirectAttemptCooldown:           10 * time.Second,
		DirectAttemptManualRecoverAfter: 20 * time.Second,
	}

	reason, schedule := directAttemptReasonWithPolicy(api.PeerTransportState{
		ActiveKind:              "relay",
		ReportedAt:              now,
		LastDirectAttemptAt:     now.Add(-3 * time.Second),
		LastDirectAttemptResult: "relay_kept",
	}, api.PeerTransportState{
		ActiveKind:          "relay",
		ReportedAt:          now,
		LastDirectAttemptAt: now.Add(-3 * time.Second),
	}, now, policy)
	if schedule || reason != "" {
		t.Fatalf("expected cooldown to suppress scheduling, got reason=%q schedule=%v", reason, schedule)
	}
}

func TestDirectAttemptReasonUsesRelayActiveBeforeManualRecover(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow:        30 * time.Second,
		DirectAttemptCooldown:           5 * time.Second,
		DirectAttemptManualRecoverAfter: 20 * time.Second,
	}

	reason, schedule := directAttemptReasonWithPolicy(api.PeerTransportState{
		ActiveKind: "relay",
		ReportedAt: now,
	}, api.PeerTransportState{
		ActiveKind: "direct",
		ReportedAt: now,
	}, now, policy)
	if !schedule || reason != "relay_active" {
		t.Fatalf("expected relay_active before manual recovery criteria, got reason=%q schedule=%v", reason, schedule)
	}
}

func TestNewDirectAttemptPairUsesReasonSpecificProfiles(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		DirectAttemptLead:                 150 * time.Millisecond,
		DirectAttemptWindow:               600 * time.Millisecond,
		DirectAttemptBurstInterval:        80 * time.Millisecond,
		DirectAttemptRetention:            2 * time.Second,
		RelayActiveAttemptLead:            200 * time.Millisecond,
		RelayActiveAttemptWindow:          900 * time.Millisecond,
		RelayActiveAttemptBurstInterval:   60 * time.Millisecond,
		ManualRecoverAttemptLead:          250 * time.Millisecond,
		ManualRecoverAttemptWindow:        1500 * time.Millisecond,
		ManualRecoverAttemptBurstInterval: 50 * time.Millisecond,
	}
	nodeA := api.Node{ID: "node-a"}
	nodeB := api.Node{ID: "node-b"}

	relayPair := newDirectAttemptPair(now, nodeA, nodeB, []string{"198.51.100.10:51820"}, []string{"198.51.100.11:51820"}, "relay_active", policy)
	if relayPair.ExecuteAt.Sub(now) != policy.RelayActiveAttemptLead {
		t.Fatalf("expected relay_active lead %s, got %s", policy.RelayActiveAttemptLead, relayPair.ExecuteAt.Sub(now))
	}
	if relayPair.Window != policy.RelayActiveAttemptWindow {
		t.Fatalf("expected relay_active window %s, got %s", policy.RelayActiveAttemptWindow, relayPair.Window)
	}
	if relayPair.BurstInterval != policy.RelayActiveAttemptBurstInterval {
		t.Fatalf("expected relay_active burst interval %s, got %s", policy.RelayActiveAttemptBurstInterval, relayPair.BurstInterval)
	}

	manualPair := newDirectAttemptPair(now, nodeA, nodeB, []string{"198.51.100.10:51820"}, []string{"198.51.100.11:51820"}, "manual_recover", policy)
	if manualPair.ExecuteAt.Sub(now) != policy.ManualRecoverAttemptLead {
		t.Fatalf("expected manual_recover lead %s, got %s", policy.ManualRecoverAttemptLead, manualPair.ExecuteAt.Sub(now))
	}
	if manualPair.Window != policy.ManualRecoverAttemptWindow {
		t.Fatalf("expected manual_recover window %s, got %s", policy.ManualRecoverAttemptWindow, manualPair.Window)
	}
	if manualPair.BurstInterval != policy.ManualRecoverAttemptBurstInterval {
		t.Fatalf("expected manual_recover burst interval %s, got %s", policy.ManualRecoverAttemptBurstInterval, manualPair.BurstInterval)
	}

	freshPair := newDirectAttemptPair(now, nodeA, nodeB, []string{"198.51.100.10:51820"}, []string{"198.51.100.11:51820"}, "fresh_endpoints", policy)
	if freshPair.ExecuteAt.Sub(now) != policy.DirectAttemptLead {
		t.Fatalf("expected fresh_endpoints lead %s, got %s", policy.DirectAttemptLead, freshPair.ExecuteAt.Sub(now))
	}
	if freshPair.Window != policy.DirectAttemptWindow {
		t.Fatalf("expected fresh_endpoints window %s, got %s", policy.DirectAttemptWindow, freshPair.Window)
	}
	if freshPair.BurstInterval != policy.DirectAttemptBurstInterval {
		t.Fatalf("expected fresh_endpoints burst interval %s, got %s", policy.DirectAttemptBurstInterval, freshPair.BurstInterval)
	}
}
