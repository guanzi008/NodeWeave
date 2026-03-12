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

	dataplaneMu       sync.Mutex
	dataplaneRuntime  *activeDataplane
	attemptMu         sync.Mutex
	scheduledAttempts map[string]context.CancelFunc
}

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
		scheduledAttempts: map[string]context.CancelFunc{},
	}

	if currentState, err := state.Load(cfg.StatePath); err == nil {
		svc.state = currentState
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
		peerTransportStates = peerTransportStatesFromReport(transport.Snapshot())
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
	s.scheduleDirectAttempts(resp.DirectAttempts)
	return resp, nil
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
	if err := s.reloadDataplane(ctx); err != nil {
		return err
	}
	defer s.stopDataplane()
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
	return s.startDataplaneWithConfig(ctx, *config)
}

func (s *Service) reloadDataplane(ctx context.Context) error {
	config, err := s.loadDataplaneConfig()
	if err != nil {
		return err
	}
	if config == nil {
		s.dataplaneMu.Lock()
		defer s.dataplaneMu.Unlock()
		s.stopDataplaneLocked()
		return s.clearTransportReport()
	}

	s.dataplaneMu.Lock()
	defer s.dataplaneMu.Unlock()

	if s.dataplaneRuntime != nil && s.dataplaneRuntime.signature == config.signature {
		if config.mode != "secure-udp" {
			return s.clearTransportReport()
		}
		return nil
	}

	s.stopDataplaneLocked()

	if _, err := s.startDataplaneWithConfig(ctx, *config); err != nil {
		return err
	}
	if config.mode != "secure-udp" {
		return s.clearTransportReport()
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

func peerTransportStatesFromReport(report secureudp.Report) []api.PeerTransportState {
	states := make([]api.PeerTransportState, 0, len(report.Peers))
	for _, peer := range report.Peers {
		state := api.PeerTransportState{
			PeerNodeID:                peer.NodeID,
			ActiveKind:                peer.ActiveKind,
			ActiveAddress:             peer.ActiveAddress,
			ReportedAt:                report.GeneratedAt,
			LastDirectAttemptAt:       peer.LastDirectAttemptAt,
			LastDirectAttemptResult:   peer.LastDirectAttemptResult,
			LastDirectSuccessAt:       peer.LastDirectSuccessAt,
			ConsecutiveDirectFailures: peer.ConsecutiveDirectFailures,
		}
		if state.PeerNodeID == "" {
			continue
		}
		states = append(states, state)
	}
	return states
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

func (s *Service) scheduleDirectAttempts(attempts []api.DirectAttemptInstruction) {
	transport := s.sharedSTUNTransport()
	if transport == nil || len(attempts) == 0 {
		return
	}

	now := time.Now().UTC()
	for _, instruction := range attempts {
		attemptID := strings.TrimSpace(instruction.AttemptID)
		if attemptID == "" {
			continue
		}
		window := time.Duration(instruction.Window) * time.Millisecond
		if window <= 0 {
			window = 600 * time.Millisecond
		}
		if !instruction.ExecuteAt.IsZero() && now.After(instruction.ExecuteAt.Add(window)) {
			continue
		}

		s.attemptMu.Lock()
		if _, exists := s.scheduledAttempts[attemptID]; exists {
			s.attemptMu.Unlock()
			continue
		}
		attemptCtx, cancel := context.WithCancel(context.Background())
		s.scheduledAttempts[attemptID] = cancel
		s.attemptMu.Unlock()

		go func(instruction api.DirectAttemptInstruction, transport *secureudp.Transport, attemptCtx context.Context) {
			defer func() {
				s.attemptMu.Lock()
				delete(s.scheduledAttempts, strings.TrimSpace(instruction.AttemptID))
				s.attemptMu.Unlock()
			}()

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
			if err != nil && !errors.Is(err, context.Canceled) {
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
