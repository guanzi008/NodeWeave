package api

import "time"

type HealthResponse struct {
	Status string `json:"status"`
}

type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type User struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

type Device struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	UserID       string    `json:"user_id"`
	Name         string    `json:"name"`
	Platform     string    `json:"platform"`
	Version      string    `json:"version"`
	Status       string    `json:"status"`
	Capabilities []string  `json:"capabilities,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type Node struct {
	ID              string                `json:"id"`
	DeviceID        string                `json:"device_id"`
	OverlayIP       string                `json:"overlay_ip"`
	PublicKey       string                `json:"public_key"`
	RelayRegion     string                `json:"relay_region"`
	Status          string                `json:"status"`
	Endpoints       []string              `json:"endpoints,omitempty"`
	EndpointRecords []EndpointObservation `json:"endpoint_records,omitempty"`
	LastSeenAt      time.Time             `json:"last_seen_at"`
	CreatedAt       time.Time             `json:"created_at"`
	AuthToken       string                `json:"-"`
}

type NATSample struct {
	Server           string `json:"server"`
	Status           string `json:"status"`
	RTTMillis        int64  `json:"rtt_millis,omitempty"`
	ReflexiveAddress string `json:"reflexive_address,omitempty"`
	Error            string `json:"error,omitempty"`
}

type NATReport struct {
	GeneratedAt              time.Time   `json:"generated_at,omitempty"`
	MappingBehavior          string      `json:"mapping_behavior,omitempty"`
	SampleCount              int         `json:"sample_count,omitempty"`
	SelectedReflexiveAddress string      `json:"selected_reflexive_address,omitempty"`
	Reachable                bool        `json:"reachable"`
	Samples                  []NATSample `json:"samples,omitempty"`
}

type PeerTransportState struct {
	PeerNodeID                string    `json:"peer_node_id"`
	ActiveKind                string    `json:"active_kind,omitempty"`
	ActiveAddress             string    `json:"active_address,omitempty"`
	ReportedAt                time.Time `json:"reported_at,omitempty"`
	LastDirectAttemptAt       time.Time `json:"last_direct_attempt_at,omitempty"`
	LastDirectAttemptResult   string    `json:"last_direct_attempt_result,omitempty"`
	LastDirectSuccessAt       time.Time `json:"last_direct_success_at,omitempty"`
	ConsecutiveDirectFailures int       `json:"consecutive_direct_failures,omitempty"`
}

type PeerRecoveryState struct {
	PeerNodeID                 string    `json:"peer_node_id"`
	Blocked                    bool      `json:"blocked"`
	BlockReason                string    `json:"block_reason,omitempty"`
	BlockedUntil               time.Time `json:"blocked_until,omitempty"`
	NextProbeAt                time.Time `json:"next_probe_at,omitempty"`
	ProbeLimited               bool      `json:"probe_limited,omitempty"`
	ProbeBudget                int       `json:"probe_budget,omitempty"`
	ProbeFailures              int       `json:"probe_failures,omitempty"`
	ProbeRemaining             int       `json:"probe_remaining,omitempty"`
	ProbeRefillAt              time.Time `json:"probe_refill_at,omitempty"`
	LastIssuedAttemptID        string    `json:"last_issued_attempt_id,omitempty"`
	LastIssuedAttemptReason    string    `json:"last_issued_attempt_reason,omitempty"`
	LastIssuedAttemptAt        time.Time `json:"last_issued_attempt_at,omitempty"`
	LastIssuedAttemptExecuteAt time.Time `json:"last_issued_attempt_execute_at,omitempty"`
}

type Route struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	NetworkCIDR string    `json:"network_cidr"`
	ViaNodeID   string    `json:"via_node_id"`
	Priority    int       `json:"priority"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type DNSZone struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}

