package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/services/controlplane/internal/config"
)

type MemoryStore struct {
	mu               sync.RWMutex
	cfg              config.Config
	tenant           api.Tenant
	adminUser        api.User
	devices          map[string]api.Device
	nodes            map[string]api.Node
	natReports       map[string]api.NATReport
	peerTransports   map[string]map[string]api.PeerTransportState
	directAttempts   map[string]directAttemptPair
	routes           map[string]api.Route
	dnsZones         map[string]api.DNSZone
	bootstrapVersion int
	ipCounter        int
}

func NewMemoryStore(cfg config.Config) *MemoryStore {
	now := time.Now().UTC()

	tenant := api.Tenant{
		ID:        "tenant-default",
		Name:      "Default Tenant",
		Status:    "active",
		CreatedAt: now,
	}

	adminUser := api.User{
		ID:          "user-admin",
		TenantID:    tenant.ID,
		Email:       cfg.AdminEmail,
		DisplayName: "Platform Admin",
		Status:      "active",
		Role:        "admin",
		CreatedAt:   now,
	}

	zone := api.DNSZone{
		ID:        "zone-internal",
		TenantID:  tenant.ID,
		Name:      cfg.DNSDomain,
		Type:      "internal",
		CreatedAt: now,
	}

	return &MemoryStore{
		cfg:              cfg,
		tenant:           tenant,
		adminUser:        adminUser,
		devices:          map[string]api.Device{},
		nodes:            map[string]api.Node{},
		natReports:       map[string]api.NATReport{},
		peerTransports:   map[string]map[string]api.PeerTransportState{},
		directAttempts:   map[string]directAttemptPair{},
		routes:           map[string]api.Route{},
		dnsZones:         map[string]api.DNSZone{zone.ID: zone},
		bootstrapVersion: 1,
	}
}

func (s *MemoryStore) Close() error {
	return nil
}

func (s *MemoryStore) ValidateAdminCredentials(email, password string) bool {
	return email == s.cfg.AdminEmail && hashPassword(password) == hashPassword(s.cfg.AdminPassword)
}

func (s *MemoryStore) AdminUser() api.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.adminUser
}

