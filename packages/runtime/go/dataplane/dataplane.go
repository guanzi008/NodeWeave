package dataplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
	"time"

	"nodeweave/packages/runtime/go/overlay"
	"nodeweave/packages/runtime/go/session"
)

type Config struct {
	ListenAddress string
	SessionReport session.Report
}

type Route struct {
	NetworkCIDR      string `json:"network_cidr"`
	PrefixBits       int    `json:"prefix_bits"`
	PeerNodeID       string `json:"peer_node_id"`
	OverlayIP        string `json:"overlay_ip,omitempty"`
	CandidateAddress string `json:"candidate_address"`
	CandidateKind    string `json:"candidate_kind"`
	CandidateRegion  string `json:"candidate_region,omitempty"`
	RoutePriority    int    `json:"route_priority"`
}

type Spec struct {
	GeneratedAt   time.Time `json:"generated_at"`
	NodeID        string    `json:"node_id"`
	ListenAddress string    `json:"listen_address,omitempty"`
	Routes        []Route   `json:"routes"`
}

type Frame struct {
	Type          string    `json:"type"`
	SourceNodeID  string    `json:"source_node_id"`
	TargetNodeID  string    `json:"target_node_id,omitempty"`
	DestinationIP string    `json:"destination_ip"`
	SentAt        time.Time `json:"sent_at"`
	Payload       []byte    `json:"payload"`
}

type InboundPacket struct {
	SourceNodeID  string
	TargetNodeID  string
	DestinationIP string
	Payload       []byte
}

type Sink interface {
	HandleInbound(context.Context, InboundPacket) error
}

type Transport interface {
	Address() string
	Send(context.Context, string, Frame) error
	Serve(context.Context, func(context.Context, Frame, net.Addr) error) error
	Close() error
}

type Engine struct {
	spec      Spec
	transport Transport
	sink      Sink
}

type UDPTransport struct {
	conn *net.UDPConn
}

func Build(snapshot overlay.Snapshot, sessions session.Spec, cfg Config) (Spec, error) {
	reportSelections := make(map[string]string, len(cfg.SessionReport.Peers))
	for _, peerReport := range cfg.SessionReport.Peers {
		selected := strings.TrimSpace(peerReport.SelectedCandidate)
		if selected == "" {
			continue
		}
		reportSelections[peerReport.NodeID] = selected
	}

	peerCandidates := make(map[string]session.Candidate)
	peerOverlay := make(map[string]string)
	for _, peer := range snapshot.Peers {
		peerOverlay[peer.NodeID] = peer.OverlayIP
	}
	for _, peer := range sessions.Peers {
		candidate, ok := selectCandidate(peer, reportSelections[peer.NodeID])
		if ok {
			peerCandidates[peer.NodeID] = candidate
		}
	}

	routes := make([]Route, 0, len(snapshot.Peers)+len(snapshot.Routes)+1)
	seen := map[string]struct{}{}

	for _, peer := range snapshot.Peers {
		candidate, ok := peerCandidates[peer.NodeID]
		if !ok || strings.TrimSpace(candidate.Address) == "" {
			continue
		}
		hostCIDR, err := hostPrefix(peer.OverlayIP)
		if err != nil {
			return Spec{}, err
		}
		key := peer.NodeID + "|" + hostCIDR
		seen[key] = struct{}{}
		routes = append(routes, routeFromCandidate(peer.NodeID, peer.OverlayIP, hostCIDR, 1000, candidate))
	}

	for _, route := range snapshot.Routes {
		candidate, ok := peerCandidates[route.ViaNodeID]
		if !ok || strings.TrimSpace(candidate.Address) == "" {
			continue
		}
		key := route.ViaNodeID + "|" + route.NetworkCIDR
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		routes = append(routes, routeFromCandidate(route.ViaNodeID, peerOverlay[route.ViaNodeID], route.NetworkCIDR, route.Priority, candidate))
	}

	if snapshot.ExitNode != nil && snapshot.ExitNode.AllowInternet && snapshot.ExitNode.NodeID != "" {
		candidate, ok := peerCandidates[snapshot.ExitNode.NodeID]
		if ok && strings.TrimSpace(candidate.Address) != "" {
			key := snapshot.ExitNode.NodeID + "|0.0.0.0/0"
			if _, exists := seen[key]; !exists {
				routes = append(routes, routeFromCandidate(snapshot.ExitNode.NodeID, peerOverlay[snapshot.ExitNode.NodeID], "0.0.0.0/0", -1, candidate))
			}
		}
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].PrefixBits == routes[j].PrefixBits {
			if routes[i].RoutePriority == routes[j].RoutePriority {
				if routes[i].PeerNodeID == routes[j].PeerNodeID {
					return routes[i].NetworkCIDR < routes[j].NetworkCIDR
				}
				return routes[i].PeerNodeID < routes[j].PeerNodeID
			}
			return routes[i].RoutePriority > routes[j].RoutePriority
		}
		return routes[i].PrefixBits > routes[j].PrefixBits
	})

	return Spec{
		GeneratedAt:   time.Now().UTC(),
		NodeID:        snapshot.NodeID,
		ListenAddress: strings.TrimSpace(cfg.ListenAddress),
		Routes:        routes,
	}, nil
}

