package store

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/packages/runtime/go/session"
)

const (
	nodeOnlineWindow         = 30 * time.Second
	endpointFreshnessWindow  = 45 * time.Second
	transportFreshnessWindow = 30 * time.Second
	directAttemptCooldown    = 10 * time.Second
	directAttemptLead        = 150 * time.Millisecond
	directAttemptWindow      = 600 * time.Millisecond
	directAttemptBurst       = 80 * time.Millisecond
	directAttemptRetention   = 2 * time.Second
	maxNATSamples            = 4
)

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
	if strings.ToLower(strings.TrimSpace(node.Status)) != "online" {
		return false
	}
	if node.LastSeenAt.IsZero() {
		return false
	}
	return now.Sub(node.LastSeenAt.UTC()) <= nodeOnlineWindow
}

func freshDirectCandidateAddresses(node api.Node, now time.Time) []string {
	candidates := session.FreshDirectCandidates(now, node.Endpoints, node.EndpointRecords, endpointFreshnessWindow)
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
		if state.ReportedAt.IsZero() {
			state.ReportedAt = now
		} else {
			state.ReportedAt = state.ReportedAt.UTC()
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
	if state.ReportedAt.IsZero() {
		return false
	}
	return now.Sub(state.ReportedAt.UTC()) <= transportFreshnessWindow
}

func directAttemptCoolingDown(state api.PeerTransportState, now time.Time) bool {
	if !transportStateFresh(state, now) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(state.LastDirectAttemptResult)) {
	case "timeout", "relay_kept":
		return now.Sub(state.ReportedAt.UTC()) < directAttemptCooldown
	default:
		return false
	}
}

func directAttemptReason(selfState, peerState api.PeerTransportState, now time.Time) (string, bool) {
	selfFresh := transportStateFresh(selfState, now)
	peerFresh := transportStateFresh(peerState, now)
	if selfFresh && peerFresh && selfState.ActiveKind == "direct" && peerState.ActiveKind == "direct" {
		return "", false
	}
	if directAttemptCoolingDown(selfState, now) || directAttemptCoolingDown(peerState, now) {
		return "", false
	}
	if (selfFresh && selfState.ActiveKind == "relay") || (peerFresh && peerState.ActiveKind == "relay") {
		return "relay_active", true
	}
	return "fresh_endpoints", true
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

func newDirectAttemptPair(now time.Time, nodeA api.Node, nodeB api.Node, nodeACandidates, nodeBCandidates []string, reason string) directAttemptPair {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	executeAt := now.Add(directAttemptLead)
	window := directAttemptWindow
	burstInterval := directAttemptBurst
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
		ExpiresAt:       executeAt.Add(window).Add(directAttemptRetention),
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
