package api

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

const (
	DirectAttemptPhasePrimary   = "primary"
	DirectAttemptPhaseSecondary = "secondary"
)

type DirectAttemptCandidate struct {
	Address    string    `json:"address"`
	Source     string    `json:"source,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitempty"`
	Priority   int       `json:"priority,omitempty"`
	Phase      string    `json:"phase,omitempty"`
}

func DirectAttemptPhaseForSource(source string) string {
	switch normalizeEndpointSource(source) {
	case "stun", "listener":
		return DirectAttemptPhasePrimary
	default:
		return DirectAttemptPhaseSecondary
	}
}

func NormalizeDirectAttemptPhase(phase, source string) string {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case DirectAttemptPhasePrimary, DirectAttemptPhaseSecondary:
		return strings.ToLower(strings.TrimSpace(phase))
	default:
		return DirectAttemptPhaseForSource(source)
	}
}

func NormalizeDirectAttemptCandidates(candidates []DirectAttemptCandidate, fallbackObservedAt time.Time) []DirectAttemptCandidate {
	normalized := make([]DirectAttemptCandidate, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate.Address = strings.TrimSpace(candidate.Address)
		if candidate.Address == "" {
			continue
		}
		if _, ok := seen[candidate.Address]; ok {
			continue
		}
		candidate.Source = normalizeEndpointSource(candidate.Source)
		if candidate.ObservedAt.IsZero() {
			if !fallbackObservedAt.IsZero() {
				candidate.ObservedAt = fallbackObservedAt.UTC()
			}
		} else {
			candidate.ObservedAt = candidate.ObservedAt.UTC()
		}
		candidate.Phase = NormalizeDirectAttemptPhase(candidate.Phase, candidate.Source)
		seen[candidate.Address] = struct{}{}
		normalized = append(normalized, candidate)
	}
	return normalized
}

func SortDirectAttemptCandidates(candidates []DirectAttemptCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if !left.ObservedAt.Equal(right.ObservedAt) {
			return left.ObservedAt.After(right.ObservedAt)
		}
		leftRank := EndpointSourcePriority(left.Source)
		rightRank := EndpointSourcePriority(right.Source)
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		if left.Priority != right.Priority {
			return left.Priority > right.Priority
		}
		return left.Address < right.Address
	})
}

func DirectAttemptCandidateAddresses(candidates []DirectAttemptCandidate) []string {
	addresses := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		address := strings.TrimSpace(candidate.Address)
		if address == "" {
			continue
		}
		addresses = append(addresses, address)
	}
	return addresses
}

func MigrateLegacyDirectAttemptCandidates(addresses []string, issuedAt, executeAt time.Time) []DirectAttemptCandidate {
	fallback := issuedAt.UTC()
	if fallback.IsZero() {
		fallback = executeAt.UTC()
	}
	migrated := make([]DirectAttemptCandidate, 0, len(addresses))
	for idx, address := range addresses {
		address = strings.TrimSpace(address)
		if address == "" {
			continue
		}
		migrated = append(migrated, DirectAttemptCandidate{
			Address:    address,
			Source:     "heartbeat",
			ObservedAt: fallback,
			Priority:   1000 - idx,
			Phase:      DirectAttemptPhasePrimary,
		})
	}
	return NormalizeDirectAttemptCandidates(migrated, fallback)
}

func UnmarshalDirectAttemptCandidatesJSON(raw []byte, issuedAt, executeAt time.Time) ([]DirectAttemptCandidate, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	var structured []DirectAttemptCandidate
	if err := json.Unmarshal(raw, &structured); err == nil {
		fallback := issuedAt.UTC()
		if fallback.IsZero() {
			fallback = executeAt.UTC()
		}
		return NormalizeDirectAttemptCandidates(structured, fallback), nil
	}

	var legacy []string
	if err := json.Unmarshal(raw, &legacy); err == nil {
		return MigrateLegacyDirectAttemptCandidates(legacy, issuedAt, executeAt), nil
	}

	var structuredErr error
	if err := json.Unmarshal(raw, &structured); err != nil {
		structuredErr = err
	}
	var legacyErr error
	if err := json.Unmarshal(raw, &legacy); err != nil {
		legacyErr = err
	}
	if structuredErr != nil {
		return nil, structuredErr
	}
	return nil, legacyErr
}
