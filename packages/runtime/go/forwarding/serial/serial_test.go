package serial

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestNormalizePortConfigDefaults(t *testing.T) {
	cfg := NormalizePortConfig(PortConfig{Name: "COM3"})
	if cfg.BaudRate != 115200 || cfg.DataBits != 8 || cfg.StopBits != 1 || cfg.Parity != "none" || cfg.ReadTimeoutMillis != 1000 {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}

func TestBuildSessionID(t *testing.T) {
	id := BuildSessionID(SessionSpec{
		NodeID:     "node-a",
		PeerNodeID: "node-b",
		Local:      PortConfig{Name: "COM3"},
		Remote:     PortConfig{Name: "/dev/ttyUSB0"},
		Transport:  "tcp-encap",
	})
	if id == "" {
		t.Fatalf("expected session id")
	}
}

func TestSessionBridgesTraffic(t *testing.T) {
	localApp, localSerial := net.Pipe()
	defer localApp.Close()
	remoteApp, remoteSerial := net.Pipe()
	defer remoteApp.Close()

	session := NewSession(SessionSpec{
		NodeID:     "node-a",
		PeerNodeID: "node-b",
		Local:      PortConfig{Name: "COM3"},
		Remote:     PortConfig{Name: "COM9"},
	}, localSerial, remoteSerial)

	done := make(chan SessionReport, 1)
	go func() {
		done <- session.Run(context.Background())
	}()

	payloadA := []byte("ping-serial")
	if _, err := localApp.Write(payloadA); err != nil {
		t.Fatalf("write local app: %v", err)
	}
	gotA := make([]byte, len(payloadA))
	if _, err := remoteApp.Read(gotA); err != nil {
		t.Fatalf("read remote app: %v", err)
	}
	if string(gotA) != string(payloadA) {
		t.Fatalf("unexpected forwarded payload: %q", gotA)
	}

	payloadB := []byte("pong-serial")
	if _, err := remoteApp.Write(payloadB); err != nil {
		t.Fatalf("write remote app: %v", err)
	}
	gotB := make([]byte, len(payloadB))
	if _, err := localApp.Read(gotB); err != nil {
		t.Fatalf("read local app: %v", err)
	}
	if string(gotB) != string(payloadB) {
		t.Fatalf("unexpected reverse payload: %q", gotB)
	}

	_ = localApp.Close()
	_ = remoteApp.Close()

	select {
	case report := <-done:
		if report.BytesLocalToRemote == 0 || report.BytesRemoteToLocal == 0 {
			t.Fatalf("expected byte counters to be populated: %#v", report)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for session to finish")
	}
}

func TestSessionCancel(t *testing.T) {
	localApp, localSerial := net.Pipe()
	defer localApp.Close()
	remoteApp, remoteSerial := net.Pipe()
	defer remoteApp.Close()

	session := NewSession(SessionSpec{
		Local:  PortConfig{Name: "COM1"},
		Remote: PortConfig{Name: "COM2"},
	}, localSerial, remoteSerial)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan SessionReport, 1)
	go func() {
		done <- session.Run(ctx)
	}()

	cancel()

	select {
	case report := <-done:
		if report.Status != "cancelled" && report.Status != "completed" {
			t.Fatalf("expected cancellation-compatible report, got %#v", report)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cancelled session")
	}
}
