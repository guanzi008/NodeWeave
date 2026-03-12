package store

import (
	"testing"
	"time"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/services/controlplane/internal/config"
)

func TestDirectAttemptReasonUsesManualRecoverAfterCooldown(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow:               30 * time.Second,
		DirectAttemptCooldown:                  5 * time.Second,
		DirectAttemptManualRecoverAfter:        20 * time.Second,
		DirectAttemptTimeoutManualRecoverAfter: 20 * time.Second,
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
		TransportFreshnessWindow:                 30 * time.Second,
		DirectAttemptCooldown:                    10 * time.Second,
		DirectAttemptManualRecoverAfter:          20 * time.Second,
		DirectAttemptRelayKeptManualRecoverAfter: 20 * time.Second,
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
		TransportFreshnessWindow:               30 * time.Second,
		DirectAttemptCooldown:                  5 * time.Second,
		DirectAttemptManualRecoverAfter:        20 * time.Second,
		DirectAttemptTimeoutManualRecoverAfter: 20 * time.Second,
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

func TestDirectAttemptReasonUsesResultSpecificManualRecoverThresholds(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow:                 30 * time.Second,
		DirectAttemptCooldown:                    2 * time.Second,
		DirectAttemptManualRecoverAfter:          20 * time.Second,
		DirectAttemptTimeoutManualRecoverAfter:   12 * time.Second,
		DirectAttemptRelayKeptManualRecoverAfter: 4 * time.Second,
	}

	reason, schedule := directAttemptReasonWithPolicy(api.PeerTransportState{
		ActiveKind:              "relay",
		ReportedAt:              now,
		LastDirectAttemptAt:     now.Add(-6 * time.Second),
		LastDirectAttemptResult: "relay_kept",
	}, api.PeerTransportState{
		ActiveKind:          "relay",
		ReportedAt:          now,
		LastDirectAttemptAt: now.Add(-6 * time.Second),
	}, now, policy)
	if !schedule || reason != "manual_recover" {
		t.Fatalf("expected relay_kept to escalate to manual_recover, got reason=%q schedule=%v", reason, schedule)
	}

	reason, schedule = directAttemptReasonWithPolicy(api.PeerTransportState{
		ActiveKind:              "relay",
		ReportedAt:              now,
		LastDirectAttemptAt:     now.Add(-6 * time.Second),
		LastDirectAttemptResult: "timeout",
	}, api.PeerTransportState{
		ActiveKind:          "relay",
		ReportedAt:          now,
		LastDirectAttemptAt: now.Add(-6 * time.Second),
	}, now, policy)
	if !schedule || reason != "relay_active" {
		t.Fatalf("expected timeout to remain relay_active before its manual threshold, got reason=%q schedule=%v", reason, schedule)
	}
}

func TestDirectAttemptReasonSuppressesAfterFailureBudget(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow:           30 * time.Second,
		DirectAttemptCooldown:              2 * time.Second,
		DirectAttemptFailureSuppressAfter:  3,
		DirectAttemptFailureSuppressWindow: 90 * time.Second,
		DirectAttemptTimeoutSuppressAfter:  3,
		DirectAttemptTimeoutSuppressWindow: 90 * time.Second,
	}

	reason, schedule := directAttemptReasonWithPolicy(api.PeerTransportState{
		ActiveKind:                "relay",
		ReportedAt:                now,
		LastDirectAttemptAt:       now.Add(-6 * time.Second),
		LastDirectAttemptResult:   "timeout",
		ConsecutiveDirectFailures: 3,
	}, api.PeerTransportState{
		ActiveKind:          "relay",
		ReportedAt:          now,
		LastDirectAttemptAt: now.Add(-6 * time.Second),
	}, now, policy)
	if schedule || reason != "" {
		t.Fatalf("expected failure budget suppression to stop scheduling, got reason=%q schedule=%v", reason, schedule)
	}
}

