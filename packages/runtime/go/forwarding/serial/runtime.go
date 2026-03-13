package serial

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"nodeweave/packages/runtime/go/overlay"
)

const (
	DefaultTCPBasePort      = 43100
	DefaultReconnectDelay   = 2 * time.Second
	DefaultDialTimeout      = 5 * time.Second
	DefaultAcceptKeepAlive  = 30 * time.Second
	reportStatusConfigured  = "configured"
	reportStatusListening   = "listening"
	reportStatusDialing     = "dialing"
	reportStatusRunning     = "running"
	reportStatusReconnecting = "reconnecting"
)

type PortOpener interface {
	Open(cfg PortConfig) (io.ReadWriteCloser, error)
}

type ListenerFactory func(network, address string) (net.Listener, error)

type DialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

type LoggerFunc func(format string, args ...any)

type RuntimeConfig struct {
	LocalNodeID    string
	Snapshot       overlay.Snapshot
	BasePort       int
	ReconnectDelay time.Duration
	DialTimeout    time.Duration
	Opener         PortOpener
	Listen         ListenerFactory
	DialContext    DialContextFunc
	Logger         LoggerFunc
	OnReport       func(SessionReport)
}

type ResolvedSession struct {
	Spec         SessionSpec `json:"spec"`
	LocalNodeID  string      `json:"local_node_id"`
	PeerNodeID   string      `json:"peer_node_id"`
	LocalPort    PortConfig  `json:"local_port"`
	RemotePort   PortConfig  `json:"remote_port"`
	LocalOverlay string      `json:"local_overlay"`
	PeerOverlay  string      `json:"peer_overlay"`
	Role         string      `json:"role"`
	ListenAddr   string      `json:"listen_addr"`
	DialAddr     string      `json:"dial_addr"`
	TCPPort      int         `json:"tcp_port"`
}

type Manager struct {
	cfg      RuntimeConfig
	sessions []ResolvedSession

	mu      sync.RWMutex
	reports map[string]SessionReport

	cancel context.CancelFunc
	done   chan struct{}
}

func NewManager(cfg RuntimeConfig, specs []SessionSpec) (*Manager, error) {
	if strings.TrimSpace(cfg.LocalNodeID) == "" {
		return nil, errors.New("serial forwarding requires local node id")
	}
	if cfg.BasePort <= 0 {
		cfg.BasePort = DefaultTCPBasePort
	}
	if cfg.ReconnectDelay <= 0 {
		cfg.ReconnectDelay = DefaultReconnectDelay
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = DefaultDialTimeout
	}
	if cfg.Opener == nil {
		cfg.Opener = GoSerialPortOpener{}
	}
	if cfg.Listen == nil {
		cfg.Listen = net.Listen
	}
	if cfg.DialContext == nil {
		dialer := &net.Dialer{Timeout: cfg.DialTimeout, KeepAlive: DefaultAcceptKeepAlive}
		cfg.DialContext = dialer.DialContext
	}

	resolved := make([]ResolvedSession, 0, len(specs))
	reports := make(map[string]SessionReport, len(specs))
	for _, spec := range specs {
		spec = NormalizeSessionSpec(spec)
		if strings.TrimSpace(spec.SessionID) == "" {
			continue
		}
		session, err := ResolveRuntime(spec, cfg.Snapshot, cfg.LocalNodeID, cfg.BasePort)
		if err != nil {
			report := ConfiguredReport(spec, cfg.LocalNodeID)
			report.Status = "error"
			report.LastError = err.Error()
			report.ClosedBy = "resolve"
			report.StartedAt = time.Now().UTC()
			report.EndedAt = report.StartedAt
			reports[spec.SessionID] = report
			continue
		}
		resolved = append(resolved, session)
		reports[session.Spec.SessionID] = configuredRuntimeReport(session, reportStatusConfigured, "")
	}
	sort.Slice(resolved, func(i, j int) bool {
		return resolved[i].Spec.SessionID < resolved[j].Spec.SessionID
	})
	return &Manager{
		cfg:      cfg,
		sessions: resolved,
		reports:  reports,
		done:     make(chan struct{}),
	}, nil
}

