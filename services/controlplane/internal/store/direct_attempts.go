package store

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/packages/runtime/go/session"
	"nodeweave/services/controlplane/internal/config"
)

const (
	maxNATSamples                       = 4
	maxPrimaryDirectAttemptCandidates   = 4
	maxSecondaryDirectAttemptCandidates = 2
)

type directAttemptPolicy struct {
	NodeOnlineWindow                                    time.Duration
	EndpointFreshnessWindow                             time.Duration
	TransportFreshnessWindow                            time.Duration
	DirectAttemptCooldown                               time.Duration
	DirectAttemptTimeoutCooldown                        time.Duration
	DirectAttemptRelayKeptCooldown                      time.Duration
	DirectAttemptLead                                   time.Duration
	DirectAttemptWindow                                 time.Duration
	DirectAttemptBurstInterval                          time.Duration
	DirectAttemptRetention                              time.Duration
	DirectAttemptManualRecoverAfter                     time.Duration
	DirectAttemptTimeoutManualRecoverAfter              time.Duration
	DirectAttemptRelayKeptManualRecoverAfter            time.Duration
	DirectAttemptFailureSuppressAfter                   int
	DirectAttemptTimeoutSuppressAfter                   int
	DirectAttemptRelayKeptSuppressAfter                 int
	DirectAttemptFailureSuppressWindow                  time.Duration
	DirectAttemptTimeoutSuppressWindow                  time.Duration
	DirectAttemptRelayKeptSuppressWindow                time.Duration
	DirectAttemptSuppressedProbeInterval                time.Duration
	DirectAttemptTimeoutSuppressedProbeInterval         time.Duration
	DirectAttemptRelayKeptSuppressedProbeInterval       time.Duration
	DirectAttemptSuppressedProbeLimit                   int
	DirectAttemptTimeoutSuppressedProbeLimit            int
	DirectAttemptRelayKeptSuppressedProbeLimit          int
	DirectAttemptSuppressedProbeRefillInterval          time.Duration
	DirectAttemptTimeoutSuppressedProbeRefillInterval   time.Duration
	DirectAttemptRelayKeptSuppressedProbeRefillInterval time.Duration
	RelayActiveAttemptLead                              time.Duration
	RelayActiveAttemptWindow                            time.Duration
	RelayActiveAttemptBurstInterval                     time.Duration
	PrimaryUpgradeAttemptLead                           time.Duration
	PrimaryUpgradeAttemptWindow                         time.Duration
	PrimaryUpgradeAttemptBurstInterval                  time.Duration
	PrimaryUpgradeAttemptCooldown                       time.Duration
	PrimaryUpgradeAttemptManualRecoverAfter             time.Duration
	PrimaryUpgradeAttemptSuppressAfter                  int
	PrimaryUpgradeAttemptSuppressWindow                 time.Duration
	PrimaryUpgradeAttemptSuppressedProbeInterval        time.Duration
	PrimaryUpgradeAttemptSuppressedProbeLimit           int
	PrimaryUpgradeAttemptSuppressedProbeRefillInterval  time.Duration
	ManualRecoverAttemptLead                            time.Duration
	ManualRecoverAttemptWindow                          time.Duration
	ManualRecoverAttemptBurstInterval                   time.Duration
}

