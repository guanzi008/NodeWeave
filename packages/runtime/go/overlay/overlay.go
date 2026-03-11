package overlay

import (
	"fmt"
	"net/netip"
	"sort"
	"time"

	"nodeweave/packages/contracts/go/api"
)

type Config struct {
	InterfaceName string
	MTU           int
}

type InterfaceState struct {
	Name        string `json:"name"`
	AddressCIDR string `json:"address_cidr"`
	MTU         int    `json:"mtu"`
}

type PeerState struct {
	NodeID          string                    `json:"node_id"`
	OverlayIP       string                    `json:"overlay_ip"`
	PublicKey       string                    `json:"public_key"`
	Endpoints       []string                  `json:"endpoints,omitempty"`
	EndpointRecords []api.EndpointObservation `json:"endpoint_records,omitempty"`
	RelayRegion     string                    `json:"relay_region"`
	AllowedIPs      []string                  `json:"allowed_ips"`
	Status          string                    `json:"status"`
}

type RouteState struct {
	NetworkCIDR string `json:"network_cidr"`
	ViaNodeID   string `json:"via_node_id"`
	ViaOverlay  string `json:"via_overlay"`
	Priority    int    `json:"priority"`
}

type Snapshot struct {
	GeneratedAt time.Time           `json:"generated_at"`
	Version     int                 `json:"version"`
	Backend     string              `json:"backend"`
	NodeID      string              `json:"node_id"`
	Interface   InterfaceState      `json:"interface"`
	Peers       []PeerState         `json:"peers"`
	Routes      []RouteState        `json:"routes"`
	DNS         api.DNSConfig       `json:"dns"`
	Relays      []api.RelayNode     `json:"relays"`
	ACL         api.ACLSnapshot     `json:"acl"`
	ExitNode    *api.ExitNodeConfig `json:"exit_node,omitempty"`
}

func Compile(bootstrap api.BootstrapConfig, cfg Config, backend string) (Snapshot, error) {
	if bootstrap.Node.ID == "" || bootstrap.Node.OverlayIP == "" {
		return Snapshot{}, fmt.Errorf("bootstrap node is incomplete")
	}
	if cfg.InterfaceName == "" {
		cfg.InterfaceName = "nw0"
	}
	if cfg.MTU <= 0 {
		cfg.MTU = 1280
	}

	interfaceCIDR, err := addressCIDR(bootstrap.Node.OverlayIP, bootstrap.OverlayCIDR)
	if err != nil {
		return Snapshot{}, err
	}

	peerStates := make([]PeerState, 0, len(bootstrap.Peers))
	peerOverlayByNodeID := make(map[string]string, len(bootstrap.Peers))
	for _, peer := range bootstrap.Peers {
		peerOverlayByNodeID[peer.NodeID] = peer.OverlayIP
		peerStates = append(peerStates, PeerState{
			NodeID:          peer.NodeID,
			OverlayIP:       peer.OverlayIP,
			PublicKey:       peer.PublicKey,
			Endpoints:       append([]string(nil), peer.Endpoints...),
			EndpointRecords: append([]api.EndpointObservation(nil), peer.EndpointRecords...),
			RelayRegion:     peer.RelayRegion,
			AllowedIPs:      append([]string(nil), peer.AllowedIPs...),
			Status:          peer.Status,
		})
	}

	routeStates := make([]RouteState, 0, len(bootstrap.Routes))
	for _, route := range bootstrap.Routes {
		routeStates = append(routeStates, RouteState{
			NetworkCIDR: route.NetworkCIDR,
			ViaNodeID:   route.ViaNodeID,
			ViaOverlay:  peerOverlayByNodeID[route.ViaNodeID],
			Priority:    route.Priority,
		})
	}

	sort.Slice(peerStates, func(i, j int) bool {
		return peerStates[i].NodeID < peerStates[j].NodeID
	})
	sort.Slice(routeStates, func(i, j int) bool {
		if routeStates[i].Priority == routeStates[j].Priority {
			return routeStates[i].NetworkCIDR < routeStates[j].NetworkCIDR
		}
		return routeStates[i].Priority > routeStates[j].Priority
	})

	return Snapshot{
		GeneratedAt: time.Now().UTC(),
		Version:     bootstrap.Version,
		Backend:     backend,
		NodeID:      bootstrap.Node.ID,
		Interface: InterfaceState{
			Name:        cfg.InterfaceName,
			AddressCIDR: interfaceCIDR,
			MTU:         cfg.MTU,
		},
		Peers:    peerStates,
		Routes:   routeStates,
		DNS:      bootstrap.DNS,
		Relays:   append([]api.RelayNode(nil), bootstrap.Relays...),
		ACL:      bootstrap.ACL,
		ExitNode: bootstrap.ExitNode,
	}, nil
}

func addressCIDR(ip, overlayCIDR string) (string, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", fmt.Errorf("parse node overlay ip: %w", err)
	}
	prefix, err := netip.ParsePrefix(overlayCIDR)
	if err != nil {
		return "", fmt.Errorf("parse overlay cidr: %w", err)
	}
	return netip.PrefixFrom(addr, prefix.Bits()).String(), nil
}
