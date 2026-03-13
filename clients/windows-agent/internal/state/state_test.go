package state

import (
	"path/filepath"
	"testing"

	"nodeweave/packages/runtime/go/forwarding/serial"
	"nodeweave/packages/runtime/go/forwarding/usb"
)

func TestSaveAndLoadSerialForwards(t *testing.T) {
	tmpDir := t.TempDir()
	specs := []serial.SessionSpec{
		{
			NodeID:     "node-win",
			PeerNodeID: "node-linux",
			Transport:  "tcp-encap",
			Local:      serial.PortConfig{Name: "COM3"},
			Remote:     serial.PortConfig{Name: "/dev/ttyUSB0"},
		},
	}
	path := filepath.Join(tmpDir, "serial-forwards.json")
	reportPath := filepath.Join(tmpDir, "serial-forward-report.json")

	if err := SaveSerialForwards(path, specs); err != nil {
		t.Fatalf("save serial forwards: %v", err)
	}
	if err := SaveSerialForwardReport(reportPath, []serial.SessionReport{serial.ConfiguredReport(specs[0], "windows-agent")}); err != nil {
		t.Fatalf("save serial forward report: %v", err)
	}

	gotSpecs, err := LoadSerialForwards(path)
	if err != nil {
		t.Fatalf("load serial forwards: %v", err)
	}
	if len(gotSpecs) != 1 || gotSpecs[0].Local.Name != "COM3" || gotSpecs[0].Remote.Name != "/dev/ttyUSB0" {
		t.Fatalf("unexpected serial forward roundtrip: %#v", gotSpecs)
	}

	gotReports, err := LoadSerialForwardReport(reportPath)
	if err != nil {
		t.Fatalf("load serial forward report: %v", err)
	}
	if len(gotReports) != 1 || gotReports[0].ClosedBy != "windows-agent" {
		t.Fatalf("unexpected serial report roundtrip: %#v", gotReports)
	}
}

func TestSaveAndLoadUSBForwards(t *testing.T) {
	tmpDir := t.TempDir()
	specs := []usb.SessionSpec{
		{
			NodeID:     "node-win",
			PeerNodeID: "node-linux",
			Transport:  "usbip-encap",
			Local:      usb.DeviceDescriptor{BusID: "1", DeviceID: "4", VendorID: "1d6b", ProductID: "0002"},
			Remote:     usb.DeviceDescriptor{VendorID: "1d6b", ProductID: "0002", Interface: "0"},
		},
	}
	path := filepath.Join(tmpDir, "usb-forwards.json")
	reportPath := filepath.Join(tmpDir, "usb-forward-report.json")

	if err := SaveUSBForwards(path, specs); err != nil {
		t.Fatalf("save usb forwards: %v", err)
	}
	if err := SaveUSBForwardReport(reportPath, []usb.SessionReport{usb.ConfiguredReport(specs[0], "windows-agent")}); err != nil {
		t.Fatalf("save usb forward report: %v", err)
	}

	gotSpecs, err := LoadUSBForwards(path)
	if err != nil {
		t.Fatalf("load usb forwards: %v", err)
	}
	if len(gotSpecs) != 1 || gotSpecs[0].Local.VendorID != "1d6b" || gotSpecs[0].Remote.Interface != "0" {
		t.Fatalf("unexpected usb forward roundtrip: %#v", gotSpecs)
	}

	gotReports, err := LoadUSBForwardReport(reportPath)
	if err != nil {
		t.Fatalf("load usb forward report: %v", err)
	}
	if len(gotReports) != 1 || gotReports[0].ClaimedBy != "windows-agent" {
		t.Fatalf("unexpected usb report roundtrip: %#v", gotReports)
	}
}