func (s *MemoryStore) CreateDeviceAndNode(req api.DeviceRegistrationRequest) (api.DeviceRegistrationResponse, error) {
	if strings.TrimSpace(req.RegistrationToken) != s.cfg.RegistrationToken {
		return api.DeviceRegistrationResponse{}, ErrUnauthorized
	}
	if strings.TrimSpace(req.DeviceName) == "" || strings.TrimSpace(req.Platform) == "" || strings.TrimSpace(req.PublicKey) == "" {
		return api.DeviceRegistrationResponse{}, fmt.Errorf("%w: device_name, platform and public_key are required", ErrInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = s.tenant.ID
	}
	if tenantID != s.tenant.ID {
		return api.DeviceRegistrationResponse{}, ErrNotFound
	}

	userID := req.UserID
	if userID == "" {
		userID = s.adminUser.ID
	}
	if userID != s.adminUser.ID {
		return api.DeviceRegistrationResponse{}, ErrNotFound
	}

	now := time.Now().UTC()
	device := api.Device{
		ID:           newID("dev"),
		TenantID:     tenantID,
		UserID:       userID,
		Name:         req.DeviceName,
		Platform:     req.Platform,
		Version:      req.Version,
		Status:       "active",
		Capabilities: append([]string(nil), req.Capabilities...),
		CreatedAt:    now,
	}

	node := api.Node{
		ID:         newID("node"),
		DeviceID:   device.ID,
		OverlayIP:  overlayIPFromSlot(s.nextOverlaySlotLocked()),
		PublicKey:  req.PublicKey,
		Status:     "registered",
		CreatedAt:  now,
		LastSeenAt: now,
		AuthToken:  newToken("node"),
	}

	s.devices[device.ID] = device
	s.nodes[node.ID] = node

	bootstrap := s.currentBootstrapLocked(node.ID)

	return api.DeviceRegistrationResponse{
		Device:    device,
		Node:      sanitizeNode(node),
		NodeToken: node.AuthToken,
		Bootstrap: bootstrap,
	}, nil
}

func (s *MemoryStore) UpdateHeartbeat(nodeID, token string, req api.HeartbeatRequest) (api.HeartbeatResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.nodes[nodeID]
	if !ok {
		return api.HeartbeatResponse{}, ErrNotFound
	}
	if token == "" || token != node.AuthToken {
		return api.HeartbeatResponse{}, ErrUnauthorized
	}

	now := time.Now().UTC()
	publicKeyChanged := false
	if publicKey := strings.TrimSpace(req.PublicKey); publicKey != "" && publicKey != node.PublicKey {
		node.PublicKey = publicKey
		publicKeyChanged = true
	}
	endpointRecords, endpoints := api.NormalizeEndpointObservations(now, req.Endpoints, req.EndpointRecords)
	endpointsChanged := !api.EndpointObservationsEqual(node.EndpointRecords, endpointRecords)
	node.Endpoints = endpoints
	node.EndpointRecords = endpointRecords
	if req.RelayRegion != "" {
		node.RelayRegion = req.RelayRegion
	}
	if req.Status != "" {
		node.Status = req.Status
	} else {
		node.Status = "online"
	}
	node.LastSeenAt = now
	s.nodes[node.ID] = node
	s.natReports[node.ID] = sanitizeNATReport(now, req.NATReport)
	s.peerTransports[node.ID] = peerTransportStateLookup(sanitizePeerTransportStates(now, req.PeerTransportStates))
	if publicKeyChanged || endpointsChanged {
		s.bootstrapVersion++
	}

	s.pruneDirectAttemptsLocked(now)
	s.scheduleDirectAttemptsLocked(now, node.ID)

	response := api.HeartbeatResponse{
		Node:               sanitizeNode(node),
		BootstrapVersion:   s.bootstrapVersion,
		DirectAttempts:     s.directAttemptsForNodeLocked(node.ID, now),
		PeerRecoveryStates: s.directAttemptRecoveryStatesForNodeLocked(node.ID, now),
	}
	return response, nil
}

func (s *MemoryStore) GetBootstrap(nodeID, token string) (api.BootstrapConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, ok := s.nodes[nodeID]
	if !ok {
		return api.BootstrapConfig{}, ErrNotFound
	}
	if token == "" || token != node.AuthToken {
		return api.BootstrapConfig{}, ErrUnauthorized
	}
	return s.currentBootstrapLocked(nodeID), nil
}

func (s *MemoryStore) ListNodes() []api.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nodes := make([]api.Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		nodes = append(nodes, sanitizeNode(node))
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].CreatedAt.Before(nodes[j].CreatedAt)
	})
	return nodes
}

func (s *MemoryStore) ListRoutes() []api.Route {
	s.mu.RLock()
	defer s.mu.RUnlock()

	routes := make([]api.Route, 0, len(s.routes))
	for _, route := range s.routes {
		routes = append(routes, route)
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Priority == routes[j].Priority {
			return routes[i].CreatedAt.Before(routes[j].CreatedAt)
		}
		return routes[i].Priority > routes[j].Priority
	})
	return routes
}

