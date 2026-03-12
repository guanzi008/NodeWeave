package store

import (
	"testing"
	"time"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/services/controlplane/internal/config"
)

func storeDirectAttemptCandidate(address string) api.DirectAttemptCandidate {
	return api.DirectAttemptCandidate{
		Address:  address,
		Source:   "heartbeat",
		Priority: 1000,
		Phase:    api.DirectAttemptPhasePrimary,
	}
}

func storeDirectAttemptCandidateForSource(address, source string, observedAt time.Time) api.DirectAttemptCandidate {
	return api.DirectAttemptCandidate{
		Address:    address,
		Source:     source,
		ObservedAt: observedAt,
		Priority:   1000,
		Phase:      api.DirectAttemptPhaseForSource(source),
	}
}

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

func TestFreshDirectAttemptCandidatesWithPolicyAssignsPhasesAndLimits(t *testing.T) {
	now := time.Now().UTC()
	node := api.Node{
		ID: "node-a",
		EndpointRecords: []api.EndpointObservation{
			{Address: "198.51.100.11:51820", Source: "listener", ObservedAt: now.Add(-1 * time.Second)},
			{Address: "198.51.100.12:51820", Source: "stun", ObservedAt: now.Add(-2 * time.Second)},
			{Address: "198.51.100.13:51820", Source: "stun", ObservedAt: now.Add(-3 * time.Second)},
			{Address: "198.51.100.14:51820", Source: "listener", ObservedAt: now.Add(-4 * time.Second)},
			{Address: "198.51.100.15:51820", Source: "listener", ObservedAt: now.Add(-5 * time.Second)},
			{Address: "198.51.100.21:51820", Source: "static", ObservedAt: now.Add(-9 * time.Second)},
			{Address: "198.51.100.22:51820", Source: "heartbeat", ObservedAt: now.Add(-10 * time.Second)},
			{Address: "198.51.100.23:51820", Source: "static", ObservedAt: now.Add(-2 * time.Hour)},
		},
	}
	policy := directAttemptPolicy{
		EndpointFreshnessWindow: time.Minute,
	}

	got := freshDirectAttemptCandidatesWithPolicy(node, now, policy)
	if len(got) != 6 {
		t.Fatalf("expected 6 phased candidates, got %#v", got)
	}

	wantAddresses := []string{
		"198.51.100.11:51820",
		"198.51.100.12:51820",
		"198.51.100.13:51820",
		"198.51.100.14:51820",
		"198.51.100.21:51820",
		"198.51.100.22:51820",
	}
	for idx, address := range wantAddresses {
		if got[idx].Address != address {
			t.Fatalf("unexpected candidate order at %d: %#v", idx, got)
		}
	}
	for idx := 0; idx < 4; idx++ {
		if got[idx].Phase != api.DirectAttemptPhasePrimary {
			t.Fatalf("expected primary phase for candidate %d, got %#v", idx, got[idx])
		}
	}
	for idx := 4; idx < len(got); idx++ {
		if got[idx].Phase != api.DirectAttemptPhaseSecondary {
			t.Fatalf("expected secondary phase for candidate %d, got %#v", idx, got[idx])
		}
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

func TestDirectAttemptReasonWithPolicyCandidatesSkipsSecondaryOnlyFreshEndpoints(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow: 30 * time.Second,
	}

	reason, schedule := directAttemptReasonWithPolicyCandidates(
		[]api.DirectAttemptCandidate{storeDirectAttemptCandidateForSource("198.51.100.10:51820", "heartbeat", now)},
		[]api.DirectAttemptCandidate{storeDirectAttemptCandidateForSource("203.0.113.20:51820", "static", now)},
		api.PeerTransportState{},
		api.PeerTransportState{},
		now,
		policy,
	)
	if schedule || reason != "" {
		t.Fatalf("expected secondary-only direct candidates to stay idle, got reason=%q schedule=%v", reason, schedule)
	}
}

func TestDirectAttemptReasonWithPolicyCandidatesBypassesCooldownOnPrimaryUpgrade(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow:               30 * time.Second,
		DirectAttemptCooldown:                  10 * time.Second,
		DirectAttemptManualRecoverAfter:        20 * time.Second,
		DirectAttemptTimeoutManualRecoverAfter: 20 * time.Second,
	}

	reason, schedule := directAttemptReasonWithPolicyCandidates(
		[]api.DirectAttemptCandidate{storeDirectAttemptCandidateForSource("198.51.100.10:51820", "stun", now)},
		[]api.DirectAttemptCandidate{storeDirectAttemptCandidateForSource("203.0.113.20:51820", "listener", now)},
		api.PeerTransportState{
			ActiveKind:              "relay",
			ReportedAt:              now,
			LastDirectAttemptAt:     now.Add(-3 * time.Second),
			LastDirectAttemptResult: "timeout",
			LastDirectAttemptPhase:  api.DirectAttemptPhaseSecondary,
		},
		api.PeerTransportState{
			ActiveKind: "relay",
			ReportedAt: now,
		},
		now,
		policy,
	)
	if !schedule || reason != "relay_active" {
		t.Fatalf("expected primary upgrade to bypass cooldown and restore relay_active scheduling, got reason=%q schedule=%v", reason, schedule)
	}
}

