package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"nodeweave/clients/linux-agent/internal/config"
	"nodeweave/clients/linux-agent/internal/state"
	"nodeweave/packages/contracts/go/api"
	contractsclient "nodeweave/packages/contracts/go/client"
	"nodeweave/packages/runtime/go/dataplane"
	"nodeweave/packages/runtime/go/driver"
	"nodeweave/packages/runtime/go/driver/dryrun"
	"nodeweave/packages/runtime/go/driver/linuxexec"
	linuxplandriver "nodeweave/packages/runtime/go/driver/linuxplan"
	"nodeweave/packages/runtime/go/forwarding/serial"
	"nodeweave/packages/runtime/go/forwarding/usb"
	"nodeweave/packages/runtime/go/overlay"
	linuxplan "nodeweave/packages/runtime/go/plan/linux"
	"nodeweave/packages/runtime/go/secureudp"
	"nodeweave/packages/runtime/go/session"
	"nodeweave/packages/runtime/go/stun"
	"nodeweave/packages/runtime/go/tunnel"
)

type Service struct {
	cfg           config.Config
	client        *contractsclient.Client
	state         state.File
	runtimeDriver driver.Driver

	forwardingMu        sync.Mutex
	forwardingSignature string
	serialManager       *serial.Manager
	usbManager          *usb.Manager

	dataplaneMu       sync.Mutex
	dataplaneRuntime  *activeDataplane
	attemptMu         sync.Mutex
	attemptReports    map[string]state.DirectAttemptReportEntry
	pendingAttempts   map[string]api.DirectAttemptInstruction
	scheduledAttempts map[string]context.CancelFunc
	recoveryMu        sync.RWMutex
	recoveryStates    map[string]api.PeerRecoveryState
	warmupMu          sync.Mutex
	warmupWakeCh      chan struct{}
}

const directAttemptReportHistoryLimit = 128

type activeDataplane struct {
	signature     string
	listenAddress string
	secureUDP     *secureudp.Transport
	cancel        context.CancelFunc
	closer        io.Closer
	done          chan struct{}
}

type dataplaneConfig struct {
	mode        string
	spec        dataplane.Spec
	sessionSpec session.Spec
	identity    identity
	signature   string
}

type dataplaneSignature struct {
	Mode      string         `json:"mode"`
	Identity  string         `json:"identity,omitempty"`
	Spec      dataplane.Spec `json:"spec"`
	Session   session.Spec   `json:"session,omitempty"`
	Tunnel    string         `json:"tunnel,omitempty"`
	TunnelDev string         `json:"tunnel_device,omitempty"`
}

func New(cfg config.Config) (*Service, error) {
	svc := &Service{
		cfg:               cfg,
		client:            contractsclient.New(cfg.ServerURL),
		runtimeDriver:     newRuntimeDriver(cfg),
		attemptReports:    map[string]state.DirectAttemptReportEntry{},
		pendingAttempts:   map[string]api.DirectAttemptInstruction{},
		scheduledAttempts: map[string]context.CancelFunc{},
		recoveryStates:    map[string]api.PeerRecoveryState{},
		warmupWakeCh:      make(chan struct{}, 1),
	}

	if currentState, err := state.Load(cfg.StatePath); err == nil {
		svc.state = currentState
	}
	if recoveryStates, err := state.LoadRecoveryStates(cfg.RecoveryStatePath); err == nil {
		svc.setRecoveryStates(recoveryStates)
	}
	if report, err := state.LoadDirectAttemptReport(cfg.DirectAttemptReportPath); err == nil {
		for _, entry := range report.Entries {
			attemptID := strings.TrimSpace(entry.AttemptID)
			if attemptID == "" {
				continue
			}
			entry.AttemptID = attemptID
			entry.PeerNodeID = strings.TrimSpace(entry.PeerNodeID)
			entry.Profile = strings.TrimSpace(entry.Profile)
			fallback := entry.IssuedAt
			if fallback.IsZero() {
				fallback = entry.ExecuteAt
			}
			entry.Candidates = api.NormalizeDirectAttemptCandidates(entry.Candidates, fallback)
			if strings.TrimSpace(entry.Phase) == "" && len(entry.Candidates) > 0 {
				entry.Phase = api.NormalizeDirectAttemptPhase(entry.Candidates[0].Phase, entry.Candidates[0].Source)
			}
			svc.attemptReports[attemptID] = entry
		}
	}
	if attempts, err := state.LoadDirectAttempts(cfg.DirectAttemptPath); err == nil {
		now := time.Now().UTC()
		svc.attemptMu.Lock()
		changed := svc.queueDirectAttemptsLocked(attempts, now)
		expired := svc.pruneExpiredPendingDirectAttemptsLocked(now)
		for _, instruction := range expired {
			svc.markExpiredDirectAttemptReportLocked(instruction, now)
			changed = true
		}
		svc.syncPendingDirectAttemptReportsLocked(false, now)
		if changed {
			if err := svc.persistPendingDirectAttemptsLocked(); err != nil {
				log.Printf("persist normalized direct attempts: %v", err)
			}
		}
		if err := svc.persistDirectAttemptReportLocked(); err != nil {
			log.Printf("persist direct attempt report: %v", err)
		}
		svc.attemptMu.Unlock()
	}
	return svc, nil
}

func (s *Service) CurrentState() state.File {
	return s.state
}

func (s *Service) CurrentRuntime() (overlay.Snapshot, error) {
	return state.LoadRuntime(s.cfg.RuntimePath)
}

func (s *Service) EnsureEnrolled(ctx context.Context) error {
	resolvedIdentity, err := s.resolveIdentity()
	if err != nil {
		return err
	}
	if s.state.Node.ID != "" && s.state.NodeToken != "" {
		return nil
	}
	if !s.cfg.AutoEnroll {
		return errors.New("agent is not enrolled and auto_enroll is false")
	}

	resp, err := s.client.RegisterDevice(ctx, api.DeviceRegistrationRequest{
		DeviceName:        s.cfg.DeviceName,
		Platform:          s.cfg.Platform,
		Version:           s.cfg.Version,
		PublicKey:         resolvedIdentity.PublicKey,
		Capabilities:      []string{"agent"},
		RegistrationToken: s.cfg.RegistrationToken,
	})
	if err != nil {
		return fmt.Errorf("register device: %w", err)
	}

	s.state = state.File{
		ServerURL:       s.cfg.ServerURL,
		Device:          resp.Device,
		Node:            resp.Node,
		NodeToken:       resp.NodeToken,
		Bootstrap:       resp.Bootstrap,
		LastBootstrapAt: time.Now().UTC(),
	}

	if err := s.persistState(); err != nil {
		return err
	}
	return s.persistBootstrap()
}

func (s *Service) SyncBootstrap(ctx context.Context) error {
	bootstrap, err := s.client.GetBootstrap(ctx, s.state.Node.ID, s.state.NodeToken)
	if err != nil {
		return fmt.Errorf("get bootstrap: %w", err)
	}
	s.state.Bootstrap = bootstrap
	s.state.LastBootstrapAt = time.Now().UTC()
	if err := s.persistState(); err != nil {
		return err
	}
	return s.persistBootstrap()
}

