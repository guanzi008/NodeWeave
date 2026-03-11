package tunnel

import (
	"fmt"
	"net/netip"
)

type Device interface {
	Name() string
	ReadPacket() ([]byte, error)
	WritePacket([]byte) error
	Close() error
}

func DestinationIP(packet []byte) (string, error) {
	if len(packet) < 1 {
		return "", fmt.Errorf("empty packet")
	}

	version := packet[0] >> 4
	switch version {
	case 4:
		if len(packet) < 20 {
			return "", fmt.Errorf("ipv4 packet too short: %d", len(packet))
		}
		addr, ok := netip.AddrFromSlice(packet[16:20])
		if !ok {
			return "", fmt.Errorf("invalid ipv4 destination")
		}
		return addr.String(), nil
	case 6:
		if len(packet) < 40 {
			return "", fmt.Errorf("ipv6 packet too short: %d", len(packet))
		}
		addr, ok := netip.AddrFromSlice(packet[24:40])
		if !ok {
			return "", fmt.Errorf("invalid ipv6 destination")
		}
		return addr.String(), nil
	default:
		return "", fmt.Errorf("unsupported ip version %d", version)
	}
}
