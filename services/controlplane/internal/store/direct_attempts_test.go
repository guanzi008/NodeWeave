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