func (m *Manager) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	m.cancel = cancel
	if len(m.sessions) == 0 {
		close(m.done)
		return
	}
	var wg sync.WaitGroup
	for _, session := range m.sessions {
		session := session
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.runSession(ctx, session)
		}()
	}
	go func() {
		wg.Wait()
		close(m.done)
	}()
}

func (m *Manager) Close() error {
	if m.cancel != nil {
		m.cancel()
	}
	select {
	case <-m.done:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("serial forwarding manager did not stop in time")
	}
}

func (m *Manager) Reports() []SessionReport {
	m.mu.RLock()
	defer m.mu.RUnlock()
	reports := make([]SessionReport, 0, len(m.reports))
	for _, report := range m.reports {
		reports = append(reports, report)
	}
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].SessionID < reports[j].SessionID
	})
	return reports
}

func ResolveRuntime(spec SessionSpec, snapshot overlay.Snapshot, localNodeID string, basePort int) (ResolvedSession, error) {
	spec = NormalizeSessionSpec(spec)
	localNodeID = strings.TrimSpace(localNodeID)
	if localNodeID == "" {
		return ResolvedSession{}, errors.New("local node id is required")
	}
	if basePort <= 0 {
		basePort = DefaultTCPBasePort
	}

	localPort, remotePort, peerNodeID, err := effectivePorts(spec, localNodeID)
	if err != nil {
		return ResolvedSession{}, err
	}
	localOverlay := strings.TrimSpace(snapshot.Interface.AddressCIDR)
	if prefix, _, found := strings.Cut(localOverlay, "/"); found {
		localOverlay = prefix
	}
	peerOverlay := resolvePeerOverlay(snapshot, peerNodeID)
	if peerOverlay == "" {
		return ResolvedSession{}, fmt.Errorf("peer %s overlay address not found", peerNodeID)
	}

	role := "dialer"
	listenAddr := ""
	dialAddr := net.JoinHostPort(peerOverlay, fmt.Sprintf("%d", TCPPortForSession(spec.SessionID, basePort)))
	if strings.Compare(localNodeID, peerNodeID) < 0 {
		role = "listener"
		listenAddr = fmt.Sprintf(":%d", TCPPortForSession(spec.SessionID, basePort))
	}
	return ResolvedSession{
		Spec:         spec,
		LocalNodeID:  localNodeID,
		PeerNodeID:   peerNodeID,
		LocalPort:    localPort,
		RemotePort:   remotePort,
		LocalOverlay: localOverlay,
		PeerOverlay:  peerOverlay,
		Role:         role,
		ListenAddr:   listenAddr,
		DialAddr:     dialAddr,
		TCPPort:      TCPPortForSession(spec.SessionID, basePort),
	}, nil
}

func TCPPortForSession(sessionID string, basePort int) int {
	if basePort <= 0 {
		basePort = DefaultTCPBasePort
	}
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(strings.TrimSpace(sessionID)))
	return basePort + int(hasher.Sum32()%1000)
}

func effectivePorts(spec SessionSpec, localNodeID string) (PortConfig, PortConfig, string, error) {
	switch strings.TrimSpace(localNodeID) {
	case strings.TrimSpace(spec.NodeID):
		return spec.Local, spec.Remote, strings.TrimSpace(spec.PeerNodeID), nil
	case strings.TrimSpace(spec.PeerNodeID):
		return spec.Remote, spec.Local, strings.TrimSpace(spec.NodeID), nil
	default:
		return PortConfig{}, PortConfig{}, "", fmt.Errorf("session %s does not belong to local node %s", spec.SessionID, localNodeID)
	}
}

func resolvePeerOverlay(snapshot overlay.Snapshot, peerNodeID string) string {
	for _, peer := range snapshot.Peers {
		if strings.TrimSpace(peer.NodeID) == strings.TrimSpace(peerNodeID) {
			return strings.TrimSpace(peer.OverlayIP)
		}
	}
	return ""
}

func configuredRuntimeReport(session ResolvedSession, status string, lastError string) SessionReport {
	now := time.Now().UTC()
	report := SessionReport{
		SessionID:  session.Spec.SessionID,
		NodeID:     session.LocalNodeID,
		PeerNodeID: session.PeerNodeID,
		Transport:  session.Spec.Transport,
		Local:      session.LocalPort,
		Remote:     session.RemotePort,
		Status:     status,
		StartedAt:  now,
	}
	if lastError != "" {
		report.LastError = lastError
		report.EndedAt = now
	}
	return report
}

