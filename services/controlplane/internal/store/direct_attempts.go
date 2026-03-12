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
	maxNATSamples = 4
)

type directAttemptPolicy struct {
	NodeOnlineWindow                              time.Duration
	EndpointFreshnessWindow                       time.Duration
	TransportFreshnessWindow                      time.Duration
	DirectAttemptCooldown                         time.Duration
	DirectAttemptTimeoutCooldown                  time.Duration
	DirectAttemptRelayKeptCooldown                time.Duration
	DirectAttemptLead                             time.Duration
	DirectAttemptWindow                           time.Duration
	DirectAttemptBurstInterval                    time.Duration
	DirectAttemptRetention                        time.Duration
	DirectAttemptManualRecoverAfter               time.Duration
	DirectAttemptTimeoutManualRecoverAfter        time.Duration
	DirectAttemptRelayKeptManualRecoverAfter      time.Duration
	DirectAttemptFailureSuppressAfter             int
	DirectAttemptTimeoutSuppressAfter             int
	DirectAttemptRelayKeptSuppressAfter           int
	DirectAttemptFailureSuppressWindow            time.Duration
	DirectAttemptTimeoutSuppressWindow            time.Duration
	DirectAttemptRelayKeptSuppressWindow          time.Duration
	DirectAttemptSuppressedProbeInterval          time.Duration
	DirectAttemptTimeoutSuppressedProbeInterval   time.Duration
	DirectAttemptRelayKeptSuppressedProbeInterval time.Duration
	RelayActiveAttemptLead                        time.Duration
	RelayActiveAttemptWindow                      time.Duration
	RelayActiveAttemptBurstInterval               time.Duration
	ManualRecoverAttemptLead                      time.Duration
	ManualRecoverAttemptWindow                    time.Duration
	ManualRecoverAttemptBurstInterval             time.Duration
}

