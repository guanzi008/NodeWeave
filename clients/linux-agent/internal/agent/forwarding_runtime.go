package agent

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"nodeweave/clients/linux-agent/internal/state"
	"nodeweave/packages/runtime/go/forwarding/serial"
	"nodeweave/packages/runtime/go/forwarding/usb"
	"nodeweave/packages/runtime/go/overlay"
)

type forwardingSignature struct {
	NodeID   string               `json:"node_id"`
	Peers    []overlay.PeerState  `json:"peers"`
	Serial   []serial.SessionSpec `json:"serial"`
	USB      []usb.SessionSpec    `json:"usb"`
}

func (s *Service) reloadForwardingRuntimes(ctx context.Context) {
	snapshot, err := state.LoadRuntime(s.cfg.RuntimePath)
	if err != nil {
		s.stopForwardingRuntimes()
		return
	}

	serialSpecs := make([]serial.SessionSpec, 0, len(s.cfg.SerialForwards))
	for _, spec := range s.cfg.SerialForwards {
		spec = serial.NormalizeSessionSpec(spec)
		if strings.TrimSpace(spec.NodeID) == "" {
			spec.NodeID = s.state.Node.ID
		}
		serialSpecs = append(serialSpecs, spec)
	}

	usbSpecs := make([]usb.SessionSpec, 0, len(s.cfg.USBForwards))
	for _, spec := range s.cfg.USBForwards {
		spec = usb.NormalizeSessionSpec(spec)
		if strings.TrimSpace(spec.NodeID) == "" {
			spec.NodeID = s.state.Node.ID
		}
		usbSpecs = append(usbSpecs, spec)
	}

	signature := buildForwardingSignature(snapshot, serialSpecs, usbSpecs)

	s.forwardingMu.Lock()
	if s.forwardingSignature == signature {
		s.forwardingMu.Unlock()
		return
	}
	previousSerial := s.serialManager
	previousUSB := s.usbManager
	s.serialManager = nil
	s.usbManager = nil
	s.forwardingSignature = signature
	s.forwardingMu.Unlock()

	if previousSerial != nil {
		if err := previousSerial.Close(); err != nil {
			log.Printf("stop serial forwarding runtime failed: %v", err)
		}
	}
	if previousUSB != nil {
		if err := previousUSB.Close(); err != nil {
			log.Printf("stop usb forwarding runtime failed: %v", err)
		}
	}

	var serialManager *serial.Manager
	if len(serialSpecs) > 0 {
		manager, err := serial.NewManager(serial.RuntimeConfig{
			LocalNodeID: s.state.Node.ID,
			Snapshot:    snapshot,
			Logger: func(format string, args ...any) {
				log.Printf(format, args...)
			},
			OnReport: func(serial.SessionReport) {
				s.persistSerialForwardReports()
			},
		}, serialSpecs)
		if err != nil {
			log.Printf("start serial forwarding runtime failed: %v", err)
		} else {
			serialManager = manager
		}
	}

	var usbManager *usb.Manager
	if len(usbSpecs) > 0 {
		manager, err := usb.NewManager(usb.RuntimeConfig{
			LocalNodeID: s.state.Node.ID,
			Snapshot:    snapshot,
			Logger: func(format string, args ...any) {
				log.Printf(format, args...)
			},
			OnReport: func(usb.SessionReport) {
				s.persistUSBForwardReports()
			},
		}, usbSpecs)
		if err != nil {
			log.Printf("start usb forwarding runtime failed: %v", err)
		} else {
			usbManager = manager
		}
	}

	s.forwardingMu.Lock()
	s.serialManager = serialManager
	s.usbManager = usbManager
	s.forwardingMu.Unlock()

	if serialManager != nil {
		serialManager.Start(context.WithoutCancel(ctx))
	}
	if usbManager != nil {
		usbManager.Start(context.WithoutCancel(ctx))
	}

	s.persistSerialForwardReports()
	s.persistUSBForwardReports()
}

func (s *Service) stopForwardingRuntimes() {
	s.forwardingMu.Lock()
	serialManager := s.serialManager
	usbManager := s.usbManager
	s.serialManager = nil
	s.usbManager = nil
	s.forwardingSignature = ""
	s.forwardingMu.Unlock()

	if serialManager != nil {
		if err := serialManager.Close(); err != nil {
			log.Printf("stop serial forwarding runtime failed: %v", err)
		}
	}
	if usbManager != nil {
		if err := usbManager.Close(); err != nil {
			log.Printf("stop usb forwarding runtime failed: %v", err)
		}
	}
	s.persistSerialForwardReports()
	s.persistUSBForwardReports()
}

func (s *Service) persistSerialForwardReports() {
	s.forwardingMu.Lock()
	manager := s.serialManager
	s.forwardingMu.Unlock()

	var reports []serial.SessionReport
	if manager != nil {
		reports = manager.Reports()
	} else {
		reports = make([]serial.SessionReport, 0, len(s.cfg.SerialForwards))
		for _, spec := range s.cfg.SerialForwards {
			spec = serial.NormalizeSessionSpec(spec)
			if strings.TrimSpace(spec.NodeID) == "" {
				spec.NodeID = s.state.Node.ID
			}
			reports = append(reports, serial.ConfiguredReport(spec, s.cfg.Platform))
		}
	}
	if err := state.SaveSerialForwardReport(s.cfg.SerialForwardReportPath, reports); err != nil {
		log.Printf("persist serial forwarding report failed: %v", err)
	}
}

func (s *Service) persistUSBForwardReports() {
	s.forwardingMu.Lock()
	manager := s.usbManager
	s.forwardingMu.Unlock()

	var reports []usb.SessionReport
	if manager != nil {
		reports = manager.Reports()
	} else {
		reports = make([]usb.SessionReport, 0, len(s.cfg.USBForwards))
		for _, spec := range s.cfg.USBForwards {
			spec = usb.NormalizeSessionSpec(spec)
			if strings.TrimSpace(spec.NodeID) == "" {
				spec.NodeID = s.state.Node.ID
			}
			reports = append(reports, usb.ConfiguredReport(spec, s.cfg.Platform))
		}
	}
	if err := state.SaveUSBForwardReport(s.cfg.USBForwardReportPath, reports); err != nil {
		log.Printf("persist usb forwarding report failed: %v", err)
	}
}

func buildForwardingSignature(snapshot overlay.Snapshot, serialSpecs []serial.SessionSpec, usbSpecs []usb.SessionSpec) string {
	value := forwardingSignature{
		NodeID: snapshot.NodeID,
		Peers:  append([]overlay.PeerState(nil), snapshot.Peers...),
		Serial: append([]serial.SessionSpec(nil), serialSpecs...),
		USB:    append([]usb.SessionSpec(nil), usbSpecs...),
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}