func directAttemptPolicyFromConfig(cfg config.Config) directAttemptPolicy {
	policy := directAttemptPolicy{
		NodeOnlineWindow:                                    cfg.NodeOnlineWindow,
		EndpointFreshnessWindow:                             cfg.EndpointFreshnessWindow,
		TransportFreshnessWindow:                            cfg.TransportFreshnessWindow,
		DirectAttemptCooldown:                               cfg.DirectAttemptCooldown,
		DirectAttemptTimeoutCooldown:                        cfg.DirectAttemptTimeoutCooldown,
		DirectAttemptRelayKeptCooldown:                      cfg.DirectAttemptRelayKeptCooldown,
		DirectAttemptLead:                                   cfg.DirectAttemptLead,
		DirectAttemptWindow:                                 cfg.DirectAttemptWindow,
		DirectAttemptBurstInterval:                          cfg.DirectAttemptBurstInterval,
		DirectAttemptRetention:                              cfg.DirectAttemptRetention,
		DirectAttemptManualRecoverAfter:                     cfg.DirectAttemptManualRecoverAfter,
		DirectAttemptTimeoutManualRecoverAfter:              cfg.DirectAttemptTimeoutManualRecoverAfter,
		DirectAttemptRelayKeptManualRecoverAfter:            cfg.DirectAttemptRelayKeptManualRecoverAfter,
		DirectAttemptFailureSuppressAfter:                   cfg.DirectAttemptFailureSuppressAfter,
		DirectAttemptTimeoutSuppressAfter:                   cfg.DirectAttemptTimeoutSuppressAfter,
		DirectAttemptRelayKeptSuppressAfter:                 cfg.DirectAttemptRelayKeptSuppressAfter,
		DirectAttemptFailureSuppressWindow:                  cfg.DirectAttemptFailureSuppressWindow,
		DirectAttemptTimeoutSuppressWindow:                  cfg.DirectAttemptTimeoutSuppressWindow,
		DirectAttemptRelayKeptSuppressWindow:                cfg.DirectAttemptRelayKeptSuppressWindow,
		DirectAttemptSuppressedProbeInterval:                cfg.DirectAttemptSuppressedProbeInterval,
		DirectAttemptTimeoutSuppressedProbeInterval:         cfg.DirectAttemptTimeoutSuppressedProbeInterval,
		DirectAttemptRelayKeptSuppressedProbeInterval:       cfg.DirectAttemptRelayKeptSuppressedProbeInterval,
		DirectAttemptSuppressedProbeLimit:                   cfg.DirectAttemptSuppressedProbeLimit,
		DirectAttemptTimeoutSuppressedProbeLimit:            cfg.DirectAttemptTimeoutSuppressedProbeLimit,
		DirectAttemptRelayKeptSuppressedProbeLimit:          cfg.DirectAttemptRelayKeptSuppressedProbeLimit,
		DirectAttemptSuppressedProbeRefillInterval:          cfg.DirectAttemptSuppressedProbeRefillInterval,
		DirectAttemptTimeoutSuppressedProbeRefillInterval:   cfg.DirectAttemptTimeoutSuppressedProbeRefillInterval,
		DirectAttemptRelayKeptSuppressedProbeRefillInterval: cfg.DirectAttemptRelayKeptSuppressedProbeRefillInterval,
		RelayActiveAttemptLead:                              cfg.RelayActiveAttemptLead,
		RelayActiveAttemptWindow:                            cfg.RelayActiveAttemptWindow,
		RelayActiveAttemptBurstInterval:                     cfg.RelayActiveAttemptBurstInterval,
		PrimaryUpgradeAttemptLead:                           cfg.PrimaryUpgradeAttemptLead,
		PrimaryUpgradeAttemptWindow:                         cfg.PrimaryUpgradeAttemptWindow,
		PrimaryUpgradeAttemptBurstInterval:                  cfg.PrimaryUpgradeAttemptBurstInterval,
		PrimaryUpgradeAttemptCooldown:                       cfg.PrimaryUpgradeAttemptCooldown,
		PrimaryUpgradeAttemptManualRecoverAfter:             cfg.PrimaryUpgradeAttemptManualRecoverAfter,
		PrimaryUpgradeAttemptSuppressAfter:                  cfg.PrimaryUpgradeAttemptSuppressAfter,
		PrimaryUpgradeAttemptSuppressWindow:                 cfg.PrimaryUpgradeAttemptSuppressWindow,
		PrimaryUpgradeAttemptSuppressedProbeInterval:        cfg.PrimaryUpgradeAttemptSuppressedProbeInterval,
		PrimaryUpgradeAttemptSuppressedProbeLimit:           cfg.PrimaryUpgradeAttemptSuppressedProbeLimit,
		PrimaryUpgradeAttemptSuppressedProbeRefillInterval:  cfg.PrimaryUpgradeAttemptSuppressedProbeRefillInterval,
		ManualRecoverAttemptLead:                            cfg.ManualRecoverAttemptLead,
		ManualRecoverAttemptWindow:                          cfg.ManualRecoverAttemptWindow,
		ManualRecoverAttemptBurstInterval:                   cfg.ManualRecoverAttemptBurstInterval,
	}
	if policy.NodeOnlineWindow <= 0 {
		policy.NodeOnlineWindow = 30 * time.Second
	}
	if policy.EndpointFreshnessWindow <= 0 {
		policy.EndpointFreshnessWindow = 45 * time.Second
	}
	if policy.TransportFreshnessWindow <= 0 {
		policy.TransportFreshnessWindow = 30 * time.Second
	}
	if policy.DirectAttemptCooldown <= 0 {
		policy.DirectAttemptCooldown = 10 * time.Second
	}
	if policy.DirectAttemptTimeoutCooldown <= 0 {
		policy.DirectAttemptTimeoutCooldown = policy.DirectAttemptCooldown
	}
	if policy.DirectAttemptRelayKeptCooldown <= 0 {
		policy.DirectAttemptRelayKeptCooldown = policy.DirectAttemptCooldown
	}
	if policy.DirectAttemptLead <= 0 {
		policy.DirectAttemptLead = 150 * time.Millisecond
	}
	if policy.DirectAttemptWindow <= 0 {
		policy.DirectAttemptWindow = 600 * time.Millisecond
	}
	if policy.DirectAttemptBurstInterval <= 0 {
		policy.DirectAttemptBurstInterval = 80 * time.Millisecond
	}
	if policy.DirectAttemptRetention <= 0 {
		policy.DirectAttemptRetention = 2 * time.Second
	}
	if policy.DirectAttemptManualRecoverAfter <= 0 {
		policy.DirectAttemptManualRecoverAfter = 30 * time.Second
	}
	if policy.DirectAttemptTimeoutManualRecoverAfter <= 0 {
		policy.DirectAttemptTimeoutManualRecoverAfter = policy.DirectAttemptManualRecoverAfter
	}
	if policy.DirectAttemptRelayKeptManualRecoverAfter <= 0 {
		policy.DirectAttemptRelayKeptManualRecoverAfter = policy.DirectAttemptManualRecoverAfter
	}
	if policy.DirectAttemptFailureSuppressAfter < 0 {
		policy.DirectAttemptFailureSuppressAfter = 0
	}
	if policy.DirectAttemptTimeoutSuppressAfter <= 0 {
		policy.DirectAttemptTimeoutSuppressAfter = policy.DirectAttemptFailureSuppressAfter
	}
	if policy.DirectAttemptRelayKeptSuppressAfter <= 0 {
		policy.DirectAttemptRelayKeptSuppressAfter = policy.DirectAttemptFailureSuppressAfter
	}
	if policy.DirectAttemptFailureSuppressWindow < 0 {
		policy.DirectAttemptFailureSuppressWindow = 0
	}
	if policy.DirectAttemptTimeoutSuppressWindow <= 0 {
		policy.DirectAttemptTimeoutSuppressWindow = policy.DirectAttemptFailureSuppressWindow
	}
	if policy.DirectAttemptRelayKeptSuppressWindow <= 0 {
		policy.DirectAttemptRelayKeptSuppressWindow = policy.DirectAttemptFailureSuppressWindow
	}
	if policy.DirectAttemptSuppressedProbeInterval < 0 {
		policy.DirectAttemptSuppressedProbeInterval = 0
	}
	if policy.DirectAttemptTimeoutSuppressedProbeInterval <= 0 {
		policy.DirectAttemptTimeoutSuppressedProbeInterval = policy.DirectAttemptSuppressedProbeInterval
	}
	if policy.DirectAttemptRelayKeptSuppressedProbeInterval <= 0 {
		policy.DirectAttemptRelayKeptSuppressedProbeInterval = policy.DirectAttemptSuppressedProbeInterval
	}
	if policy.DirectAttemptSuppressedProbeLimit < 0 {
		policy.DirectAttemptSuppressedProbeLimit = 0
	}
	if policy.DirectAttemptTimeoutSuppressedProbeLimit <= 0 {
		policy.DirectAttemptTimeoutSuppressedProbeLimit = policy.DirectAttemptSuppressedProbeLimit
	}
	if policy.DirectAttemptRelayKeptSuppressedProbeLimit <= 0 {
		policy.DirectAttemptRelayKeptSuppressedProbeLimit = policy.DirectAttemptSuppressedProbeLimit
	}
	if policy.DirectAttemptSuppressedProbeRefillInterval < 0 {
		policy.DirectAttemptSuppressedProbeRefillInterval = 0
	}
	if policy.DirectAttemptTimeoutSuppressedProbeRefillInterval <= 0 {
		policy.DirectAttemptTimeoutSuppressedProbeRefillInterval = policy.DirectAttemptSuppressedProbeRefillInterval
	}
	if policy.DirectAttemptRelayKeptSuppressedProbeRefillInterval <= 0 {
		policy.DirectAttemptRelayKeptSuppressedProbeRefillInterval = policy.DirectAttemptSuppressedProbeRefillInterval
	}
	if policy.RelayActiveAttemptLead <= 0 {
		policy.RelayActiveAttemptLead = policy.DirectAttemptLead
	}
	if policy.RelayActiveAttemptWindow <= 0 {
		policy.RelayActiveAttemptWindow = policy.DirectAttemptWindow
	}
	if policy.RelayActiveAttemptBurstInterval <= 0 {
		policy.RelayActiveAttemptBurstInterval = policy.DirectAttemptBurstInterval
	}
	if policy.PrimaryUpgradeAttemptLead <= 0 {
		policy.PrimaryUpgradeAttemptLead = policy.RelayActiveAttemptLead
	}
	if policy.PrimaryUpgradeAttemptWindow <= 0 {
		policy.PrimaryUpgradeAttemptWindow = policy.RelayActiveAttemptWindow
	}
	if policy.PrimaryUpgradeAttemptBurstInterval <= 0 {
		policy.PrimaryUpgradeAttemptBurstInterval = policy.RelayActiveAttemptBurstInterval
	}
	if policy.PrimaryUpgradeAttemptSuppressAfter < 0 {
		policy.PrimaryUpgradeAttemptSuppressAfter = 0
	}
	if policy.PrimaryUpgradeAttemptSuppressWindow < 0 {
		policy.PrimaryUpgradeAttemptSuppressWindow = 0
	}
	if policy.PrimaryUpgradeAttemptSuppressedProbeInterval < 0 {
		policy.PrimaryUpgradeAttemptSuppressedProbeInterval = 0
	}
	if policy.PrimaryUpgradeAttemptSuppressedProbeLimit < 0 {
		policy.PrimaryUpgradeAttemptSuppressedProbeLimit = 0
	}
	if policy.PrimaryUpgradeAttemptSuppressedProbeRefillInterval < 0 {
		policy.PrimaryUpgradeAttemptSuppressedProbeRefillInterval = 0
	}
	if policy.ManualRecoverAttemptLead <= 0 {
		policy.ManualRecoverAttemptLead = policy.DirectAttemptLead
	}
	if policy.ManualRecoverAttemptWindow <= 0 {
		policy.ManualRecoverAttemptWindow = policy.DirectAttemptWindow
	}
	if policy.ManualRecoverAttemptBurstInterval <= 0 {
		policy.ManualRecoverAttemptBurstInterval = policy.DirectAttemptBurstInterval
	}
	return policy
}