func TestDirectAttemptReasonAllowsSuppressedManualRecoverProbeAfterInterval(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow:                    30 * time.Second,
		DirectAttemptCooldown:                       2 * time.Second,
		DirectAttemptFailureSuppressAfter:           3,
		DirectAttemptFailureSuppressWindow:          90 * time.Second,
		DirectAttemptTimeoutSuppressAfter:           3,
		DirectAttemptTimeoutSuppressWindow:          90 * time.Second,
		DirectAttemptSuppressedProbeInterval:        15 * time.Second,
		DirectAttemptTimeoutSuppressedProbeInterval: 15 * time.Second,
	}

	reason, schedule := directAttemptReasonWithPolicy(api.PeerTransportState{
		ActiveKind:                "relay",
		ReportedAt:                now,
		LastDirectAttemptAt:       now.Add(-16 * time.Second),
		LastDirectAttemptResult:   "timeout",
		ConsecutiveDirectFailures: 3,
	}, api.PeerTransportState{
		ActiveKind:          "relay",
		ReportedAt:          now,
		LastDirectAttemptAt: now.Add(-16 * time.Second),
	}, now, policy)
	if !schedule || reason != "manual_recover" {
		t.Fatalf("expected suppressed probe interval to reopen manual recover, got reason=%q schedule=%v", reason, schedule)
	}
}

func TestDirectAttemptReasonSkipsSuppressionAfterRecentSuccess(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow:             30 * time.Second,
		DirectAttemptCooldown:                2 * time.Second,
		DirectAttemptFailureSuppressAfter:    2,
		DirectAttemptFailureSuppressWindow:   90 * time.Second,
		DirectAttemptRelayKeptSuppressAfter:  2,
		DirectAttemptRelayKeptSuppressWindow: 90 * time.Second,
	}

	reason, schedule := directAttemptReasonWithPolicy(api.PeerTransportState{
		ActiveKind:                "relay",
		ReportedAt:                now,
		LastDirectAttemptAt:       now.Add(-6 * time.Second),
		LastDirectAttemptResult:   "relay_kept",
		LastDirectSuccessAt:       now.Add(-1 * time.Second),
		ConsecutiveDirectFailures: 2,
	}, api.PeerTransportState{
		ActiveKind:          "relay",
		ReportedAt:          now,
		LastDirectAttemptAt: now.Add(-6 * time.Second),
	}, now, policy)
	if !schedule || reason == "" {
		t.Fatalf("expected recent success to bypass suppression, got reason=%q schedule=%v", reason, schedule)
	}
}

func TestRecoveryStateForPeerUsesLongestBlock(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow:           30 * time.Second,
		DirectAttemptCooldown:              2 * time.Second,
		DirectAttemptFailureSuppressAfter:  3,
		DirectAttemptFailureSuppressWindow: 90 * time.Second,
		DirectAttemptTimeoutSuppressAfter:  3,
		DirectAttemptTimeoutSuppressWindow: 90 * time.Second,
	}

	recoveryState := recoveryStateForPeer("node-b", api.PeerTransportState{
		PeerNodeID:                "node-b",
		ActiveKind:                "relay",
		ReportedAt:                now,
		LastDirectAttemptAt:       now.Add(-3 * time.Second),
		LastDirectAttemptResult:   "timeout",
		ConsecutiveDirectFailures: 3,
	}, api.PeerTransportState{}, now, policy)
	if !recoveryState.Blocked || recoveryState.BlockReason != "suppressed_timeout_budget" {
		t.Fatalf("expected suppression state to win over shorter cooldown, got %#v", recoveryState)
	}
}

func TestRecoveryStateForPeerIncludesNextProbeAt(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow:                    30 * time.Second,
		DirectAttemptCooldown:                       2 * time.Second,
		DirectAttemptFailureSuppressAfter:           3,
		DirectAttemptFailureSuppressWindow:          90 * time.Second,
		DirectAttemptTimeoutSuppressAfter:           3,
		DirectAttemptTimeoutSuppressWindow:          90 * time.Second,
		DirectAttemptSuppressedProbeInterval:        20 * time.Second,
		DirectAttemptTimeoutSuppressedProbeInterval: 20 * time.Second,
	}

	attemptAt := now.Add(-5 * time.Second)
	recoveryState := recoveryStateForPeer("node-b", api.PeerTransportState{
		PeerNodeID:                "node-b",
		ActiveKind:                "relay",
		ReportedAt:                now,
		LastDirectAttemptAt:       attemptAt,
		LastDirectAttemptResult:   "timeout",
		ConsecutiveDirectFailures: 3,
	}, api.PeerTransportState{}, now, policy)
	want := attemptAt.Add(20 * time.Second)
	if recoveryState.NextProbeAt.IsZero() || !recoveryState.NextProbeAt.Equal(want) {
		t.Fatalf("expected next probe at %s, got %#v", want, recoveryState)
	}
}

