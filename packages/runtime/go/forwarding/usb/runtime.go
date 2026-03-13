package usb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"nodeweave/packages/runtime/go/overlay"
)

const (
	DefaultReconcileInterval = 10 * time.Second
	DefaultCommandTimeout    = 10 * time.Second
)

type Executor interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type DeviceResolver interface {
	ResolveLocalBusID(descriptor DeviceDescriptor) (string, error)
	ResolveRemoteBusID(ctx context.Context, executor Executor, host string, descriptor DeviceDescriptor) (string, error)
	ResolveAttachedPort(ctx context.Context, executor Executor, host, busID string) (string, error)
}

type RuntimeConfig struct {
	LocalNodeID        string
	Snapshot           overlay.Snapshot
	Executor           Executor
	Resolver           DeviceResolver
	ReconcileInterval  time.Duration
	CommandTimeout     time.Duration
	Logger             func(format string, args ...any)
	OnReport           func(SessionReport)
}

type ResolvedSession struct {
	Spec         SessionSpec `json:"spec"`
	LocalNodeID  string      `json:"local_node_id"`
	PeerNodeID   string      `json:"peer_node_id"`
	PeerOverlay  string      `json:"peer_overlay"`
	Role         string      `json:"role"`
	LocalDevice  DeviceDescriptor `json:"local_device"`
	RemoteDevice DeviceDescriptor `json:"remote_device"`
}

type Manager struct {
	cfg      RuntimeConfig
	sessions []ResolvedSession

	mu      sync.RWMutex
	reports map[string]SessionReport

	cancel context.CancelFunc
	done   chan struct{}
}

type OSExecutor struct{}

type LinuxResolver struct{}

func (OSExecutor) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), message)
	}
	return stdout.Bytes(), nil
}

