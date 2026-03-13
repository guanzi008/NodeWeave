package serial

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"nodeweave/packages/runtime/go/overlay"
)

type testPortOpener struct {
	ports map[string]io.ReadWriteCloser
}

func (o testPortOpener) Open(cfg PortConfig) (io.ReadWriteCloser, error) {
	return o.ports[cfg.Name], nil
}

func TestResolveRuntimeForPeerSide(t *testing.T) {
	spec := NormalizeSessionSpec(SessionSpec{
		SessionID:  "session-1",
		NodeID:     "node-a",
		PeerNodeID: "node-b",
		Transport:  "tcp-encap",
		Local:      PortConfig{Name: "/dev/ttyUSB0"},
		Remote:     PortConfig{Name: "/dev/ttyACM0"},
	})
	snapshot := overlay.Snapshot{
		NodeID: "node-b",
		Peers: []overlay.PeerState{
			{NodeID: "node-a", OverlayIP: "127.0.0.1"},
		},
	}
	resolved, err := ResolveRuntime(spec, snapshot, "node-b", 43100)
	if err != nil {
		t.Fatalf("resolve runtime: %v", err)
	}
	if resolved.LocalPort.Name != "/dev/ttyACM0" {
		t.Fatalf("expected peer-side local port, got %q", resolved.LocalPort.Name)
	}
	if resolved.PeerNodeID != "node-a" {
		t.Fatalf("unexpected peer node id: %q", resolved.PeerNodeID)
	}
	if resolved.Role != "dialer" {
		t.Fatalf("unexpected role: %q", resolved.Role)
	}
}

func TestManagerBridgesSerialTrafficAcrossTCPEncap(t *testing.T) {
	spec := NormalizeSessionSpec(SessionSpec{
		SessionID:  "serial-link",
		NodeID:     "node-a",
		PeerNodeID: "node-b",
		Transport:  "tcp-encap",
		Local:      PortConfig{Name: "/dev/ttyUSB0"},
		Remote:     PortConfig{Name: "/dev/ttyACM0"},
	})
	snapshot := overlay.Snapshot{
		NodeID: "node-a",
		Peers: []overlay.PeerState{
			{NodeID: "node-b", OverlayIP: "127.0.0.1"},
		},
	}
	peerSnapshot := overlay.Snapshot{
		NodeID: "node-b",
		Peers: []overlay.PeerState{
			{NodeID: "node-a", OverlayIP: "127.0.0.1"},
		},
	}

	portA, externalA := net.Pipe()
	portB, externalB := net.Pipe()
	defer externalA.Close()
	defer externalB.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	managerA, err := NewManager(RuntimeConfig{
		LocalNodeID: "node-a",
		Snapshot:    snapshot,
		Opener: testPortOpener{ports: map[string]io.ReadWriteCloser{
			"/dev/ttyUSB0": portA,
		}},
	}, []SessionSpec{spec})
	if err != nil {
		t.Fatalf("new manager A: %v", err)
	}
	managerB, err := NewManager(RuntimeConfig{
		LocalNodeID: "node-b",
		Snapshot:    peerSnapshot,
		Opener: testPortOpener{ports: map[string]io.ReadWriteCloser{
			"/dev/ttyACM0": portB,
		}},
	}, []SessionSpec{spec})
	if err != nil {
		t.Fatalf("new manager B: %v", err)
	}

	managerA.Start(ctx)
	managerB.Start(ctx)
	defer managerA.Close()
	defer managerB.Close()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		reportsA := managerA.Reports()
		reportsB := managerB.Reports()
		if len(reportsA) == 1 && len(reportsB) == 1 && reportsA[0].Status == reportStatusRunning && reportsB[0].Status == reportStatusRunning {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	payload := []byte("hello-over-serial")
	go func() {
		_, _ = externalA.Write(payload)
	}()

	buffer := make([]byte, len(payload))
	if _, err := io.ReadFull(externalB, buffer); err != nil {
		t.Fatalf("read bridged payload: %v", err)
	}
	if string(buffer) != string(payload) {
		t.Fatalf("unexpected bridged payload: %q", string(buffer))
	}
}