type directAttemptProfile struct {
	Lead          time.Duration
	Window        time.Duration
	BurstInterval time.Duration
}

func (p directAttemptPolicy) profileForReason(reason string) directAttemptProfile {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "relay_active":
		return directAttemptProfile{
			Lead:          p.RelayActiveAttemptLead,
			Window:        p.RelayActiveAttemptWindow,
			BurstInterval: p.RelayActiveAttemptBurstInterval,
		}
	case "manual_recover":
		return directAttemptProfile{
			Lead:          p.ManualRecoverAttemptLead,
			Window:        p.ManualRecoverAttemptWindow,
			BurstInterval: p.ManualRecoverAttemptBurstInterval,
		}
	default:
		return directAttemptProfile{
			Lead:          p.DirectAttemptLead,
			Window:        p.DirectAttemptWindow,
			BurstInterval: p.DirectAttemptBurstInterval,
		}
	}
}

func (p directAttemptPolicy) profileForAttempt(reason string, selfCandidates, peerCandidates []api.DirectAttemptCandidate, selfState, peerState api.PeerTransportState, now time.Time) (string, directAttemptProfile) {
	if shouldBypassDirectAttemptCooldownForPrimaryUpgrade(selfCandidates, peerCandidates, selfState, peerState, now, p) {
		return "primary_upgrade", directAttemptProfile{
			Lead:          p.PrimaryUpgradeAttemptLead,
			Window:        p.PrimaryUpgradeAttemptWindow,
			BurstInterval: p.PrimaryUpgradeAttemptBurstInterval,
		}
	}
	return strings.ToLower(strings.TrimSpace(reason)), p.profileForReason(reason)
}

func normalizeDirectAttemptProfileName(profile string) string {
	return strings.ToLower(strings.TrimSpace(profile))
}

func isPrimaryUpgradeAttemptProfile(profile string) bool {
	return normalizeDirectAttemptProfileName(profile) == "primary_upgrade"
}

type directAttemptPair struct {
	AttemptID       string
	NodeAID         string
	NodeBID         string
	NodeACandidates []api.DirectAttemptCandidate
	NodeBCandidates []api.DirectAttemptCandidate
	Profile         string
	IssuedAt        time.Time
	ExecuteAt       time.Time
	Window          time.Duration
	BurstInterval   time.Duration
	Reason          string
	ExpiresAt       time.Time
}

type directAttemptBlockState struct {
	Blocked        bool
	Reason         string
	Until          time.Time
	NextProbeAt    time.Time
	ProbeLimited   bool
	ProbeBudget    int
	ProbeFailures  int
	ProbeRemaining int
	ProbeRefillAt  time.Time
}

type directAttemptDecision struct {
	Status string
	Reason string
	At     time.Time
	NextAt time.Time
}

func pairKey(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == right {
		return left
	}
	if left > right {
		left, right = right, left
	}
	return left + "|" + right
}

func (p directAttemptPair) instructionFor(nodeID string) (api.DirectAttemptInstruction, bool) {
	var (
		peerNodeID string
		candidates []api.DirectAttemptCandidate
	)
	switch strings.TrimSpace(nodeID) {
	case strings.TrimSpace(p.NodeAID):
		peerNodeID = p.NodeBID
		candidates = p.NodeACandidates
	case strings.TrimSpace(p.NodeBID):
		peerNodeID = p.NodeAID
		candidates = p.NodeBCandidates
	default:
		return api.DirectAttemptInstruction{}, false
	}
	return api.DirectAttemptInstruction{
		AttemptID:     p.AttemptID,
		PeerNodeID:    peerNodeID,
		IssuedAt:      p.IssuedAt,
		ExecuteAt:     p.ExecuteAt,
		Window:        p.Window.Milliseconds(),
		BurstInterval: p.BurstInterval.Milliseconds(),
		Candidates:    append([]api.DirectAttemptCandidate(nil), candidates...),
		Profile:       strings.TrimSpace(p.Profile),
		Reason:        p.Reason,
	}, true
}

func sanitizeNATReport(now time.Time, report api.NATReport) api.NATReport {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if report.GeneratedAt.IsZero() {
		report.GeneratedAt = now
	} else {
		report.GeneratedAt = report.GeneratedAt.UTC()
	}
	report.MappingBehavior = normalizeMappingBehavior(report.MappingBehavior)

	samples := make([]api.NATSample, 0, len(report.Samples))
	for _, sample := range report.Samples {
		server := strings.TrimSpace(sample.Server)
		if server == "" {
			continue
		}
		samples = append(samples, api.NATSample{
			Server:           server,
			Status:           strings.TrimSpace(sample.Status),
			RTTMillis:        sample.RTTMillis,
			ReflexiveAddress: strings.TrimSpace(sample.ReflexiveAddress),
			Error:            strings.TrimSpace(sample.Error),
		})
		if len(samples) >= maxNATSamples {
			break
		}
	}
	report.Samples = samples
	report.SampleCount = len(samples)
	report.SelectedReflexiveAddress = strings.TrimSpace(report.SelectedReflexiveAddress)
	if report.SelectedReflexiveAddress == "" {
		for _, sample := range samples {
			if strings.EqualFold(sample.Status, "reachable") && sample.ReflexiveAddress != "" {
				report.SelectedReflexiveAddress = sample.ReflexiveAddress
				break
			}
		}
	}
	reachable := false
	for _, sample := range samples {
		if strings.EqualFold(sample.Status, "reachable") && sample.ReflexiveAddress != "" {
			reachable = true
			break
		}
	}
	report.Reachable = reachable
	if report.MappingBehavior == "" {
		report.MappingBehavior = "unknown"
	}
	return report
}

func normalizeMappingBehavior(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "stable_port", "varying_port":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}

func isNodeOnline(node api.Node, now time.Time) bool {
	return isNodeOnlineWithPolicy(node, now, directAttemptPolicyFromConfig(config.Config{}))
}

func isNodeOnlineWithPolicy(node api.Node, now time.Time, policy directAttemptPolicy) bool {
	if strings.ToLower(strings.TrimSpace(node.Status)) != "online" {
		return false
	}
	if node.LastSeenAt.IsZero() {
		return false
	}
	return now.Sub(node.LastSeenAt.UTC()) <= policy.NodeOnlineWindow
}

