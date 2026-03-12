package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/packages/runtime/go/overlay"
)

type Config struct {
	ListenAddress string
}

type Candidate struct {
	Kind       string    `json:"kind"`
	Address    string    `json:"address"`
	Region     string    `json:"region,omitempty"`
	Source     string    `json:"source,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitempty"`
	Priority   int       `json:"priority"`
}

type Peer struct {
	NodeID             string      `json:"node_id"`
	OverlayIP          string      `json:"overlay_ip"`
	PublicKey          string      `json:"public_key"`
	PreferredCandidate string      `json:"preferred_candidate,omitempty"`
	Candidates         []Candidate `json:"candidates"`
}

type Spec struct {
	GeneratedAt   time.Time `json:"generated_at"`
	NodeID        string    `json:"node_id"`
	ListenAddress string    `json:"listen_address,omitempty"`
	Peers         []Peer    `json:"peers"`
}

type ProbeConfig struct {
	Mode    string
	Timeout time.Duration
}

type CandidateResult struct {
	Kind        string `json:"kind"`
	Address     string `json:"address"`
	Region      string `json:"region,omitempty"`
	Priority    int    `json:"priority"`
	Status      string `json:"status"`
	RTTMillis   int64  `json:"rtt_millis,omitempty"`
	Error       string `json:"error,omitempty"`
	RespondedBy string `json:"responded_by,omitempty"`
}

type PeerReport struct {
	NodeID             string            `json:"node_id"`
	PreferredCandidate string            `json:"preferred_candidate,omitempty"`
	SelectedCandidate  string            `json:"selected_candidate,omitempty"`
	Reachable          bool              `json:"reachable"`
	Candidates         []CandidateResult `json:"candidates"`
}

type Report struct {
	GeneratedAt   time.Time    `json:"generated_at"`
	NodeID        string       `json:"node_id"`
	ListenAddress string       `json:"listen_address,omitempty"`
	Mode          string       `json:"mode"`
	Peers         []PeerReport `json:"peers"`
}

type probeMessage struct {
	Type       string    `json:"type"`
	Nonce      string    `json:"nonce"`
	SourceNode string    `json:"source_node"`
	TargetNode string    `json:"target_node,omitempty"`
	SentAt     time.Time `json:"sent_at"`
}

type Responder struct {
	conn   net.PacketConn
	nodeID string
}

func Build(snapshot overlay.Snapshot, cfg Config) Spec {
	peers := make([]Peer, 0, len(snapshot.Peers))
	for _, peer := range snapshot.Peers {
		candidates := buildDirectCandidates(peer)
		for idx, relay := range snapshot.Relays {
			priority := 400 - idx
			if relay.Region != "" && relay.Region == peer.RelayRegion {
				priority = 500 - idx
			}
			candidates = append(candidates, Candidate{
				Kind:     "relay",
				Address:  relay.Address,
				Region:   relay.Region,
				Priority: priority,
			})
		}

		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].Priority == candidates[j].Priority {
				if candidates[i].Kind == candidates[j].Kind {
					return candidates[i].Address < candidates[j].Address
				}
				return candidates[i].Kind < candidates[j].Kind
			}
			return candidates[i].Priority > candidates[j].Priority
		})

		peerSpec := Peer{
			NodeID:     peer.NodeID,
			OverlayIP:  peer.OverlayIP,
			PublicKey:  peer.PublicKey,
			Candidates: candidates,
		}
		if len(candidates) > 0 {
			peerSpec.PreferredCandidate = candidates[0].Address
		}
		peers = append(peers, peerSpec)
	}

	sort.Slice(peers, func(i, j int) bool {
		return peers[i].NodeID < peers[j].NodeID
	})

	return Spec{
		GeneratedAt:   time.Now().UTC(),
		NodeID:        snapshot.NodeID,
		ListenAddress: strings.TrimSpace(cfg.ListenAddress),
		Peers:         peers,
	}
}

func DirectCandidates(endpoints []string, observations []api.EndpointObservation) []Candidate {
	records, fallbackEndpoints := api.NormalizeEndpointObservations(time.Now().UTC(), endpoints, observations)
	candidates := make([]Candidate, 0, len(records))
	for idx, record := range records {
		candidates = append(candidates, Candidate{
			Kind:       "direct",
			Address:    record.Address,
			Source:     record.Source,
			ObservedAt: record.ObservedAt,
			Priority:   1000 - idx,
		})
	}
	if len(candidates) > 0 {
		return candidates
	}

	candidates = make([]Candidate, 0, len(fallbackEndpoints))
	for idx, endpoint := range fallbackEndpoints {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			continue
		}
		candidates = append(candidates, Candidate{
			Kind:       "direct",
			Address:    endpoint,
			Source:     "heartbeat",
			ObservedAt: time.Now().UTC(),
			Priority:   1000 - idx,
		})
	}
	return candidates
}

