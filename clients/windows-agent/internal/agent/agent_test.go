package agent

import (
	"path/filepath"
	"testing"

	"nodeweave/clients/windows-agent/internal/config"
	"nodeweave/clients/windows-agent/internal/state"
	"nodeweave/packages/contracts/go/api"
	"nodeweave/packages/runtime/go/forwarding/serial"
	"nodeweave/packages/runtime/go/forwarding/usb"
)

func TestPersistForwardingState(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{
		Platform:                "windows-agent",
		SerialForwardPath:       filepath.Join(tmpDir, "serial-forwards.json"),
		SerialForwardReportPath: filepath.Join(tmpDir, "serial-forward-report.json"),
		USBForwardPath:          filepath.Join(tmpDir, "usb-forwards.json"),
		USBForwardReportPath:    filepath.Join(tmpDir, "usb-forward-report.json"),
		SerialForwards: []serial.SessionSpec{
			{
				PeerNodeID: "node-linux",
				Local:      serial.PortConfig{Name: "COM5"},
				Remote:     serial.PortConfig{Name: "/dev/ttyUSB1"},
			},
		},
		USBForwards: []usb.SessionSpec{
			{
				PeerNodeID: "node-linux",
				Local:      usb.DeviceDescriptor{BusID: "1", DeviceID: "7", VendorID: "1d6b", ProductID: "0002"},
				Remote:     usb.DeviceDescriptor{VendorID: "1d6b", ProductID: "0002"},
			},
		},
	}

	svc := &Service{
		cfg: cfg,
		state: state.File{
			Node: api.Node{ID: "node-win"},
		},
	}

	if err := svc.persistForwardingState(); err != nil {
		t.Fatalf("persist forwarding state: %v", err)
	}

	serialSpecs, err := state.LoadSerialForwards(cfg.SerialForwardPath)
	if err != nil {
		t.Fatalf("load serial forwards: %v", err)
	}
	if len(serialSpecs) != 1 || serialSpecs[0].NodeID != "node-win" {
		t.Fatalf("unexpected serial forwarding state: %#v", serialSpecs)
	}

	serialReports, err := state.LoadSerialForwardReport(cfg.SerialForwardReportPath)
	if err != nil {
		t.Fatalf("load serial forward report: %v", err)
	}
	if len(serialReports) != 1 || serialReports[0].ClosedBy != "windows-agent" {
		t.Fatalf("unexpected serial forwarding report: %#v", serialReports)
	}

	usbSpecs, err := state.LoadUSBForwards(cfg.USBForwardPath)
	if err != nil {
		t.Fatalf("load usb forwards: %v", err)
	}
	if len(usbSpecs) != 1 || usbSpecs[0].NodeID != "node-win" {
		t.Fatalf("unexpected usb forwarding state: %#v", usbSpecs)
	}

	usbReports, err := state.LoadUSBForwardReport(cfg.USBForwardReportPath)
	if err != nil {
		t.Fatalf("load usb forward report: %v", err)
	}
	if len(usbReports) != 1 || usbReports[0].ClaimedBy != "windows-agent" {
		t.Fatalf("unexpected usb forwarding report: %#v", usbReports)
	}
}