func freshDirectCandidateAddresses(node api.Node, now time.Time) []string {
	return freshDirectCandidateAddressesWithPolicy(node, now, directAttemptPolicyFromConfig(config.Config{}))
}

func freshDirectCandidateAddressesWithPolicy(node api.Node, now time.Time, policy directAttemptPolicy) []string {
	candidates := freshDirectAttemptCandidatesWithPolicy(node, now, policy)
	return dedupeStrings(api.DirectAttemptCandidateAddresses(candidates))
}

func freshDirectAttemptCandidatesWithPolicy(node api.Node, now time.Time, policy directAttemptPolicy) []api.DirectAttemptCandidate {
	sessionCandidates := session.FreshDirectCandidates(now, node.Endpoints, node.EndpointRecords, policy.EndpointFreshnessWindow)
	candidates := make([]api.DirectAttemptCandidate, 0, len(sessionCandidates))
	for _, candidate := range sessionCandidates {
		if strings.ToLower(strings.TrimSpace(candidate.Kind)) != "direct" {
			continue
		}
		address := strings.TrimSpace(candidate.Address)
		if address == "" {
			continue
		}
		candidates = append(candidates, api.DirectAttemptCandidate{
			Address:    address,
			Source:     candidate.Source,
			ObservedAt: candidate.ObservedAt,
			Priority:   candidate.Priority,
			Phase:      api.DirectAttemptPhaseForSource(candidate.Source),
		})
	}
	candidates = api.NormalizeDirectAttemptCandidates(candidates, now)
	api.SortDirectAttemptCandidates(candidates)

	primary := make([]api.DirectAttemptCandidate, 0, maxPrimaryDirectAttemptCandidates)
	secondary := make([]api.DirectAttemptCandidate, 0, maxSecondaryDirectAttemptCandidates)
	for _, candidate := range candidates {
		switch api.NormalizeDirectAttemptPhase(candidate.Phase, candidate.Source) {
		case api.DirectAttemptPhasePrimary:
			if len(primary) < maxPrimaryDirectAttemptCandidates {
				primary = append(primary, candidate)
			}
		default:
			if len(secondary) < maxSecondaryDirectAttemptCandidates {
				secondary = append(secondary, candidate)
			}
		}
	}
	return append(primary, secondary...)
}

func natSummaryForPeer(report api.NATReport) (mapping string, reachable bool, reportedAt time.Time) {
	sanitized := sanitizeNATReport(time.Now().UTC(), report)
	return sanitized.MappingBehavior, sanitized.Reachable, sanitized.GeneratedAt
}

func sanitizePeerTransportStates(now time.Time, states []api.PeerTransportState) []api.PeerTransportState {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	byPeer := make(map[string]api.PeerTransportState, len(states))
	for _, state := range states {
		peerNodeID := strings.TrimSpace(state.PeerNodeID)
		if peerNodeID == "" {
			continue
		}
		state.PeerNodeID = peerNodeID
		state.ActiveKind = strings.ToLower(strings.TrimSpace(state.ActiveKind))
		state.ActiveAddress = strings.TrimSpace(state.ActiveAddress)
		state.LastDirectAttemptResult = strings.TrimSpace(state.LastDirectAttemptResult)
		state.LastDirectAttemptProfile = strings.TrimSpace(state.LastDirectAttemptProfile)
		state.LastDirectAttemptReachedSource = strings.TrimSpace(state.LastDirectAttemptReachedSource)
		state.LastDirectAttemptPhase = strings.TrimSpace(state.LastDirectAttemptPhase)
		if state.LastDirectAttemptCandidateCount < 0 {
			state.LastDirectAttemptCandidateCount = 0
		}
		if state.LastDirectSuccessAt.IsZero() {
			state.LastDirectSuccessAt = time.Time{}
		} else {
			state.LastDirectSuccessAt = state.LastDirectSuccessAt.UTC()
		}
		if state.ConsecutiveDirectFailures < 0 {
			state.ConsecutiveDirectFailures = 0
		}
		if state.ReportedAt.IsZero() {
			state.ReportedAt = now
		} else {
			state.ReportedAt = state.ReportedAt.UTC()
		}
		if state.LastDirectAttemptAt.IsZero() {
			state.LastDirectAttemptAt = state.ReportedAt
		} else {
			state.LastDirectAttemptAt = state.LastDirectAttemptAt.UTC()
		}
		existing, ok := byPeer[peerNodeID]
		if !ok || state.ReportedAt.After(existing.ReportedAt) {
			byPeer[peerNodeID] = state
		}
	}
	result := make([]api.PeerTransportState, 0, len(byPeer))
	for _, state := range byPeer {
		result = append(result, state)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].PeerNodeID < result[j].PeerNodeID
	})
	return result
}

func peerTransportStateLookup(states []api.PeerTransportState) map[string]api.PeerTransportState {
	result := make(map[string]api.PeerTransportState, len(states))
	for _, state := range states {
		result[strings.TrimSpace(state.PeerNodeID)] = state
	}
	return result
}

func transportStateFresh(state api.PeerTransportState, now time.Time) bool {
	return transportStateFreshWithPolicy(state, now, directAttemptPolicyFromConfig(config.Config{}))
}

func transportStateFreshWithPolicy(state api.PeerTransportState, now time.Time, policy directAttemptPolicy) bool {
	if state.ReportedAt.IsZero() {
		return false
	}
	return now.Sub(state.ReportedAt.UTC()) <= policy.TransportFreshnessWindow
}

func directAttemptCoolingDown(state api.PeerTransportState, now time.Time) bool {
	return directAttemptCoolingDownWithPolicy(state, now, directAttemptPolicyFromConfig(config.Config{}))
}

func directAttemptCoolingDownWithPolicy(state api.PeerTransportState, now time.Time, policy directAttemptPolicy) bool {
	return directAttemptCooldownStateWithPolicy(state, now, policy).Blocked
}

func directAttemptReason(selfState, peerState api.PeerTransportState, now time.Time) (string, bool) {
	return directAttemptReasonWithPolicy(selfState, peerState, now, directAttemptPolicyFromConfig(config.Config{}))
}

func directAttemptReasonWithPolicy(selfState, peerState api.PeerTransportState, now time.Time, policy directAttemptPolicy) (string, bool) {
	return directAttemptReasonWithPolicyCandidates(
		api.MigrateLegacyDirectAttemptCandidates(nil, time.Time{}, time.Time{}),
		api.MigrateLegacyDirectAttemptCandidates(nil, time.Time{}, time.Time{}),
		selfState,
		peerState,
		now,
		policy,
	)
}

