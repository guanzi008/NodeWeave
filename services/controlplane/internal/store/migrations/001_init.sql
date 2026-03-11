CREATE TABLE IF NOT EXISTS metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    status TEXT NOT NULL,
    role TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);

CREATE TABLE IF NOT EXISTS devices (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    platform TEXT NOT NULL,
    version TEXT NOT NULL,
    status TEXT NOT NULL,
    capabilities_json TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS nodes (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL UNIQUE,
    overlay_ip TEXT NOT NULL UNIQUE,
    public_key TEXT NOT NULL,
    relay_region TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    endpoints_json TEXT NOT NULL DEFAULT '[]',
    endpoint_records_json TEXT NOT NULL DEFAULT '[]',
    last_seen_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    auth_token TEXT NOT NULL,
    FOREIGN KEY (device_id) REFERENCES devices(id)
);

CREATE TABLE IF NOT EXISTS routes (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    network_cidr TEXT NOT NULL UNIQUE,
    via_node_id TEXT NOT NULL,
    priority INTEGER NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    FOREIGN KEY (via_node_id) REFERENCES nodes(id)
);

CREATE TABLE IF NOT EXISTS dns_zones (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);

CREATE TABLE IF NOT EXISTS node_nat_reports (
    node_id TEXT PRIMARY KEY,
    report_json TEXT NOT NULL DEFAULT '{}',
    updated_at TEXT NOT NULL,
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

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
);

CREATE TABLE IF NOT EXISTS node_peer_transport_states (
    node_id TEXT NOT NULL,
    peer_node_id TEXT NOT NULL,
    state_json TEXT NOT NULL DEFAULT '{}',
    updated_at TEXT NOT NULL,
    PRIMARY KEY (node_id, peer_node_id),
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE,
    FOREIGN KEY (peer_node_id) REFERENCES nodes(id) ON DELETE CASCADE
);