func NewManager(cfg RuntimeConfig, specs []SessionSpec) (*Manager, error) {
	if strings.TrimSpace(cfg.LocalNodeID) == "" {
		return nil, errors.New("usb forwarding requires local node id")
	}
	if cfg.Executor == nil {
		cfg.Executor = OSExecutor{}
	}
	if cfg.Resolver == nil {
		cfg.Resolver = LinuxResolver{}
	}
	if cfg.ReconcileInterval <= 0 {
		cfg.ReconcileInterval = DefaultReconcileInterval
	}
	if cfg.CommandTimeout <= 0 {
		cfg.CommandTimeout = DefaultCommandTimeout
	}
	reports := make(map[string]SessionReport, len(specs))
	resolved := make([]ResolvedSession, 0, len(specs))
	for _, spec := range specs {
		spec = NormalizeSessionSpec(spec)
		if strings.TrimSpace(spec.SessionID) == "" {
			continue
		}
		session, err := ResolveRuntime(spec, cfg.Snapshot, cfg.LocalNodeID)
		if err != nil {
			report := ConfiguredReport(spec, cfg.LocalNodeID)
			report.Status = "error"
			report.LastError = err.Error()
			report.UpdatedAt = time.Now().UTC()
			reports[spec.SessionID] = report
			continue
		}
		resolved = append(resolved, session)
		report := ConfiguredReport(spec, cfg.LocalNodeID)
		report.Status = "configured"
		report.Local = session.LocalDevice
		report.Remote = session.RemoteDevice
		report.UpdatedAt = time.Now().UTC()
		reports[spec.SessionID] = report
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

func ResolveRuntime(spec SessionSpec, snapshot overlay.Snapshot, localNodeID string) (ResolvedSession, error) {
	spec = NormalizeSessionSpec(spec)
	localNodeID = strings.TrimSpace(localNodeID)
	if localNodeID == "" {
		return ResolvedSession{}, errors.New("local node id is required")
	}
	var role string
	var peerNodeID string
	var localDevice DeviceDescriptor
	var remoteDevice DeviceDescriptor
	switch localNodeID {
	case strings.TrimSpace(spec.NodeID):
		role = "exporter"
		peerNodeID = strings.TrimSpace(spec.PeerNodeID)
		localDevice = spec.Local
		remoteDevice = spec.Remote
	case strings.TrimSpace(spec.PeerNodeID):
		role = "attacher"
		peerNodeID = strings.TrimSpace(spec.NodeID)
		localDevice = spec.Remote
		remoteDevice = spec.Local
	default:
		return ResolvedSession{}, fmt.Errorf("session %s does not belong to local node %s", spec.SessionID, localNodeID)
	}
	peerOverlay := ""
	for _, peer := range snapshot.Peers {
		if strings.TrimSpace(peer.NodeID) == peerNodeID {
			peerOverlay = strings.TrimSpace(peer.OverlayIP)
			break
		}
	}
	if peerOverlay == "" {
		return ResolvedSession{}, fmt.Errorf("peer %s overlay address not found", peerNodeID)
	}
	return ResolvedSession{
		Spec:         spec,
		LocalNodeID:  localNodeID,
		PeerNodeID:   peerNodeID,
		PeerOverlay:  peerOverlay,
		Role:         role,
		LocalDevice:  localDevice,
		RemoteDevice: remoteDevice,
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
		return errors.New("usb forwarding manager did not stop in time")
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

func (m *Manager) runSession(ctx context.Context, session ResolvedSession) {
	ticker := time.NewTicker(m.cfg.ReconcileInterval)
	defer ticker.Stop()
	var attachedPort string
	var exportedBusID string
	for {
		if ctx.Err() != nil {
			if session.Role == "attacher" && attachedPort != "" {
				m.detachAttachedPort(context.Background(), attachedPort)
			}
			if session.Role == "exporter" && exportedBusID != "" {
				m.unbindExport(context.Background(), exportedBusID)
			}
			m.setReport(m.baseReport(session, "cancelled", "", "context"))
			return
		}

		switch session.Role {
		case "exporter":
			busID, err := m.cfg.Resolver.ResolveLocalBusID(session.LocalDevice)
			if err != nil {
				m.setReport(m.baseReport(session, "error", err.Error(), "resolve"))
				break
			}
			exportedBusID = busID
			if err := m.ensureUSBIPDaemon(ctx); err != nil {
				m.setReport(m.baseReport(session, "error", err.Error(), "daemon"))
				break
			}
			if err := m.bindExport(ctx, busID); err != nil {
				m.setReport(m.baseReport(session, "error", err.Error(), "bind"))
				break
			}
			report := m.baseReport(session, "exported", "", "")
			report.ClaimedBy = busID
			m.setReport(report)
		case "attacher":
			busID, err := m.cfg.Resolver.ResolveRemoteBusID(ctx, m.cfg.Executor, session.PeerOverlay, session.RemoteDevice)
			if err != nil {
				m.setReport(m.baseReport(session, "error", err.Error(), "resolve"))
				break
			}
			port, err := m.cfg.Resolver.ResolveAttachedPort(ctx, m.cfg.Executor, session.PeerOverlay, busID)
			if err == nil && port != "" {
				attachedPort = port
				report := m.baseReport(session, "attached", "", "")
				report.ClaimedBy = port
				m.setReport(report)
				break
			}
			if err := m.attachRemote(ctx, session.PeerOverlay, busID); err != nil {
				m.setReport(m.baseReport(session, "error", err.Error(), "attach"))
				break
			}
			port, err = m.cfg.Resolver.ResolveAttachedPort(ctx, m.cfg.Executor, session.PeerOverlay, busID)
			if err != nil {
				m.setReport(m.baseReport(session, "error", err.Error(), "port"))
				break
			}
			attachedPort = port
			report := m.baseReport(session, "attached", "", "")
			report.ClaimedBy = port
			m.setReport(report)
		}

		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}

func (m *Manager) baseReport(session ResolvedSession, status, lastError, claimedBy string) SessionReport {
	report := ConfiguredReport(session.Spec, session.LocalNodeID)
	report.Status = status
	report.Local = session.LocalDevice
	report.Remote = session.RemoteDevice
	report.LastError = strings.TrimSpace(lastError)
	report.ClaimedBy = strings.TrimSpace(claimedBy)
	report.UpdatedAt = time.Now().UTC()
	return report
}

func (m *Manager) setReport(report SessionReport) {
	m.mu.Lock()
	m.reports[report.SessionID] = report
	m.mu.Unlock()
	if m.cfg.OnReport != nil {
		m.cfg.OnReport(report)
	}
}

func (m *Manager) ensureUSBIPDaemon(ctx context.Context) error {
	commandCtx, cancel := context.WithTimeout(ctx, m.cfg.CommandTimeout)
	defer cancel()
	_, err := m.cfg.Executor.Run(commandCtx, "usbipd", "-D")
	if err == nil {
		return nil
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "already") || strings.Contains(lower, "exists") {
		return nil
	}
	return err
}

func (m *Manager) bindExport(ctx context.Context, busID string) error {
	commandCtx, cancel := context.WithTimeout(ctx, m.cfg.CommandTimeout)
	defer cancel()
	_, err := m.cfg.Executor.Run(commandCtx, "usbip", "bind", "-b", busID)
	if err == nil {
		return nil
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "already") || strings.Contains(lower, "is already bound") {
		return nil
	}
	return err
}

func (m *Manager) unbindExport(ctx context.Context, busID string) {
	commandCtx, cancel := context.WithTimeout(ctx, m.cfg.CommandTimeout)
	defer cancel()
	_, _ = m.cfg.Executor.Run(commandCtx, "usbip", "unbind", "-b", busID)
}

func (m *Manager) attachRemote(ctx context.Context, host, busID string) error {
	commandCtx, cancel := context.WithTimeout(ctx, m.cfg.CommandTimeout)
	defer cancel()
	_, err := m.cfg.Executor.Run(commandCtx, "usbip", "attach", "-r", host, "-b", busID)
	return err
}

func (m *Manager) detachAttachedPort(ctx context.Context, port string) {
	commandCtx, cancel := context.WithTimeout(ctx, m.cfg.CommandTimeout)
	defer cancel()
	_, _ = m.cfg.Executor.Run(commandCtx, "usbip", "detach", "-p", port)
}

func (LinuxResolver) ResolveLocalBusID(descriptor DeviceDescriptor) (string, error) {
	descriptor = NormalizeDeviceDescriptor(descriptor)
	entries, err := os.ReadDir("/sys/bus/usb/devices")
	if err != nil {
		return "", fmt.Errorf("read /sys/bus/usb/devices: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		base := filepath.Join("/sys/bus/usb/devices", entry.Name())
		vendor := strings.ToLower(strings.TrimSpace(readTrimmed(base + "/idVendor")))
		product := strings.ToLower(strings.TrimSpace(readTrimmed(base + "/idProduct")))
		if descriptor.VendorID != "" && vendor != descriptor.VendorID {
			continue
		}
		if descriptor.ProductID != "" && product != descriptor.ProductID {
			continue
		}
		busNum := strings.TrimSpace(readTrimmed(base + "/busnum"))
		devNum := strings.TrimSpace(readTrimmed(base + "/devnum"))
		if descriptor.BusID != "" && busNum != descriptor.BusID {
			continue
		}
		if descriptor.DeviceID != "" && devNum != descriptor.DeviceID {
			continue
		}
		return entry.Name(), nil
	}
	return "", fmt.Errorf("usb device %s:%s bus=%s dev=%s not found", descriptor.VendorID, descriptor.ProductID, descriptor.BusID, descriptor.DeviceID)
}

func (LinuxResolver) ResolveRemoteBusID(ctx context.Context, executor Executor, host string, descriptor DeviceDescriptor) (string, error) {
	output, err := executor.Run(ctx, "usbip", "list", "-r", host)
	if err != nil {
		return "", err
	}
	descriptor = NormalizeDeviceDescriptor(descriptor)
	busIDPattern := regexp.MustCompile(`(?m)^\s*-\s+([[:alnum:]\-\.]+):\s+.*\(([0-9a-fA-F]{4}):([0-9a-fA-F]{4})\)`)
	matches := busIDPattern.FindAllStringSubmatch(string(output), -1)
	for _, match := range matches {
		busID := strings.TrimSpace(match[1])
		vendor := strings.ToLower(strings.TrimSpace(match[2]))
		product := strings.ToLower(strings.TrimSpace(match[3]))
		if descriptor.VendorID != "" && descriptor.VendorID != vendor {
			continue
		}
		if descriptor.ProductID != "" && descriptor.ProductID != product {
			continue
		}
		return busID, nil
	}
	return "", fmt.Errorf("remote usb device %s:%s not found on %s", descriptor.VendorID, descriptor.ProductID, host)
}

func (LinuxResolver) ResolveAttachedPort(ctx context.Context, executor Executor, host, busID string) (string, error) {
	output, err := executor.Run(ctx, "usbip", "port")
	if err != nil {
		return "", err
	}
	portPattern := regexp.MustCompile(`(?m)^Port\s+(\d+):`)
	targetPattern := regexp.MustCompile(`usbip://` + regexp.QuoteMeta(host) + `:[0-9]+/` + regexp.QuoteMeta(busID))
	lines := strings.Split(string(output), "\n")
	currentPort := ""
	for _, line := range lines {
		if match := portPattern.FindStringSubmatch(line); match != nil {
			currentPort = match[1]
			continue
		}
		if currentPort != "" && targetPattern.MatchString(line) {
			return currentPort, nil
		}
	}
	return "", fmt.Errorf("attached port for %s/%s not found", host, busID)
}

func readTrimmed(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}
