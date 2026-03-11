package store

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"nodeweave/packages/contracts/go/api"
	"nodeweave/services/controlplane/internal/config"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type SQLiteStore struct {
	db  *sql.DB
	cfg config.Config
}

func NewSQLiteStore(cfg config.Config) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.SQLitePath), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite dir: %w", err)
	}

	db, err := sql.Open("sqlite", cfg.SQLitePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	store := &SQLiteStore{db: db, cfg: cfg}
	if err := store.configure(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.seed(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) configure() error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA foreign_keys = ON;",
	}
	for _, pragma := range pragmas {
		if _, err := s.db.Exec(pragma); err != nil {
			return fmt.Errorf("apply sqlite pragma: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) migrate() error {
	raw, err := migrationFiles.ReadFile("migrations/001_init.sql")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	if _, err := s.db.Exec(string(raw)); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	if err := s.ensureNodeEndpointRecordsColumn(); err != nil {
		return err
	}
	if err := s.ensureNodeNATReportsTable(); err != nil {
		return err
	}
	if err := s.ensureDirectAttemptsTable(); err != nil {
		return err
	}
	if err := s.ensureNodePeerTransportStatesTable(); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) ensureNodeEndpointRecordsColumn() error {
	rows, err := s.db.Query(`PRAGMA table_info(nodes)`)
	if err != nil {
		return fmt.Errorf("inspect nodes table: %w", err)
	}
	defer rows.Close()

	hasColumn := false
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("scan nodes table info: %w", err)
		}
		if name == "endpoint_records_json" {
			hasColumn = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate nodes table info: %w", err)
	}
	if hasColumn {
		return nil
	}
	if _, err := s.db.Exec(`ALTER TABLE nodes ADD COLUMN endpoint_records_json TEXT NOT NULL DEFAULT '[]'`); err != nil {
		return fmt.Errorf("add endpoint_records_json column: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ensureNodeNATReportsTable() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS node_nat_reports (
			node_id TEXT PRIMARY KEY,
			report_json TEXT NOT NULL DEFAULT '{}',
			updated_at TEXT NOT NULL,
			FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return fmt.Errorf("ensure node_nat_reports table: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ensureDirectAttemptsTable() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS direct_attempts (
			attempt_id TEXT PRIMARY KEY,
			pair_key TEXT NOT NULL,
			node_a_id TEXT NOT NULL,
			node_b_id TEXT NOT NULL,
			node_a_candidates_json TEXT NOT NULL DEFAULT '[]',
			node_b_candidates_json TEXT NOT NULL DEFAULT '[]',
			execute_at TEXT NOT NULL,
			window_millis INTEGER NOT NULL,
			burst_interval_millis INTEGER NOT NULL,
			reason TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			FOREIGN KEY (node_a_id) REFERENCES nodes(id) ON DELETE CASCADE,
			FOREIGN KEY (node_b_id) REFERENCES nodes(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return fmt.Errorf("ensure direct_attempts table: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ensureNodePeerTransportStatesTable() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS node_peer_transport_states (
			node_id TEXT NOT NULL,
			peer_node_id TEXT NOT NULL,
			state_json TEXT NOT NULL DEFAULT '{}',
			updated_at TEXT NOT NULL,
			PRIMARY KEY (node_id, peer_node_id),
			FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE,
			FOREIGN KEY (peer_node_id) REFERENCES nodes(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return fmt.Errorf("ensure node_peer_transport_states table: %w", err)
	}
	return nil
}

func (s *SQLiteStore) seed() error {
	now := time.Now().UTC()
	passwordHash := hashPassword(s.cfg.AdminPassword)

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin seed tx: %w", err)
	}
	defer rollback(tx)

	if _, err := tx.Exec(`
		INSERT INTO tenants (id, name, status, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, status = excluded.status
	`, "tenant-default", "Default Tenant", "active", now.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("seed tenant: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO users (id, tenant_id, email, display_name, status, role, password_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET email = excluded.email, display_name = excluded.display_name, status = excluded.status, role = excluded.role, password_hash = excluded.password_hash
	`, "user-admin", "tenant-default", s.cfg.AdminEmail, "Platform Admin", "active", "admin", passwordHash, now.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("seed admin user: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO dns_zones (id, tenant_id, name, type, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, type = excluded.type
	`, "zone-internal", "tenant-default", s.cfg.DNSDomain, "internal", now.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("seed dns zone: %w", err)
	}

	if err := s.ensureMetadata(tx, "bootstrap_version", "1"); err != nil {
		return err
	}
	if err := s.ensureMetadata(tx, "overlay_next_ip", "10"); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seed tx: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ensureMetadata(tx *sql.Tx, key, value string) error {
	if _, err := tx.Exec(`
		INSERT INTO metadata (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO NOTHING
	`, key, value, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("seed metadata %s: %w", key, err)
	}
	return nil
}

func (s *SQLiteStore) ValidateAdminCredentials(email, password string) bool {
	row := s.db.QueryRow(`SELECT password_hash FROM users WHERE email = ? AND role = 'admin' AND status = 'active'`, email)
	var passwordHash string
	if err := row.Scan(&passwordHash); err != nil {
		return false
	}
	return passwordHash == hashPassword(password)
}

func (s *SQLiteStore) AdminUser() api.User {
	row := s.db.QueryRow(`
		SELECT id, tenant_id, email, display_name, status, role, created_at
		FROM users
		WHERE id = ?
	`, "user-admin")

	user, err := scanUser(row)
	if err != nil {
		return api.User{}
	}
	return user
}

func (s *SQLiteStore) CreateDeviceAndNode(req api.DeviceRegistrationRequest) (api.DeviceRegistrationResponse, error) {
	if strings.TrimSpace(req.RegistrationToken) != s.cfg.RegistrationToken {
		return api.DeviceRegistrationResponse{}, ErrUnauthorized
	}
	if strings.TrimSpace(req.DeviceName) == "" || strings.TrimSpace(req.Platform) == "" || strings.TrimSpace(req.PublicKey) == "" {
		return api.DeviceRegistrationResponse{}, fmt.Errorf("%w: device_name, platform and public_key are required", ErrInvalid)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return api.DeviceRegistrationResponse{}, fmt.Errorf("begin registration tx: %w", err)
	}
	defer rollback(tx)

	now := time.Now().UTC()
	tenantID := "tenant-default"
	if req.TenantID != "" {
		tenantID = req.TenantID
	}
	userID := "user-admin"
	if req.UserID != "" {
		userID = req.UserID
	}

	if tenantID != "tenant-default" || userID != "user-admin" {
		return api.DeviceRegistrationResponse{}, ErrNotFound
	}

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

	capabilitiesJSON, err := json.Marshal(device.Capabilities)
	if err != nil {
		return api.DeviceRegistrationResponse{}, fmt.Errorf("marshal capabilities: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO devices (id, tenant_id, user_id, name, platform, version, status, capabilities_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, device.ID, device.TenantID, device.UserID, device.Name, device.Platform, device.Version, device.Status, string(capabilitiesJSON), device.CreatedAt.Format(time.RFC3339Nano)); err != nil {
		return api.DeviceRegistrationResponse{}, fmt.Errorf("insert device: %w", err)
	}

	nextSlot, err := s.nextOverlaySlotTx(tx)
	if err != nil {
		return api.DeviceRegistrationResponse{}, err
	}

	node := api.Node{
		ID:         newID("node"),
		DeviceID:   device.ID,
		OverlayIP:  overlayIPFromSlot(nextSlot),
		PublicKey:  req.PublicKey,
		Status:     "registered",
		CreatedAt:  now,
		LastSeenAt: now,
		AuthToken:  newToken("node"),
	}

	if _, err := tx.Exec(`
		INSERT INTO nodes (id, device_id, overlay_ip, public_key, relay_region, status, endpoints_json, endpoint_records_json, last_seen_at, created_at, auth_token)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, node.ID, node.DeviceID, node.OverlayIP, node.PublicKey, node.RelayRegion, node.Status, "[]", "[]", node.LastSeenAt.Format(time.RFC3339Nano), node.CreatedAt.Format(time.RFC3339Nano), node.AuthToken); err != nil {
		return api.DeviceRegistrationResponse{}, fmt.Errorf("insert node: %w", err)
	}

	bootstrap, err := s.currentBootstrapTx(tx, node.ID)
	if err != nil {
		return api.DeviceRegistrationResponse{}, err
	}

	if err := tx.Commit(); err != nil {
		return api.DeviceRegistrationResponse{}, fmt.Errorf("commit registration tx: %w", err)
	}

	return api.DeviceRegistrationResponse{
		Device:    device,
		Node:      sanitizeNode(node),
		NodeToken: node.AuthToken,
		Bootstrap: bootstrap,
	}, nil
}

func (s *SQLiteStore) UpdateHeartbeat(nodeID, token string, req api.HeartbeatRequest) (api.HeartbeatResponse, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return api.HeartbeatResponse{}, fmt.Errorf("begin heartbeat tx: %w", err)
	}
	defer rollback(tx)

	node, err := s.loadNodeTx(tx, nodeID)
	if err != nil {
		return api.HeartbeatResponse{}, err
	}
	if token == "" || token != node.AuthToken {
		return api.HeartbeatResponse{}, ErrUnauthorized
	}

	now := time.Now().UTC()
	endpointRecords, endpoints := api.NormalizeEndpointObservations(now, req.Endpoints, req.EndpointRecords)
	endpointsJSON, err := json.Marshal(endpoints)
	if err != nil {
		return api.HeartbeatResponse{}, fmt.Errorf("marshal endpoints: %w", err)
	}
	endpointRecordsJSON, err := json.Marshal(endpointRecords)
	if err != nil {
		return api.HeartbeatResponse{}, fmt.Errorf("marshal endpoint records: %w", err)
	}

	if req.RelayRegion != "" {
		node.RelayRegion = req.RelayRegion
	}
	if req.Status != "" {
		node.Status = req.Status
	} else {
		node.Status = "online"
	}
	publicKeyChanged := false
	if publicKey := strings.TrimSpace(req.PublicKey); publicKey != "" && publicKey != node.PublicKey {
		node.PublicKey = publicKey
		publicKeyChanged = true
	}
	endpointsChanged := !api.EndpointObservationsEqual(node.EndpointRecords, endpointRecords)
	node.Endpoints = endpoints
	node.EndpointRecords = endpointRecords
	node.LastSeenAt = now
	natReport := sanitizeNATReport(now, req.NATReport)
	peerTransportStates := sanitizePeerTransportStates(now, req.PeerTransportStates)

	if _, err := tx.Exec(`
		UPDATE nodes
		SET public_key = ?, relay_region = ?, status = ?, endpoints_json = ?, endpoint_records_json = ?, last_seen_at = ?
		WHERE id = ?
	`, node.PublicKey, node.RelayRegion, node.Status, string(endpointsJSON), string(endpointRecordsJSON), node.LastSeenAt.Format(time.RFC3339Nano), node.ID); err != nil {
		return api.HeartbeatResponse{}, fmt.Errorf("update node heartbeat: %w", err)
	}
	if publicKeyChanged || endpointsChanged {
		if err := s.incrementBootstrapVersionTx(tx); err != nil {
			return api.HeartbeatResponse{}, err
		}
	}
	if err := s.saveNATReportTx(tx, node.ID, natReport); err != nil {
		return api.HeartbeatResponse{}, err
	}
	if err := s.replacePeerTransportStatesTx(tx, node.ID, peerTransportStates); err != nil {
		return api.HeartbeatResponse{}, err
	}
	if err := s.deleteExpiredDirectAttemptsTx(tx, now); err != nil {
		return api.HeartbeatResponse{}, err
	}
	if err := s.scheduleDirectAttemptsTx(tx, now, node.ID); err != nil {
		return api.HeartbeatResponse{}, err
	}

	bootstrapVersion, err := s.bootstrapVersionTx(tx)
	if err != nil {
		return api.HeartbeatResponse{}, err
	}
	directAttempts, err := s.directAttemptsForNodeTx(tx, node.ID, now)
	if err != nil {
		return api.HeartbeatResponse{}, err
	}

	if err := tx.Commit(); err != nil {
		return api.HeartbeatResponse{}, fmt.Errorf("commit heartbeat tx: %w", err)
	}

	return api.HeartbeatResponse{
		Node:             sanitizeNode(node),
		BootstrapVersion: bootstrapVersion,
		DirectAttempts:   directAttempts,
	}, nil
}

func (s *SQLiteStore) GetBootstrap(nodeID, token string) (api.BootstrapConfig, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return api.BootstrapConfig{}, fmt.Errorf("begin bootstrap tx: %w", err)
	}
	defer rollback(tx)

	node, err := s.loadNodeTx(tx, nodeID)
	if err != nil {
		return api.BootstrapConfig{}, err
	}
	if token == "" || token != node.AuthToken {
		return api.BootstrapConfig{}, ErrUnauthorized
	}

	bootstrap, err := s.currentBootstrapTx(tx, nodeID)
	if err != nil {
		return api.BootstrapConfig{}, err
	}
	if err := tx.Commit(); err != nil {
		return api.BootstrapConfig{}, fmt.Errorf("commit bootstrap tx: %w", err)
	}
	return bootstrap, nil
}

func (s *SQLiteStore) ListNodes() []api.Node {
	rows, err := s.db.Query(`
		SELECT id, device_id, overlay_ip, public_key, relay_region, status, endpoints_json, endpoint_records_json, last_seen_at, created_at, auth_token
		FROM nodes
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	nodes := make([]api.Node, 0)
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			continue
		}
		nodes = append(nodes, sanitizeNode(node))
	}
	return nodes
}

func (s *SQLiteStore) ListRoutes() []api.Route {
	rows, err := s.db.Query(`
		SELECT id, tenant_id, network_cidr, via_node_id, priority, status, created_at
		FROM routes
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	routes := make([]api.Route, 0)
	for rows.Next() {
		route, err := scanRoute(rows)
		if err != nil {
			continue
		}
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

func (s *SQLiteStore) CreateRoute(req api.CreateRouteRequest) (api.Route, error) {
	if strings.TrimSpace(req.NetworkCIDR) == "" || strings.TrimSpace(req.ViaNodeID) == "" {
		return api.Route{}, fmt.Errorf("%w: network_cidr and via_node_id are required", ErrInvalid)
	}

	prefix, err := netip.ParsePrefix(req.NetworkCIDR)
	if err != nil {
		return api.Route{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return api.Route{}, fmt.Errorf("begin create route tx: %w", err)
	}
	defer rollback(tx)

	tenantID := "tenant-default"
	if req.TenantID != "" {
		tenantID = req.TenantID
	}
	if tenantID != "tenant-default" {
		return api.Route{}, ErrNotFound
	}

	if _, err := s.loadNodeTx(tx, req.ViaNodeID); err != nil {
		return api.Route{}, err
	}

	existingRoutes, err := s.routesTx(tx)
	if err != nil {
		return api.Route{}, err
	}
	for _, route := range existingRoutes {
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

	if _, err := tx.Exec(`
		INSERT INTO routes (id, tenant_id, network_cidr, via_node_id, priority, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, route.ID, route.TenantID, route.NetworkCIDR, route.ViaNodeID, route.Priority, route.Status, route.CreatedAt.Format(time.RFC3339Nano)); err != nil {
		return api.Route{}, fmt.Errorf("insert route: %w", err)
	}

	if err := s.incrementBootstrapVersionTx(tx); err != nil {
		return api.Route{}, err
	}
	if err := tx.Commit(); err != nil {
		return api.Route{}, fmt.Errorf("commit create route tx: %w", err)
	}
	return route, nil
}

func (s *SQLiteStore) ListDNSZones() []api.DNSZone {
	rows, err := s.db.Query(`
		SELECT id, tenant_id, name, type, created_at
		FROM dns_zones
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	zones := make([]api.DNSZone, 0)
	for rows.Next() {
		zone, err := scanDNSZone(rows)
		if err != nil {
			continue
		}
		zones = append(zones, zone)
	}
	return zones
}

func (s *SQLiteStore) nextOverlaySlotTx(tx *sql.Tx) (int, error) {
	slot, err := s.metadataIntTx(tx, "overlay_next_ip")
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE metadata SET value = ?, updated_at = ? WHERE key = ?`, fmt.Sprintf("%d", slot+1), time.Now().UTC().Format(time.RFC3339Nano), "overlay_next_ip"); err != nil {
		return 0, fmt.Errorf("update overlay_next_ip: %w", err)
	}
	return slot, nil
}

func (s *SQLiteStore) currentBootstrapTx(tx *sql.Tx, selfNodeID string) (api.BootstrapConfig, error) {
	version, err := s.bootstrapVersionTx(tx)
	if err != nil {
		return api.BootstrapConfig{}, err
	}
	routes, err := s.routesTx(tx)
	if err != nil {
		return api.BootstrapConfig{}, err
	}

	relays := make([]api.RelayNode, 0, len(s.cfg.RelayAddresses))
	for _, address := range s.cfg.RelayAddresses {
		relays = append(relays, api.RelayNode{
			Region:  deriveRelayRegion(address),
			Address: address,
		})
	}

	selfNode, err := s.loadNodeTx(tx, selfNodeID)
	if err != nil {
		return api.BootstrapConfig{}, err
	}

	peers, err := s.peersTx(tx, selfNodeID, routes)
	if err != nil {
		return api.BootstrapConfig{}, err
	}

	var exitNode *api.ExitNodeConfig
	if s.cfg.ExitNodeID != "" && s.cfg.ExitNodeID != selfNodeID {
		if _, err := s.loadNodeTx(tx, s.cfg.ExitNodeID); err == nil {
			exitNode = &api.ExitNodeConfig{
				Mode:          s.cfg.ExitNodeMode,
				NodeID:        s.cfg.ExitNodeID,
				AllowLAN:      s.cfg.ExitNodeAllowLAN,
				AllowInternet: s.cfg.ExitNodeAllowInternet,
				DNSMode:       s.cfg.ExitNodeDNSMode,
			}
		} else if err != ErrNotFound {
			return api.BootstrapConfig{}, err
		}
	}

	return api.BootstrapConfig{
		Version:     version,
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
			Version:       version,
			DefaultAction: "deny",
		},
		ExitNode: exitNode,
	}, nil
}

func (s *SQLiteStore) peersTx(tx *sql.Tx, selfNodeID string, routes []api.Route) ([]api.Peer, error) {
	rows, err := tx.Query(`
		SELECT id, device_id, overlay_ip, public_key, relay_region, status, endpoints_json, endpoint_records_json, last_seen_at, created_at, auth_token
		FROM nodes
		WHERE id <> ?
		ORDER BY id ASC
	`, selfNodeID)
	if err != nil {
		return nil, fmt.Errorf("query peers: %w", err)
	}
	defer rows.Close()

	natReports, err := s.loadAllNATReportsTx(tx)
	if err != nil {
		return nil, err
	}
	transportStates, err := s.loadPeerTransportStatesForTargetTx(tx, selfNodeID)
	if err != nil {
		return nil, err
	}

	peers := make([]api.Peer, 0)
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
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
		if report, ok := natReports[node.ID]; ok {
			peer.NATMappingBehavior, peer.NATReachable, peer.NATReportedAt = natSummaryForPeer(report)
		}
		if transportState, ok := transportStates[node.ID]; ok {
			peer.ObservedTransportKind = transportState.ActiveKind
			peer.ObservedTransportAddress = transportState.ActiveAddress
			peer.ObservedTransportReportedAt = transportState.ReportedAt
			peer.ObservedLastDirectAttemptResult = transportState.LastDirectAttemptResult
		}
		peers = append(peers, peer)
	}
	return peers, nil
}

func (s *SQLiteStore) bootstrapVersionTx(tx *sql.Tx) (int, error) {
	return s.metadataIntTx(tx, "bootstrap_version")
}

func (s *SQLiteStore) incrementBootstrapVersionTx(tx *sql.Tx) error {
	current, err := s.metadataIntTx(tx, "bootstrap_version")
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE metadata SET value = ?, updated_at = ? WHERE key = ?`, fmt.Sprintf("%d", current+1), time.Now().UTC().Format(time.RFC3339Nano), "bootstrap_version"); err != nil {
		return fmt.Errorf("increment bootstrap_version: %w", err)
	}
	return nil
}

func (s *SQLiteStore) metadataIntTx(tx *sql.Tx, key string) (int, error) {
	var raw string
	if err := tx.QueryRow(`SELECT value FROM metadata WHERE key = ?`, key).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("query metadata %s: %w", key, err)
	}
	var value int
	if _, err := fmt.Sscanf(raw, "%d", &value); err != nil {
		return 0, fmt.Errorf("parse metadata %s: %w", key, err)
	}
	return value, nil
}

func (s *SQLiteStore) routesTx(tx *sql.Tx) ([]api.Route, error) {
	rows, err := tx.Query(`
		SELECT id, tenant_id, network_cidr, via_node_id, priority, status, created_at
		FROM routes
	`)
	if err != nil {
		return nil, fmt.Errorf("query routes: %w", err)
	}
	defer rows.Close()

	routes := make([]api.Route, 0)
	for rows.Next() {
		route, err := scanRoute(rows)
		if err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Priority == routes[j].Priority {
			return routes[i].CreatedAt.Before(routes[j].CreatedAt)
		}
		return routes[i].Priority > routes[j].Priority
	})
	return routes, nil
}

func (s *SQLiteStore) saveNATReportTx(tx *sql.Tx, nodeID string, report api.NATReport) error {
	raw, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal nat report: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO node_nat_reports (node_id, report_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET report_json = excluded.report_json, updated_at = excluded.updated_at
	`, nodeID, string(raw), report.GeneratedAt.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("save nat report: %w", err)
	}
	return nil
}

func (s *SQLiteStore) loadAllNATReportsTx(tx *sql.Tx) (map[string]api.NATReport, error) {
	rows, err := tx.Query(`SELECT node_id, report_json FROM node_nat_reports`)
	if err != nil {
		return nil, fmt.Errorf("query nat reports: %w", err)
	}
	defer rows.Close()

	reports := make(map[string]api.NATReport)
	for rows.Next() {
		var (
			nodeID  string
			rawJSON string
		)
		if err := rows.Scan(&nodeID, &rawJSON); err != nil {
			return nil, fmt.Errorf("scan nat report: %w", err)
		}
		var report api.NATReport
		if rawJSON != "" {
			if err := json.Unmarshal([]byte(rawJSON), &report); err != nil {
				return nil, fmt.Errorf("parse nat report for %s: %w", nodeID, err)
			}
		}
		reports[nodeID] = sanitizeNATReport(time.Now().UTC(), report)
	}
	return reports, rows.Err()
}

func (s *SQLiteStore) replacePeerTransportStatesTx(tx *sql.Tx, nodeID string, states []api.PeerTransportState) error {
	if _, err := tx.Exec(`DELETE FROM node_peer_transport_states WHERE node_id = ?`, nodeID); err != nil {
		return fmt.Errorf("delete peer transport states: %w", err)
	}
	for _, state := range states {
		raw, err := json.Marshal(state)
		if err != nil {
			return fmt.Errorf("marshal peer transport state: %w", err)
		}
		if _, err := tx.Exec(`
			INSERT INTO node_peer_transport_states (node_id, peer_node_id, state_json, updated_at)
			VALUES (?, ?, ?, ?)
		`, nodeID, state.PeerNodeID, string(raw), state.ReportedAt.Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert peer transport state: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) loadPeerTransportStatesForTargetTx(tx *sql.Tx, targetNodeID string) (map[string]api.PeerTransportState, error) {
	rows, err := tx.Query(`
		SELECT node_id, state_json
		FROM node_peer_transport_states
		WHERE peer_node_id = ?
	`, targetNodeID)
	if err != nil {
		return nil, fmt.Errorf("query peer transport states for target: %w", err)
	}
	defer rows.Close()

	states := make(map[string]api.PeerTransportState)
	for rows.Next() {
		var (
			nodeID  string
			rawJSON string
		)
		if err := rows.Scan(&nodeID, &rawJSON); err != nil {
			return nil, fmt.Errorf("scan peer transport state: %w", err)
		}
		var state api.PeerTransportState
		if rawJSON != "" {
			if err := json.Unmarshal([]byte(rawJSON), &state); err != nil {
				return nil, fmt.Errorf("parse peer transport state: %w", err)
			}
		}
		states[nodeID] = state
	}
	return states, rows.Err()
}

func (s *SQLiteStore) loadPeerTransportStatesByNodeTx(tx *sql.Tx, nodeID string) (map[string]api.PeerTransportState, error) {
	rows, err := tx.Query(`
		SELECT peer_node_id, state_json
		FROM node_peer_transport_states
		WHERE node_id = ?
	`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("query peer transport states by node: %w", err)
	}
	defer rows.Close()

	states := make(map[string]api.PeerTransportState)
	for rows.Next() {
		var (
			peerNodeID string
			rawJSON    string
		)
		if err := rows.Scan(&peerNodeID, &rawJSON); err != nil {
			return nil, fmt.Errorf("scan peer transport state by node: %w", err)
		}
		var state api.PeerTransportState
		if rawJSON != "" {
			if err := json.Unmarshal([]byte(rawJSON), &state); err != nil {
				return nil, fmt.Errorf("parse peer transport state by node: %w", err)
			}
		}
		states[peerNodeID] = state
	}
	return states, rows.Err()
}

func (s *SQLiteStore) deleteExpiredDirectAttemptsTx(tx *sql.Tx, now time.Time) error {
	if _, err := tx.Exec(`DELETE FROM direct_attempts WHERE expires_at < ?`, now.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("delete expired direct attempts: %w", err)
	}
	return nil
}

func (s *SQLiteStore) directAttemptPairByKeyTx(tx *sql.Tx, key string) (directAttemptPair, bool, error) {
	row := tx.QueryRow(`
		SELECT attempt_id, node_a_id, node_b_id, node_a_candidates_json, node_b_candidates_json, execute_at, window_millis, burst_interval_millis, reason, expires_at
		FROM direct_attempts
		WHERE pair_key = ?
	`, key)
	attempt, err := scanDirectAttemptPair(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return directAttemptPair{}, false, nil
		}
		return directAttemptPair{}, false, err
	}
	return attempt, true, nil
}

func (s *SQLiteStore) insertDirectAttemptTx(tx *sql.Tx, pair directAttemptPair) error {
	leftJSON, err := json.Marshal(pair.NodeACandidates)
	if err != nil {
		return fmt.Errorf("marshal node_a candidates: %w", err)
	}
	rightJSON, err := json.Marshal(pair.NodeBCandidates)
	if err != nil {
		return fmt.Errorf("marshal node_b candidates: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO direct_attempts (
			attempt_id, pair_key, node_a_id, node_b_id, node_a_candidates_json, node_b_candidates_json,
			execute_at, window_millis, burst_interval_millis, reason, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, pair.AttemptID, pairKey(pair.NodeAID, pair.NodeBID), pair.NodeAID, pair.NodeBID, string(leftJSON), string(rightJSON),
		pair.ExecuteAt.Format(time.RFC3339Nano), pair.Window.Milliseconds(), pair.BurstInterval.Milliseconds(), pair.Reason, pair.ExpiresAt.Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("insert direct attempt: %w", err)
	}
	return nil
}

func (s *SQLiteStore) directAttemptsForNodeTx(tx *sql.Tx, nodeID string, now time.Time) ([]api.DirectAttemptInstruction, error) {
	rows, err := tx.Query(`
		SELECT attempt_id, node_a_id, node_b_id, node_a_candidates_json, node_b_candidates_json, execute_at, window_millis, burst_interval_millis, reason, expires_at
		FROM direct_attempts
		WHERE node_a_id = ? OR node_b_id = ?
	`, nodeID, nodeID)
	if err != nil {
		return nil, fmt.Errorf("query direct attempts: %w", err)
	}
	defer rows.Close()

	instructions := make([]api.DirectAttemptInstruction, 0)
	for rows.Next() {
		attempt, err := scanDirectAttemptPair(rows)
		if err != nil {
			return nil, err
		}
		if now.After(attempt.ExpiresAt) {
			continue
		}
		instruction, ok := attempt.instructionFor(nodeID)
		if !ok {
			continue
		}
		instructions = append(instructions, instruction)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate direct attempts: %w", err)
	}
	sortDirectAttempts(instructions)
	return instructions, nil
}

func (s *SQLiteStore) scheduleDirectAttemptsTx(tx *sql.Tx, now time.Time, nodeID string) error {
	node, err := s.loadNodeTx(tx, nodeID)
	if err != nil {
		return err
	}
	if !isNodeOnline(node, now) {
		return nil
	}

	nodeCandidates := freshDirectCandidateAddresses(node, now)
	if len(nodeCandidates) == 0 {
		return nil
	}
	nodeTransportStates, err := s.loadPeerTransportStatesByNodeTx(tx, nodeID)
	if err != nil {
		return err
	}

	rows, err := tx.Query(`
		SELECT id, device_id, overlay_ip, public_key, relay_region, status, endpoints_json, endpoint_records_json, last_seen_at, created_at, auth_token
		FROM nodes
		WHERE id <> ?
	`, nodeID)
	if err != nil {
		return fmt.Errorf("query peer nodes for direct attempts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		peer, err := scanNode(rows)
		if err != nil {
			return err
		}
		if !isNodeOnline(peer, now) {
			continue
		}
		peerCandidates := freshDirectCandidateAddresses(peer, now)
		if len(peerCandidates) == 0 {
			continue
		}
		key := pairKey(node.ID, peer.ID)
		existing, ok, err := s.directAttemptPairByKeyTx(tx, key)
		if err != nil {
			return err
		}
		if ok && now.Before(existing.ExpiresAt) {
			continue
		}
		peerTransportStates, err := s.loadPeerTransportStatesByNodeTx(tx, peer.ID)
		if err != nil {
			return err
		}
		reason, schedule := directAttemptReason(nodeTransportStates[peer.ID], peerTransportStates[node.ID], now)
		if !schedule {
			continue
		}
		left, right := node, peer
		leftCandidates, rightCandidates := nodeCandidates, peerCandidates
		if key != node.ID+"|"+peer.ID {
			left, right = peer, node
			leftCandidates, rightCandidates = peerCandidates, nodeCandidates
		}
		if err := s.insertDirectAttemptTx(tx, newDirectAttemptPair(now, left, right, leftCandidates, rightCandidates, reason)); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate peer nodes for direct attempts: %w", err)
	}
	return nil
}

func (s *SQLiteStore) loadNodeTx(tx *sql.Tx, nodeID string) (api.Node, error) {
	row := tx.QueryRow(`
		SELECT id, device_id, overlay_ip, public_key, relay_region, status, endpoints_json, endpoint_records_json, last_seen_at, created_at, auth_token
		FROM nodes
		WHERE id = ?
	`, nodeID)
	node, err := scanNode(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return api.Node{}, ErrNotFound
		}
		return api.Node{}, err
	}
	return node, nil
}

func scanDirectAttemptPair(row interface{ Scan(dest ...any) error }) (directAttemptPair, error) {
	var (
		pair                directAttemptPair
		nodeACandidatesJSON string
		nodeBCandidatesJSON string
		executeAt           string
		windowMillis        int64
		burstIntervalMillis int64
		expiresAt           string
	)
	if err := row.Scan(
		&pair.AttemptID,
		&pair.NodeAID,
		&pair.NodeBID,
		&nodeACandidatesJSON,
		&nodeBCandidatesJSON,
		&executeAt,
		&windowMillis,
		&burstIntervalMillis,
		&pair.Reason,
		&expiresAt,
	); err != nil {
		return directAttemptPair{}, err
	}
	if err := json.Unmarshal([]byte(nodeACandidatesJSON), &pair.NodeACandidates); err != nil {
		return directAttemptPair{}, fmt.Errorf("parse node_a direct candidates: %w", err)
	}
	if err := json.Unmarshal([]byte(nodeBCandidatesJSON), &pair.NodeBCandidates); err != nil {
		return directAttemptPair{}, fmt.Errorf("parse node_b direct candidates: %w", err)
	}
	parsedExecuteAt, err := time.Parse(time.RFC3339Nano, executeAt)
	if err != nil {
		return directAttemptPair{}, fmt.Errorf("parse direct attempt execute_at: %w", err)
	}
	parsedExpiresAt, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return directAttemptPair{}, fmt.Errorf("parse direct attempt expires_at: %w", err)
	}
	pair.ExecuteAt = parsedExecuteAt
	pair.Window = time.Duration(windowMillis) * time.Millisecond
	pair.BurstInterval = time.Duration(burstIntervalMillis) * time.Millisecond
	pair.ExpiresAt = parsedExpiresAt
	return pair, nil
}

func scanUser(row interface{ Scan(dest ...any) error }) (api.User, error) {
	var user api.User
	var createdAt string
	if err := row.Scan(&user.ID, &user.TenantID, &user.Email, &user.DisplayName, &user.Status, &user.Role, &createdAt); err != nil {
		return api.User{}, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return api.User{}, fmt.Errorf("parse user created_at: %w", err)
	}
	user.CreatedAt = parsed
	return user, nil
}

func scanNode(row interface{ Scan(dest ...any) error }) (api.Node, error) {
	var node api.Node
	var endpointsJSON string
	var endpointRecordsJSON string
	var lastSeenAt string
	var createdAt string
	if err := row.Scan(&node.ID, &node.DeviceID, &node.OverlayIP, &node.PublicKey, &node.RelayRegion, &node.Status, &endpointsJSON, &endpointRecordsJSON, &lastSeenAt, &createdAt, &node.AuthToken); err != nil {
		return api.Node{}, err
	}
	if err := json.Unmarshal([]byte(endpointsJSON), &node.Endpoints); err != nil {
		return api.Node{}, fmt.Errorf("parse node endpoints: %w", err)
	}
	if node.Endpoints == nil {
		node.Endpoints = []string{}
	}
	parsedLastSeenAt, err := time.Parse(time.RFC3339Nano, lastSeenAt)
	if err != nil {
		return api.Node{}, fmt.Errorf("parse node last_seen_at: %w", err)
	}
	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return api.Node{}, fmt.Errorf("parse node created_at: %w", err)
	}
	node.LastSeenAt = parsedLastSeenAt
	node.CreatedAt = parsedCreatedAt
	if endpointRecordsJSON != "" {
		if err := json.Unmarshal([]byte(endpointRecordsJSON), &node.EndpointRecords); err != nil {
			return api.Node{}, fmt.Errorf("parse node endpoint records: %w", err)
		}
	}
	node.EndpointRecords, node.Endpoints = api.NormalizeEndpointObservations(node.LastSeenAt, node.Endpoints, node.EndpointRecords)
	return node, nil
}

func scanRoute(row interface{ Scan(dest ...any) error }) (api.Route, error) {
	var route api.Route
	var createdAt string
	if err := row.Scan(&route.ID, &route.TenantID, &route.NetworkCIDR, &route.ViaNodeID, &route.Priority, &route.Status, &createdAt); err != nil {
		return api.Route{}, err
	}
	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return api.Route{}, fmt.Errorf("parse route created_at: %w", err)
	}
	route.CreatedAt = parsedCreatedAt
	return route, nil
}

func scanDNSZone(row interface{ Scan(dest ...any) error }) (api.DNSZone, error) {
	var zone api.DNSZone
	var createdAt string
	if err := row.Scan(&zone.ID, &zone.TenantID, &zone.Name, &zone.Type, &createdAt); err != nil {
		return api.DNSZone{}, err
	}
	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return api.DNSZone{}, fmt.Errorf("parse zone created_at: %w", err)
	}
	zone.CreatedAt = parsedCreatedAt
	return zone, nil
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}