func directAttemptReasonWithPolicyCandidates(selfCandidates, peerCandidates []api.DirectAttemptCandidate, selfState, peerState api.PeerTransportState, now time.Time, policy directAttemptPolicy) (string, bool) {
	selfFresh := transportStateFreshWithPolicy(selfState, now, policy)
	peerFresh := transportStateFreshWithPolicy(peerState, now, policy)
	if selfFresh && peerFresh && selfState.ActiveKind == "direct" && peerState.ActiveKind == "direct" {
		return "", false
	}
	cooldownBlock := laterBlockState(
		directAttemptCooldownStateWithPolicy(selfState, now, policy),
		directAttemptCooldownStateWithPolicy(peerState, now, policy),
	)
	if cooldownBlock.Blocked && !shouldBypassDirectAttemptCooldownForPrimaryUpgrade(selfCandidates, peerCandidates, selfState, peerState, now, policy) {
		return "", false
	}
	suppressionBlock := laterBlockState(
		directAttemptSuppressionStateWithPolicy(selfState, now, policy),
		directAttemptSuppressionStateWithPolicy(peerState, now, policy),
	)
	if suppressionBlock.Blocked {
		if suppressionBlock.NextProbeAt.IsZero() || now.Before(suppressionBlock.NextProbeAt) {
			return "", false
		}
		return "manual_recover", true
	}
	if (selfFresh && selfState.ActiveKind == "relay") || (peerFresh && peerState.ActiveKind == "relay") {
		if shouldUseManualRecover(selfState, now, policy) || shouldUseManualRecover(peerState, now, policy) {
			return "manual_recover", true
		}
		return "relay_active", true
	}
	if !hasPrimaryDirectAttemptCandidates(selfCandidates) && !hasPrimaryDirectAttemptCandidates(peerCandidates) {
		return "", false
	}
	return "fresh_endpoints", true
}

func hasPrimaryDirectAttemptCandidates(candidates []api.DirectAttemptCandidate) bool {
	for _, candidate := range candidates {
		if api.NormalizeDirectAttemptPhase(candidate.Phase, candidate.Source) == api.DirectAttemptPhasePrimary {
			return true
		}
	}
	return false
}

func shouldBypassDirectAttemptCooldownForPrimaryUpgrade(selfCandidates, peerCandidates []api.DirectAttemptCandidate, selfState, peerState api.PeerTransportState, now time.Time, policy directAttemptPolicy) bool {
	if !hasPrimaryDirectAttemptCandidates(selfCandidates) && !hasPrimaryDirectAttemptCandidates(peerCandidates) {
		return false
	}
	return stateHasPrimaryUpgrade(selfCandidates, selfState, now, policy) || stateHasPrimaryUpgrade(peerCandidates, peerState, now, policy)
}

func stateHasPrimaryUpgrade(candidates []api.DirectAttemptCandidate, state api.PeerTransportState, now time.Time, policy directAttemptPolicy) bool {
	if !transportStateFreshWithPolicy(state, now, policy) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(state.LastDirectAttemptResult)) {
	case "timeout", "relay_kept":
	default:
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(state.LastDirectAttemptPhase), api.DirectAttemptPhaseSecondary) {
		return false
	}
	attemptAt := state.LastDirectAttemptAt.UTC()
	if attemptAt.IsZero() {
		return false
	}
	return hasNewerPrimaryCandidateSince(candidates, attemptAt)
}

func hasNewerPrimaryCandidateSince(candidates []api.DirectAttemptCandidate, since time.Time) bool {
	if since.IsZero() {
		return false
	}
	for _, candidate := range candidates {
		if api.NormalizeDirectAttemptPhase(candidate.Phase, candidate.Source) != api.DirectAttemptPhasePrimary {
			continue
		}
		if !candidate.ObservedAt.IsZero() && candidate.ObservedAt.After(since) {
			return true
		}
	}
	return false
}

func directAttemptSuppressedWithPolicy(state api.PeerTransportState, now time.Time, policy directAttemptPolicy) bool {
	return directAttemptSuppressionStateWithPolicy(state, now, policy).Blocked
}

func shouldUseManualRecover(state api.PeerTransportState, now time.Time, policy directAttemptPolicy) bool {
	if !transportStateFreshWithPolicy(state, now, policy) {
		return false
	}
	if strings.ToLower(strings.TrimSpace(state.ActiveKind)) != "relay" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(state.LastDirectAttemptResult)) {
	case "timeout", "relay_kept":
	default:
		return false
	}
	attemptAt := state.LastDirectAttemptAt.UTC()
	if attemptAt.IsZero() {
		attemptAt = state.ReportedAt.UTC()
	}
	return now.Sub(attemptAt) >= policy.manualRecoverAfterForAttempt(state.LastDirectAttemptResult, state.LastDirectAttemptProfile)
}

func (p directAttemptPolicy) cooldownForResult(result string) time.Duration {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "timeout":
		if p.DirectAttemptTimeoutCooldown > 0 {
			return p.DirectAttemptTimeoutCooldown
		}
		return p.DirectAttemptCooldown
	case "relay_kept":
		if p.DirectAttemptRelayKeptCooldown > 0 {
			return p.DirectAttemptRelayKeptCooldown
		}
		return p.DirectAttemptCooldown
	default:
		return 0
	}
}

func (p directAttemptPolicy) cooldownForAttempt(result, profile string) time.Duration {
	if isPrimaryUpgradeAttemptProfile(profile) && p.PrimaryUpgradeAttemptCooldown > 0 {
		return p.PrimaryUpgradeAttemptCooldown
	}
	return p.cooldownForResult(result)
}

func (p directAttemptPolicy) manualRecoverAfterForResult(result string) time.Duration {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "timeout":
		if p.DirectAttemptTimeoutManualRecoverAfter > 0 {
			return p.DirectAttemptTimeoutManualRecoverAfter
		}
		return p.DirectAttemptManualRecoverAfter
	case "relay_kept":
		if p.DirectAttemptRelayKeptManualRecoverAfter > 0 {
			return p.DirectAttemptRelayKeptManualRecoverAfter
		}
		return p.DirectAttemptManualRecoverAfter
	default:
		return p.DirectAttemptManualRecoverAfter
	}
}

func (p directAttemptPolicy) manualRecoverAfterForAttempt(result, profile string) time.Duration {
	if isPrimaryUpgradeAttemptProfile(profile) && p.PrimaryUpgradeAttemptManualRecoverAfter > 0 {
		return p.PrimaryUpgradeAttemptManualRecoverAfter
	}
	return p.manualRecoverAfterForResult(result)
}

func (p directAttemptPolicy) suppressAfterForResult(result string) int {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "timeout":
		if p.DirectAttemptTimeoutSuppressAfter > 0 {
			return p.DirectAttemptTimeoutSuppressAfter
		}
		return p.DirectAttemptFailureSuppressAfter
	case "relay_kept":
		if p.DirectAttemptRelayKeptSuppressAfter > 0 {
			return p.DirectAttemptRelayKeptSuppressAfter
		}
		return p.DirectAttemptFailureSuppressAfter
	default:
		return 0
	}
}

func (p directAttemptPolicy) suppressAfterForAttempt(result, profile string) int {
	if isPrimaryUpgradeAttemptProfile(profile) && p.PrimaryUpgradeAttemptSuppressAfter > 0 {
		return p.PrimaryUpgradeAttemptSuppressAfter
	}
	return p.suppressAfterForResult(result)
}

func (p directAttemptPolicy) suppressWindowForResult(result string) time.Duration {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "timeout":
		if p.DirectAttemptTimeoutSuppressWindow > 0 {
			return p.DirectAttemptTimeoutSuppressWindow
		}
		return p.DirectAttemptFailureSuppressWindow
	case "relay_kept":
		if p.DirectAttemptRelayKeptSuppressWindow > 0 {
			return p.DirectAttemptRelayKeptSuppressWindow
		}
		return p.DirectAttemptFailureSuppressWindow
	default:
		return 0
	}
}