func TestDirectAttemptReasonWithPolicyCandidatesRequiresNewerPrimaryObservationForUpgrade(t *testing.T) {
	now := time.Now().UTC()
	attemptAt := now.Add(-3 * time.Second)
	policy := directAttemptPolicy{
		TransportFreshnessWindow:               30 * time.Second,
		DirectAttemptCooldown:                  10 * time.Second,
		DirectAttemptManualRecoverAfter:        20 * time.Second,
		DirectAttemptTimeoutManualRecoverAfter: 20 * time.Second,
	}

	reason, schedule := directAttemptReasonWithPolicyCandidates(
		[]api.DirectAttemptCandidate{storeDirectAttemptCandidateForSource("198.51.100.10:51820", "stun", attemptAt.Add(-1*time.Second))},
		[]api.DirectAttemptCandidate{storeDirectAttemptCandidateForSource("203.0.113.20:51820", "listener", attemptAt.Add(-1500*time.Millisecond))},
		api.PeerTransportState{
			ActiveKind:              "relay",
			ReportedAt:              now,
			LastDirectAttemptAt:     attemptAt,
			LastDirectAttemptResult: "timeout",
			LastDirectAttemptPhase:  api.DirectAttemptPhaseSecondary,
		},
		api.PeerTransportState{
			ActiveKind: "relay",
			ReportedAt: now,
		},
		now,
		policy,
	)
	if schedule || reason != "" {
		t.Fatalf("expected stale primary observations to stay in cooldown, got reason=%q schedule=%v", reason, schedule)
	}
}

func TestDirectAttemptReasonUsesPrimaryUpgradeManualRecoverThreshold(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow:                30 * time.Second,
		DirectAttemptCooldown:                   2 * time.Second,
		DirectAttemptManualRecoverAfter:         4 * time.Second,
		DirectAttemptTimeoutManualRecoverAfter:  6 * time.Second,
		PrimaryUpgradeAttemptManualRecoverAfter: 20 * time.Second,
	}

	reason, schedule := directAttemptReasonWithPolicy(api.PeerTransportState{
		ActiveKind:               "relay",
		ReportedAt:               now,
		LastDirectAttemptAt:      now.Add(-8 * time.Second),
		LastDirectAttemptResult:  "timeout",
		LastDirectAttemptProfile: "primary_upgrade",
	}, api.PeerTransportState{
		ActiveKind: "relay",
		ReportedAt: now,
	}, now, policy)
	if !schedule || reason != "relay_active" {
		t.Fatalf("expected primary_upgrade timeout to stay relay_active until its longer manual threshold, got reason=%q schedule=%v", reason, schedule)
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
		DirectAttemptSuppressedProbeLimit:           2,
		DirectAttemptTimeoutSuppressedProbeLimit:    2,
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

func TestDirectAttemptReasonStopsAfterSuppressedProbeBudgetExhausted(t *testing.T) {
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
		DirectAttemptSuppressedProbeLimit:           2,
		DirectAttemptTimeoutSuppressedProbeLimit:    2,
	}

	reason, schedule := directAttemptReasonWithPolicy(api.PeerTransportState{
		ActiveKind:                "relay",
		ReportedAt:                now,
		LastDirectAttemptAt:       now.Add(-16 * time.Second),
		LastDirectAttemptResult:   "timeout",
		ConsecutiveDirectFailures: 5,
	}, api.PeerTransportState{
		ActiveKind:          "relay",
		ReportedAt:          now,
		LastDirectAttemptAt: now.Add(-16 * time.Second),
	}, now, policy)
	if schedule || reason != "" {
		t.Fatalf("expected exhausted suppressed probe budget to stop scheduling, got reason=%q schedule=%v", reason, schedule)
	}
}