func directAttemptPolicyFromConfig(cfg config.Config) directAttemptPolicy {
	policy := directAttemptPolicy{
		NodeOnlineWindow:                              cfg.NodeOnlineWindow,
		EndpointFreshnessWindow:                       cfg.EndpointFreshnessWindow,
		TransportFreshnessWindow:                      cfg.TransportFreshnessWindow,
		DirectAttemptCooldown:                         cfg.DirectAttemptCooldown,
		DirectAttemptTimeoutCooldown:                  cfg.DirectAttemptTimeoutCooldown,
		DirectAttemptRelayKeptCooldown:                cfg.DirectAttemptRelayKeptCooldown,
		DirectAttemptLead:                             cfg.DirectAttemptLead,
		DirectAttemptWindow:                           cfg.DirectAttemptWindow,
		DirectAttemptBurstInterval:                    cfg.DirectAttemptBurstInterval,
		DirectAttemptRetention:                        cfg.DirectAttemptRetention,
		DirectAttemptManualRecoverAfter:               cfg.DirectAttemptManualRecoverAfter,
		DirectAttemptTimeoutManualRecoverAfter:        cfg.DirectAttemptTimeoutManualRecoverAfter,
		DirectAttemptRelayKeptManualRecoverAfter:      cfg.DirectAttemptRelayKeptManualRecoverAfter,
		DirectAttemptFailureSuppressAfter:             cfg.DirectAttemptFailureSuppressAfter,
		DirectAttemptTimeoutSuppressAfter:             cfg.DirectAttemptTimeoutSuppressAfter,
		DirectAttemptRelayKeptSuppressAfter:           cfg.DirectAttemptRelayKeptSuppressAfter,
		DirectAttemptFailureSuppressWindow:            cfg.DirectAttemptFailureSuppressWindow,
		DirectAttemptTimeoutSuppressWindow:            cfg.DirectAttemptTimeoutSuppressWindow,
		DirectAttemptRelayKeptSuppressWindow:          cfg.DirectAttemptRelayKeptSuppressWindow,
		DirectAttemptSuppressedProbeInterval:          cfg.DirectAttemptSuppressedProbeInterval,
		DirectAttemptTimeoutSuppressedProbeInterval:   cfg.DirectAttemptTimeoutSuppressedProbeInterval,
		DirectAttemptRelayKeptSuppressedProbeInterval: cfg.DirectAttemptRelayKeptSuppressedProbeInterval,
		RelayActiveAttemptLead:                        cfg.RelayActiveAttemptLead,
		RelayActiveAttemptWindow:                      cfg.RelayActiveAttemptWindow,
		RelayActiveAttemptBurstInterval:               cfg.RelayActiveAttemptBurstInterval,
		ManualRecoverAttemptLead:                      cfg.ManualRecoverAttemptLead,
		ManualRecoverAttemptWindow:                    cfg.ManualRecoverAttemptWindow,
		ManualRecoverAttemptBurstInterval:             cfg.ManualRecoverAttemptBurstInterval,
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
	if policy.RelayActiveAttemptLead <= 0 {
		policy.RelayActiveAttemptLead = policy.DirectAttemptLead
	}
	if policy.RelayActiveAttemptWindow <= 0 {
		policy.RelayActiveAttemptWindow = policy.DirectAttemptWindow
	}
	if policy.RelayActiveAttemptBurstInterval <= 0 {
		policy.RelayActiveAttemptBurstInterval = policy.DirectAttemptBurstInterval
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

type directAttemptPair struct {
	AttemptID       string
	NodeAID         string
	NodeBID         string
	NodeACandidates []string
	NodeBCandidates []string
	ExecuteAt       time.Time
	Window          time.Duration
	BurstInterval   time.Duration
	Reason          string
	ExpiresAt       time.Time
}

type directAttemptBlockState struct {
	Blocked     bool
	Reason      string
	Until       time.Time
	NextProbeAt time.Time
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
		candidates []string
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
		ExecuteAt:     p.ExecuteAt,
		Window:        p.Window.Milliseconds(),
		BurstInterval: p.BurstInterval.Milliseconds(),
		Candidates:    append([]string(nil), candidates...),
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
	candidates := session.FreshDirectCandidates(now, node.Endpoints, node.EndpointRecords, policy.EndpointFreshnessWindow)
	addresses := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.ToLower(strings.TrimSpace(candidate.Kind)) != "direct" {
			continue
		}
		address := strings.TrimSpace(candidate.Address)
		if address == "" {
			continue
		}
		addresses = append(addresses, address)
	}
	return dedupeStrings(addresses)
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
	selfFresh := transportStateFreshWithPolicy(selfState, now, policy)
	peerFresh := transportStateFreshWithPolicy(peerState, now, policy)
	if selfFresh && peerFresh && selfState.ActiveKind == "direct" && peerState.ActiveKind == "direct" {
		return "", false
	}
	cooldownBlock := laterBlockState(
		directAttemptCooldownStateWithPolicy(selfState, now, policy),
		directAttemptCooldownStateWithPolicy(peerState, now, policy),
	)
	if cooldownBlock.Blocked {
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
	return "fresh_endpoints", true
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
	return now.Sub(attemptAt) >= policy.manualRecoverAfterForResult(state.LastDirectAttemptResult)
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

func directAttemptCooldownStateWithPolicy(state api.PeerTransportState, now time.Time, policy directAttemptPolicy) directAttemptBlockState {
	if !transportStateFreshWithPolicy(state, now, policy) {
		return directAttemptBlockState{}
	}
	cooldown := policy.cooldownForResult(state.LastDirectAttemptResult)
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
	threshold := policy.suppressAfterForResult(state.LastDirectAttemptResult)
	window := policy.suppressWindowForResult(state.LastDirectAttemptResult)
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
	nextProbeAt := time.Time{}
	probeInterval := policy.suppressedProbeIntervalForResult(state.LastDirectAttemptResult)
	if probeInterval > 0 {
		candidate := attemptAt.Add(probeInterval)
		if candidate.Before(until) {
			nextProbeAt = candidate
		}
	}
	return directAttemptBlockState{
		Blocked:     true,
		Reason:      suppressionReasonForResult(state.LastDirectAttemptResult),
		Until:       until,
		NextProbeAt: nextProbeAt,
	}
}

func directAttemptBlockStateWithPolicy(state api.PeerTransportState, now time.Time, policy directAttemptPolicy) directAttemptBlockState {
	cooldownState := directAttemptCooldownStateWithPolicy(state, now, policy)
	suppressionState := directAttemptSuppressionStateWithPolicy(state, now, policy)
	return laterBlockState(cooldownState, suppressionState)
}

func recoveryStateForPeer(peerNodeID string, selfState, peerState api.PeerTransportState, now time.Time, policy directAttemptPolicy) api.PeerRecoveryState {
	block := laterBlockState(
		directAttemptBlockStateWithPolicy(selfState, now, policy),
		directAttemptBlockStateWithPolicy(peerState, now, policy),
	)
	return api.PeerRecoveryState{
		PeerNodeID:   strings.TrimSpace(peerNodeID),
		Blocked:      block.Blocked,
		BlockReason:  block.Reason,
		BlockedUntil: block.Until,
		NextProbeAt:  block.NextProbeAt,
	}
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

func newDirectAttemptPair(now time.Time, nodeA api.Node, nodeB api.Node, nodeACandidates, nodeBCandidates []string, reason string, policy directAttemptPolicy) directAttemptPair {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	profile := policy.profileForReason(reason)
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
		NodeACandidates: dedupeStrings(nodeACandidates),
		NodeBCandidates: dedupeStrings(nodeBCandidates),
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