func (s *Service) ApplyRuntime(ctx context.Context) error {
	snapshot, err := overlay.Compile(s.state.Bootstrap, overlay.Config{
		InterfaceName: s.cfg.InterfaceName,
		MTU:           s.cfg.InterfaceMTU,
	}, s.runtimeDriver.Name())
	if err != nil {
		return fmt.Errorf("compile overlay runtime: %w", err)
	}

	if err := state.SaveRuntime(s.cfg.RuntimePath, snapshot); err != nil {
		return err
	}
	plan := linuxplan.Build(snapshot)
	if err := state.SavePlan(s.cfg.PlanPath, plan); err != nil {
		return err
	}
	sessionSpec := session.Build(snapshot, session.Config{
		ListenAddress: s.cfg.SessionListenAddress,
	})
	if err := state.SaveSession(s.cfg.SessionPath, sessionSpec); err != nil {
		return err
	}

	sessionReport, sessionErr := session.Probe(ctx, sessionSpec, session.ProbeConfig{
		Mode:    s.cfg.SessionProbeMode,
		Timeout: s.cfg.SessionProbeTimeout,
	})
	if err := state.SaveSessionReport(s.cfg.SessionReportPath, sessionReport); err != nil {
		return err
	}

	dataplaneSpec, err := dataplane.Build(snapshot, sessionSpec, dataplane.Config{
		ListenAddress: s.cfg.DataplaneListenAddress,
		SessionReport: sessionReport,
	})
	if err != nil {
		return fmt.Errorf("compile dataplane spec: %w", err)
	}
	if err := state.SaveDataplane(s.cfg.DataplanePath, dataplaneSpec); err != nil {
		return err
	}

	report, applyErr := s.runtimeDriver.Apply(ctx, snapshot)
	if err := state.SaveApplyReport(s.cfg.ApplyReportPath, report); err != nil {
		return err
	}
	if err := s.persistForwardingState(); err != nil {
		return err
	}
	if applyErr != nil {
		return fmt.Errorf("apply overlay runtime: %w", applyErr)
	}
	if sessionErr != nil {
		return fmt.Errorf("probe overlay sessions: %w", sessionErr)
	}
	return nil
}

func (s *Service) SendHeartbeat(ctx context.Context) error {
	_, err := s.sendHeartbeat(ctx)
	return err
}

func (s *Service) persistForwardingState() error {
	serialSpecs := make([]serial.SessionSpec, 0, len(s.cfg.SerialForwards))
	serialReports := make([]serial.SessionReport, 0, len(s.cfg.SerialForwards))
	for _, spec := range s.cfg.SerialForwards {
		spec = serial.NormalizeSessionSpec(spec)
		if strings.TrimSpace(spec.NodeID) == "" {
			spec.NodeID = s.state.Node.ID
		}
		serialSpecs = append(serialSpecs, spec)
		serialReports = append(serialReports, serial.ConfiguredReport(spec, s.cfg.Platform))
	}
	if err := state.SaveSerialForwards(s.cfg.SerialForwardPath, serialSpecs); err != nil {
		return err
	}
	if err := state.SaveSerialForwardReport(s.cfg.SerialForwardReportPath, serialReports); err != nil {
		return err
	}

	usbSpecs := make([]usb.SessionSpec, 0, len(s.cfg.USBForwards))
	usbReports := make([]usb.SessionReport, 0, len(s.cfg.USBForwards))
	for _, spec := range s.cfg.USBForwards {
		spec = usb.NormalizeSessionSpec(spec)
		if strings.TrimSpace(spec.NodeID) == "" {
			spec.NodeID = s.state.Node.ID
		}
		usbSpecs = append(usbSpecs, spec)
		usbReports = append(usbReports, usb.ConfiguredReport(spec, s.cfg.Platform))
	}
	if err := state.SaveUSBForwards(s.cfg.USBForwardPath, usbSpecs); err != nil {
		return err
	}
	if err := state.SaveUSBForwardReport(s.cfg.USBForwardReportPath, usbReports); err != nil {
		return err
	}
	return nil
}

func (s *Service) sendHeartbeat(ctx context.Context) (api.HeartbeatResponse, error) {
	resolvedIdentity, err := s.resolveIdentity()
	if err != nil {
		return api.HeartbeatResponse{}, err
	}
	now := time.Now().UTC()
	endpointRecords := make([]api.EndpointObservation, 0, len(s.cfg.AdvertiseEndpoints)+len(s.cfg.STUNServers)+1)
	for _, endpoint := range s.cfg.AdvertiseEndpoints {
		endpointRecords = append(endpointRecords, api.EndpointObservation{
			Address:    endpoint,
			Source:     "static",
			ObservedAt: now,
		})
	}
	endpointRecords = append(endpointRecords, s.dataplaneListenerEndpointRecords(now)...)

	stunReport, stunErr := s.discoverSTUN(ctx)
	if stunErr != nil {
		log.Printf("stun discovery failed: %v", stunErr)
	}
	for _, result := range stunReport.Servers {
		if result.Status != "reachable" || strings.TrimSpace(result.ReflexiveAddress) == "" {
			continue
		}
		endpointRecords = append(endpointRecords, api.EndpointObservation{
			Address:    result.ReflexiveAddress,
			Source:     "stun",
			ObservedAt: stunReport.GeneratedAt,
		})
	}
	endpointRecords, endpoints := api.NormalizeEndpointObservations(now, nil, endpointRecords)
	natReport := natReportFromSTUN(stunReport)
	peerTransportStates := []api.PeerTransportState{}
	if transport := s.sharedSTUNTransport(); transport != nil {
		s.attemptMu.Lock()
		peerTransportStates = peerTransportStatesFromReport(transport.Snapshot(), s.attemptReports)
		s.attemptMu.Unlock()
	}
	resp, err := s.client.Heartbeat(ctx, s.state.Node.ID, s.state.NodeToken, api.HeartbeatRequest{
		Endpoints:           endpoints,
		EndpointRecords:     endpointRecords,
		RelayRegion:         s.cfg.RelayRegion,
		Status:              "online",
		PublicKey:           resolvedIdentity.PublicKey,
		NATReport:           natReport,
		PeerTransportStates: peerTransportStates,
	})
	if err != nil {
		return api.HeartbeatResponse{}, fmt.Errorf("send heartbeat: %w", err)
	}
	s.state.Node = resp.Node
	s.state.LastHeartbeatAt = time.Now().UTC()
	if err := s.persistState(); err != nil {
		return api.HeartbeatResponse{}, err
	}
	if err := state.SaveRecoveryStates(s.cfg.RecoveryStatePath, resp.PeerRecoveryStates); err != nil {
		return api.HeartbeatResponse{}, fmt.Errorf("save recovery states: %w", err)
	}
	s.setRecoveryStates(resp.PeerRecoveryStates)
	s.scheduleDirectAttempts(resp.DirectAttempts)
	return resp, nil
}

func (s *Service) setRecoveryStates(states []api.PeerRecoveryState) {
	index := make(map[string]api.PeerRecoveryState, len(states))
	for _, recoveryState := range states {
		peerNodeID := strings.TrimSpace(recoveryState.PeerNodeID)
		if peerNodeID == "" {
			continue
		}
		recoveryState.PeerNodeID = peerNodeID
		index[peerNodeID] = recoveryState
	}
	s.recoveryMu.Lock()
	s.recoveryStates = index
	s.recoveryMu.Unlock()
	now := time.Now().UTC()
	s.attemptMu.Lock()
	if s.reconcilePendingDirectAttemptsLocked(now) {
		if err := s.persistPendingDirectAttemptsLocked(); err != nil {
			log.Printf("persist direct attempts failed: %v", err)
		}
		if err := s.persistDirectAttemptReportLocked(); err != nil {
			log.Printf("persist direct attempt report failed: %v", err)
		}
	}
	s.attemptMu.Unlock()
	s.signalWarmupRecheck()
}

func (s *Service) ensureWarmupWakeCh() chan struct{} {
	s.warmupMu.Lock()
	defer s.warmupMu.Unlock()
	if s.warmupWakeCh == nil {
		s.warmupWakeCh = make(chan struct{}, 1)
	}
	return s.warmupWakeCh
}

func (s *Service) signalWarmupRecheck() {
	wakeCh := s.ensureWarmupWakeCh()
	select {
	case wakeCh <- struct{}{}:
	default:
	}
}

func (s *Service) recoveryStateForPeer(peerNodeID string) (api.PeerRecoveryState, bool) {
	peerNodeID = strings.TrimSpace(peerNodeID)
	if peerNodeID == "" {
		return api.PeerRecoveryState{}, false
	}
	s.recoveryMu.RLock()
	recoveryState, ok := s.recoveryStates[peerNodeID]
	s.recoveryMu.RUnlock()
	return recoveryState, ok
}

func (s *Service) warmupBlockedUntil(peerNodeID string, now time.Time) (time.Time, bool) {
	recoveryState, ok := s.recoveryStateForPeer(peerNodeID)
	if !ok || !recoveryState.Blocked {
		return time.Time{}, false
	}
	if !recoveryState.NextProbeAt.IsZero() && recoveryState.NextProbeAt.After(now) {
		return recoveryState.NextProbeAt, true
	}
	if recoveryState.NextProbeAt.IsZero() && !recoveryState.BlockedUntil.IsZero() && recoveryState.BlockedUntil.After(now) {
		return recoveryState.BlockedUntil, true
	}
	return time.Time{}, false
}