func TestDirectAttemptReasonAllowsSuppressedProbeAfterRefill(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow:                          30 * time.Second,
		DirectAttemptCooldown:                             2 * time.Second,
		DirectAttemptFailureSuppressAfter:                 3,
		DirectAttemptFailureSuppressWindow:                90 * time.Second,
		DirectAttemptTimeoutSuppressAfter:                 3,
		DirectAttemptTimeoutSuppressWindow:                90 * time.Second,
		DirectAttemptSuppressedProbeInterval:              15 * time.Second,
		DirectAttemptTimeoutSuppressedProbeInterval:       15 * time.Second,
		DirectAttemptSuppressedProbeLimit:                 2,
		DirectAttemptTimeoutSuppressedProbeLimit:          2,
		DirectAttemptSuppressedProbeRefillInterval:        30 * time.Second,
		DirectAttemptTimeoutSuppressedProbeRefillInterval: 30 * time.Second,
	}

	reason, schedule := directAttemptReasonWithPolicy(api.PeerTransportState{
		ActiveKind:                "relay",
		ReportedAt:                now,
		LastDirectAttemptAt:       now.Add(-31 * time.Second),
		LastDirectAttemptResult:   "timeout",
		ConsecutiveDirectFailures: 5,
	}, api.PeerTransportState{
		ActiveKind:          "relay",
		ReportedAt:          now,
		LastDirectAttemptAt: now.Add(-31 * time.Second),
	}, now, policy)
	if !schedule || reason != "manual_recover" {
		t.Fatalf("expected probe refill to reopen manual_recover, got reason=%q schedule=%v", reason, schedule)
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
	selfNode := api.Node{ID: "node-a", Status: "online", LastSeenAt: now}
	peerNode := api.Node{ID: "node-b", Status: "online", LastSeenAt: now}
	policy := directAttemptPolicy{
		TransportFreshnessWindow:           30 * time.Second,
		DirectAttemptCooldown:              2 * time.Second,
		DirectAttemptFailureSuppressAfter:  3,
		DirectAttemptFailureSuppressWindow: 90 * time.Second,
		DirectAttemptTimeoutSuppressAfter:  3,
		DirectAttemptTimeoutSuppressWindow: 90 * time.Second,
	}

	recoveryState := recoveryStateForPeer(selfNode, peerNode, []string{"198.51.100.10:51820"}, []string{"203.0.113.20:51820"}, api.PeerTransportState{
		PeerNodeID:                "node-b",
		ActiveKind:                "relay",
		ReportedAt:                now,
		LastDirectAttemptAt:       now.Add(-3 * time.Second),
		LastDirectAttemptResult:   "timeout",
		ConsecutiveDirectFailures: 3,
	}, api.PeerTransportState{}, now, policy, nil)
	if !recoveryState.Blocked || recoveryState.BlockReason != "suppressed_timeout_budget" {
		t.Fatalf("expected suppression state to win over shorter cooldown, got %#v", recoveryState)
	}
	if recoveryState.DecisionStatus != "blocked" || recoveryState.DecisionNextAt.IsZero() || !recoveryState.DecisionNextAt.Equal(recoveryState.BlockedUntil) {
		t.Fatalf("expected blocked decision to expose next transition time, got %#v", recoveryState)
	}
}

