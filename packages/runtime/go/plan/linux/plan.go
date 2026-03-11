package linux

import (
	"fmt"
	"net/netip"
	"strings"
	"time"

	"nodeweave/packages/runtime/go/overlay"
)

type Operation struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Description string   `json:"description"`
	Command     []string `json:"command"`
	Interface   string   `json:"interface,omitempty"`
	AddressCIDR string   `json:"address_cidr,omitempty"`
	RouteCIDR   string   `json:"route_cidr,omitempty"`
	RouteDev    string   `json:"route_dev,omitempty"`
	MTU         int      `json:"mtu,omitempty"`
	Nameservers []string `json:"nameservers,omitempty"`
	Domain      string   `json:"domain,omitempty"`
}

type Plan struct {
	GeneratedAt time.Time   `json:"generated_at"`
	NodeID      string      `json:"node_id"`
	Interface   string      `json:"interface"`
	Operations  []Operation `json:"operations"`
}

func Build(snapshot overlay.Snapshot) Plan {
	ops := []Operation{
		{
			ID:          "interface-create",
			Kind:        "interface_create",
			Description: "ensure TUN interface exists",
			Command:     []string{"ip", "tuntap", "add", "dev", snapshot.Interface.Name, "mode", "tun"},
			Interface:   snapshot.Interface.Name,
		},
		{
			ID:          "interface-link-up",
			Kind:        "interface_link",
			Description: "set interface MTU and bring it up",
			Command:     []string{"ip", "link", "set", "dev", snapshot.Interface.Name, "mtu", fmt.Sprintf("%d", snapshot.Interface.MTU), "up"},
			Interface:   snapshot.Interface.Name,
			MTU:         snapshot.Interface.MTU,
		},
		{
			ID:          "interface-address",
			Kind:        "interface_address",
			Description: "assign overlay address",
			Command:     []string{"ip", "addr", "replace", snapshot.Interface.AddressCIDR, "dev", snapshot.Interface.Name},
			Interface:   snapshot.Interface.Name,
			AddressCIDR: snapshot.Interface.AddressCIDR,
		},
	}

	peerByNodeID := make(map[string]overlay.PeerState, len(snapshot.Peers))
	routeSet := map[string]struct{}{}
	for _, peer := range snapshot.Peers {
		peerByNodeID[peer.NodeID] = peer

		hostCIDR, err := hostPrefix(peer.OverlayIP)
		if err == nil && hostCIDR != "" {
			routeSet[hostCIDR] = struct{}{}
			ops = append(ops, routeOperation(
				fmt.Sprintf("peer-host-%s", peer.NodeID),
				fmt.Sprintf("route peer %s overlay address into interface", peer.NodeID),
				hostCIDR,
				snapshot.Interface.Name,
			))
		}
	}

	for _, route := range snapshot.Routes {
		if route.ViaNodeID == "" || route.ViaNodeID == snapshot.NodeID {
			continue
		}
		if _, ok := peerByNodeID[route.ViaNodeID]; !ok {
			continue
		}
		if _, ok := routeSet[route.NetworkCIDR]; ok {
			continue
		}
		routeSet[route.NetworkCIDR] = struct{}{}
		ops = append(ops, routeOperation(
			fmt.Sprintf("route-static-%s", sanitizeID(route.NetworkCIDR)),
			fmt.Sprintf("route subnet %s via overlay peer %s", route.NetworkCIDR, route.ViaNodeID),
			route.NetworkCIDR,
			snapshot.Interface.Name,
		))
	}

	for _, peer := range snapshot.Peers {
		hostCIDR, _ := hostPrefix(peer.OverlayIP)
		for _, allowedIP := range peer.AllowedIPs {
			if allowedIP == "" || allowedIP == hostCIDR {
				continue
			}
			if _, ok := routeSet[allowedIP]; ok {
				continue
			}
			routeSet[allowedIP] = struct{}{}
			ops = append(ops, routeOperation(
				fmt.Sprintf("route-peer-%s-%s", peer.NodeID, sanitizeID(allowedIP)),
				fmt.Sprintf("route peer %s traffic into overlay", peer.NodeID),
				allowedIP,
				snapshot.Interface.Name,
			))
		}
	}

	if snapshot.ExitNode != nil && snapshot.ExitNode.AllowInternet && snapshot.ExitNode.NodeID != "" && snapshot.ExitNode.NodeID != snapshot.NodeID {
		if _, ok := peerByNodeID[snapshot.ExitNode.NodeID]; ok {
			if _, exists := routeSet["0.0.0.0/0"]; !exists {
				routeSet["0.0.0.0/0"] = struct{}{}
				ops = append(ops, routeOperation(
					fmt.Sprintf("exit-default-v4-%s", snapshot.ExitNode.NodeID),
					fmt.Sprintf("route default IPv4 traffic into overlay via exit node %s", snapshot.ExitNode.NodeID),
					"0.0.0.0/0",
					snapshot.Interface.Name,
				))
			}
		}
	}

	if len(snapshot.DNS.Nameservers) > 0 {
		ops = append(ops, Operation{
			ID:          "dns-servers",
			Kind:        "dns_servers",
			Description: "configure per-link DNS",
			Command:     append([]string{"resolvectl", "dns", snapshot.Interface.Name}, snapshot.DNS.Nameservers...),
			Interface:   snapshot.Interface.Name,
			Nameservers: append([]string(nil), snapshot.DNS.Nameservers...),
		})
	}
	if strings.TrimSpace(snapshot.DNS.Domain) != "" {
		ops = append(ops, Operation{
			ID:          "dns-domain",
			Kind:        "dns_domain",
			Description: "configure per-link DNS domain",
			Command:     []string{"resolvectl", "domain", snapshot.Interface.Name, "~" + snapshot.DNS.Domain},
			Interface:   snapshot.Interface.Name,
			Domain:      snapshot.DNS.Domain,
		})
	}

	return Plan{
		GeneratedAt: time.Now().UTC(),
		NodeID:      snapshot.NodeID,
		Interface:   snapshot.Interface.Name,
		Operations:  ops,
	}
}

func routeOperation(id, description, cidr, dev string) Operation {
	return Operation{
		ID:          id,
		Kind:        "route_replace",
		Description: description,
		Command:     []string{"ip", "route", "replace", cidr, "dev", dev},
		RouteCIDR:   cidr,
		RouteDev:    dev,
	}
}

func hostPrefix(ip string) (string, error) {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return "", err
	}
	bits := 32
	if addr.Is6() {
		bits = 128
	}
	return netip.PrefixFrom(addr, bits).String(), nil
}

func sanitizeID(value string) string {
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, ":", "_")
	value = strings.ReplaceAll(value, ".", "_")
	return value
}
