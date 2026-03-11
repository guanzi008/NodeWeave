package api

import (
	"sort"
	"strings"
	"time"
)

type EndpointObservation struct {
	Address    string    `json:"address"`
	Source     string    `json:"source,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitempty"`
}

func NormalizeEndpointObservations(now time.Time, endpoints []string, observations []EndpointObservation) ([]EndpointObservation, []string) {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	normalized := make([]EndpointObservation, 0, len(endpoints)+len(observations))
	byAddress := make(map[string]int, len(endpoints)+len(observations))

	add := func(observation EndpointObservation) {
		address := strings.TrimSpace(observation.Address)
		if address == "" {
			return
		}
		observation.Address = address
		observation.Source = normalizeEndpointSource(observation.Source)
		if observation.ObservedAt.IsZero() {
			observation.ObservedAt = now
		} else {
			observation.ObservedAt = observation.ObservedAt.UTC()
		}

		if idx, ok := byAddress[address]; ok {
			current := normalized[idx]
			if observation.ObservedAt.After(current.ObservedAt) {
				normalized[idx] = observation
				return
			}
			if observation.ObservedAt.Equal(current.ObservedAt) && endpointSourceRank(observation.Source) > endpointSourceRank(current.Source) {
				normalized[idx] = observation
			}
			return
		}

		byAddress[address] = len(normalized)
		normalized = append(normalized, observation)
	}

	for _, observation := range observations {
		add(observation)
	}
	for _, endpoint := range endpoints {
		address := strings.TrimSpace(endpoint)
		if address == "" {
			continue
		}
		if _, ok := byAddress[address]; ok {
			continue
		}
		add(EndpointObservation{
			Address:    address,
			Source:     "heartbeat",
			ObservedAt: now,
		})
	}

	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].ObservedAt.Equal(normalized[j].ObservedAt) {
			leftRank := endpointSourceRank(normalized[i].Source)
			rightRank := endpointSourceRank(normalized[j].Source)
			if leftRank == rightRank {
				return normalized[i].Address < normalized[j].Address
			}
			return leftRank > rightRank
		}
		return normalized[i].ObservedAt.After(normalized[j].ObservedAt)
	})

	addresses := make([]string, 0, len(normalized))
	for _, observation := range normalized {
		addresses = append(addresses, observation.Address)
	}

	return normalized, addresses
}

func EndpointObservationsEqual(left, right []EndpointObservation) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if strings.TrimSpace(left[idx].Address) != strings.TrimSpace(right[idx].Address) {
			return false
		}
		if normalizeEndpointSource(left[idx].Source) != normalizeEndpointSource(right[idx].Source) {
			return false
		}
	}
	return true
}

func EndpointSourcePriority(source string) int {
	return endpointSourceRank(source)
}

func endpointSourceRank(source string) int {
	switch normalizeEndpointSource(source) {
	case "stun":
		return 30
	case "static":
		return 25
	case "listener":
		return 20
	case "heartbeat":
		return 10
	default:
		return 0
	}
}

func normalizeEndpointSource(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	switch source {
	case "", "heartbeat":
		return "heartbeat"
	case "stun", "static", "listener":
		return source
	default:
		return source
	}
}