func TestRecoveryStateForPeerIncludesNextProbeAt(t *testing.T) {
	now := time.Now().UTC()
	selfNode := api.Node{ID: "node-a", Status: "online", LastSeenAt: now}
	peerNode := api.Node{ID: "node-b", Status: "online", LastSeenAt: now}
	policy := directAttemptPolicy{
		TransportFreshnessWindow:                          30 * time.Second,
		DirectAttemptCooldown:                             2 * time.Second,
		DirectAttemptFailureSuppressAfter:                 3,
		DirectAttemptFailureSuppressWindow:                90 * time.Second,
		DirectAttemptTimeoutSuppressAfter:                 3,
		DirectAttemptTimeoutSuppressWindow:                90 * time.Second,
		DirectAttemptSuppressedProbeInterval:              20 * time.Second,
		DirectAttemptTimeoutSuppressedProbeInterval:       20 * time.Second,
		DirectAttemptSuppressedProbeLimit:                 2,
		DirectAttemptTimeoutSuppressedProbeLimit:          2,
		DirectAttemptSuppressedProbeRefillInterval:        30 * time.Second,
		DirectAttemptTimeoutSuppressedProbeRefillInterval: 30 * time.Second,
	}

	attemptAt := now.Add(-5 * time.Second)
	recoveryState := recoveryStateForPeer(selfNode, peerNode, []string{"198.51.100.10:51820"}, []string{"203.0.113.20:51820"}, api.PeerTransportState{
		PeerNodeID:                "node-b",
		ActiveKind:                "relay",
		ReportedAt:                now,
		LastDirectAttemptAt:       attemptAt,
		LastDirectAttemptResult:   "timeout",
		ConsecutiveDirectFailures: 3,
	}, api.PeerTransportState{}, now, policy, nil)
	want := attemptAt.Add(20 * time.Second)
	if recoveryState.NextProbeAt.IsZero() || !recoveryState.NextProbeAt.Equal(want) {
		t.Fatalf("expected next probe at %s, got %#v", want, recoveryState)
	}
	if !recoveryState.ProbeLimited || recoveryState.ProbeBudget != 2 || recoveryState.ProbeFailures != 0 || recoveryState.ProbeRemaining != 2 {
		t.Fatalf("expected recovery state to include probe budget details, got %#v", recoveryState)
	}
	if !recoveryState.ProbeRefillAt.IsZero() {
		t.Fatalf("expected no refill timestamp while budget is untouched, got %#v", recoveryState)
	}
}

func TestRecoveryStateForPeerIncludesProbeRefillAtAfterBudgetExhausted(t *testing.T) {
	now := time.Now().UTC()
	selfNode := api.Node{ID: "node-a", Status: "online", LastSeenAt: now}
	peerNode := api.Node{ID: "node-b", Status: "online", LastSeenAt: now}
	policy := directAttemptPolicy{
		TransportFreshnessWindow:                          30 * time.Second,
		DirectAttemptCooldown:                             2 * time.Second,
		DirectAttemptFailureSuppressAfter:                 3,
		DirectAttemptFailureSuppressWindow:                90 * time.Second,
		DirectAttemptTimeoutSuppressAfter:                 3,
		DirectAttemptTimeoutSuppressWindow:                90 * time.Second,
		DirectAttemptSuppressedProbeInterval:              15 * time.Second,
		DirectAttemptTimeoutSuppressedProbeInterval:       15 * time.Second,
		DirectAttemptSuppressedProbeLimit:                 2,
		DirectAttemptTimeoutSuppressedProbeLimit:          2,
		DirectAttemptSuppressedProbeRefillInterval:        30 * time.Second,
		DirectAttemptTimeoutSuppressedProbeRefillInterval: 30 * time.Second,
	}

	attemptAt := now.Add(-16 * time.Second)
	recoveryState := recoveryStateForPeer(selfNode, peerNode, []string{"198.51.100.10:51820"}, []string{"203.0.113.20:51820"}, api.PeerTransportState{
		PeerNodeID:                "node-b",
		ActiveKind:                "relay",
		ReportedAt:                now,
		LastDirectAttemptAt:       attemptAt,
		LastDirectAttemptResult:   "timeout",
		ConsecutiveDirectFailures: 5,
	}, api.PeerTransportState{}, now, policy, nil)
	wantRefill := attemptAt.Add(30 * time.Second)
	if recoveryState.ProbeRemaining != 0 || recoveryState.ProbeRefillAt.IsZero() || !recoveryState.ProbeRefillAt.Equal(wantRefill) {
		t.Fatalf("expected exhausted budget to expose refill time %s, got %#v", wantRefill, recoveryState)
	}
}