func (s *Service) heartbeatAndRefreshBootstrap(ctx context.Context) error {
	resp, err := s.sendHeartbeat(ctx)
	if err != nil {
		return err
	}
	return s.refreshBootstrapIfNeeded(ctx, resp.BootstrapVersion)
}

func (s *Service) refreshBootstrapIfNeeded(ctx context.Context, observedVersion int) error {
	if observedVersion <= 0 || observedVersion <= s.state.Bootstrap.Version {
		return nil
	}
	log.Printf("heartbeat observed newer bootstrap version=%d current=%d; refreshing bootstrap", observedVersion, s.state.Bootstrap.Version)
	if err := s.SyncBootstrap(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("refresh bootstrap after heartbeat: %w", err)
	}
	if err := s.ApplyRuntime(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("refresh runtime after heartbeat: %w", err)
	}
	s.reloadForwardingRuntimes(context.WithoutCancel(ctx))
	if err := s.reloadDataplane(ctx); err != nil {
		return fmt.Errorf("refresh dataplane after heartbeat: %w", err)
	}
	return nil
}

func (s *Service) Run(ctx context.Context) error {
	if err := s.EnsureEnrolled(ctx); err != nil {
		return err
	}
	if responder, err := s.startResponder(ctx); err != nil {
		return err
	} else if responder != nil {
		defer func() {
			_ = responder.Close()
		}()
	}
	if err := s.SyncBootstrap(ctx); err != nil {
		return err
	}
	if err := s.ApplyRuntime(ctx); err != nil {
		return err
	}
	s.reloadForwardingRuntimes(ctx)
	if err := s.reloadDataplane(ctx); err != nil {
		return err
	}
	defer s.stopDataplane()
	defer s.stopForwardingRuntimes()
	if err := s.heartbeatAndRefreshBootstrap(ctx); err != nil {
		return err
	}

	heartbeatTicker := time.NewTicker(s.cfg.HeartbeatInterval)
	defer heartbeatTicker.Stop()

	bootstrapTicker := time.NewTicker(s.cfg.BootstrapInterval)
	defer bootstrapTicker.Stop()

	log.Printf("linux-agent started for node %s", s.state.Node.ID)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeatTicker.C:
			if err := s.heartbeatAndRefreshBootstrap(context.WithoutCancel(ctx)); err != nil {
				log.Printf("heartbeat failed: %v", err)
			}
		case <-bootstrapTicker.C:
			if err := s.SyncBootstrap(context.WithoutCancel(ctx)); err != nil {
				log.Printf("bootstrap sync failed: %v", err)
				continue
			}
			if err := s.ApplyRuntime(context.WithoutCancel(ctx)); err != nil {
				log.Printf("runtime apply failed: %v", err)
				continue
			}
			s.reloadForwardingRuntimes(context.WithoutCancel(ctx))
			if err := s.reloadDataplane(ctx); err != nil {
				log.Printf("dataplane reload failed: %v", err)
				continue
			}
			if err := s.heartbeatAndRefreshBootstrap(context.WithoutCancel(ctx)); err != nil {
				log.Printf("post-reload heartbeat failed: %v", err)
			}
		}
	}
}

func (s *Service) persistState() error {
	s.state.ServerURL = s.cfg.ServerURL
	return state.Save(s.cfg.StatePath, s.state)
}

func (s *Service) persistBootstrap() error {
	if s.cfg.BootstrapPath == "" {
		return nil
	}
	raw, err := json.MarshalIndent(s.state.Bootstrap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal bootstrap: %w", err)
	}
	if err := os.MkdirAll(filepathDir(s.cfg.BootstrapPath), 0o755); err != nil {
		return fmt.Errorf("create bootstrap dir: %w", err)
	}
	if err := os.WriteFile(s.cfg.BootstrapPath, raw, 0o644); err != nil {
		return fmt.Errorf("write bootstrap snapshot: %w", err)
	}
	return nil
}

func filepathDir(path string) string {
	return filepath.Dir(path)
}

func (s *Service) startResponder(ctx context.Context) (*session.Responder, error) {
	if strings.ToLower(strings.TrimSpace(s.cfg.SessionProbeMode)) != "udp" {
		return nil, nil
	}
	if strings.TrimSpace(s.cfg.SessionListenAddress) == "" {
		return nil, nil
	}

	responder, err := session.NewResponder(s.cfg.SessionListenAddress, s.state.Node.ID)
	if err != nil {
		return nil, fmt.Errorf("start udp responder: %w", err)
	}
	log.Printf("session responder listening on %s", responder.Address())
	go func() {
		if err := responder.Serve(ctx); err != nil && ctx.Err() == nil {
			log.Printf("session responder failed: %v", err)
		}
	}()
	return responder, nil
}

func (s *Service) startDataplane(ctx context.Context) (io.Closer, error) {
	config, err := s.loadDataplaneConfig()
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, nil
	}
	closer, err := s.startDataplaneWithConfig(ctx, *config)
	if err != nil {
		return nil, err
	}
	if config.mode == "secure-udp" {
		s.scheduleDirectAttempts(nil)
	}
	return closer, nil
}

func (s *Service) reloadDataplane(ctx context.Context) error {
	config, err := s.loadDataplaneConfig()
	if err != nil {
		return err
	}
	if config == nil {
		s.dataplaneMu.Lock()
		s.stopDataplaneLocked()
		s.dataplaneMu.Unlock()
		return s.clearTransportReport()
	}

	rescheduleDirectAttempts := false
	s.dataplaneMu.Lock()
	if s.dataplaneRuntime != nil && s.dataplaneRuntime.signature == config.signature {
		s.dataplaneMu.Unlock()
		if config.mode != "secure-udp" {
			return s.clearTransportReport()
		}
		return nil
	}

	s.stopDataplaneLocked()

	if _, err := s.startDataplaneWithConfig(ctx, *config); err != nil {
		s.dataplaneMu.Unlock()
		return err
	}
	rescheduleDirectAttempts = config.mode == "secure-udp"
	s.dataplaneMu.Unlock()
	if config.mode != "secure-udp" {
		return s.clearTransportReport()
	}
	if rescheduleDirectAttempts {
		s.scheduleDirectAttempts(nil)
	}
	return nil
}

func (s *Service) stopDataplane() {
	s.dataplaneMu.Lock()
	defer s.dataplaneMu.Unlock()
	s.stopDataplaneLocked()
}

func (s *Service) stopDataplaneLocked() {
	s.cancelScheduledDirectAttempts()
	if s.dataplaneRuntime == nil {
		return
	}
	if s.dataplaneRuntime.cancel != nil {
		s.dataplaneRuntime.cancel()
	}
	if s.dataplaneRuntime.closer != nil {
		_ = s.dataplaneRuntime.closer.Close()
	}
	if s.dataplaneRuntime.done != nil {
		<-s.dataplaneRuntime.done
	}
	s.dataplaneRuntime = nil
}

func (s *Service) cancelScheduledDirectAttempts() {
	s.attemptMu.Lock()
	defer s.attemptMu.Unlock()
	for attemptID, cancel := range s.scheduledAttempts {
		if cancel != nil {
			cancel()
		}
		delete(s.scheduledAttempts, attemptID)
	}
}

func (s *Service) dataplaneListenerEndpointRecords(observedAt time.Time) []api.EndpointObservation {
	s.dataplaneMu.Lock()
	defer s.dataplaneMu.Unlock()

	if s.dataplaneRuntime == nil {
		return nil
	}
	address, ok := normalizeAdvertisableListenerAddress(s.dataplaneRuntime.listenAddress)
	if !ok {
		return nil
	}
	return []api.EndpointObservation{{
		Address:    address,
		Source:     "listener",
		ObservedAt: observedAt,
	}}
}