func (s *MemoryStore) CreateRoute(req api.CreateRouteRequest) (api.Route, error) {
	if strings.TrimSpace(req.NetworkCIDR) == "" || strings.TrimSpace(req.ViaNodeID) == "" {
		return api.Route{}, fmt.Errorf("%w: network_cidr and via_node_id are required", ErrInvalid)
	}

	prefix, err := netip.ParsePrefix(req.NetworkCIDR)
	if err != nil {
		return api.Route{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = s.tenant.ID
	}
	if tenantID != s.tenant.ID {
		return api.Route{}, ErrNotFound
	}
	if _, ok := s.nodes[req.ViaNodeID]; !ok {
		return api.Route{}, ErrNotFound
	}

	for _, route := range s.routes {
		existingPrefix, parseErr := netip.ParsePrefix(route.NetworkCIDR)
		if parseErr != nil {
			continue
		}
		if prefixesOverlap(existingPrefix, prefix) {
			return api.Route{}, fmt.Errorf("%w: route %s overlaps with existing route %s", ErrConflict, req.NetworkCIDR, route.NetworkCIDR)
		}
	}

	route := api.Route{
		ID:          newID("route"),
		TenantID:    tenantID,
		NetworkCIDR: prefix.Masked().String(),
		ViaNodeID:   req.ViaNodeID,
		Priority:    req.Priority,
		Status:      "active",
		CreatedAt:   time.Now().UTC(),
	}

	s.routes[route.ID] = route
	s.bootstrapVersion++
	return route, nil
}

func (s *MemoryStore) ListDNSZones() []api.DNSZone {
	s.mu.RLock()
	defer s.mu.RUnlock()

	zones := make([]api.DNSZone, 0, len(s.dnsZones))
	for _, zone := range s.dnsZones {
		zones = append(zones, zone)
	}

	sort.Slice(zones, func(i, j int) bool {
		return zones[i].CreatedAt.Before(zones[j].CreatedAt)
	})
	return zones
}

func (s *MemoryStore) currentBootstrapLocked(selfNodeID string) api.BootstrapConfig {
	selfNode, ok := s.nodes[selfNodeID]
	if !ok {
		selfNode = api.Node{}
	}
	selfTransportStates := s.peerTransports[selfNodeID]
	policy := directAttemptPolicyFromConfig(s.cfg)
	now := time.Now().UTC()
	selfCandidates := freshDirectCandidateAddressesWithPolicy(selfNode, now, policy)

	routes := make([]api.Route, 0, len(s.routes))
	for _, route := range s.routes {
		routes = append(routes, route)
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Priority == routes[j].Priority {
			return routes[i].CreatedAt.Before(routes[j].CreatedAt)
		}
		return routes[i].Priority > routes[j].Priority
	})

	relays := make([]api.RelayNode, 0, len(s.cfg.RelayAddresses))
	for _, address := range s.cfg.RelayAddresses {
		relays = append(relays, api.RelayNode{
			Region:  deriveRelayRegion(address),
			Address: address,
		})
	}

	peers := make([]api.Peer, 0, len(s.nodes))
	for _, node := range s.nodes {
		if node.ID == selfNodeID {
			continue
		}
		peerCandidates := freshDirectCandidateAddressesWithPolicy(node, now, policy)
		peerTransportState := api.PeerTransportState{}
		if peerStates, ok := s.peerTransports[node.ID]; ok {
			peerTransportState = peerStates[selfNodeID]
		}
		latestAttempt, hasAttempt := s.latestDirectAttemptForPairLocked(selfNodeID, node.ID, now)
		var latestAttemptPtr *directAttemptPair
		if hasAttempt {
			latestAttemptPtr = &latestAttempt
		}
		recoveryState := recoveryStateForPeer(selfNode, node, selfCandidates, peerCandidates, selfTransportStates[node.ID], peerTransportState, now, policy, latestAttemptPtr)
		peer := api.Peer{
			NodeID:          node.ID,
			OverlayIP:       node.OverlayIP,
			PublicKey:       node.PublicKey,
			Endpoints:       append([]string(nil), node.Endpoints...),
			EndpointRecords: append([]api.EndpointObservation(nil), node.EndpointRecords...),
			RelayRegion:     node.RelayRegion,
			AllowedIPs:      allowedIPsForNode(node.ID, node.OverlayIP, routes),
			Status:          node.Status,
			LastSeenAt:      node.LastSeenAt,
		}
		if report, ok := s.natReports[node.ID]; ok {
			peer.NATMappingBehavior, peer.NATReachable, peer.NATReportedAt = natSummaryForPeer(report)
		}
		if !peerTransportState.ReportedAt.IsZero() {
			peer.ObservedTransportKind = peerTransportState.ActiveKind
			peer.ObservedTransportAddress = peerTransportState.ActiveAddress
			peer.ObservedTransportReportedAt = peerTransportState.ReportedAt
			peer.ObservedLastDirectAttemptAt = peerTransportState.LastDirectAttemptAt
			peer.ObservedLastDirectAttemptResult = peerTransportState.LastDirectAttemptResult
			peer.ObservedLastDirectSuccessAt = peerTransportState.LastDirectSuccessAt
			peer.ObservedConsecutiveDirectFailures = peerTransportState.ConsecutiveDirectFailures
		}
		if recoveryState.Blocked {
			peer.ObservedDirectRecoveryBlocked = recoveryState.Blocked
			peer.ObservedDirectRecoveryBlockReason = recoveryState.BlockReason
			peer.ObservedDirectRecoveryBlockedUntil = recoveryState.BlockedUntil
			peer.ObservedDirectRecoveryNextProbeAt = recoveryState.NextProbeAt
			peer.ObservedDirectRecoveryProbeLimited = recoveryState.ProbeLimited
			peer.ObservedDirectRecoveryProbeBudget = recoveryState.ProbeBudget
			peer.ObservedDirectRecoveryProbeFailures = recoveryState.ProbeFailures
			peer.ObservedDirectRecoveryProbeRemaining = recoveryState.ProbeRemaining
			peer.ObservedDirectRecoveryProbeRefillAt = recoveryState.ProbeRefillAt
		}
		if recoveryState.LastIssuedAttemptID != "" {
			peer.ObservedDirectRecoveryLastIssuedAttemptID = recoveryState.LastIssuedAttemptID
			peer.ObservedDirectRecoveryLastIssuedAttemptReason = recoveryState.LastIssuedAttemptReason
			peer.ObservedDirectRecoveryLastIssuedAttemptAt = recoveryState.LastIssuedAttemptAt
			peer.ObservedDirectRecoveryLastIssuedAttemptExecuteAt = recoveryState.LastIssuedAttemptExecuteAt
		}
		if recoveryState.DecisionStatus != "" {
			peer.ObservedDirectRecoveryDecisionStatus = recoveryState.DecisionStatus
			peer.ObservedDirectRecoveryDecisionReason = recoveryState.DecisionReason
			peer.ObservedDirectRecoveryDecisionAt = recoveryState.DecisionAt
			peer.ObservedDirectRecoveryDecisionNextAt = recoveryState.DecisionNextAt
		}
		peers = append(peers, peer)
	}

	sort.Slice(peers, func(i, j int) bool {
		return peers[i].NodeID < peers[j].NodeID
	})

	var exitNode *api.ExitNodeConfig
	if s.cfg.ExitNodeID != "" && s.cfg.ExitNodeID != selfNodeID {
		if _, ok := s.nodes[s.cfg.ExitNodeID]; ok {
			exitNode = &api.ExitNodeConfig{
				Mode:          s.cfg.ExitNodeMode,
				NodeID:        s.cfg.ExitNodeID,
				AllowLAN:      s.cfg.ExitNodeAllowLAN,
				AllowInternet: s.cfg.ExitNodeAllowInternet,
				DNSMode:       s.cfg.ExitNodeDNSMode,
			}
		}
	}

	return api.BootstrapConfig{
		Version:     s.bootstrapVersion,
		OverlayCIDR: "100.64.0.0/10",
		Node:        sanitizeNode(selfNode),
		Peers:       peers,
		DNS: api.DNSConfig{
			Domain:      s.cfg.DNSDomain,
			Nameservers: []string{"100.64.0.53"},
		},
		Routes: routes,
		Relays: relays,
		ACL: api.ACLSnapshot{
			Version:       s.bootstrapVersion,
			DefaultAction: "deny",
		},
		ExitNode: exitNode,
	}
}