func TestRecoveryStateForPeerIncludesLatestIssuedAttemptTrace(t *testing.T) {
	now := time.Now().UTC()
	selfNode := api.Node{ID: "node-a", Status: "online", LastSeenAt: now}
	peerNode := api.Node{ID: "node-b", Status: "online", LastSeenAt: now}
	recoveryState := recoveryStateForPeer(
		selfNode,
		peerNode,
		[]string{"198.51.100.10:51820"},
		[]string{"203.0.113.20:51820"},
		api.PeerTransportState{},
		api.PeerTransportState{},
		now,
		directAttemptPolicy{},
		&directAttemptPair{
			AttemptID: "attempt-node-a-node-b-1",
			Reason:    "relay_active",
			Profile:   "primary_upgrade",
			IssuedAt:  now.Add(-1 * time.Second),
			ExecuteAt: now.Add(200 * time.Millisecond),
			ExpiresAt: now.Add(2 * time.Second),
		},
	)
	if recoveryState.LastIssuedAttemptID != "attempt-node-a-node-b-1" || recoveryState.LastIssuedAttemptReason != "relay_active" || recoveryState.LastIssuedAttemptProfile != "primary_upgrade" {
		t.Fatalf("expected recovery state to include issued attempt trace, got %#v", recoveryState)
	}
	if recoveryState.LastIssuedAttemptAt.IsZero() || recoveryState.LastIssuedAttemptExecuteAt.IsZero() {
		t.Fatalf("expected recovery state to include issued/execute timestamps, got %#v", recoveryState)
	}
	if recoveryState.DecisionStatus != "attempt_issued" || recoveryState.DecisionNextAt.IsZero() || !recoveryState.DecisionNextAt.Equal(recoveryState.LastIssuedAttemptExecuteAt) {
		t.Fatalf("expected issued attempt decision to point at execute_at, got %#v", recoveryState)
	}
}