func (s *Service) loadDataplaneConfig() (*dataplaneConfig, error) {
	mode := strings.ToLower(strings.TrimSpace(s.cfg.DataplaneMode))
	if mode == "" || mode == "off" {
		return nil, nil
	}

	spec, err := state.LoadDataplane(s.cfg.DataplanePath)
	if err != nil {
		return nil, fmt.Errorf("load dataplane spec: %w", err)
	}
	if strings.TrimSpace(spec.ListenAddress) == "" {
		return nil, nil
	}

	config := dataplaneConfig{
		mode: mode,
		spec: spec,
	}

	var signature dataplaneSignature
	signature.Mode = mode
	signature.Spec = spec
	signature.Spec.GeneratedAt = time.Time{}
	signature.Tunnel = strings.ToLower(strings.TrimSpace(s.cfg.TunnelMode))
	signature.TunnelDev = strings.TrimSpace(s.cfg.TunnelName)
	if signature.TunnelDev == "" {
		signature.TunnelDev = strings.TrimSpace(s.cfg.InterfaceName)
	}

	switch mode {
	case "udp":
	case "secure-udp":
		sessionSpec, err := state.LoadSession(s.cfg.SessionPath)
		if err != nil {
			return nil, fmt.Errorf("load session spec: %w", err)
		}
		resolvedIdentity, err := s.resolveIdentity()
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(resolvedIdentity.PrivateKey) == "" {
			return nil, fmt.Errorf("secure-udp dataplane requires private_key_path")
		}
		config.sessionSpec = sessionSpec
		config.identity = resolvedIdentity
		signature.Session = sessionSpec
		signature.Session.GeneratedAt = time.Time{}
		signature.Identity = resolvedIdentity.PublicKey
	default:
		return nil, fmt.Errorf("unsupported dataplane mode %q", s.cfg.DataplaneMode)
	}

	rawSignature, err := json.Marshal(signature)
	if err != nil {
		return nil, fmt.Errorf("marshal dataplane signature: %w", err)
	}
	config.signature = string(rawSignature)
	return &config, nil
}

func (s *Service) startDataplaneWithConfig(parentCtx context.Context, config dataplaneConfig) (io.Closer, error) {
	runtimeCtx, cancel := context.WithCancel(parentCtx)

	runtime := &activeDataplane{
		signature: config.signature,
		cancel:    cancel,
		done:      make(chan struct{}),
	}

	var transport dataplane.Transport
	var err error
	switch config.mode {
	case "udp":
		transport, err = dataplane.ListenUDP(config.spec.ListenAddress)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("start dataplane udp transport: %w", err)
		}
	case "secure-udp":
		secureTransport, err := secureudp.Listen(secureudp.Config{
			NodeID:         config.spec.NodeID,
			ListenAddress:  config.spec.ListenAddress,
			PrivateKey:     config.identity.PrivateKey,
			Peers:          config.sessionSpec.Peers,
			RelayAddresses: relayAddresses(config.sessionSpec),
		})
		if err != nil {
			cancel()
			return nil, fmt.Errorf("start secure udp dataplane transport: %w", err)
		}
		runtime.secureUDP = secureTransport
		if err := announceRelays(runtimeCtx, secureTransport, config.sessionSpec); err != nil {
			cancel()
			_ = secureTransport.Close()
			return nil, err
		}
		s.startTransportReportSync(runtimeCtx, secureTransport)
		transport = secureTransport
	default:
		cancel()
		return nil, fmt.Errorf("unsupported dataplane mode %q", s.cfg.DataplaneMode)
	}
	runtime.closer = transport
	runtime.listenAddress = transport.Address()

	var sink dataplane.Sink = loggingSink{}
	closer := io.Closer(transport)
	if strings.ToLower(strings.TrimSpace(s.cfg.TunnelMode)) == "linux" {
		tunName := strings.TrimSpace(s.cfg.TunnelName)
		if tunName == "" {
			tunName = s.cfg.InterfaceName
		}
		device, err := tunnel.OpenLinux(tunName)
		if err != nil {
			cancel()
			_ = transport.Close()
			return nil, fmt.Errorf("open linux tun device: %w", err)
		}
		pump := tunnel.NewPump(device)
		engine := dataplane.NewEngine(config.spec, transport, pump)
		pump.AttachEngine(engine)
		log.Printf("dataplane listener mode=%s listening on %s with tunnel %s", config.mode, transport.Address(), device.Name())
		go func() {
			defer close(runtime.done)
			if err := pump.Run(runtimeCtx); err != nil && runtimeCtx.Err() == nil {
				log.Printf("dataplane pump failed: %v", err)
			}
		}()
		if config.mode == "secure-udp" {
			if secureTransport, ok := transport.(*secureudp.Transport); ok {
				s.startDirectWarmup(runtimeCtx, secureTransport, config.sessionSpec)
			}
		}
		runtime.closer = multiCloser{closers: []io.Closer{device, transport}}
		s.dataplaneRuntime = runtime
		return runtime.closer, nil
	}

	engine := dataplane.NewEngine(config.spec, transport, sink)
	log.Printf("dataplane listener mode=%s listening on %s", config.mode, transport.Address())
	go func() {
		defer close(runtime.done)
		if err := engine.Serve(runtimeCtx); err != nil && runtimeCtx.Err() == nil {
			log.Printf("dataplane listener failed: %v", err)
		}
	}()
	if config.mode == "secure-udp" {
		if secureTransport, ok := transport.(*secureudp.Transport); ok {
			s.startDirectWarmup(runtimeCtx, secureTransport, config.sessionSpec)
		}
	}
	runtime.closer = closer
	s.dataplaneRuntime = runtime
	return runtime.closer, nil
}

func announceRelays(ctx context.Context, transport *secureudp.Transport, spec session.Spec) error {
	addresses := relayAddresses(spec)
	if len(addresses) == 0 {
		return nil
	}
	for _, address := range addresses {
		if err := transport.Announce(ctx, address); err != nil {
			return fmt.Errorf("announce relay %s: %w", address, err)
		}
	}
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, address := range addresses {
					if err := transport.Announce(context.WithoutCancel(ctx), address); err != nil && ctx.Err() == nil {
						log.Printf("relay announce failed for %s: %v", address, err)
					}
				}
			}
		}
	}()
	return nil
}

func relayAddresses(spec session.Spec) []string {
	seen := make(map[string]struct{})
	addresses := make([]string, 0)
	for _, peer := range spec.Peers {
		for _, candidate := range peer.Candidates {
			if candidate.Kind != "relay" {
				continue
			}
			address := strings.TrimSpace(candidate.Address)
			if address == "" {
				continue
			}
			if _, ok := seen[address]; ok {
				continue
			}
			seen[address] = struct{}{}
			addresses = append(addresses, address)
		}
	}
	return addresses
}

func natReportFromSTUN(report stun.Report) api.NATReport {
	natReport := api.NATReport{
		GeneratedAt:              report.GeneratedAt,
		MappingBehavior:          report.MappingBehavior,
		SampleCount:              report.SampleCount,
		SelectedReflexiveAddress: report.SelectedReflexiveAddress,
		Reachable:                report.Reachable,
		Samples:                  make([]api.NATSample, 0, len(report.Servers)),
	}
	for _, sample := range report.Servers {
		natReport.Samples = append(natReport.Samples, api.NATSample{
			Server:           sample.Server,
			Status:           sample.Status,
			RTTMillis:        sample.RTTMillis,
			ReflexiveAddress: sample.ReflexiveAddress,
			Error:            sample.Error,
		})
	}
	return natReport
}