func FreshDirectCandidates(now time.Time, endpoints []string, observations []api.EndpointObservation, maxAge time.Duration) []Candidate {
	if maxAge <= 0 {
		return DirectCandidates(endpoints, observations)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cutoff := now.Add(-maxAge)
	filtered := make([]api.EndpointObservation, 0, len(observations))
	for _, observation := range observations {
		if observation.ObservedAt.IsZero() || !observation.ObservedAt.Before(cutoff) {
			filtered = append(filtered, observation)
		}
	}
	return DirectCandidates(endpoints, filtered)
}

func buildDirectCandidates(peer overlay.PeerState) []Candidate {
	return DirectCandidates(peer.Endpoints, peer.EndpointRecords)
}

func Probe(ctx context.Context, spec Spec, cfg ProbeConfig) (Report, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "off"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 1500 * time.Millisecond
	}
	if mode != "off" && mode != "udp" {
		return Report{}, fmt.Errorf("unsupported probe mode %q", cfg.Mode)
	}

	report := Report{
		GeneratedAt:   time.Now().UTC(),
		NodeID:        spec.NodeID,
		ListenAddress: spec.ListenAddress,
		Mode:          mode,
		Peers:         make([]PeerReport, 0, len(spec.Peers)),
	}

	for _, peer := range spec.Peers {
		peerReport := PeerReport{
			NodeID:             peer.NodeID,
			PreferredCandidate: peer.PreferredCandidate,
			Candidates:         make([]CandidateResult, 0, len(peer.Candidates)),
		}

		for _, candidate := range peer.Candidates {
			result := CandidateResult{
				Kind:     candidate.Kind,
				Address:  candidate.Address,
				Region:   candidate.Region,
				Priority: candidate.Priority,
			}

			switch mode {
			case "off":
				result.Status = "disabled"
			case "udp":
				rtt, respondedBy, err := probeUDP(ctx, spec.NodeID, peer.NodeID, candidate.Address, cfg.Timeout)
				if err != nil {
					if errors.Is(err, context.DeadlineExceeded) {
						result.Status = "timeout"
					} else {
						result.Status = "error"
					}
					result.Error = err.Error()
				} else {
					result.Status = "reachable"
					result.RTTMillis = rtt.Milliseconds()
					result.RespondedBy = respondedBy
					if !peerReport.Reachable {
						peerReport.Reachable = true
						peerReport.SelectedCandidate = candidate.Address
					}
				}
			}

			peerReport.Candidates = append(peerReport.Candidates, result)
		}

		if mode == "off" {
			peerReport.SelectedCandidate = peer.PreferredCandidate
		}

		report.Peers = append(report.Peers, peerReport)
	}

	return report, nil
}

func NewResponder(listenAddress, nodeID string) (*Responder, error) {
	listenAddress = strings.TrimSpace(listenAddress)
	if listenAddress == "" {
		return nil, errors.New("listen address is required")
	}
	conn, err := net.ListenPacket("udp", listenAddress)
	if err != nil {
		return nil, fmt.Errorf("listen udp responder: %w", err)
	}
	return &Responder{
		conn:   conn,
		nodeID: nodeID,
	}, nil
}

func (r *Responder) Address() string {
	if r == nil || r.conn == nil {
		return ""
	}
	return r.conn.LocalAddr().String()
}

func (r *Responder) Close() error {
	if r == nil || r.conn == nil {
		return nil
	}
	return r.conn.Close()
}

func (r *Responder) Serve(ctx context.Context) error {
	if r == nil || r.conn == nil {
		return errors.New("responder is not initialized")
	}

	buffer := make([]byte, 2048)
	for {
		if err := r.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			return fmt.Errorf("set responder deadline: %w", err)
		}

		n, addr, err := r.conn.ReadFrom(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if ctx.Err() != nil {
					return nil
				}
				continue
			}
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("read udp probe: %w", err)
		}

		var probe probeMessage
		if err := json.Unmarshal(buffer[:n], &probe); err != nil {
			continue
		}
		if probe.Type != "probe" || probe.Nonce == "" {
			continue
		}

		ack := probeMessage{
			Type:       "ack",
			Nonce:      probe.Nonce,
			SourceNode: r.nodeID,
			TargetNode: probe.SourceNode,
			SentAt:     time.Now().UTC(),
		}
		raw, err := json.Marshal(ack)
		if err != nil {
			return fmt.Errorf("marshal udp ack: %w", err)
		}
		if _, err := r.conn.WriteTo(raw, addr); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("write udp ack: %w", err)
		}
	}
}

func probeUDP(ctx context.Context, sourceNodeID, targetNodeID, address string, timeout time.Duration) (time.Duration, string, error) {
	remoteAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return 0, "", fmt.Errorf("resolve udp address: %w", err)
	}

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return 0, "", fmt.Errorf("listen udp probe socket: %w", err)
	}
	defer conn.Close()

	nonce, err := randomNonce(8)
	if err != nil {
		return 0, "", err
	}

	payload, err := json.Marshal(probeMessage{
		Type:       "probe",
		Nonce:      nonce,
		SourceNode: sourceNodeID,
		TargetNode: targetNodeID,
		SentAt:     time.Now().UTC(),
	})
	if err != nil {
		return 0, "", fmt.Errorf("marshal udp probe: %w", err)
	}

	startedAt := time.Now()
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0, "", context.DeadlineExceeded
		}
		if remaining < timeout {
			timeout = remaining
		}
	}
	if err := conn.SetDeadline(startedAt.Add(timeout)); err != nil {
		return 0, "", fmt.Errorf("set udp probe deadline: %w", err)
	}
	if _, err := conn.WriteToUDP(payload, remoteAddr); err != nil {
		return 0, "", fmt.Errorf("write udp probe: %w", err)
	}

	buffer := make([]byte, 2048)
	for {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if ctx.Err() != nil {
					return 0, "", ctx.Err()
				}
				return 0, "", context.DeadlineExceeded
			}
			if ctx.Err() != nil {
				return 0, "", ctx.Err()
			}
			return 0, "", fmt.Errorf("read udp probe response: %w", err)
		}

		var response probeMessage
		if err := json.Unmarshal(buffer[:n], &response); err != nil {
			continue
		}
		if response.Type != "ack" || response.Nonce != nonce {
			continue
		}
		return time.Since(startedAt), response.SourceNode, nil
	}
}

func randomNonce(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate probe nonce: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