func (m *Manager) runSession(ctx context.Context, session ResolvedSession) {
	logger := m.cfg.Logger
	if logger == nil {
		logger = func(string, ...any) {}
	}
	if session.Role == "listener" {
		m.runListenerSession(ctx, session, logger)
		return
	}
	m.runDialerSession(ctx, session, logger)
}

func (m *Manager) runListenerSession(ctx context.Context, session ResolvedSession, logger LoggerFunc) {
	listener, err := m.cfg.Listen("tcp", session.ListenAddr)
	if err != nil {
		m.setReport(configuredRuntimeReport(session, "error", err.Error()))
		return
	}
	defer listener.Close()
	m.setReport(configuredRuntimeReport(session, reportStatusListening, ""))
	logger("serial forwarding session %s listening on %s", session.Spec.SessionID, session.ListenAddr)
	tcpListener, _ := listener.(*net.TCPListener)
	for {
		if ctx.Err() != nil {
			m.setStoppedReport(session, "cancelled", "context", "")
			return
		}
		if tcpListener != nil {
			_ = tcpListener.SetDeadline(time.Now().Add(1 * time.Second))
		}
		conn, err := listener.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			if ctx.Err() != nil {
				m.setStoppedReport(session, "cancelled", "context", "")
				return
			}
			m.setReport(configuredRuntimeReport(session, "error", err.Error()))
			time.Sleep(m.cfg.ReconnectDelay)
			continue
		}
		m.runBridgedSession(ctx, session, conn)
	}
}

func (m *Manager) runDialerSession(ctx context.Context, session ResolvedSession, logger LoggerFunc) {
	for {
		if ctx.Err() != nil {
			m.setStoppedReport(session, "cancelled", "context", "")
			return
		}
		m.setReport(configuredRuntimeReport(session, reportStatusDialing, ""))
		conn, err := m.cfg.DialContext(ctx, "tcp", session.DialAddr)
		if err != nil {
			if ctx.Err() != nil {
				m.setStoppedReport(session, "cancelled", "context", "")
				return
			}
			m.setReport(configuredRuntimeReport(session, reportStatusReconnecting, err.Error()))
			time.Sleep(m.cfg.ReconnectDelay)
			continue
		}
		logger("serial forwarding session %s connected to %s", session.Spec.SessionID, session.DialAddr)
		m.runBridgedSession(ctx, session, conn)
		if ctx.Err() != nil {
			return
		}
		time.Sleep(m.cfg.ReconnectDelay)
	}
}

func (m *Manager) runBridgedSession(ctx context.Context, session ResolvedSession, conn io.ReadWriteCloser) {
	defer conn.Close()
	localPort, err := m.cfg.Opener.Open(session.LocalPort)
	if err != nil {
		m.setReport(configuredRuntimeReport(session, "error", err.Error()))
		return
	}
	defer localPort.Close()

	running := configuredRuntimeReport(session, reportStatusRunning, "")
	running.StartedAt = time.Now().UTC()
	running.EndedAt = time.Time{}
	m.setReport(running)

	spec := SessionSpec{
		SessionID:  session.Spec.SessionID,
		NodeID:     session.LocalNodeID,
		PeerNodeID: session.PeerNodeID,
		Transport:  session.Spec.Transport,
		Local:      session.LocalPort,
		Remote:     session.RemotePort,
		CreatedAt:  session.Spec.CreatedAt,
	}
	report := NewSession(spec, localPort, conn).Run(ctx)
	if report.Status == "completed" && ctx.Err() == nil {
		report.Status = reportStatusReconnecting
		report.LastError = ""
	}
	m.setReport(report)
}

func (m *Manager) setStoppedReport(session ResolvedSession, status, closedBy, lastError string) {
	report := configuredRuntimeReport(session, status, lastError)
	report.ClosedBy = closedBy
	if report.EndedAt.IsZero() {
		report.EndedAt = time.Now().UTC()
	}
	m.setReport(report)
}

func (m *Manager) setReport(report SessionReport) {
	m.mu.Lock()
	m.reports[report.SessionID] = report
	m.mu.Unlock()
	if m.cfg.OnReport != nil {
		m.cfg.OnReport(report)
	}
}