func peerTransportStatesFromReport(report secureudp.Report, attemptReports map[string]state.DirectAttemptReportEntry) []api.PeerTransportState {
	reportProfiles := latestDirectAttemptReportsByPeer(attemptReports)
	states := make([]api.PeerTransportState, 0, len(report.Peers))
	for _, peer := range report.Peers {
		state := api.PeerTransportState{
			PeerNodeID:                      peer.NodeID,
			ActiveKind:                      peer.ActiveKind,
			ActiveAddress:                   peer.ActiveAddress,
			ReportedAt:                      report.GeneratedAt,
			LastDirectAttemptAt:             peer.LastDirectAttemptAt,
			LastDirectAttemptResult:         peer.LastDirectAttemptResult,
			LastDirectAttemptProfile:        "",
			LastDirectAttemptReachedSource:  peer.LastDirectAttemptReachedSource,
			LastDirectAttemptPhase:          peer.LastDirectAttemptPhase,
			LastDirectAttemptCandidateCount: peer.LastDirectAttemptCandidateCount,
			LastDirectSuccessAt:             peer.LastDirectSuccessAt,
			ConsecutiveDirectFailures:       peer.ConsecutiveDirectFailures,
		}
		if state.PeerNodeID == "" {
			continue
		}
		if reportEntry, ok := reportProfiles[state.PeerNodeID]; ok {
			applyDirectAttemptReportToTransportState(&state, reportEntry)
		}
		states = append(states, state)
	}
	return states
}

func latestDirectAttemptReportsByPeer(attemptReports map[string]state.DirectAttemptReportEntry) map[string]state.DirectAttemptReportEntry {
	byPeer := make(map[string]state.DirectAttemptReportEntry, len(attemptReports))
	for _, entry := range attemptReports {
		peerNodeID := strings.TrimSpace(entry.PeerNodeID)
		if peerNodeID == "" {
			continue
		}
		current, ok := byPeer[peerNodeID]
		if !ok || directAttemptReportSortTime(entry).After(directAttemptReportSortTime(current)) {
			byPeer[peerNodeID] = entry
		}
	}
	return byPeer
}

func directAttemptReportSortTime(entry state.DirectAttemptReportEntry) time.Time {
	switch {
	case !entry.CompletedAt.IsZero():
		return entry.CompletedAt.UTC()
	case !entry.StartedAt.IsZero():
		return entry.StartedAt.UTC()
	case !entry.ScheduledAt.IsZero():
		return entry.ScheduledAt.UTC()
	case !entry.LastUpdatedAt.IsZero():
		return entry.LastUpdatedAt.UTC()
	case !entry.ExecuteAt.IsZero():
		return entry.ExecuteAt.UTC()
	default:
		return entry.IssuedAt.UTC()
	}
}

func applyDirectAttemptReportToTransportState(transportState *api.PeerTransportState, entry state.DirectAttemptReportEntry) {
	if transportState == nil {
		return
	}
	profile := strings.TrimSpace(entry.Profile)
	if profile == "" {
		return
	}
	reportAt := directAttemptReportSortTime(entry)
	if !transportState.LastDirectAttemptAt.IsZero() && !reportAt.IsZero() {
		delta := transportState.LastDirectAttemptAt.Sub(reportAt)
		if delta < 0 {
			delta = -delta
		}
		if delta > 15*time.Second {
			return
		}
	}
	transportState.LastDirectAttemptProfile = profile
	if transportState.LastDirectAttemptAt.IsZero() && !reportAt.IsZero() {
		transportState.LastDirectAttemptAt = reportAt
	}
	if strings.TrimSpace(transportState.LastDirectAttemptResult) == "" {
		transportState.LastDirectAttemptResult = strings.TrimSpace(entry.Result)
	}
	if strings.TrimSpace(transportState.LastDirectAttemptReachedSource) == "" {
		transportState.LastDirectAttemptReachedSource = strings.TrimSpace(entry.ReachedSource)
	}
	if strings.TrimSpace(transportState.LastDirectAttemptPhase) == "" {
		transportState.LastDirectAttemptPhase = strings.TrimSpace(entry.Phase)
	}
	if transportState.LastDirectAttemptCandidateCount == 0 && len(entry.Candidates) > 0 {
		transportState.LastDirectAttemptCandidateCount = len(entry.Candidates)
	}
}

func (s *Service) discoverSTUN(ctx context.Context) (stun.Report, error) {
	servers := normalizedStrings(s.cfg.STUNServers)
	if len(servers) == 0 {
		return stun.Report{}, s.clearSTUNReport()
	}

	var (
		report stun.Report
		err    error
	)
	if transport := s.sharedSTUNTransport(); transport != nil {
		report, err = transport.DiscoverSTUN(ctx, servers, s.cfg.STUNTimeout)
	} else {
		report, err = stun.Discover(ctx, servers, s.cfg.STUNTimeout)
	}
	if saveErr := s.saveSTUNReport(report); saveErr != nil {
		if err == nil {
			err = saveErr
		} else {
			err = fmt.Errorf("%w; save stun report: %v", err, saveErr)
		}
	}
	return report, err
}

func (s *Service) sharedSTUNTransport() *secureudp.Transport {
	s.dataplaneMu.Lock()
	defer s.dataplaneMu.Unlock()
	if s.dataplaneRuntime == nil {
		return nil
	}
	return s.dataplaneRuntime.secureUDP
}

func directAttemptWindow(instruction api.DirectAttemptInstruction) time.Duration {
	window := time.Duration(instruction.Window) * time.Millisecond
	if window <= 0 {
		return 600 * time.Millisecond
	}
	return window
}

func normalizeDirectAttempt(instruction api.DirectAttemptInstruction) (api.DirectAttemptInstruction, bool) {
	instruction.AttemptID = strings.TrimSpace(instruction.AttemptID)
	instruction.PeerNodeID = strings.TrimSpace(instruction.PeerNodeID)
	instruction.Profile = strings.TrimSpace(instruction.Profile)
	instruction.Reason = strings.TrimSpace(instruction.Reason)
	if !instruction.IssuedAt.IsZero() {
		instruction.IssuedAt = instruction.IssuedAt.UTC()
	}
	if instruction.AttemptID == "" || instruction.PeerNodeID == "" {
		return api.DirectAttemptInstruction{}, false
	}
	if !instruction.ExecuteAt.IsZero() {
		instruction.ExecuteAt = instruction.ExecuteAt.UTC()
	}
	fallback := instruction.IssuedAt
	if fallback.IsZero() {
		fallback = instruction.ExecuteAt
	}
	instruction.Candidates = api.NormalizeDirectAttemptCandidates(instruction.Candidates, fallback)
	return instruction, true
}

func directAttemptExpired(instruction api.DirectAttemptInstruction, now time.Time) bool {
	if instruction.ExecuteAt.IsZero() {
		return false
	}
	return now.After(instruction.ExecuteAt.Add(directAttemptWindow(instruction)))
}

func isTerminalDirectAttemptStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "completed", "expired", "cancelled":
		return true
	default:
		return false
	}
}

func (s *Service) touchDirectAttemptReportLocked(instruction api.DirectAttemptInstruction, now time.Time) {
	if s.attemptReports == nil {
		s.attemptReports = make(map[string]state.DirectAttemptReportEntry)
	}
	attemptID := strings.TrimSpace(instruction.AttemptID)
	if attemptID == "" {
		return
	}
	entry := s.attemptReports[attemptID]
	entry.AttemptID = attemptID
	entry.PeerNodeID = strings.TrimSpace(instruction.PeerNodeID)
	entry.IssuedAt = instruction.IssuedAt
	entry.ExecuteAt = instruction.ExecuteAt
	entry.Window = instruction.Window
	entry.BurstInterval = instruction.BurstInterval
	entry.Reason = strings.TrimSpace(instruction.Reason)
	entry.Profile = strings.TrimSpace(instruction.Profile)
	entry.Candidates = append([]api.DirectAttemptCandidate(nil), instruction.Candidates...)
	if len(entry.Candidates) > 0 {
		entry.Phase = api.NormalizeDirectAttemptPhase(entry.Candidates[0].Phase, entry.Candidates[0].Source)
	}
	if entry.QueuedAt.IsZero() {
		entry.QueuedAt = now
	}
	if strings.TrimSpace(entry.Status) == "" || isTerminalDirectAttemptStatus(entry.Status) {
		entry.Status = "queued"
		entry.Result = "queued"
		entry.WaitReason = ""
		entry.LastError = ""
		entry.ScheduledAt = time.Time{}
		entry.StartedAt = time.Time{}
		entry.CompletedAt = time.Time{}
		entry.ReachedAddress = ""
		entry.ReachedSource = ""
		entry.ActiveAddress = ""
	}
	entry.LastUpdatedAt = now
	s.attemptReports[attemptID] = entry
}

