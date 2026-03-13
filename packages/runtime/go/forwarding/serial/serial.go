package serial

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type PortConfig struct {
	Name              string `json:"name"`
	BaudRate          int    `json:"baud_rate,omitempty"`
	DataBits          int    `json:"data_bits,omitempty"`
	StopBits          int    `json:"stop_bits,omitempty"`
	Parity            string `json:"parity,omitempty"`
	ReadTimeoutMillis int    `json:"read_timeout_millis,omitempty"`
}

type SessionSpec struct {
	SessionID  string     `json:"session_id"`
	NodeID     string     `json:"node_id,omitempty"`
	PeerNodeID string     `json:"peer_node_id,omitempty"`
	Transport  string     `json:"transport,omitempty"`
	Local      PortConfig `json:"local"`
	Remote     PortConfig `json:"remote"`
	CreatedAt  time.Time  `json:"created_at,omitempty"`
}

type SessionReport struct {
	SessionID          string     `json:"session_id"`
	NodeID             string     `json:"node_id,omitempty"`
	PeerNodeID         string     `json:"peer_node_id,omitempty"`
	Transport          string     `json:"transport,omitempty"`
	Local              PortConfig `json:"local"`
	Remote             PortConfig `json:"remote"`
	StartedAt          time.Time  `json:"started_at"`
	EndedAt            time.Time  `json:"ended_at,omitempty"`
	Status             string     `json:"status"`
	ClosedBy           string     `json:"closed_by,omitempty"`
	BytesLocalToRemote int64      `json:"bytes_local_to_remote,omitempty"`
	BytesRemoteToLocal int64      `json:"bytes_remote_to_local,omitempty"`
	LastError          string     `json:"last_error,omitempty"`
}

func ConfiguredReport(spec SessionSpec, closedBy string) SessionReport {
	spec = NormalizeSessionSpec(spec)
	return SessionReport{
		SessionID:  spec.SessionID,
		NodeID:     spec.NodeID,
		PeerNodeID: spec.PeerNodeID,
		Transport:  spec.Transport,
		Local:      spec.Local,
		Remote:     spec.Remote,
		Status:     "configured",
		ClosedBy:   strings.TrimSpace(closedBy),
	}
}

type Session struct {
	spec   SessionSpec
	local  io.ReadWriteCloser
	remote io.ReadWriteCloser
}

func NormalizePortConfig(cfg PortConfig) PortConfig {
	cfg.Name = strings.TrimSpace(cfg.Name)
	if cfg.BaudRate <= 0 {
		cfg.BaudRate = 115200
	}
	if cfg.DataBits <= 0 {
		cfg.DataBits = 8
	}
	if cfg.StopBits <= 0 {
		cfg.StopBits = 1
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Parity)) {
	case "", "none":
		cfg.Parity = "none"
	case "odd", "even":
		cfg.Parity = strings.ToLower(strings.TrimSpace(cfg.Parity))
	default:
		cfg.Parity = "none"
	}
	if cfg.ReadTimeoutMillis <= 0 {
		cfg.ReadTimeoutMillis = 1000
	}
	return cfg
}

func NormalizeSessionSpec(spec SessionSpec) SessionSpec {
	spec.Transport = strings.ToLower(strings.TrimSpace(spec.Transport))
	if spec.Transport == "" {
		spec.Transport = "tcp-encap"
	}
	spec.Local = NormalizePortConfig(spec.Local)
	spec.Remote = NormalizePortConfig(spec.Remote)
	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = time.Now().UTC()
	} else {
		spec.CreatedAt = spec.CreatedAt.UTC()
	}
	if strings.TrimSpace(spec.SessionID) == "" {
		spec.SessionID = buildSessionIDParts(spec)
	}
	return spec
}

func BuildSessionID(spec SessionSpec) string {
	spec.Transport = strings.ToLower(strings.TrimSpace(spec.Transport))
	if spec.Transport == "" {
		spec.Transport = "tcp-encap"
	}
	return buildSessionIDParts(spec)
}

func NewSession(spec SessionSpec, local, remote io.ReadWriteCloser) *Session {
	return &Session{
		spec:   NormalizeSessionSpec(spec),
		local:  local,
		remote: remote,
	}
}

func (s *Session) Run(ctx context.Context) SessionReport {
	report := SessionReport{
		SessionID:  s.spec.SessionID,
		NodeID:     s.spec.NodeID,
		PeerNodeID: s.spec.PeerNodeID,
		Transport:  s.spec.Transport,
		Local:      s.spec.Local,
		Remote:     s.spec.Remote,
		StartedAt:  time.Now().UTC(),
		Status:     "running",
	}

	if s.local == nil || s.remote == nil {
		report.Status = "error"
		report.LastError = "serial forwarding requires both local and remote streams"
		report.EndedAt = time.Now().UTC()
		return report
	}

	done := make(chan copyResult, 2)
	var bytesLocalToRemote int64
	var bytesRemoteToLocal int64
	var closeOnce sync.Once
	closeStreams := func() {
		closeOnce.Do(func() {
			_ = s.local.Close()
			_ = s.remote.Close()
		})
	}

	go func() {
		<-ctx.Done()
		closeStreams()
	}()

	go s.copyLoop(done, s.remote, s.local, "local", &bytesLocalToRemote)
	go s.copyLoop(done, s.local, s.remote, "remote", &bytesRemoteToLocal)

	var first copyResult
	for i := 0; i < 2; i++ {
		result := <-done
		if i == 0 {
			first = result
			closeStreams()
		}
	}

	report.BytesLocalToRemote = atomic.LoadInt64(&bytesLocalToRemote)
	report.BytesRemoteToLocal = atomic.LoadInt64(&bytesRemoteToLocal)
	report.EndedAt = time.Now().UTC()
	switch {
	case ctx.Err() != nil:
		report.Status = "cancelled"
		report.ClosedBy = "context"
	case first.err != nil:
		report.Status = "error"
		report.ClosedBy = first.direction
		report.LastError = first.err.Error()
	default:
		report.Status = "completed"
		report.ClosedBy = first.direction
	}
	return report
}

type copyResult struct {
	direction string
	err       error
}

func (s *Session) copyLoop(done chan<- copyResult, dst io.Writer, src io.Reader, direction string, counter *int64) {
	buffer := make([]byte, 32*1024)
	for {
		n, err := src.Read(buffer)
		if n > 0 {
			written, writeErr := dst.Write(buffer[:n])
			atomic.AddInt64(counter, int64(written))
			if writeErr != nil {
				done <- copyResult{direction: direction, err: fmt.Errorf("write %s stream: %w", direction, writeErr)}
				return
			}
			if written != n {
				done <- copyResult{direction: direction, err: fmt.Errorf("write %s stream: short write", direction)}
				return
			}
		}
		if err != nil {
			if err == io.EOF {
				done <- copyResult{direction: direction}
				return
			}
			done <- copyResult{direction: direction, err: fmt.Errorf("read %s stream: %w", direction, err)}
			return
		}
	}
}

func serialIDPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, ":", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func buildSessionIDParts(spec SessionSpec) string {
	parts := []string{
		serialIDPart(spec.NodeID),
		serialIDPart(spec.PeerNodeID),
		serialIDPart(spec.Local.Name),
		serialIDPart(spec.Remote.Name),
		serialIDPart(spec.Transport),
	}
	return strings.Join(parts, "-")
}