func TestDirectAttemptCoolingDownUsesResultSpecificCooldowns(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow:       30 * time.Second,
		DirectAttemptCooldown:          10 * time.Second,
		DirectAttemptTimeoutCooldown:   15 * time.Second,
		DirectAttemptRelayKeptCooldown: 3 * time.Second,
	}

	timeoutCooling := directAttemptCoolingDownWithPolicy(api.PeerTransportState{
		ActiveKind:              "relay",
		ReportedAt:              now,
		LastDirectAttemptAt:     now.Add(-6 * time.Second),
		LastDirectAttemptResult: "timeout",
	}, now, policy)
	if !timeoutCooling {
		t.Fatal("expected timeout result to remain in cooldown")
	}

	relayKeptCooling := directAttemptCoolingDownWithPolicy(api.PeerTransportState{
		ActiveKind:              "relay",
		ReportedAt:              now,
		LastDirectAttemptAt:     now.Add(-6 * time.Second),
		LastDirectAttemptResult: "relay_kept",
	}, now, policy)
	if relayKeptCooling {
		t.Fatal("expected relay_kept cooldown to expire sooner than timeout")
	}
}

func TestDirectAttemptPolicyDefaultsResultCooldownsToBase(t *testing.T) {
	policy := directAttemptPolicyFromConfig(config.Config{
		DirectAttemptCooldown: 7 * time.Second,
	})
	if policy.DirectAttemptTimeoutCooldown != 7*time.Second {
		t.Fatalf("expected timeout cooldown to fall back to base value, got %s", policy.DirectAttemptTimeoutCooldown)
	}
	if policy.DirectAttemptRelayKeptCooldown != 7*time.Second {
		t.Fatalf("expected relay_kept cooldown to fall back to base value, got %s", policy.DirectAttemptRelayKeptCooldown)
	}
}

func TestDirectAttemptPolicyDefaultsResultSpecificManualRecoverThresholdsToBase(t *testing.T) {
	policy := directAttemptPolicyFromConfig(config.Config{
		DirectAttemptManualRecoverAfter: 11 * time.Second,
	})
	if policy.DirectAttemptTimeoutManualRecoverAfter != 11*time.Second {
		t.Fatalf("expected timeout manual recover threshold to fall back to base value, got %s", policy.DirectAttemptTimeoutManualRecoverAfter)
	}
	if policy.DirectAttemptRelayKeptManualRecoverAfter != 11*time.Second {
		t.Fatalf("expected relay_kept manual recover threshold to fall back to base value, got %s", policy.DirectAttemptRelayKeptManualRecoverAfter)
	}
}

func TestDirectAttemptPolicyDefaultsResultSpecificSuppressionToBase(t *testing.T) {
	policy := directAttemptPolicyFromConfig(config.Config{
		DirectAttemptFailureSuppressAfter:  5,
		DirectAttemptFailureSuppressWindow: 45 * time.Second,
	})
	if policy.DirectAttemptTimeoutSuppressAfter != 5 {
		t.Fatalf("expected timeout suppression threshold to fall back to base value, got %d", policy.DirectAttemptTimeoutSuppressAfter)
	}
	if policy.DirectAttemptRelayKeptSuppressAfter != 5 {
		t.Fatalf("expected relay_kept suppression threshold to fall back to base value, got %d", policy.DirectAttemptRelayKeptSuppressAfter)
	}
	if policy.DirectAttemptTimeoutSuppressWindow != 45*time.Second {
		t.Fatalf("expected timeout suppression window to fall back to base value, got %s", policy.DirectAttemptTimeoutSuppressWindow)
	}
	if policy.DirectAttemptRelayKeptSuppressWindow != 45*time.Second {
		t.Fatalf("expected relay_kept suppression window to fall back to base value, got %s", policy.DirectAttemptRelayKeptSuppressWindow)
	}
}

func TestDirectAttemptPolicyDefaultsResultSpecificSuppressedProbeIntervalToBase(t *testing.T) {
	policy := directAttemptPolicyFromConfig(config.Config{
		DirectAttemptSuppressedProbeInterval: 14 * time.Second,
	})
	if policy.DirectAttemptTimeoutSuppressedProbeInterval != 14*time.Second {
		t.Fatalf("expected timeout suppressed probe interval to fall back to base value, got %s", policy.DirectAttemptTimeoutSuppressedProbeInterval)
	}
	if policy.DirectAttemptRelayKeptSuppressedProbeInterval != 14*time.Second {
		t.Fatalf("expected relay_kept suppressed probe interval to fall back to base value, got %s", policy.DirectAttemptRelayKeptSuppressedProbeInterval)
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
