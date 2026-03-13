package usb

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"nodeweave/packages/runtime/go/overlay"
)

type fakeExecutor struct {
	outputs map[string][]byte
	errs    map[string]error
	calls   []string
}

func (f *fakeExecutor) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := strings.TrimSpace(name + " " + strings.Join(args, " "))
	f.calls = append(f.calls, key)
	if err, ok := f.errs[key]; ok {
		return nil, err
	}
	if out, ok := f.outputs[key]; ok {
		return out, nil
	}
	return nil, fmt.Errorf("unexpected command: %s", key)
}

func TestResolveRuntimeForAttachSide(t *testing.T) {
	spec := NormalizeSessionSpec(SessionSpec{
		SessionID:  "usb-link",
		NodeID:     "node-a",
		PeerNodeID: "node-b",
		Transport:  "usbip-encap",
		Local:      DeviceDescriptor{VendorID: "1234", ProductID: "5678"},
		Remote:     DeviceDescriptor{VendorID: "4321", ProductID: "8765"},
	})
	snapshot := overlay.Snapshot{
		NodeID: "node-b",
		Peers: []overlay.PeerState{
			{NodeID: "node-a", OverlayIP: "100.64.0.11"},
		},
	}
	resolved, err := ResolveRuntime(spec, snapshot, "node-b")
	if err != nil {
		t.Fatalf("resolve runtime: %v", err)
	}
	if resolved.Role != "attacher" {
		t.Fatalf("unexpected role: %q", resolved.Role)
	}
	if resolved.PeerNodeID != "node-a" {
		t.Fatalf("unexpected peer node id: %q", resolved.PeerNodeID)
	}
	if resolved.RemoteDevice.VendorID != "1234" {
		t.Fatalf("unexpected remote device mapping: %#v", resolved.RemoteDevice)
	}
}

func TestLinuxResolverResolveRemoteBusID(t *testing.T) {
	executor := &fakeExecutor{
		outputs: map[string][]byte{
			"usbip list -r 100.64.0.11": []byte(`
Exportable USB devices
======================
 - 1-2: Demo Device (1234:5678)
     : /sys/devices/pci0000:00/...
`),
		},
	}
	resolver := LinuxResolver{}
	busID, err := resolver.ResolveRemoteBusID(context.Background(), executor, "100.64.0.11", DeviceDescriptor{
		VendorID:  "1234",
		ProductID: "5678",
	})
	if err != nil {
		t.Fatalf("resolve remote bus id: %v", err)
	}
	if busID != "1-2" {
		t.Fatalf("unexpected bus id: %q", busID)
	}
}

func TestLinuxResolverResolveAttachedPort(t *testing.T) {
	executor := &fakeExecutor{
		outputs: map[string][]byte{
			"usbip port": []byte(`
Imported USB devices
====================
Port 00: <Port in Use> at High Speed(480Mbps)
       1234:5678 -> usbip://100.64.0.11:3240/1-2
`),
		},
	}
	resolver := LinuxResolver{}
	port, err := resolver.ResolveAttachedPort(context.Background(), executor, "100.64.0.11", "1-2")
	if err != nil {
		t.Fatalf("resolve attached port: %v", err)
	}
	if port != "00" {
		t.Fatalf("unexpected port: %q", port)
	}
}

type fakeResolver struct {
	localBusID  string
	remoteBusID string
	port        string
}

func (r fakeResolver) ResolveLocalBusID(DeviceDescriptor) (string, error) {
	return r.localBusID, nil
}

func (r fakeResolver) ResolveRemoteBusID(context.Context, Executor, string, DeviceDescriptor) (string, error) {
	return r.remoteBusID, nil
}

func (r fakeResolver) ResolveAttachedPort(context.Context, Executor, string, string) (string, error) {
	if r.port == "" {
		return "", fmt.Errorf("not attached")
	}
	return r.port, nil
}

func TestManagerExporterBindsDevice(t *testing.T) {
	executor := &fakeExecutor{
		outputs: map[string][]byte{
			"usbipd -D":            []byte(""),
			"usbip bind -b 1-2":    []byte(""),
			"usbip unbind -b 1-2":  []byte(""),
		},
	}
	spec := NormalizeSessionSpec(SessionSpec{
		SessionID:  "usb-link",
		NodeID:     "node-a",
		PeerNodeID: "node-b",
		Local:      DeviceDescriptor{VendorID: "1234", ProductID: "5678"},
		Remote:     DeviceDescriptor{VendorID: "9999", ProductID: "0001"},
	})
	manager, err := NewManager(RuntimeConfig{
		LocalNodeID:       "node-a",
		Snapshot:          overlay.Snapshot{NodeID: "node-a", Peers: []overlay.PeerState{{NodeID: "node-b", OverlayIP: "100.64.0.12"}}},
		Executor:          executor,
		Resolver:          fakeResolver{localBusID: "1-2"},
		ReconcileInterval: 50 * time.Millisecond,
		CommandTimeout:    time.Second,
	}, []SessionSpec{spec})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	manager.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	cancel()
	if err := manager.Close(); err != nil {
		t.Fatalf("close manager: %v", err)
	}
	reports := manager.Reports()
	if len(reports) != 1 {
		t.Fatalf("unexpected reports: %#v", reports)
	}
	if reports[0].ClaimedBy != "1-2" && reports[0].Status != "cancelled" {
		t.Fatalf("unexpected exporter report: %#v", reports[0])
	}
	if len(executor.calls) == 0 || executor.calls[0] != "usbipd -D" {
		t.Fatalf("unexpected executor calls: %#v", executor.calls)
	}
}

func TestManagerAttacherReportsAttachedPort(t *testing.T) {
	executor := &fakeExecutor{}
	spec := NormalizeSessionSpec(SessionSpec{
		SessionID:  "usb-link",
		NodeID:     "node-a",
		PeerNodeID: "node-b",
		Local:      DeviceDescriptor{VendorID: "1234", ProductID: "5678"},
		Remote:     DeviceDescriptor{VendorID: "9999", ProductID: "0001"},
	})
	manager, err := NewManager(RuntimeConfig{
		LocalNodeID:       "node-b",
		Snapshot:          overlay.Snapshot{NodeID: "node-b", Peers: []overlay.PeerState{{NodeID: "node-a", OverlayIP: "100.64.0.11"}}},
		Executor:          executor,
		Resolver:          fakeResolver{remoteBusID: "1-2", port: "00"},
		ReconcileInterval: 50 * time.Millisecond,
		CommandTimeout:    time.Second,
	}, []SessionSpec{spec})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	manager.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	cancel()
	if err := manager.Close(); err != nil {
		t.Fatalf("close manager: %v", err)
	}
	reports := manager.Reports()
	if len(reports) != 1 {
		t.Fatalf("unexpected reports: %#v", reports)
	}
	if reports[0].ClaimedBy != "00" && reports[0].Status != "cancelled" {
		t.Fatalf("unexpected attach report: %#v", reports[0])
	}
}
