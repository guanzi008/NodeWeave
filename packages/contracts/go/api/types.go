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
	PeerNodeID              string    `json:"peer_node_id"`
	ActiveKind              string    `json:"active_kind,omitempty"`
	ActiveAddress           string    `json:"active_address,omitempty"`
	ReportedAt              time.Time `json:"reported_at,omitempty"`
	LastDirectAttemptAt     time.Time `json:"last_direct_attempt_at,omitempty"`
	LastDirectAttemptResult string    `json:"last_direct_attempt_result,omitempty"`
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
	NodeID                          string                `json:"node_id"`
	OverlayIP                       string                `json:"overlay_ip"`
	PublicKey                       string                `json:"public_key"`
	Endpoints                       []string              `json:"endpoints,omitempty"`
	EndpointRecords                 []EndpointObservation `json:"endpoint_records,omitempty"`
	RelayRegion                     string                `json:"relay_region"`
	AllowedIPs                      []string              `json:"allowed_ips,omitempty"`
	Status                          string                `json:"status"`
	LastSeenAt                      time.Time             `json:"last_seen_at"`
	NATMappingBehavior              string                `json:"nat_mapping_behavior,omitempty"`
	NATReachable                    bool                  `json:"nat_reachable"`
	NATReportedAt                   time.Time             `json:"nat_reported_at,omitempty"`
	ObservedTransportKind           string                `json:"observed_transport_kind,omitempty"`
	ObservedTransportAddress        string                `json:"observed_transport_address,omitempty"`
	ObservedTransportReportedAt     time.Time             `json:"observed_transport_reported_at,omitempty"`
	ObservedLastDirectAttemptAt     time.Time             `json:"observed_last_direct_attempt_at,omitempty"`
	ObservedLastDirectAttemptResult string                `json:"observed_last_direct_attempt_result,omitempty"`
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
	ExecuteAt     time.Time `json:"execute_at"`
	Window        int64     `json:"window,omitempty"`
	BurstInterval int64     `json:"burst_interval,omitempty"`
	Candidates    []string  `json:"candidates,omitempty"`
	Reason        string    `json:"reason,omitempty"`
}

type HeartbeatResponse struct {
	Node             Node                       `json:"node"`
	BootstrapVersion int                        `json:"bootstrap_version"`
	DirectAttempts   []DirectAttemptInstruction `json:"direct_attempts,omitempty"`
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
