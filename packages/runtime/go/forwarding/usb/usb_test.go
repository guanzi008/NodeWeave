package usb

import "testing"

func TestNormalizeDeviceDescriptor(t *testing.T) {
	device := NormalizeDeviceDescriptor(DeviceDescriptor{
		BusID:     " 1-2 ",
		DeviceID:  " 03 ",
		VendorID:  "0ABC ",
		ProductID: " 00EF",
		Interface: " 0 ",
	})
	if device.BusID != "1-2" || device.DeviceID != "03" || device.VendorID != "0abc" || device.ProductID != "00ef" || device.Interface != "0" {
		t.Fatalf("unexpected normalized descriptor: %#v", device)
	}
}

func TestBuildSessionID(t *testing.T) {
	id := BuildSessionID(SessionSpec{
		NodeID:     "node-a",
		PeerNodeID: "node-b",
		Local: DeviceDescriptor{
			VendorID:  "1d6b",
			ProductID: "0002",
		},
		Remote: DeviceDescriptor{
			BusID:    "2-1",
			DeviceID: "7",
		},
	})
	if id == "" {
		t.Fatal("expected session id")
	}
}

func TestCompatiblePair(t *testing.T) {
	if !CompatiblePair(
		DeviceDescriptor{VendorID: "1d6b", ProductID: "0002"},
		DeviceDescriptor{VendorID: "1d6b", ProductID: "0002"},
	) {
		t.Fatal("expected pair to be compatible")
	}
	if CompatiblePair(
		DeviceDescriptor{VendorID: "1d6b", ProductID: "0002"},
		DeviceDescriptor{VendorID: "1d6b", ProductID: "0003"},
	) {
		t.Fatal("expected pair mismatch to be incompatible")
	}
}

func TestConfiguredReport(t *testing.T) {
	report := ConfiguredReport(SessionSpec{
		NodeID:     "node-a",
		PeerNodeID: "node-b",
		Local: DeviceDescriptor{
			BusID:    "1-2",
			DeviceID: "3",
		},
		Remote: DeviceDescriptor{
			VendorID:  "1d6b",
			ProductID: "0002",
		},
	}, "linux-agent")
	if report.Status != "configured" || report.ClaimedBy != "linux-agent" || report.SessionID == "" {
		t.Fatalf("unexpected configured report: %#v", report)
	}
}