func TestRecoveryStateForPeerIncludesDecisionWhenNotScheduled(t *testing.T) {
	now := time.Now().UTC()
	selfNode := api.Node{ID: "node-a", Status: "online", LastSeenAt: now}
	peerNode := api.Node{ID: "node-b", Status: "offline", LastSeenAt: now.Add(-2 * time.Minute)}

	recoveryState := recoveryStateForPeer(
		selfNode,
		peerNode,
		[]string{"198.51.100.10:51820"},
		nil,
		api.PeerTransportState{PeerNodeID: "node-b"},
		api.PeerTransportState{},
		now,
		directAttemptPolicy{NodeOnlineWindow: 30 * time.Second, EndpointFreshnessWindow: 45 * time.Second},
		nil,
	)
	if recoveryState.DecisionStatus != "peer_offline" || recoveryState.DecisionReason != "peer_node_offline" {
		t.Fatalf("expected offline decision metadata, got %#v", recoveryState)
	}
	if recoveryState.DecisionAt.IsZero() {
		t.Fatalf("expected decision timestamp, got %#v", recoveryState)
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

func TestDirectAttemptCoolingDownUsesPrimaryUpgradeSpecificCooldown(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow:      30 * time.Second,
		DirectAttemptCooldown:         3 * time.Second,
		DirectAttemptTimeoutCooldown:  4 * time.Second,
		PrimaryUpgradeAttemptCooldown: 12 * time.Second,
	}

	cooling := directAttemptCoolingDownWithPolicy(api.PeerTransportState{
		ActiveKind:               "relay",
		ReportedAt:               now,
		LastDirectAttemptAt:      now.Add(-6 * time.Second),
		LastDirectAttemptResult:  "timeout",
		LastDirectAttemptProfile: "primary_upgrade",
	}, now, policy)
	if !cooling {
		t.Fatal("expected primary_upgrade timeout to stay in its longer cooldown")
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

func TestDirectAttemptPolicyDefaultsResultSpecificSuppressedProbeLimitToBase(t *testing.T) {
	policy := directAttemptPolicyFromConfig(config.Config{
		DirectAttemptSuppressedProbeLimit: 3,
	})
	if policy.DirectAttemptTimeoutSuppressedProbeLimit != 3 {
		t.Fatalf("expected timeout suppressed probe limit to fall back to base value, got %d", policy.DirectAttemptTimeoutSuppressedProbeLimit)
	}
	if policy.DirectAttemptRelayKeptSuppressedProbeLimit != 3 {
		t.Fatalf("expected relay_kept suppressed probe limit to fall back to base value, got %d", policy.DirectAttemptRelayKeptSuppressedProbeLimit)
	}
}

func TestDirectAttemptPolicyDefaultsResultSpecificSuppressedProbeRefillIntervalToBase(t *testing.T) {
	policy := directAttemptPolicyFromConfig(config.Config{
		DirectAttemptSuppressedProbeRefillInterval: 21 * time.Second,
	})
	if policy.DirectAttemptTimeoutSuppressedProbeRefillInterval != 21*time.Second {
		t.Fatalf("expected timeout suppressed probe refill interval to fall back to base value, got %s", policy.DirectAttemptTimeoutSuppressedProbeRefillInterval)
	}
	if policy.DirectAttemptRelayKeptSuppressedProbeRefillInterval != 21*time.Second {
		t.Fatalf("expected relay_kept suppressed probe refill interval to fall back to base value, got %s", policy.DirectAttemptRelayKeptSuppressedProbeRefillInterval)
	}
}

func TestDirectAttemptPolicyDefaultsPrimaryUpgradeProfileToRelayActive(t *testing.T) {
	policy := directAttemptPolicyFromConfig(config.Config{
		DirectAttemptLead:                  150 * time.Millisecond,
		DirectAttemptWindow:                600 * time.Millisecond,
		DirectAttemptBurstInterval:         80 * time.Millisecond,
		RelayActiveAttemptLead:             200 * time.Millisecond,
		RelayActiveAttemptWindow:           900 * time.Millisecond,
		RelayActiveAttemptBurstInterval:    60 * time.Millisecond,
		PrimaryUpgradeAttemptLead:          0,
		PrimaryUpgradeAttemptWindow:        0,
		PrimaryUpgradeAttemptBurstInterval: 0,
	})
	if policy.PrimaryUpgradeAttemptLead != policy.RelayActiveAttemptLead {
		t.Fatalf("expected primary_upgrade lead to fall back to relay_active, got %s", policy.PrimaryUpgradeAttemptLead)
	}
	if policy.PrimaryUpgradeAttemptWindow != policy.RelayActiveAttemptWindow {
		t.Fatalf("expected primary_upgrade window to fall back to relay_active, got %s", policy.PrimaryUpgradeAttemptWindow)
	}
	if policy.PrimaryUpgradeAttemptBurstInterval != policy.RelayActiveAttemptBurstInterval {
		t.Fatalf("expected primary_upgrade burst interval to fall back to relay_active, got %s", policy.PrimaryUpgradeAttemptBurstInterval)
	}
}

func TestDirectAttemptSuppressionStateUsesPrimaryUpgradeSpecificSettings(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		TransportFreshnessWindow:                           30 * time.Second,
		DirectAttemptFailureSuppressAfter:                  4,
		DirectAttemptFailureSuppressWindow:                 45 * time.Second,
		DirectAttemptTimeoutSuppressAfter:                  4,
		DirectAttemptTimeoutSuppressWindow:                 45 * time.Second,
		DirectAttemptTimeoutSuppressedProbeInterval:        10 * time.Second,
		DirectAttemptTimeoutSuppressedProbeLimit:           2,
		DirectAttemptTimeoutSuppressedProbeRefillInterval:  30 * time.Second,
		PrimaryUpgradeAttemptSuppressAfter:                 2,
		PrimaryUpgradeAttemptSuppressWindow:                2 * time.Minute,
		PrimaryUpgradeAttemptSuppressedProbeInterval:       25 * time.Second,
		PrimaryUpgradeAttemptSuppressedProbeLimit:          1,
		PrimaryUpgradeAttemptSuppressedProbeRefillInterval: 50 * time.Second,
	}

	attemptAt := now.Add(-8 * time.Second)
	state := directAttemptSuppressionStateWithPolicy(api.PeerTransportState{
		ActiveKind:                "relay",
		ReportedAt:                now,
		LastDirectAttemptAt:       attemptAt,
		LastDirectAttemptResult:   "timeout",
		LastDirectAttemptProfile:  "primary_upgrade",
		ConsecutiveDirectFailures: 2,
	}, now, policy)
	if !state.Blocked || state.Reason != "suppressed_timeout_budget" {
		t.Fatalf("expected primary_upgrade suppression to apply early, got %#v", state)
	}
	if want := attemptAt.Add(2 * time.Minute); !state.Until.Equal(want) {
		t.Fatalf("expected primary_upgrade suppression window until %s, got %#v", want, state)
	}
	if want := attemptAt.Add(25 * time.Second); !state.NextProbeAt.Equal(want) {
		t.Fatalf("expected primary_upgrade probe interval at %s, got %#v", want, state)
	}
	if !state.ProbeLimited || state.ProbeBudget != 1 || state.ProbeRemaining != 1 {
		t.Fatalf("expected primary_upgrade probe budget override, got %#v", state)
	}
}

func TestNewDirectAttemptPairUsesReasonSpecificProfiles(t *testing.T) {
	now := time.Now().UTC()
	policy := directAttemptPolicy{
		DirectAttemptLead:                  150 * time.Millisecond,
		DirectAttemptWindow:                600 * time.Millisecond,
		DirectAttemptBurstInterval:         80 * time.Millisecond,
		DirectAttemptRetention:             2 * time.Second,
		RelayActiveAttemptLead:             200 * time.Millisecond,
		RelayActiveAttemptWindow:           900 * time.Millisecond,
		RelayActiveAttemptBurstInterval:    60 * time.Millisecond,
		PrimaryUpgradeAttemptLead:          300 * time.Millisecond,
		PrimaryUpgradeAttemptWindow:        1100 * time.Millisecond,
		PrimaryUpgradeAttemptBurstInterval: 40 * time.Millisecond,
		ManualRecoverAttemptLead:           250 * time.Millisecond,
		ManualRecoverAttemptWindow:         1500 * time.Millisecond,
		ManualRecoverAttemptBurstInterval:  50 * time.Millisecond,
	}
	nodeA := api.Node{ID: "node-a"}
	nodeB := api.Node{ID: "node-b"}

	relayPair := newDirectAttemptPair(now, nodeA, nodeB, []api.DirectAttemptCandidate{storeDirectAttemptCandidate("198.51.100.10:51820")}, []api.DirectAttemptCandidate{storeDirectAttemptCandidate("198.51.100.11:51820")}, "relay_active", policy, api.PeerTransportState{}, api.PeerTransportState{})
	if relayPair.ExecuteAt.Sub(now) != policy.RelayActiveAttemptLead {
		t.Fatalf("expected relay_active lead %s, got %s", policy.RelayActiveAttemptLead, relayPair.ExecuteAt.Sub(now))
	}
	if relayPair.Window != policy.RelayActiveAttemptWindow {
		t.Fatalf("expected relay_active window %s, got %s", policy.RelayActiveAttemptWindow, relayPair.Window)
	}
	if relayPair.BurstInterval != policy.RelayActiveAttemptBurstInterval {
		t.Fatalf("expected relay_active burst interval %s, got %s", policy.RelayActiveAttemptBurstInterval, relayPair.BurstInterval)
	}
	if relayPair.Profile != "relay_active" {
		t.Fatalf("expected relay_active profile label, got %#v", relayPair)
	}

	manualPair := newDirectAttemptPair(now, nodeA, nodeB, []api.DirectAttemptCandidate{storeDirectAttemptCandidate("198.51.100.10:51820")}, []api.DirectAttemptCandidate{storeDirectAttemptCandidate("198.51.100.11:51820")}, "manual_recover", policy, api.PeerTransportState{}, api.PeerTransportState{})
	if manualPair.ExecuteAt.Sub(now) != policy.ManualRecoverAttemptLead {
		t.Fatalf("expected manual_recover lead %s, got %s", policy.ManualRecoverAttemptLead, manualPair.ExecuteAt.Sub(now))
	}
	if manualPair.Window != policy.ManualRecoverAttemptWindow {
		t.Fatalf("expected manual_recover window %s, got %s", policy.ManualRecoverAttemptWindow, manualPair.Window)
	}
	if manualPair.BurstInterval != policy.ManualRecoverAttemptBurstInterval {
		t.Fatalf("expected manual_recover burst interval %s, got %s", policy.ManualRecoverAttemptBurstInterval, manualPair.BurstInterval)
	}

	freshPair := newDirectAttemptPair(now, nodeA, nodeB, []api.DirectAttemptCandidate{storeDirectAttemptCandidate("198.51.100.10:51820")}, []api.DirectAttemptCandidate{storeDirectAttemptCandidate("198.51.100.11:51820")}, "fresh_endpoints", policy, api.PeerTransportState{}, api.PeerTransportState{})
	if freshPair.ExecuteAt.Sub(now) != policy.DirectAttemptLead {
		t.Fatalf("expected fresh_endpoints lead %s, got %s", policy.DirectAttemptLead, freshPair.ExecuteAt.Sub(now))
	}
	if freshPair.Window != policy.DirectAttemptWindow {
		t.Fatalf("expected fresh_endpoints window %s, got %s", policy.DirectAttemptWindow, freshPair.Window)
	}
	if freshPair.BurstInterval != policy.DirectAttemptBurstInterval {
		t.Fatalf("expected fresh_endpoints burst interval %s, got %s", policy.DirectAttemptBurstInterval, freshPair.BurstInterval)
	}

	primaryUpgradePair := newDirectAttemptPair(
		now,
		nodeA,
		nodeB,
		[]api.DirectAttemptCandidate{storeDirectAttemptCandidateForSource("198.51.100.10:51820", "stun", now)},
		[]api.DirectAttemptCandidate{storeDirectAttemptCandidateForSource("198.51.100.11:51820", "listener", now)},
		"relay_active",
		policy,
		api.PeerTransportState{
			ActiveKind:              "relay",
			ReportedAt:              now,
			LastDirectAttemptAt:     now.Add(-3 * time.Second),
			LastDirectAttemptResult: "timeout",
			LastDirectAttemptPhase:  api.DirectAttemptPhaseSecondary,
		},
		api.PeerTransportState{},
	)
	if primaryUpgradePair.ExecuteAt.Sub(now) != policy.PrimaryUpgradeAttemptLead {
		t.Fatalf("expected primary_upgrade lead %s, got %s", policy.PrimaryUpgradeAttemptLead, primaryUpgradePair.ExecuteAt.Sub(now))
	}
	if primaryUpgradePair.Window != policy.PrimaryUpgradeAttemptWindow {
		t.Fatalf("expected primary_upgrade window %s, got %s", policy.PrimaryUpgradeAttemptWindow, primaryUpgradePair.Window)
	}
	if primaryUpgradePair.BurstInterval != policy.PrimaryUpgradeAttemptBurstInterval {
		t.Fatalf("expected primary_upgrade burst interval %s, got %s", policy.PrimaryUpgradeAttemptBurstInterval, primaryUpgradePair.BurstInterval)
	}
	if primaryUpgradePair.Profile != "primary_upgrade" {
		t.Fatalf("expected primary_upgrade profile label, got %#v", primaryUpgradePair)
	}
}