func (s *Service) setDirectAttemptWaitingLocked(instruction api.DirectAttemptInstruction, now time.Time, waitReason string) {
	s.touchDirectAttemptReportLocked(instruction, now)
	entry := s.attemptReports[strings.TrimSpace(instruction.AttemptID)]
	entry.Status = "waiting_transport"
	entry.Result = "queued"
	entry.WaitReason = strings.TrimSpace(waitReason)
	entry.LastUpdatedAt = now
	s.attemptReports[entry.AttemptID] = entry
}

func (s *Service) markScheduledDirectAttemptReportLocked(instruction api.DirectAttemptInstruction, now time.Time) {
	s.touchDirectAttemptReportLocked(instruction, now)
	entry := s.attemptReports[strings.TrimSpace(instruction.AttemptID)]
	entry.Status = "scheduled"
	entry.Result = "scheduled"
	entry.WaitReason = ""
	entry.LastError = ""
	entry.ScheduledAt = now
	entry.LastUpdatedAt = now
	s.attemptReports[entry.AttemptID] = entry
}

func (s *Service) markExecutingDirectAttemptReportLocked(instruction api.DirectAttemptInstruction, now time.Time) {
	s.touchDirectAttemptReportLocked(instruction, now)
	entry := s.attemptReports[strings.TrimSpace(instruction.AttemptID)]
	entry.Status = "executing"
	entry.Result = "executing"
	entry.WaitReason = ""
	entry.LastError = ""
	if entry.ScheduledAt.IsZero() {
		entry.ScheduledAt = now
	}
	entry.StartedAt = now
	entry.LastUpdatedAt = now
	s.attemptReports[entry.AttemptID] = entry
}

func (s *Service) markCompletedDirectAttemptReportLocked(instruction api.DirectAttemptInstruction, result secureudp.DirectAttemptResult, err error, now time.Time) {
	s.touchDirectAttemptReportLocked(instruction, now)
	entry := s.attemptReports[strings.TrimSpace(instruction.AttemptID)]
	entry.Status = "completed"
	entry.Result = strings.TrimSpace(result.Result)
	if entry.Result == "" {
		entry.Result = "error"
	}
	entry.WaitReason = ""
	entry.LastError = ""
	if err != nil {
		entry.LastError = err.Error()
	} else if strings.TrimSpace(result.Error) != "" {
		entry.LastError = strings.TrimSpace(result.Error)
	}
	if entry.ScheduledAt.IsZero() {
		entry.ScheduledAt = now
	}
	if entry.StartedAt.IsZero() && !result.StartedAt.IsZero() {
		entry.StartedAt = result.StartedAt
	}
	if entry.StartedAt.IsZero() {
		entry.StartedAt = now
	}
	entry.CompletedAt = now
	if !result.CompletedAt.IsZero() {
		entry.CompletedAt = result.CompletedAt
	}
	entry.ReachedAddress = strings.TrimSpace(result.ReachedAddress)
	entry.ReachedSource = strings.TrimSpace(result.ReachedSource)
	if strings.TrimSpace(result.Phase) != "" {
		entry.Phase = strings.TrimSpace(result.Phase)
	}
	entry.ActiveAddress = strings.TrimSpace(result.ActiveAddress)
	entry.LastUpdatedAt = now
	s.attemptReports[entry.AttemptID] = entry
}

func (s *Service) markCanceledDirectAttemptReportLocked(instruction api.DirectAttemptInstruction, now time.Time, waitReason string, err error) {
	s.touchDirectAttemptReportLocked(instruction, now)
	entry := s.attemptReports[strings.TrimSpace(instruction.AttemptID)]
	entry.Status = "waiting_transport"
	entry.Result = "cancelled"
	entry.WaitReason = strings.TrimSpace(waitReason)
	if err != nil {
		entry.LastError = err.Error()
	}
	entry.LastUpdatedAt = now
	entry.CompletedAt = time.Time{}
	s.attemptReports[entry.AttemptID] = entry
}

func (s *Service) markRemovedDirectAttemptReportLocked(instruction api.DirectAttemptInstruction, now time.Time, waitReason string) {
	s.touchDirectAttemptReportLocked(instruction, now)
	entry := s.attemptReports[strings.TrimSpace(instruction.AttemptID)]
	entry.Status = "cancelled"
	entry.Result = "cancelled"
	entry.WaitReason = strings.TrimSpace(waitReason)
	entry.LastError = ""
	entry.CompletedAt = now
	entry.LastUpdatedAt = now
	s.attemptReports[entry.AttemptID] = entry
}

func (s *Service) markExpiredDirectAttemptReportLocked(instruction api.DirectAttemptInstruction, now time.Time) {
	s.touchDirectAttemptReportLocked(instruction, now)
	entry := s.attemptReports[strings.TrimSpace(instruction.AttemptID)]
	entry.Status = "expired"
	entry.Result = "expired"
	entry.WaitReason = ""
	entry.LastError = "direct attempt window expired before execution"
	entry.CompletedAt = now
	entry.LastUpdatedAt = now
	s.attemptReports[entry.AttemptID] = entry
}

func (s *Service) syncPendingDirectAttemptReportsLocked(transportReady bool, now time.Time) {
	for _, instruction := range s.pendingAttempts {
		s.touchDirectAttemptReportLocked(instruction, now)
		if transportReady {
			continue
		}
		s.setDirectAttemptWaitingLocked(instruction, now, "transport_unavailable")
	}
}

func shouldRetainDirectAttempt(instruction api.DirectAttemptInstruction, recoveryState api.PeerRecoveryState) (bool, string) {
	attemptID := strings.TrimSpace(instruction.AttemptID)
	if attemptID == "" {
		return false, "invalid_attempt_id"
	}
	switch strings.TrimSpace(recoveryState.DecisionStatus) {
	case "":
		return true, ""
	case "attempt_issued":
		if strings.TrimSpace(recoveryState.LastIssuedAttemptID) == attemptID {
			return true, ""
		}
		return false, "superseded_by_new_attempt"
	case "blocked":
		return false, "controlplane_blocked"
	case "direct_active":
		return false, "direct_already_active"
	case "local_offline", "peer_offline", "local_no_direct_candidate", "peer_no_direct_candidate":
		return false, strings.TrimSpace(recoveryState.DecisionStatus)
	default:
		return false, "superseded_by_recovery_state"
	}
}

func (s *Service) reconcilePendingDirectAttemptsLocked(now time.Time) bool {
	if len(s.pendingAttempts) == 0 {
		return false
	}
	changed := false
	for attemptID, instruction := range s.pendingAttempts {
		recoveryState, ok := s.recoveryStateForPeer(strings.TrimSpace(instruction.PeerNodeID))
		if !ok {
			continue
		}
		keep, waitReason := shouldRetainDirectAttempt(instruction, recoveryState)
		if keep {
			continue
		}
		if cancel, ok := s.scheduledAttempts[attemptID]; ok {
			cancel()
		}
		delete(s.pendingAttempts, attemptID)
		s.markRemovedDirectAttemptReportLocked(instruction, now, waitReason)
		changed = true
	}
	return changed
}

func (s *Service) trimDirectAttemptReportsLocked() {
	if len(s.attemptReports) <= directAttemptReportHistoryLimit {
		return
	}
	terminalIDs := make([]string, 0, len(s.attemptReports))
	for attemptID, entry := range s.attemptReports {
		if isTerminalDirectAttemptStatus(entry.Status) {
			terminalIDs = append(terminalIDs, attemptID)
		}
	}
	sort.Slice(terminalIDs, func(i, j int) bool {
		left := s.attemptReports[terminalIDs[i]]
		right := s.attemptReports[terminalIDs[j]]
		if left.LastUpdatedAt.Equal(right.LastUpdatedAt) {
			return left.AttemptID < right.AttemptID
		}
		return left.LastUpdatedAt.Before(right.LastUpdatedAt)
	})
	for len(s.attemptReports) > directAttemptReportHistoryLimit && len(terminalIDs) > 0 {
		delete(s.attemptReports, terminalIDs[0])
		terminalIDs = terminalIDs[1:]
	}
}

