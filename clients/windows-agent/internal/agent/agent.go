package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"nodeweave/clients/windows-agent/internal/config"
	"nodeweave/clients/windows-agent/internal/state"
	"nodeweave/packages/contracts/go/api"
	contractsclient "nodeweave/packages/contracts/go/client"
	"nodeweave/packages/runtime/go/overlay"
)

type Service struct {
	cfg    config.Config
	client *contractsclient.Client
	state  state.File
}

func New(cfg config.Config) (*Service, error) {
	currentState := state.File{
		ServerURL: cfg.ServerURL,
	}
	if loaded, err := state.Load(cfg.StatePath); err == nil {
		currentState = loaded
		if strings.TrimSpace(currentState.ServerURL) == "" {
			currentState.ServerURL = cfg.ServerURL
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	return &Service{
		cfg:    cfg,
		client: contractsclient.New(cfg.ServerURL),
		state:  currentState,
	}, nil
}

func (s *Service) CurrentState() state.File {
	return s.state
}

func (s *Service) EnsureEnrolled(ctx context.Context) error {
	if strings.TrimSpace(s.state.Node.ID) != "" && strings.TrimSpace(s.state.NodeToken) != "" {
		return nil
	}

	publicKey := strings.TrimSpace(s.cfg.PublicKey)
	if publicKey == "" {
		publicKey = "windows-agent-devpub"
	}

	resp, err := s.client.RegisterDevice(ctx, api.DeviceRegistrationRequest{
		DeviceName:        s.cfg.DeviceName,
		Platform:          s.cfg.Platform,
		Version:           s.cfg.Version,
		PublicKey:         publicKey,
		Capabilities:      []string{"windows", "agent"},
		RegistrationToken: s.cfg.RegistrationToken,
	})
	if err != nil {
		return err
	}

	s.state.ServerURL = s.cfg.ServerURL
	s.state.Device = resp.Device
	s.state.Node = resp.Node
	s.state.NodeToken = resp.NodeToken
	return state.Save(s.cfg.StatePath, s.state)
}

func (s *Service) SyncBootstrap(ctx context.Context) error {
	if strings.TrimSpace(s.state.Node.ID) == "" || strings.TrimSpace(s.state.NodeToken) == "" {
		return fmt.Errorf("agent is not enrolled")
	}

	bootstrap, err := s.client.GetBootstrap(ctx, s.state.Node.ID, s.state.NodeToken)
	if err != nil {
		return err
	}

	snapshot, err := overlay.Compile(bootstrap, overlay.Config{
		InterfaceName: s.cfg.InterfaceName,
		MTU:           s.cfg.InterfaceMTU,
	}, "windows-dry-run")
	if err != nil {
		return fmt.Errorf("compile overlay runtime: %w", err)
	}

	s.state.Bootstrap = bootstrap
	s.state.LastBootstrapAt = time.Now().UTC()
	if err := state.SaveBootstrap(s.cfg.BootstrapPath, bootstrap); err != nil {
		return err
	}
	if err := state.SaveRuntime(s.cfg.RuntimePath, snapshot); err != nil {
		return err
	}
	return state.Save(s.cfg.StatePath, s.state)
}

func (s *Service) Heartbeat(ctx context.Context) (api.HeartbeatResponse, error) {
	if strings.TrimSpace(s.state.Node.ID) == "" || strings.TrimSpace(s.state.NodeToken) == "" {
		return api.HeartbeatResponse{}, fmt.Errorf("agent is not enrolled")
	}

	records := make([]api.EndpointObservation, 0, len(s.cfg.AdvertiseEndpoints))
	for _, endpoint := range s.cfg.AdvertiseEndpoints {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			continue
		}
		records = append(records, api.EndpointObservation{
			Address:    endpoint,
			Source:     "static",
			ObservedAt: time.Now().UTC(),
		})
	}

	resp, err := s.client.Heartbeat(ctx, s.state.Node.ID, s.state.NodeToken, api.HeartbeatRequest{
		Status:          "online",
		EndpointRecords: records,
		RelayRegion:     s.cfg.RelayRegion,
	})
	if err != nil {
		return api.HeartbeatResponse{}, err
	}

	s.state.Node = resp.Node
	s.state.LastHeartbeatAt = time.Now().UTC()
	if err := state.Save(s.cfg.StatePath, s.state); err != nil {
		return api.HeartbeatResponse{}, err
	}
	if resp.BootstrapVersion > s.state.Bootstrap.Version {
		if err := s.SyncBootstrap(ctx); err != nil {
			return api.HeartbeatResponse{}, err
		}
	}
	return resp, nil
}

func (s *Service) Run(ctx context.Context) error {
	if s.cfg.AutoEnroll {
		if err := s.EnsureEnrolled(ctx); err != nil {
			return err
		}
	}
	if err := s.SyncBootstrap(ctx); err != nil {
		return err
	}
	if _, err := s.Heartbeat(ctx); err != nil {
		return err
	}

	heartbeatTicker := time.NewTicker(s.cfg.HeartbeatInterval)
	defer heartbeatTicker.Stop()
	bootstrapTicker := time.NewTicker(s.cfg.BootstrapInterval)
	defer bootstrapTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-heartbeatTicker.C:
			if _, err := s.Heartbeat(ctx); err != nil {
				return err
			}
		case <-bootstrapTicker.C:
			if err := s.SyncBootstrap(ctx); err != nil {
				return err
			}
		}
	}
}