func (s *MemoryStore) directAttemptRecoveryStatesForNodeLocked(nodeID string, now time.Time) []api.PeerRecoveryState {
	selfNode, ok := s.nodes[nodeID]
	if !ok {
		return nil
	}
	selfStates := s.peerTransports[nodeID]
	policy := directAttemptPolicyFromConfig(s.cfg)
	selfCandidates := freshDirectCandidateAddressesWithPolicy(selfNode, now, policy)
	states := make([]api.PeerRecoveryState, 0, len(s.nodes))
	for peerID, peerNode := range s.nodes {
		if peerID == nodeID {
			continue
		}
		peerCandidates := freshDirectCandidateAddressesWithPolicy(peerNode, now, policy)
		peerState := api.PeerTransportState{}
		if reportedByPeer, ok := s.peerTransports[peerID]; ok {
			peerState = reportedByPeer[nodeID]
		}
		latestAttempt, hasAttempt := s.latestDirectAttemptForPairLocked(nodeID, peerID, now)
		var latestAttemptPtr *directAttemptPair
		if hasAttempt {
			latestAttemptPtr = &latestAttempt
		}
		recoveryState := recoveryStateForPeer(selfNode, peerNode, selfCandidates, peerCandidates, selfStates[peerID], peerState, now, policy, latestAttemptPtr)
		states = append(states, recoveryState)
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].PeerNodeID < states[j].PeerNodeID
	})
	return states
}