func (p directAttemptPolicy) suppressWindowForAttempt(result, profile string) time.Duration {
	if isPrimaryUpgradeAttemptProfile(profile) && p.PrimaryUpgradeAttemptSuppressWindow > 0 {
		return p.PrimaryUpgradeAttemptSuppressWindow
	}
	return p.suppressWindowForResult(result)
}

func (p directAttemptPolicy) suppressedProbeIntervalForResult(result string) time.Duration {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "timeout":
		if p.DirectAttemptTimeoutSuppressedProbeInterval > 0 {
			return p.DirectAttemptTimeoutSuppressedProbeInterval
		}
		return p.DirectAttemptSuppressedProbeInterval
	case "relay_kept":
		if p.DirectAttemptRelayKeptSuppressedProbeInterval > 0 {
			return p.DirectAttemptRelayKeptSuppressedProbeInterval
		}
		return p.DirectAttemptSuppressedProbeInterval
	default:
		return 0
	}
}

func (p directAttemptPolicy) suppressedProbeIntervalForAttempt(result, profile string) time.Duration {
	if isPrimaryUpgradeAttemptProfile(profile) && p.PrimaryUpgradeAttemptSuppressedProbeInterval > 0 {
		return p.PrimaryUpgradeAttemptSuppressedProbeInterval
	}
	return p.suppressedProbeIntervalForResult(result)
}

func (p directAttemptPolicy) suppressedProbeLimitForResult(result string) int {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "timeout":
		if p.DirectAttemptTimeoutSuppressedProbeLimit > 0 {
			return p.DirectAttemptTimeoutSuppressedProbeLimit
		}
		return p.DirectAttemptSuppressedProbeLimit
	case "relay_kept":
		if p.DirectAttemptRelayKeptSuppressedProbeLimit > 0 {
			return p.DirectAttemptRelayKeptSuppressedProbeLimit
		}
		return p.DirectAttemptSuppressedProbeLimit
	default:
		return 0
	}
}

func (p directAttemptPolicy) suppressedProbeLimitForAttempt(result, profile string) int {
	if isPrimaryUpgradeAttemptProfile(profile) && p.PrimaryUpgradeAttemptSuppressedProbeLimit > 0 {
		return p.PrimaryUpgradeAttemptSuppressedProbeLimit
	}
	return p.suppressedProbeLimitForResult(result)
}

func (p directAttemptPolicy) suppressedProbeRefillIntervalForResult(result string) time.Duration {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "timeout":
		if p.DirectAttemptTimeoutSuppressedProbeRefillInterval > 0 {
			return p.DirectAttemptTimeoutSuppressedProbeRefillInterval
		}
		return p.DirectAttemptSuppressedProbeRefillInterval
	case "relay_kept":
		if p.DirectAttemptRelayKeptSuppressedProbeRefillInterval > 0 {
			return p.DirectAttemptRelayKeptSuppressedProbeRefillInterval
		}
		return p.DirectAttemptSuppressedProbeRefillInterval
	default:
		return 0
	}
}

func (p directAttemptPolicy) suppressedProbeRefillIntervalForAttempt(result, profile string) time.Duration {
	if isPrimaryUpgradeAttemptProfile(profile) && p.PrimaryUpgradeAttemptSuppressedProbeRefillInterval > 0 {
		return p.PrimaryUpgradeAttemptSuppressedProbeRefillInterval
	}
	return p.suppressedProbeRefillIntervalForResult(result)
}

func directAttemptCooldownStateWithPolicy(state api.PeerTransportState, now time.Time, policy directAttemptPolicy) directAttemptBlockState {
	if !transportStateFreshWithPolicy(state, now, policy) {
		return directAttemptBlockState{}
	}
	cooldown := policy.cooldownForAttempt(state.LastDirectAttemptResult, state.LastDirectAttemptProfile)
	if cooldown <= 0 {
		return directAttemptBlockState{}
	}
	attemptAt := state.LastDirectAttemptAt.UTC()
	if attemptAt.IsZero() {
		attemptAt = state.ReportedAt.UTC()
	}
	until := attemptAt.Add(cooldown)
	if !now.Before(until) {
		return directAttemptBlockState{}
	}
	return directAttemptBlockState{
		Blocked: true,
		Reason:  cooldownReasonForResult(state.LastDirectAttemptResult),
		Until:   until,
	}
}

func directAttemptSuppressionStateWithPolicy(state api.PeerTransportState, now time.Time, policy directAttemptPolicy) directAttemptBlockState {
	if !transportStateFreshWithPolicy(state, now, policy) {
		return directAttemptBlockState{}
	}
	threshold := policy.suppressAfterForAttempt(state.LastDirectAttemptResult, state.LastDirectAttemptProfile)
	window := policy.suppressWindowForAttempt(state.LastDirectAttemptResult, state.LastDirectAttemptProfile)
	if threshold <= 0 || window <= 0 {
		return directAttemptBlockState{}
	}
	if state.ConsecutiveDirectFailures < threshold {
		return directAttemptBlockState{}
	}
	attemptAt := state.LastDirectAttemptAt.UTC()
	if attemptAt.IsZero() {
		attemptAt = state.ReportedAt.UTC()
	}
	if !state.LastDirectSuccessAt.IsZero() && state.LastDirectSuccessAt.After(attemptAt) {
		return directAttemptBlockState{}
	}
	until := attemptAt.Add(window)
	if !now.Before(until) {
		return directAttemptBlockState{}
	}
	probeLimit := policy.suppressedProbeLimitForAttempt(state.LastDirectAttemptResult, state.LastDirectAttemptProfile)
	extraFailures := state.ConsecutiveDirectFailures - threshold
	if extraFailures < 0 {
		extraFailures = 0
	}
	refillInterval := policy.suppressedProbeRefillIntervalForAttempt(state.LastDirectAttemptResult, state.LastDirectAttemptProfile)
	refilled := 0
	if refillInterval > 0 && extraFailures > 0 {
		refilled = int(now.Sub(attemptAt) / refillInterval)
		if refilled < 0 {
			refilled = 0
		}
		if refilled > extraFailures {
			refilled = extraFailures
		}
	}
	effectiveFailures := extraFailures - refilled
	if effectiveFailures < 0 {
		effectiveFailures = 0
	}
	probeRemaining := 0
	probeLimited := probeLimit > 0
	if probeLimited {
		probeRemaining = probeLimit - effectiveFailures
		if probeRemaining < 0 {
			probeRemaining = 0
		}
	}
	nextProbeAt := time.Time{}
	probeInterval := policy.suppressedProbeIntervalForAttempt(state.LastDirectAttemptResult, state.LastDirectAttemptProfile)
	if probeInterval > 0 && (!probeLimited || probeRemaining > 0) {
		candidate := attemptAt.Add(probeInterval)
		if candidate.Before(until) {
			nextProbeAt = candidate
		}
	}
	probeRefillAt := time.Time{}
	if probeLimited && effectiveFailures > 0 && refillInterval > 0 {
		candidate := attemptAt.Add(time.Duration(refilled+1) * refillInterval)
		if candidate.Before(until) || candidate.Equal(until) {
			probeRefillAt = candidate
		}
	}
	if probeLimited && probeRemaining == 0 && !probeRefillAt.IsZero() {
		if nextProbeAt.IsZero() || probeRefillAt.After(nextProbeAt) {
			nextProbeAt = probeRefillAt
		}
	}
	return directAttemptBlockState{
		Blocked:        true,
		Reason:         suppressionReasonForResult(state.LastDirectAttemptResult),
		Until:          until,
		NextProbeAt:    nextProbeAt,
		ProbeLimited:   probeLimited,
		ProbeBudget:    probeLimit,
		ProbeFailures:  effectiveFailures,
		ProbeRemaining: probeRemaining,
		ProbeRefillAt:  probeRefillAt,
	}
}