type Peer struct {
	NodeID                                           string                `json:"node_id"`
	OverlayIP                                        string                `json:"overlay_ip"`
	PublicKey                                        string                `json:"public_key"`
	Endpoints                                        []string              `json:"endpoints,omitempty"`
	EndpointRecords                                  []EndpointObservation `json:"endpoint_records,omitempty"`
	RelayRegion                                      string                `json:"relay_region"`
	AllowedIPs                                       []string              `json:"allowed_ips,omitempty"`
	Status                                           string                `json:"status"`
	LastSeenAt                                       time.Time             `json:"last_seen_at"`
	NATMappingBehavior                               string                `json:"nat_mapping_behavior,omitempty"`
	NATReachable                                     bool                  `json:"nat_reachable"`
	NATReportedAt                                    time.Time             `json:"nat_reported_at,omitempty"`
	ObservedTransportKind                            string                `json:"observed_transport_kind,omitempty"`
	ObservedTransportAddress                         string                `json:"observed_transport_address,omitempty"`
	ObservedTransportReportedAt                      time.Time             `json:"observed_transport_reported_at,omitempty"`
	ObservedLastDirectAttemptAt                      time.Time             `json:"observed_last_direct_attempt_at,omitempty"`
	ObservedLastDirectAttemptResult                  string                `json:"observed_last_direct_attempt_result,omitempty"`
	ObservedLastDirectSuccessAt                      time.Time             `json:"observed_last_direct_success_at,omitempty"`
	ObservedConsecutiveDirectFailures                int                   `json:"observed_consecutive_direct_failures,omitempty"`
	ObservedDirectRecoveryBlocked                    bool                  `json:"observed_direct_recovery_blocked,omitempty"`
	ObservedDirectRecoveryBlockReason                string                `json:"observed_direct_recovery_block_reason,omitempty"`
	ObservedDirectRecoveryBlockedUntil               time.Time             `json:"observed_direct_recovery_blocked_until,omitempty"`
	ObservedDirectRecoveryNextProbeAt                time.Time             `json:"observed_direct_recovery_next_probe_at,omitempty"`
	ObservedDirectRecoveryProbeLimited               bool                  `json:"observed_direct_recovery_probe_limited,omitempty"`
	ObservedDirectRecoveryProbeBudget                int                   `json:"observed_direct_recovery_probe_budget,omitempty"`
	ObservedDirectRecoveryProbeFailures              int                   `json:"observed_direct_recovery_probe_failures,omitempty"`
	ObservedDirectRecoveryProbeRemaining             int                   `json:"observed_direct_recovery_probe_remaining,omitempty"`
	ObservedDirectRecoveryProbeRefillAt              time.Time             `json:"observed_direct_recovery_probe_refill_at,omitempty"`
	ObservedDirectRecoveryLastIssuedAttemptID        string                `json:"observed_direct_recovery_last_issued_attempt_id,omitempty"`
	ObservedDirectRecoveryLastIssuedAttemptReason    string                `json:"observed_direct_recovery_last_issued_attempt_reason,omitempty"`
	ObservedDirectRecoveryLastIssuedAttemptAt        time.Time             `json:"observed_direct_recovery_last_issued_attempt_at,omitempty"`
	ObservedDirectRecoveryLastIssuedAttemptExecuteAt time.Time             `json:"observed_direct_recovery_last_issued_attempt_execute_at,omitempty"`
}

type RelayNode struct {
	Region  string `json:"region"`
	Address string `json:"address"`
}

type DNSConfig struct {
	Domain      string   `json:"domain"`
	Nameservers []string `json:"nameservers"`
}

type ACLSnapshot struct {
	Version       int    `json:"version"`
	DefaultAction string `json:"default_action"`
}

type ExitNodeConfig struct {
	Mode          string `json:"mode"`
	NodeID        string `json:"node_id"`
	AllowLAN      bool   `json:"allow_lan"`
	AllowInternet bool   `json:"allow_internet"`
	DNSMode       string `json:"dns_mode"`
}

type BootstrapConfig struct {
	Version     int             `json:"version"`
	OverlayCIDR string          `json:"overlay_cidr"`
	Node        Node            `json:"node"`
	Peers       []Peer          `json:"peers"`
	DNS         DNSConfig       `json:"dns"`
	Routes      []Route         `json:"routes"`
	Relays      []RelayNode     `json:"relays"`
	ACL         ACLSnapshot     `json:"acl"`
	ExitNode    *ExitNodeConfig `json:"exit_node,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	User        User   `json:"user"`
}

type DeviceRegistrationRequest struct {
	TenantID          string   `json:"tenant_id,omitempty"`
	UserID            string   `json:"user_id,omitempty"`
	DeviceName        string   `json:"device_name"`
	Platform          string   `json:"platform"`
	Version           string   `json:"version"`
	PublicKey         string   `json:"public_key"`
	Capabilities      []string `json:"capabilities,omitempty"`
	RegistrationToken string   `json:"registration_token"`
}

type DeviceRegistrationResponse struct {
	Device    Device          `json:"device"`
	Node      Node            `json:"node"`
	NodeToken string          `json:"node_token"`
	Bootstrap BootstrapConfig `json:"bootstrap"`
}

type HeartbeatRequest struct {
	Endpoints           []string              `json:"endpoints,omitempty"`
	EndpointRecords     []EndpointObservation `json:"endpoint_records,omitempty"`
	RelayRegion         string                `json:"relay_region,omitempty"`
	Status              string                `json:"status,omitempty"`
	PublicKey           string                `json:"public_key,omitempty"`
	NATReport           NATReport             `json:"nat_report,omitempty"`
	PeerTransportStates []PeerTransportState  `json:"peer_transport_states,omitempty"`
}

type DirectAttemptInstruction struct {
	AttemptID     string    `json:"attempt_id"`
	PeerNodeID    string    `json:"peer_node_id"`
	IssuedAt      time.Time `json:"issued_at,omitempty"`
	ExecuteAt     time.Time `json:"execute_at"`
	Window        int64     `json:"window,omitempty"`
	BurstInterval int64     `json:"burst_interval,omitempty"`
	Candidates    []string  `json:"candidates,omitempty"`
	Reason        string    `json:"reason,omitempty"`
}

type HeartbeatResponse struct {
	Node               Node                       `json:"node"`
	BootstrapVersion   int                        `json:"bootstrap_version"`
	DirectAttempts     []DirectAttemptInstruction `json:"direct_attempts,omitempty"`
	PeerRecoveryStates []PeerRecoveryState        `json:"peer_recovery_states,omitempty"`
}

type CreateRouteRequest struct {
	TenantID    string `json:"tenant_id,omitempty"`
	NetworkCIDR string `json:"network_cidr"`
	ViaNodeID   string `json:"via_node_id"`
	Priority    int    `json:"priority"`
}

type NodeListResponse struct {
	Items []Node `json:"items"`
}

type RouteListResponse struct {
	Items []Route `json:"items"`
}

type DNSZoneListResponse struct {
	Items []DNSZone `json:"items"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
