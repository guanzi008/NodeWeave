package relay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"nodeweave/packages/runtime/go/secureudp"
)

type Config struct {
	ListenAddress string
	MappingTTL    time.Duration
}

type Service struct {
	conn       *net.UDPConn
	mappingTTL time.Duration

	mu       sync.RWMutex
	bindings map[string]binding
}

type binding struct {
	address    *net.UDPAddr
	lastSeenAt time.Time
}

func Listen(cfg Config) (*Service, error) {
	listenAddress := strings.TrimSpace(cfg.ListenAddress)
	if listenAddress == "" {
		return nil, errors.New("listen address is required")
	}
	if cfg.MappingTTL <= 0 {
		cfg.MappingTTL = 2 * time.Minute
	}
	udpAddress, err := net.ResolveUDPAddr("udp", listenAddress)
	if err != nil {
		return nil, fmt.Errorf("resolve relay listen address: %w", err)
	}
	conn, err := net.ListenUDP("udp", udpAddress)
	if err != nil {
		return nil, fmt.Errorf("listen relay: %w", err)
	}
	return &Service{
		conn:       conn,
		mappingTTL: cfg.MappingTTL,
		bindings:   make(map[string]binding),
	}, nil
}

func (s *Service) Address() string {
	if s == nil || s.conn == nil {
		return ""
	}
	return s.conn.LocalAddr().String()
}

func (s *Service) Close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

func (s *Service) Serve(ctx context.Context) error {
	if s == nil || s.conn == nil {
		return errors.New("relay service is not initialized")
	}
	buffer := make([]byte, 64*1024)
	for {
		if err := s.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			return fmt.Errorf("set relay read deadline: %w", err)
		}
		n, sourceAddr, err := s.conn.ReadFromUDP(buffer)
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
			return fmt.Errorf("read relay packet: %w", err)
		}

		metadata, err := secureudp.InspectPacket(buffer[:n])
		if err != nil || strings.TrimSpace(metadata.SourceNodeID) == "" {
			continue
		}
		s.recordBinding(metadata.SourceNodeID, sourceAddr)
		if metadata.Type == "announce" || strings.TrimSpace(metadata.TargetNodeID) == "" {
			continue
		}
		targetAddr, ok := s.lookupBinding(metadata.TargetNodeID)
		if !ok {
			continue
		}
		if _, err := s.conn.WriteToUDP(buffer[:n], targetAddr); err != nil && ctx.Err() == nil {
			return fmt.Errorf("forward relay packet to %s: %w", metadata.TargetNodeID, err)
		}
	}
}

func (s *Service) recordBinding(nodeID string, address *net.UDPAddr) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(now)
	s.bindings[nodeID] = binding{
		address:    cloneUDPAddr(address),
		lastSeenAt: now,
	}
}

func (s *Service) lookupBinding(nodeID string) (*net.UDPAddr, bool) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(now)
	entry, ok := s.bindings[nodeID]
	if !ok || entry.address == nil {
		return nil, false
	}
	return cloneUDPAddr(entry.address), true
}

func (s *Service) cleanupLocked(now time.Time) {
	for nodeID, entry := range s.bindings {
		if now.Sub(entry.lastSeenAt) > s.mappingTTL {
			delete(s.bindings, nodeID)
		}
	}
}

func cloneUDPAddr(address *net.UDPAddr) *net.UDPAddr {
	if address == nil {
		return nil
	}
	ip := append([]byte(nil), address.IP...)
	return &net.UDPAddr{
		IP:   ip,
		Port: address.Port,
		Zone: address.Zone,
	}
}