func (s *Service) persistDirectAttemptReportLocked() error {
	report := state.DirectAttemptReport{
		GeneratedAt: time.Now().UTC(),
		Entries:     make([]state.DirectAttemptReportEntry, 0, len(s.attemptReports)),
	}
	s.trimDirectAttemptReportsLocked()
	for _, entry := range s.attemptReports {
		report.Entries = append(report.Entries, entry)
	}
	sort.Slice(report.Entries, func(i, j int) bool {
		if report.Entries[i].LastUpdatedAt.Equal(report.Entries[j].LastUpdatedAt) {
			return report.Entries[i].AttemptID < report.Entries[j].AttemptID
		}
		return report.Entries[i].LastUpdatedAt.After(report.Entries[j].LastUpdatedAt)
	})
	return state.SaveDirectAttemptReport(s.cfg.DirectAttemptReportPath, report)
}

func (s *Service) queueDirectAttemptsLocked(attempts []api.DirectAttemptInstruction, now time.Time) bool {
	if s.pendingAttempts == nil {
		s.pendingAttempts = make(map[string]api.DirectAttemptInstruction)
	}
	changed := false
	for _, instruction := range attempts {
		instruction, ok := normalizeDirectAttempt(instruction)
		if !ok {
			continue
		}
		if directAttemptExpired(instruction, now) {
			s.markExpiredDirectAttemptReportLocked(instruction, now)
			changed = true
			continue
		}
		for existingID, existing := range s.pendingAttempts {
			if existingID == instruction.AttemptID || strings.TrimSpace(existing.PeerNodeID) != strings.TrimSpace(instruction.PeerNodeID) {
				continue
			}
			if cancel, ok := s.scheduledAttempts[existingID]; ok {
				cancel()
			}
			delete(s.pendingAttempts, existingID)
			s.markRemovedDirectAttemptReportLocked(existing, now, "superseded_by_new_attempt")
			changed = true
		}
		s.pendingAttempts[instruction.AttemptID] = instruction
		s.touchDirectAttemptReportLocked(instruction, now)
		changed = true
	}
	return changed
}

func (s *Service) pruneExpiredPendingDirectAttemptsLocked(now time.Time) []api.DirectAttemptInstruction {
	if len(s.pendingAttempts) == 0 {
		return nil
	}
	expired := make([]api.DirectAttemptInstruction, 0)
	for attemptID, instruction := range s.pendingAttempts {
		if !directAttemptExpired(instruction, now) {
			continue
		}
		delete(s.pendingAttempts, attemptID)
		expired = append(expired, instruction)
	}
	return expired
}

func (s *Service) persistPendingDirectAttemptsLocked() error {
	attempts := make([]api.DirectAttemptInstruction, 0, len(s.pendingAttempts))
	for _, instruction := range s.pendingAttempts {
		attempts = append(attempts, instruction)
	}
	sort.Slice(attempts, func(i, j int) bool {
		if attempts[i].ExecuteAt.Equal(attempts[j].ExecuteAt) {
			if attempts[i].PeerNodeID == attempts[j].PeerNodeID {
				return attempts[i].AttemptID < attempts[j].AttemptID
			}
			return attempts[i].PeerNodeID < attempts[j].PeerNodeID
		}
		if attempts[i].ExecuteAt.IsZero() {
			return true
		}
		if attempts[j].ExecuteAt.IsZero() {
			return false
		}
		return attempts[i].ExecuteAt.Before(attempts[j].ExecuteAt)
	})
	return state.SaveDirectAttempts(s.cfg.DirectAttemptPath, attempts)
}

func (s *Service) finalizePendingDirectAttempt(attemptID string, remove bool, now time.Time) {
	attemptID = strings.TrimSpace(attemptID)
	if attemptID == "" {
		return
	}
	s.attemptMu.Lock()
	changed := false
	if remove {
		if _, ok := s.pendingAttempts[attemptID]; ok {
			delete(s.pendingAttempts, attemptID)
			changed = true
		}
	} else {
		expired := s.pruneExpiredPendingDirectAttemptsLocked(now)
		for _, instruction := range expired {
			s.markExpiredDirectAttemptReportLocked(instruction, now)
			changed = true
		}
	}
	if changed {
		if err := s.persistPendingDirectAttemptsLocked(); err != nil {
			log.Printf("persist direct attempts failed: %v", err)
		}
	}
	if err := s.persistDirectAttemptReportLocked(); err != nil {
		log.Printf("persist direct attempt report failed: %v", err)
	}
	s.attemptMu.Unlock()
}

func (s *Service) scheduleDirectAttempts(attempts []api.DirectAttemptInstruction) {
	now := time.Now().UTC()
	s.attemptMu.Lock()
	changed := s.queueDirectAttemptsLocked(attempts, now)
	if s.reconcilePendingDirectAttemptsLocked(now) {
		changed = true
	}
	expired := s.pruneExpiredPendingDirectAttemptsLocked(now)
	for _, instruction := range expired {
		s.markExpiredDirectAttemptReportLocked(instruction, now)
		changed = true
	}
	transport := s.sharedSTUNTransport()
	s.syncPendingDirectAttemptReportsLocked(transport != nil, now)
	if changed {
		if err := s.persistPendingDirectAttemptsLocked(); err != nil {
			log.Printf("persist direct attempts failed: %v", err)
		}
	}
	if err := s.persistDirectAttemptReportLocked(); err != nil {
		log.Printf("persist direct attempt report failed: %v", err)
	}
	pending := make([]api.DirectAttemptInstruction, 0, len(s.pendingAttempts))
	for _, instruction := range s.pendingAttempts {
		pending = append(pending, instruction)
	}
	s.attemptMu.Unlock()

	if transport == nil {
		return
	}

	for _, instruction := range pending {
		attemptID := strings.TrimSpace(instruction.AttemptID)
		if attemptID == "" {
			continue
		}

		s.attemptMu.Lock()
		if _, exists := s.scheduledAttempts[attemptID]; exists {
			s.attemptMu.Unlock()
			continue
		}
		attemptCtx, cancel := context.WithCancel(context.Background())
		s.scheduledAttempts[attemptID] = cancel
		s.markScheduledDirectAttemptReportLocked(instruction, now)
		if err := s.persistDirectAttemptReportLocked(); err != nil {
			log.Printf("persist direct attempt report failed: %v", err)
		}
		s.attemptMu.Unlock()

		go func(instruction api.DirectAttemptInstruction, transport *secureudp.Transport, attemptCtx context.Context) {
			defer func() {
				s.attemptMu.Lock()
				delete(s.scheduledAttempts, strings.TrimSpace(instruction.AttemptID))
				s.attemptMu.Unlock()
			}()

			startedAt := time.Now().UTC()
			s.attemptMu.Lock()
			s.markExecutingDirectAttemptReportLocked(instruction, startedAt)
			if err := s.persistDirectAttemptReportLocked(); err != nil {
				log.Printf("persist direct attempt report failed: %v", err)
			}
			s.attemptMu.Unlock()

			result, err := transport.ExecuteDirectAttempt(attemptCtx, secureudp.DirectAttempt{
				AttemptID:     instruction.AttemptID,
				PeerNodeID:    instruction.PeerNodeID,
				Candidates:    instruction.Candidates,
				ExecuteAt:     instruction.ExecuteAt,
				Window:        time.Duration(instruction.Window) * time.Millisecond,
				BurstInterval: time.Duration(instruction.BurstInterval) * time.Millisecond,
				Reason:        instruction.Reason,
			})
			if saveErr := s.saveTransportReport(transport.Snapshot()); saveErr != nil {
				log.Printf("transport report save failed after direct attempt=%s peer=%s: %v", instruction.AttemptID, instruction.PeerNodeID, saveErr)
			}
			now := time.Now().UTC()
			if errors.Is(err, context.Canceled) {
				s.attemptMu.Lock()
				pendingInstruction, stillPending := s.pendingAttempts[strings.TrimSpace(instruction.AttemptID)]
				if stillPending && !directAttemptExpired(pendingInstruction, now) {
					s.markCanceledDirectAttemptReportLocked(pendingInstruction, now, "transport_reloading", err)
				}
				if persistErr := s.persistDirectAttemptReportLocked(); persistErr != nil {
					log.Printf("persist direct attempt report failed: %v", persistErr)
				}
				s.attemptMu.Unlock()
				s.finalizePendingDirectAttempt(instruction.AttemptID, false, now)
				return
			}
			s.attemptMu.Lock()
			s.markCompletedDirectAttemptReportLocked(instruction, result, err, now)
			if persistErr := s.persistDirectAttemptReportLocked(); persistErr != nil {
				log.Printf("persist direct attempt report failed: %v", persistErr)
			}
			s.attemptMu.Unlock()
			s.finalizePendingDirectAttempt(instruction.AttemptID, true, now)
			if err != nil {
				log.Printf(
					"direct attempt failed peer=%s attempt_id=%s execute_at=%s result=%s active=%s err=%v",
					instruction.PeerNodeID,
					instruction.AttemptID,
					instruction.ExecuteAt.Format(time.RFC3339Nano),
					result.Result,
					result.ActiveAddress,
					err,
				)
				return
			}
			log.Printf(
				"direct attempt completed peer=%s attempt_id=%s execute_at=%s result=%s reached=%s active=%s",
				instruction.PeerNodeID,
				instruction.AttemptID,
				instruction.ExecuteAt.Format(time.RFC3339Nano),
				result.Result,
				result.ReachedAddress,
				result.ActiveAddress,
			)
		}(instruction, transport, attemptCtx)
	}
}