func directAttemptBlockStateWithPolicy(state api.PeerTransportState, now time.Time, policy directAttemptPolicy) directAttemptBlockState {
	cooldownState := directAttemptCooldownStateWithPolicy(state, now, policy)
	suppressionState := directAttemptSuppressionStateWithPolicy(state, now, policy)
	return laterBlockState(cooldownState, suppressionState)
}

func applyDirectAttemptTrace(recoveryState api.PeerRecoveryState, attempt directAttemptPair) api.PeerRecoveryState {
	recoveryState.LastIssuedAttemptID = strings.TrimSpace(attempt.AttemptID)
	recoveryState.LastIssuedAttemptReason = strings.TrimSpace(attempt.Reason)
	recoveryState.LastIssuedAttemptProfile = strings.TrimSpace(attempt.Profile)
	recoveryState.LastIssuedAttemptAt = attempt.IssuedAt
	recoveryState.LastIssuedAttemptExecuteAt = attempt.ExecuteAt
	return recoveryState
}

func directAttemptDecisionNextAt(selfState, peerState api.PeerTransportState, now time.Time, block directAttemptBlockState, latestAttempt *directAttemptPair) time.Time {
	if latestAttempt != nil && !now.After(latestAttempt.ExpiresAt) {
		if latestAttempt.ExecuteAt.After(now) {
			return latestAttempt.ExecuteAt
		}
		if latestAttempt.ExpiresAt.After(now) {
			return latestAttempt.ExpiresAt
		}
		return time.Time{}
	}
	if block.Blocked {
		if !block.NextProbeAt.IsZero() {
			return block.NextProbeAt
		}
		if block.Until.After(now) {
			return block.Until
		}
	}
	return time.Time{}
}

func directAttemptDecisionForPeer(selfNode, peerNode api.Node, selfCandidates, peerCandidates []string, selfState, peerState api.PeerTransportState, now time.Time, policy directAttemptPolicy, latestAttempt *directAttemptPair) directAttemptDecision {
	return directAttemptDecisionForPeerCandidates(
		selfNode,
		peerNode,
		api.MigrateLegacyDirectAttemptCandidates(selfCandidates, now, now),
		api.MigrateLegacyDirectAttemptCandidates(peerCandidates, now, now),
		selfState,
		peerState,
		now,
		policy,
		latestAttempt,
	)
}

func directAttemptDecisionForPeerCandidates(selfNode, peerNode api.Node, selfCandidates, peerCandidates []api.DirectAttemptCandidate, selfState, peerState api.PeerTransportState, now time.Time, policy directAttemptPolicy, latestAttempt *directAttemptPair) directAttemptDecision {
	selfCandidateAddresses := api.DirectAttemptCandidateAddresses(selfCandidates)
	peerCandidateAddresses := api.DirectAttemptCandidateAddresses(peerCandidates)
	block := laterBlockState(
		directAttemptBlockStateWithPolicy(selfState, now, policy),
		directAttemptBlockStateWithPolicy(peerState, now, policy),
	)
	nextAt := directAttemptDecisionNextAt(selfState, peerState, now, block, latestAttempt)
	if latestAttempt != nil && !now.After(latestAttempt.ExpiresAt) {
		return directAttemptDecision{
			Status: "attempt_issued",
			Reason: strings.TrimSpace(latestAttempt.Reason),
			At:     latestAttempt.IssuedAt,
			NextAt: nextAt,
		}
	}
	if block.Blocked {
		return directAttemptDecision{
			Status: "blocked",
			Reason: block.Reason,
			At:     now,
			NextAt: nextAt,
		}
	}
	if !isNodeOnlineWithPolicy(selfNode, now, policy) {
		return directAttemptDecision{
			Status: "local_offline",
			Reason: "local_node_offline",
			At:     now,
		}
	}
	if !isNodeOnlineWithPolicy(peerNode, now, policy) {
		return directAttemptDecision{
			Status: "peer_offline",
			Reason: "peer_node_offline",
			At:     now,
		}
	}
	if len(selfCandidateAddresses) == 0 {
		return directAttemptDecision{
			Status: "local_no_direct_candidate",
			Reason: "local_node_has_no_fresh_direct_candidate",
			At:     now,
		}
	}
	if len(peerCandidateAddresses) == 0 {
		return directAttemptDecision{
			Status: "peer_no_direct_candidate",
			Reason: "peer_node_has_no_fresh_direct_candidate",
			At:     now,
		}
	}
	if transportStateFreshWithPolicy(selfState, now, policy) && transportStateFreshWithPolicy(peerState, now, policy) &&
		strings.EqualFold(strings.TrimSpace(selfState.ActiveKind), "direct") &&
		strings.EqualFold(strings.TrimSpace(peerState.ActiveKind), "direct") {
		return directAttemptDecision{
			Status: "direct_active",
			Reason: "both_peers_already_direct",
			At:     now,
		}
	}
	if reason, schedule := directAttemptReasonWithPolicyCandidates(selfCandidates, peerCandidates, selfState, peerState, now, policy); schedule {
		return directAttemptDecision{
			Status: "eligible",
			Reason: reason,
			At:     now,
			NextAt: now,
		}
	}
	if !hasPrimaryDirectAttemptCandidates(selfCandidates) && !hasPrimaryDirectAttemptCandidates(peerCandidates) {
		return directAttemptDecision{
			Status: "idle",
			Reason: "secondary_candidates_only",
			At:     now,
			NextAt: nextAt,
		}
	}
	return directAttemptDecision{
		Status: "idle",
		Reason: "not_scheduled",
		At:     now,
		NextAt: nextAt,
	}
}

func recoveryStateForPeer(selfNode, peerNode api.Node, selfCandidates, peerCandidates []string, selfState, peerState api.PeerTransportState, now time.Time, policy directAttemptPolicy, latestAttempt *directAttemptPair) api.PeerRecoveryState {
	return recoveryStateForPeerCandidates(
		selfNode,
		peerNode,
		api.MigrateLegacyDirectAttemptCandidates(selfCandidates, now, now),
		api.MigrateLegacyDirectAttemptCandidates(peerCandidates, now, now),
		selfState,
		peerState,
		now,
		policy,
		latestAttempt,
	)
}