func selectCandidate(peer session.Peer, selectedAddress string) (session.Candidate, bool) {
	selectedAddress = strings.TrimSpace(selectedAddress)
	if selectedAddress != "" {
		for _, candidate := range peer.Candidates {
			if strings.TrimSpace(candidate.Address) == selectedAddress {
				return candidate, true
			}
		}
	}

	preferredAddress := strings.TrimSpace(peer.PreferredCandidate)
	if preferredAddress != "" {
		for _, candidate := range peer.Candidates {
			if strings.TrimSpace(candidate.Address) == preferredAddress {
				return candidate, true
			}
		}
	}

	if len(peer.Candidates) == 0 {
		return session.Candidate{}, false
	}
	return peer.Candidates[0], true
}

func (s Spec) Resolve(destinationIP string) (Route, error) {
	addr, err := netip.ParseAddr(strings.TrimSpace(destinationIP))
	if err != nil {
		return Route{}, fmt.Errorf("parse destination ip: %w", err)
	}
	for _, route := range s.Routes {
		prefix, err := netip.ParsePrefix(route.NetworkCIDR)
		if err != nil {
			continue
		}
		if prefix.Contains(addr) {
			return route, nil
		}
	}
	return Route{}, fmt.Errorf("no dataplane route for %s", destinationIP)
}

func NewEngine(spec Spec, transport Transport, sink Sink) *Engine {
	return &Engine{spec: spec, transport: transport, sink: sink}
}

func (e *Engine) Serve(ctx context.Context) error {
	if e.transport == nil {
		return errors.New("dataplane transport is nil")
	}
	if e.sink == nil {
		return errors.New("dataplane sink is nil")
	}
	return e.transport.Serve(ctx, func(handlerCtx context.Context, frame Frame, _ net.Addr) error {
		if frame.Type != "packet" {
			return nil
		}
		if frame.TargetNodeID != "" && frame.TargetNodeID != e.spec.NodeID {
			return nil
		}
		return e.sink.HandleInbound(handlerCtx, InboundPacket{
			SourceNodeID:  frame.SourceNodeID,
			TargetNodeID:  frame.TargetNodeID,
			DestinationIP: frame.DestinationIP,
			Payload:       append([]byte(nil), frame.Payload...),
		})
	})
}

func (e *Engine) SendPacket(ctx context.Context, destinationIP string, payload []byte) error {
	if e.transport == nil {
		return errors.New("dataplane transport is nil")
	}
	route, err := e.spec.Resolve(destinationIP)
	if err != nil {
		return err
	}
	return e.transport.Send(ctx, route.CandidateAddress, Frame{
		Type:          "packet",
		SourceNodeID:  e.spec.NodeID,
		TargetNodeID:  route.PeerNodeID,
		DestinationIP: destinationIP,
		SentAt:        time.Now().UTC(),
		Payload:       append([]byte(nil), payload...),
	})
}

func ListenUDP(address string) (*UDPTransport, error) {
	udpAddress, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, fmt.Errorf("resolve udp listen address: %w", err)
	}
	conn, err := net.ListenUDP("udp", udpAddress)
	if err != nil {
		return nil, fmt.Errorf("listen udp dataplane: %w", err)
	}
	return &UDPTransport{conn: conn}, nil
}

func (t *UDPTransport) Address() string {
	if t == nil || t.conn == nil {
		return ""
	}
	return t.conn.LocalAddr().String()
}

func (t *UDPTransport) Close() error {
	if t == nil || t.conn == nil {
		return nil
	}
	return t.conn.Close()
}

func (t *UDPTransport) Send(ctx context.Context, address string, frame Frame) error {
	if t == nil || t.conn == nil {
		return errors.New("udp transport is not initialized")
	}
	remoteAddress, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return fmt.Errorf("resolve udp remote address: %w", err)
	}
	raw, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("marshal dataplane frame: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := t.conn.SetWriteDeadline(deadline); err != nil {
			return fmt.Errorf("set udp write deadline: %w", err)
		}
		defer t.conn.SetWriteDeadline(time.Time{})
	}
	if _, err := t.conn.WriteToUDP(raw, remoteAddress); err != nil {
		return fmt.Errorf("write udp dataplane frame: %w", err)
	}
	return nil
}

func (t *UDPTransport) Serve(ctx context.Context, handler func(context.Context, Frame, net.Addr) error) error {
	if t == nil || t.conn == nil {
		return errors.New("udp transport is not initialized")
	}
	buffer := make([]byte, 64*1024)
	for {
		if err := t.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			return fmt.Errorf("set udp read deadline: %w", err)
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
			return fmt.Errorf("read udp dataplane frame: %w", err)
		}

		var frame Frame
		if err := json.Unmarshal(buffer[:n], &frame); err != nil {
			continue
		}
		if err := handler(ctx, frame, addr); err != nil {
			return err
		}
	}
}

func routeFromCandidate(peerNodeID, overlayIP, networkCIDR string, routePriority int, candidate session.Candidate) Route {
	prefixBits := 0
	if prefix, err := netip.ParsePrefix(networkCIDR); err == nil {
		prefixBits = prefix.Bits()
	}
	return Route{
		NetworkCIDR:      networkCIDR,
		PrefixBits:       prefixBits,
		PeerNodeID:       peerNodeID,
		OverlayIP:        overlayIP,
		CandidateAddress: candidate.Address,
		CandidateKind:    candidate.Kind,
		CandidateRegion:  candidate.Region,
		RoutePriority:    routePriority,
	}
}

func hostPrefix(ip string) (string, error) {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return "", fmt.Errorf("parse host ip: %w", err)
	}
	bits := 32
	if addr.Is6() {
		bits = 128
	}
	return netip.PrefixFrom(addr, bits).String(), nil
}
