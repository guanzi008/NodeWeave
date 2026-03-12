package secureudp

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/packages/runtime/go/dataplane"
	"nodeweave/packages/runtime/go/session"
	"nodeweave/packages/runtime/go/stun"
)

const (
	envelopeTypeAnnounce = "announce"
	envelopeTypeHello    = "hello"
	envelopeTypeHelloAck = "hello_ack"
	envelopeTypeData     = "data"

	directAttemptCandidateSpacing = 10 * time.Millisecond
)

type Config struct {
	NodeID                 string
	ListenAddress          string
	PrivateKey             string
	Peers                  []session.Peer
	RelayAddresses         []string
	HandshakeTimeout       time.Duration
	HandshakeRetryInterval time.Duration
	DirectRetryAfter       time.Duration
	ReplayTTL              time.Duration
}

type Metadata struct {
	Type         string
	SourceNodeID string
	TargetNodeID string
	SentAt       time.Time
}

type Report struct {
	GeneratedAt      time.Time    `json:"generated_at"`
	NodeID           string       `json:"node_id"`
	ListenAddress    string       `json:"listen_address"`
	DirectRetryAfter string       `json:"direct_retry_after"`
	Peers            []PeerStatus `json:"peers"`
}

type PeerStatus struct {
	NodeID                          string            `json:"node_id"`
	ActiveAddress                   string            `json:"active_address,omitempty"`
	ActiveKind                      string            `json:"active_kind,omitempty"`
	ActiveSince                     time.Time         `json:"active_since,omitempty"`
	LastPathChangeAt                time.Time         `json:"last_path_change_at,omitempty"`
	LastDirectTryAt                 time.Time         `json:"last_direct_try_at,omitempty"`
	NextDirectRetryAt               time.Time         `json:"next_direct_retry_at,omitempty"`
	LastEstablishedAt               time.Time         `json:"last_established_at,omitempty"`
	LastSendSuccessAt               time.Time         `json:"last_send_success_at,omitempty"`
	LastReceiveAt                   time.Time         `json:"last_receive_at,omitempty"`
	LastSendErrorAt                 time.Time         `json:"last_send_error_at,omitempty"`
	LastSendError                   string            `json:"last_send_error,omitempty"`
	LastDirectAttemptID             string            `json:"last_direct_attempt_id,omitempty"`
	LastDirectAttemptReason         string            `json:"last_direct_attempt_reason,omitempty"`
	LastDirectAttemptAt             time.Time         `json:"last_direct_attempt_at,omitempty"`
	LastDirectAttemptResult         string            `json:"last_direct_attempt_result,omitempty"`
	LastDirectAttemptReachedSource  string            `json:"last_direct_attempt_reached_source,omitempty"`
	LastDirectAttemptPhase          string            `json:"last_direct_attempt_phase,omitempty"`
	LastDirectAttemptCandidateCount int               `json:"last_direct_attempt_candidate_count,omitempty"`
	LastDirectSuccessAt             time.Time         `json:"last_direct_success_at,omitempty"`
	ConsecutiveDirectFailures       int               `json:"consecutive_direct_failures,omitempty"`
	SessionsEstablished             int               `json:"sessions_established,omitempty"`
	HandshakeTimeouts               int               `json:"handshake_timeouts,omitempty"`
	RelayFallbacks                  int               `json:"relay_fallbacks,omitempty"`
	DirectRecoveries                int               `json:"direct_recoveries,omitempty"`
	SentPackets                     int               `json:"sent_packets,omitempty"`
	SentBytes                       int64             `json:"sent_bytes,omitempty"`
	ReceivedPackets                 int               `json:"received_packets,omitempty"`
	ReceivedBytes                   int64             `json:"received_bytes,omitempty"`
	Candidates                      []CandidateStatus `json:"candidates"`
}

type CandidateStatus struct {
	Kind          string    `json:"kind"`
	Address       string    `json:"address"`
	Priority      int       `json:"priority,omitempty"`
	Active        bool      `json:"active,omitempty"`
	EstablishedAt time.Time `json:"established_at,omitempty"`
}

type WarmupReport struct {
	PeerNodeID string         `json:"peer_node_id"`
	Reachable  bool           `json:"reachable"`
	Results    []WarmupResult `json:"results"`
}

