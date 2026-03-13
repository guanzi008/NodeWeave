package usb

import (
	"strings"
	"time"
)

type DeviceDescriptor struct {
	BusID        string `json:"bus_id,omitempty"`
	DeviceID     string `json:"device_id,omitempty"`
	VendorID     string `json:"vendor_id,omitempty"`
	ProductID    string `json:"product_id,omitempty"`
	Interface    string `json:"interface,omitempty"`
	SerialNumber string `json:"serial_number,omitempty"`
	ProductName  string `json:"product_name,omitempty"`
}

type SessionSpec struct {
	SessionID  string           `json:"session_id"`
	NodeID     string           `json:"node_id,omitempty"`
	PeerNodeID string           `json:"peer_node_id,omitempty"`
	Transport  string           `json:"transport,omitempty"`
	Local      DeviceDescriptor `json:"local"`
	Remote     DeviceDescriptor `json:"remote"`
	CreatedAt  time.Time        `json:"created_at,omitempty"`
}

type SessionReport struct {
	SessionID         string           `json:"session_id"`
	NodeID            string           `json:"node_id,omitempty"`
	PeerNodeID        string           `json:"peer_node_id,omitempty"`
	Transport         string           `json:"transport,omitempty"`
	Local             DeviceDescriptor `json:"local"`
	Remote            DeviceDescriptor `json:"remote"`
	Status            string           `json:"status"`
	ClaimedBy         string           `json:"claimed_by,omitempty"`
	LastError         string           `json:"last_error,omitempty"`
	BytesHostToDevice int64            `json:"bytes_host_to_device,omitempty"`
	BytesDeviceToHost int64            `json:"bytes_device_to_host,omitempty"`
	UpdatedAt         time.Time        `json:"updated_at,omitempty"`
}

func NormalizeDeviceDescriptor(descriptor DeviceDescriptor) DeviceDescriptor {
	descriptor.BusID = strings.TrimSpace(descriptor.BusID)
	descriptor.DeviceID = strings.TrimSpace(descriptor.DeviceID)
	descriptor.VendorID = strings.ToLower(strings.TrimSpace(descriptor.VendorID))
	descriptor.ProductID = strings.ToLower(strings.TrimSpace(descriptor.ProductID))
	descriptor.Interface = strings.TrimSpace(descriptor.Interface)
	descriptor.SerialNumber = strings.TrimSpace(descriptor.SerialNumber)
	descriptor.ProductName = strings.TrimSpace(descriptor.ProductName)
	return descriptor
}

func NormalizeSessionSpec(spec SessionSpec) SessionSpec {
	spec.Transport = strings.ToLower(strings.TrimSpace(spec.Transport))
	if spec.Transport == "" {
		spec.Transport = "usbip-encap"
	}
	spec.Local = NormalizeDeviceDescriptor(spec.Local)
	spec.Remote = NormalizeDeviceDescriptor(spec.Remote)
	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = time.Now().UTC()
	} else {
		spec.CreatedAt = spec.CreatedAt.UTC()
	}
	if strings.TrimSpace(spec.SessionID) == "" {
		spec.SessionID = BuildSessionID(spec)
	}
	return spec
}

func BuildSessionID(spec SessionSpec) string {
	spec.Transport = strings.ToLower(strings.TrimSpace(spec.Transport))
	if spec.Transport == "" {
		spec.Transport = "usbip-encap"
	}
	parts := []string{
		usbIDPart(spec.NodeID),
		usbIDPart(spec.PeerNodeID),
		usbDevicePart(spec.Local),
		usbDevicePart(spec.Remote),
		usbIDPart(spec.Transport),
	}
	return strings.Join(parts, "-")
}

func CompatiblePair(local, remote DeviceDescriptor) bool {
	local = NormalizeDeviceDescriptor(local)
	remote = NormalizeDeviceDescriptor(remote)
	if local.VendorID != "" && remote.VendorID != "" && local.VendorID != remote.VendorID {
		return false
	}
	if local.ProductID != "" && remote.ProductID != "" && local.ProductID != remote.ProductID {
		return false
	}
	if local.Interface != "" && remote.Interface != "" && local.Interface != remote.Interface {
		return false
	}
	return true
}

func ConfiguredReport(spec SessionSpec, claimedBy string) SessionReport {
	spec = NormalizeSessionSpec(spec)
	return SessionReport{
		SessionID:  spec.SessionID,
		NodeID:     spec.NodeID,
		PeerNodeID: spec.PeerNodeID,
		Transport:  spec.Transport,
		Local:      spec.Local,
		Remote:     spec.Remote,
		Status:     "configured",
		ClaimedBy:  strings.TrimSpace(claimedBy),
		UpdatedAt:  time.Now().UTC(),
	}
}

func usbIDPart(value string) string {
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

func usbDevicePart(descriptor DeviceDescriptor) string {
	descriptor = NormalizeDeviceDescriptor(descriptor)
	switch {
	case descriptor.BusID != "" || descriptor.DeviceID != "":
		return usbIDPart(descriptor.BusID + "-" + descriptor.DeviceID)
	case descriptor.VendorID != "" || descriptor.ProductID != "":
		return usbIDPart(descriptor.VendorID + "-" + descriptor.ProductID)
	case descriptor.SerialNumber != "":
		return usbIDPart(descriptor.SerialNumber)
	default:
		return "unknown"
	}
}