func (s *Service) startTransportReportSync(ctx context.Context, transport *secureudp.Transport) {
	if transport == nil {
		return
	}
	if err := s.saveTransportReport(transport.Snapshot()); err != nil {
		log.Printf("transport report save failed: %v", err)
	}
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.saveTransportReport(transport.Snapshot()); err != nil && ctx.Err() == nil {
					log.Printf("transport report save failed: %v", err)
				}
			}
		}
	}()
}

func (s *Service) startDirectWarmup(ctx context.Context, transport *secureudp.Transport, spec session.Spec) {
	if transport == nil || s.cfg.DirectWarmupInterval <= 0 {
		return
	}
	wakeCh := s.ensureWarmupWakeCh()

	runWarmup := func() time.Duration {
		statusByPeer := func(snapshot secureudp.Report) map[string]secureudp.PeerStatus {
			statuses := make(map[string]secureudp.PeerStatus, len(snapshot.Peers))
			for _, peerStatus := range snapshot.Peers {
				statuses[peerStatus.NodeID] = peerStatus
			}
			return statuses
		}
		computeNextWait := func(snapshot secureudp.Report, observedAt time.Time, fallback time.Duration) time.Duration {
			nextWait := fallback
			statuses := statusByPeer(snapshot)
			for _, peer := range spec.Peers {
				directCandidates := false
				for _, candidate := range peer.Candidates {
					if strings.ToLower(strings.TrimSpace(candidate.Kind)) == "direct" && strings.TrimSpace(candidate.Address) != "" {
						directCandidates = true
						break
					}
				}
				if !directCandidates {
					continue
				}
				if blockedUntil, blocked := s.warmupBlockedUntil(peer.NodeID, observedAt); blocked {
					wait := blockedUntil.Sub(observedAt)
					if wait > 0 && wait < nextWait {
						nextWait = wait
					}
					continue
				}
				currentStatus := statuses[peer.NodeID]
				if currentStatus.ActiveKind == "direct" && strings.TrimSpace(currentStatus.ActiveAddress) != "" {
					continue
				}
				wait := 200 * time.Millisecond
				if !currentStatus.NextDirectRetryAt.IsZero() && currentStatus.NextDirectRetryAt.After(observedAt) {
					wait = currentStatus.NextDirectRetryAt.Sub(observedAt)
				}
				if wait < nextWait {
					nextWait = wait
				}
			}
			if nextWait <= 0 {
				nextWait = 200 * time.Millisecond
			}
			return nextWait
		}

		snapshot := transport.Snapshot()
		observedAt := snapshot.GeneratedAt
		statuses := statusByPeer(snapshot)
		nextWait := computeNextWait(snapshot, observedAt, s.cfg.DirectWarmupInterval)
		attemptedWarmup := false
		for _, peer := range spec.Peers {
			directCandidates := make([]string, 0, len(peer.Candidates))
			for _, candidate := range peer.Candidates {
				if strings.ToLower(strings.TrimSpace(candidate.Kind)) != "direct" {
					continue
				}
				address := strings.TrimSpace(candidate.Address)
				if address == "" {
					continue
				}
				directCandidates = append(directCandidates, address)
			}
			if len(directCandidates) == 0 {
				continue
			}
			if blockedUntil, blocked := s.warmupBlockedUntil(peer.NodeID, observedAt); blocked {
				wait := blockedUntil.Sub(observedAt)
				if wait > 0 && wait < nextWait {
					nextWait = wait
				}
				continue
			}

			currentStatus := statuses[peer.NodeID]
			if currentStatus.ActiveKind == "direct" && strings.TrimSpace(currentStatus.ActiveAddress) != "" {
				continue
			}
			if !currentStatus.LastDirectTryAt.IsZero() && !currentStatus.NextDirectRetryAt.IsZero() && currentStatus.NextDirectRetryAt.After(observedAt) {
				continue
			}

			attemptedWarmup = true
			attemptCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.cfg.SessionProbeTimeout)
			report := transport.WarmupPeer(attemptCtx, peer.NodeID, directCandidates)
			cancel()
			if report.Reachable {
				log.Printf("direct warmup established peer=%s", peer.NodeID)
			}
		}
		if attemptedWarmup {
			refreshedSnapshot := transport.Snapshot()
			nextWait = computeNextWait(refreshedSnapshot, refreshedSnapshot.GeneratedAt, nextWait)
		}
		return nextWait
	}

	go func() {
		for {
			wait := runWarmup()
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-wakeCh:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				continue
			case <-timer.C:
			}
		}
	}()
}

func (s *Service) saveTransportReport(report secureudp.Report) error {
	path := strings.TrimSpace(s.cfg.TransportReportPath)
	if path == "" {
		return nil
	}
	return state.SaveTransportReport(path, report)
}

func (s *Service) clearTransportReport() error {
	path := strings.TrimSpace(s.cfg.TransportReportPath)
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove transport report: %w", err)
	}
	return nil
}

func (s *Service) saveSTUNReport(report stun.Report) error {
	path := strings.TrimSpace(s.cfg.STUNReportPath)
	if path == "" {
		return nil
	}
	return state.SaveSTUNReport(path, report)
}

func (s *Service) clearSTUNReport() error {
	path := strings.TrimSpace(s.cfg.STUNReportPath)
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stun report: %w", err)
	}
	return nil
}

func normalizedStrings(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	return normalized
}

func normalizeAdvertisableListenerAddress(address string) (string, bool) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", false
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", false
	}
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" {
		return "", false
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		return "", false
	}
	return net.JoinHostPort(host, port), true
}

func appendUniqueStrings(base []string, values ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(values))
	result := make([]string, 0, len(base)+len(values))
	appendValue := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	for _, value := range base {
		appendValue(value)
	}
	for _, value := range values {
		appendValue(value)
	}
	return result
}

func newRuntimeDriver(cfg config.Config) driver.Driver {
	mode := strings.ToLower(strings.TrimSpace(cfg.ApplyMode))
	switch mode {
	case "", "dry-run":
		return dryrun.New()
	case "linux-plan":
		return linuxplandriver.New()
	case "linux-exec":
		return linuxexec.New(linuxexec.Config{
			RequireRoot:    cfg.ExecRequireRoot,
			CommandTimeout: cfg.ExecCommandTimeout,
		})
	default:
		return dryrun.New()
	}
}

type loggingSink struct{}

func (loggingSink) HandleInbound(_ context.Context, packet dataplane.InboundPacket) error {
	log.Printf(
		"dataplane packet received src=%s dst=%s bytes=%d",
		packet.SourceNodeID,
		packet.DestinationIP,
		len(packet.Payload),
	)
	return nil
}

type multiCloser struct {
	closers []io.Closer
}

func (m multiCloser) Close() error {
	var firstErr error
	for _, closer := range m.closers {
		if closer == nil {
			continue
		}
		if err := closer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