func recoveryStateForPeerCandidates(selfNode, peerNode api.Node, selfCandidates, peerCandidates []api.DirectAttemptCandidate, selfState, peerState api.PeerTransportState, now time.Time, policy directAttemptPolicy, latestAttempt *directAttemptPair) api.PeerRecoveryState {
	block := laterBlockState(
		directAttemptBlockStateWithPolicy(selfState, now, policy),
		directAttemptBlockStateWithPolicy(peerState, now, policy),
	)
	decision := directAttemptDecisionForPeerCandidates(selfNode, peerNode, selfCandidates, peerCandidates, selfState, peerState, now, policy, latestAttempt)
	recoveryState := api.PeerRecoveryState{
		PeerNodeID:     strings.TrimSpace(peerNode.ID),
		Blocked:        block.Blocked,
		BlockReason:    block.Reason,
		BlockedUntil:   block.Until,
		NextProbeAt:    block.NextProbeAt,
		ProbeLimited:   block.ProbeLimited,
		ProbeBudget:    block.ProbeBudget,
		ProbeFailures:  block.ProbeFailures,
		ProbeRemaining: block.ProbeRemaining,
		ProbeRefillAt:  block.ProbeRefillAt,
		DecisionStatus: decision.Status,
		DecisionReason: decision.Reason,
		DecisionAt:     decision.At,
		DecisionNextAt: decision.NextAt,
	}
	if latestAttempt != nil {
		recoveryState = applyDirectAttemptTrace(recoveryState, *latestAttempt)
	}
	return recoveryState
}

func laterBlockState(left, right directAttemptBlockState) directAttemptBlockState {
	switch {
	case left.Blocked && right.Blocked:
		selected := left
		if right.Until.After(left.Until) {
			selected = right
		} else if right.Until.Equal(left.Until) && right.NextProbeAt.After(left.NextProbeAt) {
			selected = right
		}
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(selected.Reason)), "suppressed_") {
			selected.NextProbeAt = time.Time{}
			selected.ProbeLimited = false
			selected.ProbeBudget = 0
			selected.ProbeFailures = 0
			selected.ProbeRemaining = 0
			selected.ProbeRefillAt = time.Time{}
			return selected
		}
		selected.ProbeLimited = left.ProbeLimited || right.ProbeLimited
		selected.ProbeBudget = mergeProbeBudget(left, right)
		selected.ProbeFailures = maxInt(left.ProbeFailures, right.ProbeFailures)
		selected.ProbeRemaining = mergeProbeRemaining(left, right)
		selected.ProbeRefillAt = mergeProbeRefillAt(left, right, selected.ProbeRemaining)
		if selected.ProbeLimited && selected.ProbeRemaining == 0 {
			selected.NextProbeAt = time.Time{}
			if !selected.ProbeRefillAt.IsZero() {
				selected.NextProbeAt = selected.ProbeRefillAt
			}
			return selected
		}
		switch {
		case left.NextProbeAt.IsZero():
			selected.NextProbeAt = right.NextProbeAt
		case right.NextProbeAt.IsZero():
			selected.NextProbeAt = left.NextProbeAt
		case right.NextProbeAt.After(left.NextProbeAt):
			selected.NextProbeAt = right.NextProbeAt
		default:
			selected.NextProbeAt = left.NextProbeAt
		}
		return selected
	case left.Blocked:
		return left
	case right.Blocked:
		return right
	default:
		return directAttemptBlockState{}
	}
}

func mergeProbeBudget(left, right directAttemptBlockState) int {
	switch {
	case left.ProbeBudget <= 0:
		return right.ProbeBudget
	case right.ProbeBudget <= 0:
		return left.ProbeBudget
	case left.ProbeBudget < right.ProbeBudget:
		return left.ProbeBudget
	default:
		return right.ProbeBudget
	}
}

func mergeProbeRemaining(left, right directAttemptBlockState) int {
	switch {
	case left.ProbeLimited && right.ProbeLimited:
		if left.ProbeRemaining < right.ProbeRemaining {
			return left.ProbeRemaining
		}
		return right.ProbeRemaining
	case left.ProbeLimited:
		return left.ProbeRemaining
	case right.ProbeLimited:
		return right.ProbeRemaining
	default:
		return 0
	}
}

func mergeProbeRefillAt(left, right directAttemptBlockState, remaining int) time.Time {
	switch {
	case remaining <= 0:
		switch {
		case left.ProbeRefillAt.IsZero():
			return right.ProbeRefillAt
		case right.ProbeRefillAt.IsZero():
			return left.ProbeRefillAt
		case right.ProbeRefillAt.After(left.ProbeRefillAt):
			return right.ProbeRefillAt
		default:
			return left.ProbeRefillAt
		}
	default:
		switch {
		case left.ProbeRefillAt.IsZero():
			return right.ProbeRefillAt
		case right.ProbeRefillAt.IsZero():
			return left.ProbeRefillAt
		case right.ProbeRefillAt.Before(left.ProbeRefillAt):
			return right.ProbeRefillAt
		default:
			return left.ProbeRefillAt
		}
	}
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func cooldownReasonForResult(result string) string {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "timeout":
		return "cooldown_timeout"
	case "relay_kept":
		return "cooldown_relay_kept"
	default:
		return ""
	}
}

func suppressionReasonForResult(result string) string {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "timeout":
		return "suppressed_timeout_budget"
	case "relay_kept":
		return "suppressed_relay_kept_budget"
	default:
		return ""
	}
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func newDirectAttemptPair(now time.Time, nodeA api.Node, nodeB api.Node, nodeACandidates, nodeBCandidates []api.DirectAttemptCandidate, reason string, policy directAttemptPolicy, nodeAState, nodeBState api.PeerTransportState) directAttemptPair {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	profileName, profile := policy.profileForAttempt(reason, nodeACandidates, nodeBCandidates, nodeAState, nodeBState, now)
	executeAt := now.Add(profile.Lead)
	window := profile.Window
	burstInterval := profile.BurstInterval
	attemptID := fmt.Sprintf(
		"attempt-%s-%s-%d",
		nodeA.ID,
		nodeB.ID,
		executeAt.UnixMilli(),
	)
	return directAttemptPair{
		AttemptID:       attemptID,
		NodeAID:         nodeA.ID,
		NodeBID:         nodeB.ID,
		NodeACandidates: api.NormalizeDirectAttemptCandidates(nodeACandidates, now),
		NodeBCandidates: api.NormalizeDirectAttemptCandidates(nodeBCandidates, now),
		Profile:         strings.TrimSpace(profileName),
		IssuedAt:        now,
		ExecuteAt:       executeAt,
		Window:          window,
		BurstInterval:   burstInterval,
		Reason:          strings.TrimSpace(reason),
		ExpiresAt:       executeAt.Add(window).Add(policy.DirectAttemptRetention),
	}
}

func sortDirectAttempts(attempts []api.DirectAttemptInstruction) {
	sort.Slice(attempts, func(i, j int) bool {
		if attempts[i].ExecuteAt.Equal(attempts[j].ExecuteAt) {
			if attempts[i].PeerNodeID == attempts[j].PeerNodeID {
				return attempts[i].AttemptID < attempts[j].AttemptID
			}
			return attempts[i].PeerNodeID < attempts[j].PeerNodeID
		}
		return attempts[i].ExecuteAt.Before(attempts[j].ExecuteAt)
	})
}