func (s *MemoryStore) latestDirectAttemptForPairLocked(leftNodeID, rightNodeID string, now time.Time) (directAttemptPair, bool) {
	attempt, ok := s.directAttempts[pairKey(leftNodeID, rightNodeID)]
	if !ok {
		return directAttemptPair{}, false
	}
	if now.After(attempt.ExpiresAt) {
		return directAttemptPair{}, false
	}
	return attempt, true
}

func (s *MemoryStore) pruneDirectAttemptsLocked(now time.Time) {
	for key, attempt := range s.directAttempts {
		if now.After(attempt.ExpiresAt) {
			delete(s.directAttempts, key)
		}
	}
}

func (s *MemoryStore) scheduleDirectAttemptsLocked(now time.Time, nodeID string) {
	policy := directAttemptPolicyFromConfig(s.cfg)
	node, ok := s.nodes[nodeID]
	if !ok || !isNodeOnlineWithPolicy(node, now, policy) {
		return
	}
	nodeCandidates := freshDirectCandidateAddressesWithPolicy(node, now, policy)
	if len(nodeCandidates) == 0 {
		return
	}

	nodeTransportStates := s.peerTransports[node.ID]
	for _, peer := range s.nodes {
		if peer.ID == nodeID {
			continue
		}
		if !isNodeOnlineWithPolicy(peer, now, policy) {
			continue
		}
		peerCandidates := freshDirectCandidateAddressesWithPolicy(peer, now, policy)
		if len(peerCandidates) == 0 {
			continue
		}
		key := pairKey(node.ID, peer.ID)
		if existing, ok := s.directAttempts[key]; ok && now.Before(existing.ExpiresAt) {
			continue
		}
		peerTransportStates := s.peerTransports[peer.ID]
		reason, schedule := directAttemptReasonWithPolicy(nodeTransportStates[peer.ID], peerTransportStates[node.ID], now, policy)
		if !schedule {
			continue
		}
		left, right := node, peer
		leftCandidates, rightCandidates := nodeCandidates, peerCandidates
		if pairKey(node.ID, peer.ID) != node.ID+"|"+peer.ID {
			left, right = peer, node
			leftCandidates, rightCandidates = peerCandidates, nodeCandidates
		}
		s.directAttempts[key] = newDirectAttemptPair(now, left, right, leftCandidates, rightCandidates, reason, policy)
	}
}

func (s *MemoryStore) directAttemptsForNodeLocked(nodeID string, now time.Time) []api.DirectAttemptInstruction {
	attempts := make([]api.DirectAttemptInstruction, 0, len(s.directAttempts))
	for _, attempt := range s.directAttempts {
		if now.After(attempt.ExpiresAt) {
			continue
		}
		instruction, ok := attempt.instructionFor(nodeID)
		if !ok {
			continue
		}
		attempts = append(attempts, instruction)
	}
	sortDirectAttempts(attempts)
	return attempts
}

func (s *MemoryStore) nextOverlaySlotLocked() int {
	s.ipCounter++
	return s.ipCounter + 9
}

func sanitizeNode(node api.Node) api.Node {
	node.AuthToken = ""
	node.Endpoints = append([]string(nil), node.Endpoints...)
	node.EndpointRecords = append([]api.EndpointObservation(nil), node.EndpointRecords...)
	return node
}

func prefixesOverlap(a, b netip.Prefix) bool {
	a = a.Masked()
	b = b.Masked()
	return a.Contains(b.Addr()) || b.Contains(a.Addr())
}

func overlayIPFromSlot(slot int) string {
	return fmt.Sprintf("100.64.%d.%d", slot/250, slot%250)
}

func allowedIPsForNode(nodeID, overlayIP string, routes []api.Route) []string {
	allowedIPs := []string{overlayIP + "/32"}
	for _, route := range routes {
		if route.ViaNodeID == nodeID {
			allowedIPs = append(allowedIPs, route.NetworkCIDR)
		}
	}
	return allowedIPs
}

func deriveRelayRegion(address string) string {
	switch {
	case strings.Contains(address, "ap"):
		return "ap"
	case strings.Contains(address, "us"):
		return "us"
	case strings.Contains(address, "eu"):
		return "eu"
	default:
		return "global"
	}
}

func hashPassword(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func newID(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, randomHex(4))
}

func newToken(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, randomHex(16))
}

func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