type WarmupResult struct {
	Address string `json:"address"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

type DirectAttempt struct {
	AttemptID     string
	PeerNodeID    string
	Candidates    []api.DirectAttemptCandidate
	ExecuteAt     time.Time
	Window        time.Duration
	BurstInterval time.Duration
	Reason        string
}

type DirectAttemptResult struct {
	AttemptID      string    `json:"attempt_id"`
	PeerNodeID     string    `json:"peer_node_id"`
	StartedAt      time.Time `json:"started_at"`
	CompletedAt    time.Time `json:"completed_at"`
	ReachedAddress string    `json:"reached_address,omitempty"`
	ReachedSource  string    `json:"reached_source,omitempty"`
	Phase          string    `json:"phase,omitempty"`
	ActiveAddress  string    `json:"active_address,omitempty"`
	Result         string    `json:"result"`
	Error          string    `json:"error,omitempty"`
}

type Transport struct {
	conn                   *net.UDPConn
	nodeID                 string
	privateKey             *ecdh.PrivateKey
	peerPublicKeys         map[string]*ecdh.PublicKey
	peerCandidates         map[string][]session.Candidate
	relayAddresses         map[string]struct{}
	handshakeTimeout       time.Duration
	handshakeRetryInterval time.Duration
	directRetryAfter       time.Duration
	replayTTL              time.Duration

	writeMu               sync.Mutex
	stunMu                sync.Mutex
	stateMu               sync.Mutex
	activePeer            map[string]string
	lastDirectTry         map[string]time.Time
	established           map[string]time.Time
	peerStats             map[string]peerMetrics
	waiters               map[string]chan string
	stunWaiters           map[string]chan stun.BindingResponse
	seenEnvelope          map[string]time.Time
	directAttemptInFlight map[string]string
}

type peerMetrics struct {
	ActiveSince                     time.Time
	LastPathChangeAt                time.Time
	LastSendSuccessAt               time.Time
	LastReceiveAt                   time.Time
	LastSendErrorAt                 time.Time
	LastSendError                   string
	LastDirectAttemptID             string
	LastDirectAttemptReason         string
	LastDirectAttemptAt             time.Time
	LastDirectAttemptResult         string
	LastDirectAttemptReachedSource  string
	LastDirectAttemptPhase          string
	LastDirectAttemptCandidateCount int
	LastDirectSuccessAt             time.Time
	ConsecutiveDirectFailures       int
	SessionsEstablished             int
	HandshakeTimeouts               int
	RelayFallbacks                  int
	DirectRecoveries                int
	SentPackets                     int
	SentBytes                       int64
	ReceivedPackets                 int
	ReceivedBytes                   int64
}

type envelope struct {
	Version      int       `json:"version"`
	Type         string    `json:"type"`
	SourceNodeID string    `json:"source_node_id"`
	TargetNodeID string    `json:"target_node_id"`
	Nonce        []byte    `json:"nonce"`
	SentAt       time.Time `json:"sent_at"`
	Payload      []byte    `json:"payload"`
}

type helloPayload struct {
	Challenge string `json:"challenge"`
}

func GenerateKeyPair() (privateKeyHex string, publicKeyHex string, err error) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate x25519 key: %w", err)
	}
	return hex.EncodeToString(privateKey.Bytes()), hex.EncodeToString(privateKey.PublicKey().Bytes()), nil
}

func PublicKeyFromPrivateHex(privateKeyHex string) (string, error) {
	privateKey, err := parsePrivateKey(privateKeyHex)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(privateKey.PublicKey().Bytes()), nil
}

func Listen(config Config) (*Transport, error) {
	if strings.TrimSpace(config.NodeID) == "" {
		return nil, errors.New("node id is required")
	}
	if strings.TrimSpace(config.ListenAddress) == "" {
		return nil, errors.New("listen address is required")
	}

	privateKey, err := parsePrivateKey(config.PrivateKey)
	if err != nil {
		return nil, err
	}

	udpAddress, err := net.ResolveUDPAddr("udp", config.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("resolve secure udp listen address: %w", err)
	}
	conn, err := net.ListenUDP("udp", udpAddress)
	if err != nil {
		return nil, fmt.Errorf("listen secure udp: %w", err)
	}

	if config.HandshakeTimeout <= 0 {
		config.HandshakeTimeout = 1500 * time.Millisecond
	}
	if config.HandshakeRetryInterval <= 0 {
		config.HandshakeRetryInterval = 120 * time.Millisecond
	}
	if config.HandshakeRetryInterval > config.HandshakeTimeout {
		config.HandshakeRetryInterval = config.HandshakeTimeout
	}
	if config.DirectRetryAfter <= 0 {
		config.DirectRetryAfter = 15 * time.Second
	}
	if config.ReplayTTL <= 0 {
		config.ReplayTTL = 10 * time.Minute
	}

	peerPublicKeys := make(map[string]*ecdh.PublicKey, len(config.Peers))
	peerCandidates := make(map[string][]session.Candidate, len(config.Peers))
	for _, peer := range config.Peers {
		if strings.TrimSpace(peer.NodeID) == "" || strings.TrimSpace(peer.PublicKey) == "" {
			continue
		}
		publicKey, err := parsePublicKey(peer.PublicKey)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("parse peer public key %s: %w", peer.NodeID, err)
		}
		peerPublicKeys[peer.NodeID] = publicKey
		candidates := make([]session.Candidate, 0, len(peer.Candidates))
		for _, candidate := range peer.Candidates {
			candidate.Address = strings.TrimSpace(candidate.Address)
			if candidate.Address == "" {
				continue
			}
			candidates = append(candidates, candidate)
		}
		peerCandidates[peer.NodeID] = candidates
	}
	relayAddresses := make(map[string]struct{}, len(config.RelayAddresses))
	for _, address := range config.RelayAddresses {
		address = strings.TrimSpace(address)
		if address == "" {
			continue
		}
		relayAddresses[address] = struct{}{}
	}

	return &Transport{
		conn:                   conn,
		nodeID:                 config.NodeID,
		privateKey:             privateKey,
		peerPublicKeys:         peerPublicKeys,
		peerCandidates:         peerCandidates,
		relayAddresses:         relayAddresses,
		handshakeTimeout:       config.HandshakeTimeout,
		handshakeRetryInterval: config.HandshakeRetryInterval,
		directRetryAfter:       config.DirectRetryAfter,
		replayTTL:              config.ReplayTTL,
		activePeer:             map[string]string{},
		lastDirectTry:          map[string]time.Time{},
		established:            map[string]time.Time{},
		peerStats:              map[string]peerMetrics{},
		waiters:                map[string]chan string{},
		stunWaiters:            map[string]chan stun.BindingResponse{},
		seenEnvelope:           map[string]time.Time{},
		directAttemptInFlight:  map[string]string{},
	}, nil
}

func InspectPacket(raw []byte) (Metadata, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Metadata{}, fmt.Errorf("parse secure udp packet: %w", err)
	}
	return Metadata{
		Type:         env.Type,
		SourceNodeID: env.SourceNodeID,
		TargetNodeID: env.TargetNodeID,
		SentAt:       env.SentAt,
	}, nil
}

func (t *Transport) Address() string {
	if t == nil || t.conn == nil {
		return ""
	}
	return t.conn.LocalAddr().String()
}

func (t *Transport) Snapshot() Report {
	report := Report{
		GeneratedAt: time.Now().UTC(),
	}
	if t == nil {
		return report
	}

	report.NodeID = t.nodeID
	report.ListenAddress = t.Address()
	report.DirectRetryAfter = t.directRetryAfter.String()

	t.stateMu.Lock()
	activePeer := make(map[string]string, len(t.activePeer))
	for nodeID, address := range t.activePeer {
		activePeer[nodeID] = address
	}
	lastDirectTry := make(map[string]time.Time, len(t.lastDirectTry))
	for nodeID, lastTry := range t.lastDirectTry {
		lastDirectTry[nodeID] = lastTry
	}
	established := make(map[string]time.Time, len(t.established))
	for key, establishedAt := range t.established {
		established[key] = establishedAt
	}
	peerStats := make(map[string]peerMetrics, len(t.peerStats))
	for nodeID, stats := range t.peerStats {
		peerStats[nodeID] = stats
	}
	t.stateMu.Unlock()

	peerNodeIDs := make(map[string]struct{}, len(t.peerCandidates)+len(activePeer)+len(lastDirectTry)+len(peerStats))
	for nodeID := range t.peerCandidates {
		peerNodeIDs[nodeID] = struct{}{}
	}
	for nodeID := range activePeer {
		peerNodeIDs[nodeID] = struct{}{}
	}
	for nodeID := range lastDirectTry {
		peerNodeIDs[nodeID] = struct{}{}
	}
	for nodeID := range peerStats {
		peerNodeIDs[nodeID] = struct{}{}
	}
	for key := range established {
		peerNodeID, _ := splitEstablishedKey(key)
		if peerNodeID == "" {
			continue
		}
		peerNodeIDs[peerNodeID] = struct{}{}
	}

	peerIDs := make([]string, 0, len(peerNodeIDs))
	for nodeID := range peerNodeIDs {
		peerIDs = append(peerIDs, nodeID)
	}
	sort.Strings(peerIDs)

	for _, peerNodeID := range peerIDs {
		stats := peerStats[peerNodeID]
		peerStatus := PeerStatus{
			NodeID:                          peerNodeID,
			ActiveAddress:                   strings.TrimSpace(activePeer[peerNodeID]),
			ActiveKind:                      t.candidateKindForPeer(peerNodeID, activePeer[peerNodeID]),
			ActiveSince:                     stats.ActiveSince,
			LastPathChangeAt:                stats.LastPathChangeAt,
			LastDirectTryAt:                 lastDirectTry[peerNodeID],
			LastSendSuccessAt:               stats.LastSendSuccessAt,
			LastReceiveAt:                   stats.LastReceiveAt,
			LastSendErrorAt:                 stats.LastSendErrorAt,
			LastSendError:                   stats.LastSendError,
			LastDirectAttemptID:             stats.LastDirectAttemptID,
			LastDirectAttemptReason:         stats.LastDirectAttemptReason,
			LastDirectAttemptAt:             stats.LastDirectAttemptAt,
			LastDirectAttemptResult:         stats.LastDirectAttemptResult,
			LastDirectAttemptReachedSource:  stats.LastDirectAttemptReachedSource,
			LastDirectAttemptPhase:          stats.LastDirectAttemptPhase,
			LastDirectAttemptCandidateCount: stats.LastDirectAttemptCandidateCount,
			LastDirectSuccessAt:             stats.LastDirectSuccessAt,
			ConsecutiveDirectFailures:       stats.ConsecutiveDirectFailures,
			SessionsEstablished:             stats.SessionsEstablished,
			HandshakeTimeouts:               stats.HandshakeTimeouts,
			RelayFallbacks:                  stats.RelayFallbacks,
			DirectRecoveries:                stats.DirectRecoveries,
			SentPackets:                     stats.SentPackets,
			SentBytes:                       stats.SentBytes,
			ReceivedPackets:                 stats.ReceivedPackets,
			ReceivedBytes:                   stats.ReceivedBytes,
			Candidates:                      []CandidateStatus{},
		}
		if peerStatus.ActiveAddress == "" {
			peerStatus.ActiveKind = ""
		}

		candidatesByAddress := make(map[string]*CandidateStatus, len(t.peerCandidates[peerNodeID])+1)
		candidateOrder := make([]string, 0, len(t.peerCandidates[peerNodeID])+1)
		appendCandidate := func(kind, address string, priority int) {
			address = strings.TrimSpace(address)
			if address == "" {
				return
			}
			existing := candidatesByAddress[address]
			if existing == nil {
				existing = &CandidateStatus{
					Kind:     kind,
					Address:  address,
					Priority: priority,
				}
				candidatesByAddress[address] = existing
				candidateOrder = append(candidateOrder, address)
				return
			}
			if existing.Kind == "" || existing.Kind == "observed" {
				existing.Kind = kind
			}
			if existing.Priority == 0 {
				existing.Priority = priority
			}
		}

		for _, candidate := range t.peerCandidates[peerNodeID] {
			appendCandidate(strings.ToLower(strings.TrimSpace(candidate.Kind)), candidate.Address, candidate.Priority)
		}
		appendCandidate(t.candidateKindForPeer(peerNodeID, peerStatus.ActiveAddress), peerStatus.ActiveAddress, 0)

		for key, establishedAt := range established {
			establishedPeerNodeID, address := splitEstablishedKey(key)
			if establishedPeerNodeID != peerNodeID {
				continue
			}
			appendCandidate(t.candidateKindForPeer(peerNodeID, address), address, 0)
			candidate := candidatesByAddress[strings.TrimSpace(address)]
			candidate.EstablishedAt = establishedAt
			if establishedAt.After(peerStatus.LastEstablishedAt) {
				peerStatus.LastEstablishedAt = establishedAt
			}
		}

		if peerStatus.ActiveAddress != "" {
			if candidate := candidatesByAddress[peerStatus.ActiveAddress]; candidate != nil {
				candidate.Active = true
			}
		}
		if peerStatus.ActiveKind != "direct" && t.hasDirectCandidate(peerNodeID) {
			if peerStatus.LastDirectTryAt.IsZero() {
				peerStatus.NextDirectRetryAt = report.GeneratedAt
			} else {
				nextRetryAt := peerStatus.LastDirectTryAt.Add(t.directRetryAfter)
				if nextRetryAt.Before(report.GeneratedAt) {
					nextRetryAt = report.GeneratedAt
				}
				peerStatus.NextDirectRetryAt = nextRetryAt
			}
		}

		for _, address := range candidateOrder {
			candidate := *candidatesByAddress[address]
			if candidate.Kind == "" {
				candidate.Kind = "observed"
			}
			peerStatus.Candidates = append(peerStatus.Candidates, candidate)
		}

		report.Peers = append(report.Peers, peerStatus)
	}

	return report
}

func (t *Transport) Close() error {
	if t == nil || t.conn == nil {
		return nil
	}
	return t.conn.Close()
}

func (t *Transport) Announce(ctx context.Context, address string) error {
	if t == nil || t.conn == nil {
		return errors.New("secure udp transport is not initialized")
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return errors.New("relay address is required")
	}
	env := envelope{
		Version:      1,
		Type:         envelopeTypeAnnounce,
		SourceNodeID: t.nodeID,
		SentAt:       time.Now().UTC(),
	}
	return t.writeEnvelope(ctx, address, env)
}

func (t *Transport) DiscoverSTUN(ctx context.Context, servers []string, timeout time.Duration) (stun.Report, error) {
	if t == nil || t.conn == nil {
		return stun.Report{}, errors.New("secure udp transport is not initialized")
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	report := stun.Report{
		GeneratedAt: time.Now().UTC(),
		Servers:     make([]stun.Result, 0, len(servers)),
	}

	normalizedServers := make([]string, 0, len(servers))
	for _, server := range servers {
		server = strings.TrimSpace(server)
		if server == "" {
			continue
		}
		normalizedServers = append(normalizedServers, server)
	}
	if len(normalizedServers) == 0 {
		return report, nil
	}

	var errs []string
	for _, server := range normalizedServers {
		result := stun.Result{Server: server}
		reflexiveAddress, rtt, err := t.probeSTUNServer(ctx, server, timeout)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				result.Status = "timeout"
			} else {
				result.Status = "error"
			}
			result.Error = err.Error()
			errs = append(errs, fmt.Sprintf("%s: %v", server, err))
		} else {
			result.Status = "reachable"
			result.RTTMillis = rtt.Milliseconds()
			result.ReflexiveAddress = reflexiveAddress
			if !report.Reachable {
				report.Reachable = true
				report.SelectedAddress = reflexiveAddress
			}
		}
		report.Servers = append(report.Servers, result)
	}

	report = stun.FinalizeReport(report)
	if report.Reachable {
		return report, nil
	}
	if len(errs) == 0 {
		return report, errors.New("no stun server configured")
	}
	return report, fmt.Errorf("stun discovery failed: %s", strings.Join(errs, "; "))
}

func (t *Transport) Send(ctx context.Context, address string, frame dataplane.Frame) error {
	if t == nil || t.conn == nil {
		return errors.New("secure udp transport is not initialized")
	}
	if frame.TargetNodeID == "" {
		return errors.New("target node id is required for secure transport")
	}
	attempts := t.sendAddresses(frame.TargetNodeID, address)
	if len(attempts) == 0 {
		return errors.New("no candidate address is available for secure transport")
	}

	errorsByAddress := make([]string, 0, len(attempts))
	directBurst := t.leadingDirectAddresses(frame.TargetNodeID, attempts)
	if len(directBurst) > 1 {
		reachedAddress, err := t.ensureAnySession(ctx, frame.TargetNodeID, directBurst)
		if err != nil {
			t.recordSendError(frame.TargetNodeID, err)
			errorsByAddress = append(errorsByAddress, fmt.Sprintf("direct-burst(%s): %v", strings.Join(directBurst, ","), err))
		} else {
			env, err := t.encryptEnvelope(envelopeTypeData, frame.TargetNodeID, frame)
			if err != nil {
				return err
			}
			if err := t.writeEnvelope(ctx, reachedAddress, env); err != nil {
				t.recordSendError(frame.TargetNodeID, err)
				errorsByAddress = append(errorsByAddress, fmt.Sprintf("%s: %v", reachedAddress, err))
			} else {
				t.recordSendSuccess(frame.TargetNodeID, len(frame.Payload))
				return nil
			}
		}
	}
	for _, candidateAddress := range attempts {
		if len(directBurst) > 1 && containsAddress(directBurst, candidateAddress) {
			continue
		}
		if err := t.sendToAddress(ctx, candidateAddress, frame); err != nil {
			t.recordSendError(frame.TargetNodeID, err)
			errorsByAddress = append(errorsByAddress, fmt.Sprintf("%s: %v", candidateAddress, err))
			continue
		}
		t.recordSendSuccess(frame.TargetNodeID, len(frame.Payload))
		return nil
	}

	return fmt.Errorf(
		"secure transport send failed for peer %s after %d candidate(s): %s",
		frame.TargetNodeID,
		len(attempts),
		strings.Join(errorsByAddress, "; "),
	)
}

func (t *Transport) WarmupPeer(ctx context.Context, peerNodeID string, addresses []string) WarmupReport {
	report := WarmupReport{
		PeerNodeID: peerNodeID,
		Results:    []WarmupResult{},
	}
	if t == nil || t.conn == nil {
		report.Results = append(report.Results, WarmupResult{
			Status: "error",
			Error:  "secure udp transport is not initialized",
		})
		return report
	}

	attempts := dedupeAddresses(addresses)
	if len(attempts) == 0 {
		attempts = dedupeAddresses(t.directCandidateAddresses(peerNodeID))
	}
	if len(attempts) == 0 {
		report.Results = append(report.Results, WarmupResult{
			Status: "skipped",
			Error:  "no direct candidate is available",
		})
		return report
	}

	t.recordDirectTry(peerNodeID)
	reachedAddress, err := t.ensureAnySession(ctx, peerNodeID, attempts)
	if err != nil {
		for _, address := range attempts {
			report.Results = append(report.Results, WarmupResult{
				Address: address,
				Status:  "error",
				Error:   err.Error(),
			})
		}
		return report
	}

	report.Reachable = true
	reachedRecorded := false
	for _, address := range attempts {
		result := WarmupResult{
			Address: address,
			Status:  "skipped",
		}
		if strings.TrimSpace(address) == strings.TrimSpace(reachedAddress) {
			result.Status = "reachable"
			reachedRecorded = true
		}
		report.Results = append(report.Results, result)
	}
	if !reachedRecorded && strings.TrimSpace(reachedAddress) != "" {
		report.Results = append(report.Results, WarmupResult{
			Address: reachedAddress,
			Status:  "reachable",
		})
	}

	return report
}

func (t *Transport) ExecuteDirectAttempt(ctx context.Context, attempt DirectAttempt) (DirectAttemptResult, error) {
	result := DirectAttemptResult{
		AttemptID:  strings.TrimSpace(attempt.AttemptID),
		PeerNodeID: strings.TrimSpace(attempt.PeerNodeID),
		Result:     "timeout",
	}
	if t == nil || t.conn == nil {
		err := errors.New("secure udp transport is not initialized")
		result.Error = err.Error()
		return result, err
	}
	if result.AttemptID == "" {
		err := errors.New("direct attempt id is required")
		result.Error = err.Error()
		return result, err
	}
	if result.PeerNodeID == "" {
		err := errors.New("direct attempt peer node id is required")
		result.Error = err.Error()
		return result, err
	}

	candidates := api.NormalizeDirectAttemptCandidates(attempt.Candidates, attempt.ExecuteAt)
	filtered := make([]api.DirectAttemptCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if kind := t.candidateKindForPeer(result.PeerNodeID, candidate.Address); kind == "relay" || t.isRelayAddress(candidate.Address) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	if len(filtered) == 0 {
		err := errors.New("no direct candidate is available")
		result.Error = err.Error()
		return result, err
	}
	primaryCandidates, secondaryCandidates := splitDirectAttemptCandidates(filtered)
	candidateCount := len(filtered)

	if attempt.Window <= 0 {
		attempt.Window = t.handshakeTimeout
	}
	if attempt.BurstInterval <= 0 {
		attempt.BurstInterval = t.handshakeRetryInterval
	}

	now := time.Now().UTC()
	deadline := now.Add(attempt.Window)
	if !attempt.ExecuteAt.IsZero() {
		deadline = attempt.ExecuteAt.Add(attempt.Window)
	}
	startAt := now
	switch {
	case len(primaryCandidates) > 0:
		startAt = directAttemptPrewarmStart(now, attempt.ExecuteAt, attempt.Window, attempt.BurstInterval)
	case !attempt.ExecuteAt.IsZero():
		startAt = attempt.ExecuteAt
	}
	if startAt.IsZero() || startAt.Before(now) {
		startAt = now
	}
	if time.Now().UTC().Before(startAt) {
		timer := time.NewTimer(time.Until(startAt))
		defer timer.Stop()
		select {
		case <-ctx.Done():
			result.Result = "cancelled"
			result.Error = ctx.Err().Error()
			t.recordDirectAttemptResult(result.PeerNodeID, result.AttemptID, attempt.Reason, time.Now().UTC(), result.Result, "", "", candidateCount)
			return result, ctx.Err()
		case <-timer.C:
		}
	}

	if !deadline.IsZero() && time.Now().UTC().After(deadline) {
		result.Result = "cancelled"
		result.Error = "direct attempt window expired"
		completedAt := time.Now().UTC()
		result.StartedAt = completedAt
		result.CompletedAt = completedAt
		result.ActiveAddress = t.activeAddress(result.PeerNodeID)
		t.recordDirectAttemptResult(result.PeerNodeID, result.AttemptID, attempt.Reason, completedAt, result.Result, "", "", candidateCount)
		return result, nil
	}

	startedAt := time.Now().UTC()
	result.StartedAt = startedAt
	if !t.beginDirectAttempt(result.PeerNodeID, result.AttemptID) {
		err := fmt.Errorf("direct attempt already in flight for peer %s", result.PeerNodeID)
		result.Result = "cancelled"
		result.Error = err.Error()
		result.CompletedAt = time.Now().UTC()
		result.ActiveAddress = t.activeAddress(result.PeerNodeID)
		return result, err
	}
	defer t.finishDirectAttempt(result.PeerNodeID, result.AttemptID)

	previousActive := t.activeAddress(result.PeerNodeID)
	t.recordDirectTry(result.PeerNodeID)
	var (
		reachedCandidate api.DirectAttemptCandidate
		err              error
	)
	if len(primaryCandidates) > 0 {
		result.Phase = api.DirectAttemptPhasePrimary
		primaryDeadline := deadline
		if len(secondaryCandidates) > 0 && !attempt.ExecuteAt.IsZero() && time.Now().UTC().Before(attempt.ExecuteAt) {
			primaryDeadline = attempt.ExecuteAt
		}
		if !primaryDeadline.Before(time.Now().UTC()) {
			reachedCandidate, err = t.ensureAnySessionCandidatesUntil(ctx, result.PeerNodeID, primaryCandidates, primaryDeadline, attempt.BurstInterval, directAttemptCandidateSpacing)
		} else if len(secondaryCandidates) == 0 {
			err = fmt.Errorf("secure transport handshake timeout for peer %s", result.PeerNodeID)
		}
	}
	runSecondary := len(secondaryCandidates) > 0 && (len(primaryCandidates) == 0 || err != nil)
	if runSecondary && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		if !attempt.ExecuteAt.IsZero() && time.Now().UTC().Before(attempt.ExecuteAt) {
			timer := time.NewTimer(time.Until(attempt.ExecuteAt))
			defer timer.Stop()
			select {
			case <-ctx.Done():
				err = ctx.Err()
			case <-timer.C:
			}
		}
		if err == nil || (!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)) {
			result.Phase = api.DirectAttemptPhaseSecondary
			reachedCandidate, err = t.ensureAnySessionCandidatesUntil(ctx, result.PeerNodeID, secondaryCandidates, deadline, attempt.BurstInterval, directAttemptCandidateSpacing)
		}
	}
	result.CompletedAt = time.Now().UTC()
	result.ActiveAddress = t.activeAddress(result.PeerNodeID)

	switch {
	case err == nil:
		result.Result = "success"
		result.ReachedAddress = strings.TrimSpace(reachedCandidate.Address)
		result.ReachedSource = strings.TrimSpace(reachedCandidate.Source)
		if result.Phase == "" {
			result.Phase = api.NormalizeDirectAttemptPhase(reachedCandidate.Phase, reachedCandidate.Source)
		}
		if result.ActiveAddress == "" {
			result.ActiveAddress = result.ReachedAddress
		}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		result.Result = "cancelled"
		result.Error = err.Error()
	default:
		result.Error = err.Error()
		if strings.TrimSpace(previousActive) != "" && t.isRelayAddress(previousActive) {
			result.Result = "relay_kept"
		} else {
			result.Result = "timeout"
		}
	}
	t.recordDirectAttemptResult(result.PeerNodeID, result.AttemptID, attempt.Reason, result.CompletedAt, result.Result, result.ReachedSource, result.Phase, candidateCount)
	return result, err
}

func (t *Transport) Serve(ctx context.Context, handler func(context.Context, dataplane.Frame, net.Addr) error) error {
	if t == nil || t.conn == nil {
		return errors.New("secure udp transport is not initialized")
	}

	buffer := make([]byte, 64*1024)
	for {
		if err := t.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			return fmt.Errorf("set secure udp read deadline: %w", err)
		}

		n, addr, err := t.conn.ReadFrom(buffer)
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
			return fmt.Errorf("read secure udp packet: %w", err)
		}

		if response, err := stun.ParseBindingResponse(buffer[:n]); err == nil {
			t.handleSTUNResponse(response)
			continue
		}

		var env envelope
		if err := json.Unmarshal(buffer[:n], &env); err != nil {
			continue
		}
		if env.TargetNodeID != t.nodeID {
			continue
		}
		if t.isReplay(env.SourceNodeID, env.Nonce, env.SentAt) {
			continue
		}

		switch env.Type {
		case envelopeTypeHello:
			if err := t.handleHello(ctx, addr, env); err != nil {
				return err
			}
		case envelopeTypeHelloAck:
			if err := t.handleHelloAck(addr, env); err != nil {
				return err
			}
		case envelopeTypeData:
			frame, err := t.decryptFrame(env)
			if err != nil {
				continue
			}
			t.markEstablished(env.SourceNodeID, addr.String())
			t.recordReceiveSuccess(env.SourceNodeID, len(frame.Payload))
			if err := handler(ctx, frame, addr); err != nil {
				return err
			}
		}
	}
}

func (t *Transport) ensureSession(ctx context.Context, peerNodeID, address string) error {
	_, err := t.ensureAnySession(ctx, peerNodeID, []string{address})
	return err
}

func (t *Transport) ensureAnySession(ctx context.Context, peerNodeID string, addresses []string) (string, error) {
	return t.ensureAnySessionWithTiming(ctx, peerNodeID, addresses, t.handshakeTimeout, t.handshakeRetryInterval)
}

func (t *Transport) ensureAnySessionWithTiming(ctx context.Context, peerNodeID string, addresses []string, handshakeTimeout, retryInterval time.Duration) (string, error) {
	if handshakeTimeout <= 0 {
		handshakeTimeout = t.handshakeTimeout
	}
	return t.ensureAnySessionUntil(ctx, peerNodeID, addresses, time.Now().UTC().Add(handshakeTimeout), retryInterval)
}

func (t *Transport) ensureAnySessionUntil(ctx context.Context, peerNodeID string, addresses []string, deadline time.Time, retryInterval time.Duration) (string, error) {
	attempts := dedupeAddresses(addresses)
	if len(attempts) == 0 {
		return "", errors.New("no candidate address is available for secure transport")
	}
	if retryInterval <= 0 {
		retryInterval = t.handshakeRetryInterval
	}
	if deadline.IsZero() {
		deadline = time.Now().UTC().Add(t.handshakeTimeout)
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		t.recordHandshakeTimeout(peerNodeID, strings.Join(attempts, ","))
		return "", fmt.Errorf("secure transport handshake timeout for peer %s", peerNodeID)
	}
	if retryInterval > remaining {
		retryInterval = remaining
	}

	t.stateMu.Lock()
	for _, address := range attempts {
		if _, ok := t.established[t.establishedKey(peerNodeID, address)]; ok {
			t.stateMu.Unlock()
			return address, nil
		}
	}

	challenge, err := randomHex(8)
	if err != nil {
		t.stateMu.Unlock()
		return "", err
	}
	waiterKey := peerNodeID + "|" + challenge
	waiter := make(chan string, 1)
	t.waiters[waiterKey] = waiter
	t.stateMu.Unlock()

	defer func() {
		t.stateMu.Lock()
		delete(t.waiters, waiterKey)
		t.stateMu.Unlock()
	}()

	sendHello := func(address string) error {
		env, err := t.encryptEnvelope(envelopeTypeHello, peerNodeID, helloPayload{Challenge: challenge})
		if err != nil {
			return err
		}
		return t.writeEnvelope(ctx, address, env)
	}
	sendBurst := func() error {
		failures := make([]string, 0)
		for _, address := range attempts {
			if err := sendHello(address); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", address, err))
			}
		}
		if len(failures) == len(attempts) {
			return fmt.Errorf("send secure hello burst for peer %s failed: %s", peerNodeID, strings.Join(failures, "; "))
		}
		return nil
	}
	if err := sendBurst(); err != nil {
		return "", err
	}

	timer := time.NewTimer(remaining)
	defer timer.Stop()
	retryTicker := time.NewTicker(retryInterval)
	defer retryTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timer.C:
			t.recordHandshakeTimeout(peerNodeID, strings.Join(attempts, ","))
			return "", fmt.Errorf("secure transport handshake timeout for peer %s", peerNodeID)
		case responderAddress := <-waiter:
			responderAddress = strings.TrimSpace(responderAddress)
			if responderAddress == "" {
				responderAddress = t.currentEstablishedAddress(peerNodeID, attempts)
			}
			if responderAddress == "" {
				responderAddress = attempts[0]
			}
			t.markEstablished(peerNodeID, responderAddress)
			return responderAddress, nil
		case <-retryTicker.C:
			if err := sendBurst(); err != nil {
				return "", err
			}
		}
	}
}

func splitDirectAttemptCandidates(candidates []api.DirectAttemptCandidate) ([]api.DirectAttemptCandidate, []api.DirectAttemptCandidate) {
	primary := make([]api.DirectAttemptCandidate, 0, len(candidates))
	secondary := make([]api.DirectAttemptCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		switch api.NormalizeDirectAttemptPhase(candidate.Phase, candidate.Source) {
		case api.DirectAttemptPhasePrimary:
			primary = append(primary, candidate)
		default:
			secondary = append(secondary, candidate)
		}
	}
	return primary, secondary
}

func (t *Transport) ensureAnySessionCandidatesUntil(ctx context.Context, peerNodeID string, candidates []api.DirectAttemptCandidate, deadline time.Time, retryInterval, candidateSpacing time.Duration) (api.DirectAttemptCandidate, error) {
	candidates = api.NormalizeDirectAttemptCandidates(candidates, deadline)
	if len(candidates) == 0 {
		return api.DirectAttemptCandidate{}, errors.New("no candidate address is available for secure transport")
	}
	addresses := api.DirectAttemptCandidateAddresses(candidates)
	if len(addresses) == 0 {
		return api.DirectAttemptCandidate{}, errors.New("no candidate address is available for secure transport")
	}
	if retryInterval <= 0 {
		retryInterval = t.handshakeRetryInterval
	}
	if deadline.IsZero() {
		deadline = time.Now().UTC().Add(t.handshakeTimeout)
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		t.recordHandshakeTimeout(peerNodeID, strings.Join(addresses, ","))
		return api.DirectAttemptCandidate{}, fmt.Errorf("secure transport handshake timeout for peer %s", peerNodeID)
	}
	if retryInterval > remaining {
		retryInterval = remaining
	}

	t.stateMu.Lock()
	for _, candidate := range candidates {
		if _, ok := t.established[t.establishedKey(peerNodeID, candidate.Address)]; ok {
			t.stateMu.Unlock()
			return candidate, nil
		}
	}

	challenge, err := randomHex(8)
	if err != nil {
		t.stateMu.Unlock()
		return api.DirectAttemptCandidate{}, err
	}
	waiterKey := peerNodeID + "|" + challenge
	waiter := make(chan string, 1)
	t.waiters[waiterKey] = waiter
	t.stateMu.Unlock()

	defer func() {
		t.stateMu.Lock()
		delete(t.waiters, waiterKey)
		t.stateMu.Unlock()
	}()

	sendHello := func(address string) error {
		env, err := t.encryptEnvelope(envelopeTypeHello, peerNodeID, helloPayload{Challenge: challenge})
		if err != nil {
			return err
		}
		return t.writeEnvelope(ctx, address, env)
	}
	sendBurst := func() error {
		failures := make([]string, 0)
		for idx, candidate := range candidates {
			if err := sendHello(candidate.Address); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", candidate.Address, err))
			}
			if idx == len(candidates)-1 || candidateSpacing <= 0 {
				continue
			}
			timer := time.NewTimer(candidateSpacing)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		if len(failures) == len(candidates) {
			return fmt.Errorf("send secure hello burst for peer %s failed: %s", peerNodeID, strings.Join(failures, "; "))
		}
		return nil
	}
	if err := sendBurst(); err != nil {
		return api.DirectAttemptCandidate{}, err
	}

	timer := time.NewTimer(remaining)
	defer timer.Stop()
	retryTicker := time.NewTicker(retryInterval)
	defer retryTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return api.DirectAttemptCandidate{}, ctx.Err()
		case <-timer.C:
			t.recordHandshakeTimeout(peerNodeID, strings.Join(addresses, ","))
			return api.DirectAttemptCandidate{}, fmt.Errorf("secure transport handshake timeout for peer %s", peerNodeID)
		case responderAddress := <-waiter:
			responderAddress = strings.TrimSpace(responderAddress)
			if responderAddress == "" {
				responderAddress = t.currentEstablishedAddress(peerNodeID, addresses)
			}
			matched := directAttemptCandidateForAddress(candidates, responderAddress)
			if strings.TrimSpace(matched.Address) == "" {
				matched = candidates[0]
			}
			if responderAddress == "" {
				responderAddress = matched.Address
			}
			t.markEstablished(peerNodeID, responderAddress)
			return matched, nil
		case <-retryTicker.C:
			if err := sendBurst(); err != nil {
				return api.DirectAttemptCandidate{}, err
			}
		}
	}
}

func directAttemptPrewarmStart(now, executeAt time.Time, window, burstInterval time.Duration) time.Time {
	if executeAt.IsZero() || !executeAt.After(now) {
		return time.Time{}
	}
	lead := directAttemptPrewarmLead(executeAt.Sub(now), window, burstInterval)
	if lead <= 0 {
		return time.Time{}
	}
	return executeAt.Add(-lead)
}

func directAttemptPrewarmLead(untilExecute, window, burstInterval time.Duration) time.Duration {
	if untilExecute <= 0 {
		return 0
	}
	if burstInterval <= 0 {
		burstInterval = 100 * time.Millisecond
	}
	lead := burstInterval * 2
	if lead <= 0 {
		lead = burstInterval
	}
	if window > 0 {
		halfWindow := window / 2
		if halfWindow > 0 && lead > halfWindow {
			lead = halfWindow
		}
	}
	maxLead := 250 * time.Millisecond
	if lead > maxLead {
		lead = maxLead
	}
	if lead > untilExecute {
		lead = untilExecute
	}
	return lead
}

func (t *Transport) probeSTUNServer(ctx context.Context, server string, timeout time.Duration) (string, time.Duration, error) {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	remoteAddress, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return "", 0, fmt.Errorf("resolve stun server: %w", err)
	}

	request, transactionID, err := stun.NewBindingRequest()
	if err != nil {
		return "", 0, err
	}

	waiter := make(chan stun.BindingResponse, 1)
	t.stunMu.Lock()
	if t.stunWaiters == nil {
		t.stunWaiters = map[string]chan stun.BindingResponse{}
	}
	t.stunWaiters[transactionID] = waiter
	t.stunMu.Unlock()
	defer func() {
		t.stunMu.Lock()
		delete(t.stunWaiters, transactionID)
		t.stunMu.Unlock()
	}()

	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	requestCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	sentAt := time.Now().UTC()
	if err := t.writeRawPacket(requestCtx, remoteAddress.String(), request); err != nil {
		return "", 0, err
	}

	for {
		select {
		case <-requestCtx.Done():
			if errors.Is(requestCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
				return "", 0, context.DeadlineExceeded
			}
			return "", 0, requestCtx.Err()
		case response := <-waiter:
			if response.TransactionID != transactionID {
				continue
			}
			return response.ReflexiveAddress, time.Since(sentAt), nil
		}
	}
}

func (t *Transport) handleHello(ctx context.Context, addr net.Addr, env envelope) error {
	var payload helloPayload
	if err := t.decryptPayload(env, &payload); err != nil {
		return nil
	}
	t.markEstablished(env.SourceNodeID, addr.String())

	ack, err := t.encryptEnvelope(envelopeTypeHelloAck, env.SourceNodeID, helloPayload{Challenge: payload.Challenge})
	if err != nil {
		return err
	}
	return t.writeEnvelope(ctx, addr.String(), ack)
}

func (t *Transport) handleHelloAck(addr net.Addr, env envelope) error {
	var payload helloPayload
	if err := t.decryptPayload(env, &payload); err != nil {
		return nil
	}
	responderAddress := ""
	if addr != nil {
		responderAddress = strings.TrimSpace(addr.String())
		if responderAddress != "" {
			t.markEstablished(env.SourceNodeID, responderAddress)
		}
	}

	waiterKey := env.SourceNodeID + "|" + payload.Challenge
	t.stateMu.Lock()
	waiter := t.waiters[waiterKey]
	t.stateMu.Unlock()
	if waiter != nil {
		select {
		case waiter <- responderAddress:
		default:
		}
	}
	return nil
}

func (t *Transport) handleSTUNResponse(response stun.BindingResponse) {
	t.stunMu.Lock()
	waiter := t.stunWaiters[response.TransactionID]
	t.stunMu.Unlock()
	if waiter == nil {
		return
	}
	select {
	case waiter <- response:
	default:
	}
}

func (t *Transport) decryptFrame(env envelope) (dataplane.Frame, error) {
	var frame dataplane.Frame
	if err := t.decryptPayload(env, &frame); err != nil {
		return dataplane.Frame{}, err
	}
	return frame, nil
}

func (t *Transport) decryptPayload(env envelope, out any) error {
	aead, err := t.aeadForPeer(env.SourceNodeID)
	if err != nil {
		return err
	}
	payload, err := aead.Open(nil, env.Nonce, env.Payload, envelopeAAD(env))
	if err != nil {
		return fmt.Errorf("decrypt secure payload: %w", err)
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("decode secure payload: %w", err)
	}
	return nil
}

func (t *Transport) encryptEnvelope(messageType, peerNodeID string, payload any) (envelope, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return envelope{}, fmt.Errorf("marshal secure payload: %w", err)
	}

	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return envelope{}, fmt.Errorf("generate secure nonce: %w", err)
	}

	env := envelope{
		Version:      1,
		Type:         messageType,
		SourceNodeID: t.nodeID,
		TargetNodeID: peerNodeID,
		Nonce:        nonce,
		SentAt:       time.Now().UTC(),
	}
	aead, err := t.aeadForPeer(peerNodeID)
	if err != nil {
		return envelope{}, err
	}
	env.Payload = aead.Seal(nil, nonce, rawPayload, envelopeAAD(env))
	return env, nil
}

func (t *Transport) writeEnvelope(ctx context.Context, address string, env envelope) error {
	raw, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal secure envelope: %w", err)
	}
	return t.writeRawPacket(ctx, address, raw)
}

func (t *Transport) writeRawPacket(ctx context.Context, address string, raw []byte) error {
	remoteAddress, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return fmt.Errorf("resolve secure udp remote address: %w", err)
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	if deadline, ok := ctx.Deadline(); ok {
		if err := t.conn.SetWriteDeadline(deadline); err != nil {
			return fmt.Errorf("set secure udp write deadline: %w", err)
		}
		defer t.conn.SetWriteDeadline(time.Time{})
	}
	if _, err := t.conn.WriteToUDP(raw, remoteAddress); err != nil {
		return fmt.Errorf("write secure udp packet: %w", err)
	}
	return nil
}

func (t *Transport) aeadForPeer(peerNodeID string) (cipher.AEAD, error) {
	publicKey, ok := t.peerPublicKeys[peerNodeID]
	if !ok {
		return nil, fmt.Errorf("unknown peer public key for %s", peerNodeID)
	}
	sharedSecret, err := t.privateKey.ECDH(publicKey)
	if err != nil {
		return nil, fmt.Errorf("derive shared secret: %w", err)
	}

	contextBytes := []byte(sessionContextKey(t.nodeID, peerNodeID))
	material := sha256.Sum256(append(sharedSecret, contextBytes...))
	block, err := aes.NewCipher(material[:])
	if err != nil {
		return nil, fmt.Errorf("create secure cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create secure gcm: %w", err)
	}
	return aead, nil
}

func (t *Transport) markEstablished(peerNodeID, address string) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.peerStats == nil {
		t.peerStats = map[string]peerMetrics{}
	}
	address = strings.TrimSpace(address)
	now := time.Now().UTC()
	metrics := t.peerStats[peerNodeID]
	key := t.establishedKey(peerNodeID, address)
	if _, exists := t.established[key]; !exists {
		metrics.SessionsEstablished++
	}
	t.established[key] = now
	if address != "" {
		current := strings.TrimSpace(t.activePeer[peerNodeID])
		if current != "" && !t.isRelayAddress(current) && t.isRelayAddress(address) {
			t.peerStats[peerNodeID] = metrics
			return
		}
		if current != address {
			metrics.ActiveSince = now
			metrics.LastPathChangeAt = now
			if t.isRelayAddress(address) && (current == "" || !t.isRelayAddress(current)) {
				metrics.RelayFallbacks++
			}
			if !t.isRelayAddress(address) && current != "" && t.isRelayAddress(current) {
				metrics.DirectRecoveries++
			}
		}
		t.activePeer[peerNodeID] = address
	}
	t.peerStats[peerNodeID] = metrics
}

func (t *Transport) sendToAddress(ctx context.Context, address string, frame dataplane.Frame) error {
	address = strings.TrimSpace(address)
	if address == "" {
		return errors.New("candidate address is empty")
	}
	if t.isRelayAddress(address) {
		if err := t.Announce(ctx, address); err != nil {
			return err
		}
	}

	if err := t.ensureSession(ctx, frame.TargetNodeID, address); err != nil {
		return err
	}

	env, err := t.encryptEnvelope(envelopeTypeData, frame.TargetNodeID, frame)
	if err != nil {
		return err
	}
	return t.writeEnvelope(ctx, address, env)
}

func (t *Transport) sendAddresses(peerNodeID, requestedAddress string) []string {
	ordered := make([]string, 0, 4)
	seen := make(map[string]struct{})
	directAddresses := make([]string, 0, len(t.peerCandidates[peerNodeID]))
	otherAddresses := make([]string, 0, len(t.peerCandidates[peerNodeID]))
	appendAddress := func(address string) {
		address = strings.TrimSpace(address)
		if address == "" {
			return
		}
		if _, ok := seen[address]; ok {
			return
		}
		seen[address] = struct{}{}
		ordered = append(ordered, address)
	}

	for _, candidate := range t.peerCandidates[peerNodeID] {
		switch strings.ToLower(strings.TrimSpace(candidate.Kind)) {
		case "direct":
			directAddresses = append(directAddresses, candidate.Address)
		default:
			otherAddresses = append(otherAddresses, candidate.Address)
		}
	}

	activeAddress, retryDirect := t.directRecoveryState(peerNodeID, directAddresses)
	requestedAddress = strings.TrimSpace(requestedAddress)
	requestedIsDirect := containsAddress(directAddresses, requestedAddress)

	if retryDirect {
		if requestedIsDirect {
			appendAddress(requestedAddress)
		}
		for _, address := range directAddresses {
			appendAddress(address)
		}
		appendAddress(activeAddress)
		if !requestedIsDirect {
			appendAddress(requestedAddress)
		}
		for _, address := range otherAddresses {
			appendAddress(address)
		}
		return ordered
	}

	appendAddress(activeAddress)
	appendAddress(requestedAddress)
	for _, address := range directAddresses {
		appendAddress(address)
	}
	for _, address := range otherAddresses {
		appendAddress(address)
	}
	return ordered
}

func (t *Transport) directRecoveryState(peerNodeID string, directAddresses []string) (string, bool) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()

	activeAddress := strings.TrimSpace(t.activePeer[peerNodeID])
	if activeAddress == "" || !t.isRelayAddress(activeAddress) || len(directAddresses) == 0 {
		return activeAddress, false
	}

	now := time.Now().UTC()
	if lastTry := t.lastDirectTry[peerNodeID]; !lastTry.IsZero() && now.Sub(lastTry) < t.directRetryAfter {
		return activeAddress, false
	}

	t.recordDirectTryLocked(peerNodeID, now)
	return activeAddress, true
}

func (t *Transport) directCandidateAddresses(peerNodeID string) []string {
	addresses := make([]string, 0, len(t.peerCandidates[peerNodeID]))
	for _, candidate := range t.peerCandidates[peerNodeID] {
		if strings.ToLower(strings.TrimSpace(candidate.Kind)) != "direct" {
			continue
		}
		address := strings.TrimSpace(candidate.Address)
		if address == "" {
			continue
		}
		addresses = append(addresses, address)
	}
	return addresses
}

func (t *Transport) hasDirectCandidate(peerNodeID string) bool {
	for _, candidate := range t.peerCandidates[peerNodeID] {
		if strings.ToLower(strings.TrimSpace(candidate.Kind)) != "direct" {
			continue
		}
		if strings.TrimSpace(candidate.Address) != "" {
			return true
		}
	}
	return false
}

func (t *Transport) leadingDirectAddresses(peerNodeID string, addresses []string) []string {
	result := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if t.candidateKindForPeer(peerNodeID, address) != "direct" {
			break
		}
		result = append(result, strings.TrimSpace(address))
	}
	return dedupeAddresses(result)
}

func (t *Transport) currentEstablishedAddress(peerNodeID string, addresses []string) string {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()

	activeAddress := strings.TrimSpace(t.activePeer[peerNodeID])
	if containsAddress(addresses, activeAddress) {
		return activeAddress
	}
	for _, address := range addresses {
		if _, ok := t.established[t.establishedKey(peerNodeID, address)]; ok {
			return strings.TrimSpace(address)
		}
	}
	return ""
}

func (t *Transport) activeAddress(peerNodeID string) string {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	return strings.TrimSpace(t.activePeer[peerNodeID])
}

func (t *Transport) beginDirectAttempt(peerNodeID, attemptID string) bool {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.directAttemptInFlight == nil {
		t.directAttemptInFlight = map[string]string{}
	}
	if existing := strings.TrimSpace(t.directAttemptInFlight[peerNodeID]); existing != "" && existing != strings.TrimSpace(attemptID) {
		return false
	}
	t.directAttemptInFlight[peerNodeID] = strings.TrimSpace(attemptID)
	return true
}

func (t *Transport) finishDirectAttempt(peerNodeID, attemptID string) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if strings.TrimSpace(t.directAttemptInFlight[peerNodeID]) == strings.TrimSpace(attemptID) {
		delete(t.directAttemptInFlight, peerNodeID)
	}
}

func (t *Transport) recordSendSuccess(peerNodeID string, payloadBytes int) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.peerStats == nil {
		t.peerStats = map[string]peerMetrics{}
	}
	metrics := t.peerStats[peerNodeID]
	metrics.LastSendSuccessAt = time.Now().UTC()
	metrics.SentPackets++
	metrics.SentBytes += int64(payloadBytes)
	t.peerStats[peerNodeID] = metrics
}

func (t *Transport) recordDirectTry(peerNodeID string) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	t.recordDirectTryLocked(peerNodeID, time.Now().UTC())
}

func (t *Transport) recordDirectTryLocked(peerNodeID string, attemptedAt time.Time) {
	if t.lastDirectTry == nil {
		t.lastDirectTry = map[string]time.Time{}
	}
	t.lastDirectTry[peerNodeID] = attemptedAt
}

func directAttemptCandidateForAddress(candidates []api.DirectAttemptCandidate, address string) api.DirectAttemptCandidate {
	address = strings.TrimSpace(address)
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Address) == address {
			return candidate
		}
	}
	return api.DirectAttemptCandidate{}
}

func (t *Transport) recordDirectAttemptResult(peerNodeID, attemptID, reason string, attemptedAt time.Time, result, reachedSource, phase string, candidateCount int) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.peerStats == nil {
		t.peerStats = map[string]peerMetrics{}
	}
	metrics := t.peerStats[peerNodeID]
	metrics.LastDirectAttemptID = strings.TrimSpace(attemptID)
	metrics.LastDirectAttemptReason = strings.TrimSpace(reason)
	metrics.LastDirectAttemptAt = attemptedAt
	metrics.LastDirectAttemptResult = strings.TrimSpace(result)
	metrics.LastDirectAttemptReachedSource = strings.TrimSpace(reachedSource)
	metrics.LastDirectAttemptPhase = strings.TrimSpace(phase)
	metrics.LastDirectAttemptCandidateCount = candidateCount
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "success":
		metrics.LastDirectSuccessAt = attemptedAt
		metrics.ConsecutiveDirectFailures = 0
	case "timeout", "relay_kept":
		metrics.ConsecutiveDirectFailures++
	}
	t.peerStats[peerNodeID] = metrics
}

func (t *Transport) recordReceiveSuccess(peerNodeID string, payloadBytes int) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.peerStats == nil {
		t.peerStats = map[string]peerMetrics{}
	}
	metrics := t.peerStats[peerNodeID]
	metrics.LastReceiveAt = time.Now().UTC()
	metrics.ReceivedPackets++
	metrics.ReceivedBytes += int64(payloadBytes)
	t.peerStats[peerNodeID] = metrics
}

func (t *Transport) recordSendError(peerNodeID string, err error) {
	if err == nil {
		return
	}
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.peerStats == nil {
		t.peerStats = map[string]peerMetrics{}
	}
	metrics := t.peerStats[peerNodeID]
	metrics.LastSendErrorAt = time.Now().UTC()
	metrics.LastSendError = err.Error()
	t.peerStats[peerNodeID] = metrics
}

func (t *Transport) recordHandshakeTimeout(peerNodeID, address string) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.peerStats == nil {
		t.peerStats = map[string]peerMetrics{}
	}
	metrics := t.peerStats[peerNodeID]
	metrics.HandshakeTimeouts++
	metrics.LastSendErrorAt = time.Now().UTC()
	metrics.LastSendError = fmt.Sprintf(
		"secure transport handshake timeout for peer %s via %s",
		peerNodeID,
		strings.TrimSpace(address),
	)
	t.peerStats[peerNodeID] = metrics
}

func containsAddress(addresses []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, address := range addresses {
		if strings.TrimSpace(address) == target {
			return true
		}
	}
	return false
}

func dedupeAddresses(addresses []string) []string {
	seen := make(map[string]struct{}, len(addresses))
	result := make([]string, 0, len(addresses))
	for _, address := range addresses {
		address = strings.TrimSpace(address)
		if address == "" {
			continue
		}
		if _, ok := seen[address]; ok {
			continue
		}
		seen[address] = struct{}{}
		result = append(result, address)
	}
	return result
}

func splitEstablishedKey(key string) (string, string) {
	peerNodeID, address, found := strings.Cut(key, "@")
	if !found {
		return "", ""
	}
	return peerNodeID, strings.TrimSpace(address)
}

func (t *Transport) isReplay(sourceNodeID string, nonce []byte, sentAt time.Time) bool {
	key := sourceNodeID + "|" + hex.EncodeToString(nonce)
	now := time.Now().UTC()

	t.stateMu.Lock()
	defer t.stateMu.Unlock()

	for existingKey, seenAt := range t.seenEnvelope {
		if now.Sub(seenAt) > t.replayTTL {
			delete(t.seenEnvelope, existingKey)
		}
	}
	if _, exists := t.seenEnvelope[key]; exists {
		return true
	}
	if !sentAt.IsZero() && now.Sub(sentAt) > t.replayTTL {
		return true
	}
	t.seenEnvelope[key] = now
	return false
}

func (t *Transport) establishedKey(peerNodeID, address string) string {
	return peerNodeID + "@" + strings.TrimSpace(address)
}

func (t *Transport) isRelayAddress(address string) bool {
	if t == nil {
		return false
	}
	_, ok := t.relayAddresses[strings.TrimSpace(address)]
	return ok
}

func (t *Transport) candidateKindForPeer(peerNodeID, address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	for _, candidate := range t.peerCandidates[peerNodeID] {
		if strings.TrimSpace(candidate.Address) != address {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(candidate.Kind))
		if kind == "" {
			return "observed"
		}
		return kind
	}
	if t.isRelayAddress(address) {
		return "relay"
	}
	return "observed"
}

func parsePrivateKey(privateKeyHex string) (*ecdh.PrivateKey, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(privateKeyHex))
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	privateKey, err := ecdh.X25519().NewPrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	return privateKey, nil
}

func parsePublicKey(publicKeyHex string) (*ecdh.PublicKey, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(publicKeyHex))
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}
	publicKey, err := ecdh.X25519().NewPublicKey(raw)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	return publicKey, nil
}

func envelopeAAD(env envelope) []byte {
	return []byte(fmt.Sprintf("%d|%s|%s|%s", env.Version, env.Type, env.SourceNodeID, env.TargetNodeID))
}

func sessionContextKey(leftNodeID, rightNodeID string) string {
	if leftNodeID < rightNodeID {
		return leftNodeID + "|" + rightNodeID
	}
	return rightNodeID + "|" + leftNodeID
}

func randomHex(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate secure random: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
