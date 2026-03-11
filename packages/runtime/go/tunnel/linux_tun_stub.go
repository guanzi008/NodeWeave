//go:build !linux

package tunnel

import "fmt"

type LinuxTUN struct{}

func OpenLinux(name string) (*LinuxTUN, error) {
	return nil, fmt.Errorf("linux tun is not supported on this platform: %s", name)
}

func (d *LinuxTUN) Name() string {
	return ""
}

func (d *LinuxTUN) ReadPacket() ([]byte, error) {
	return nil, fmt.Errorf("linux tun is not supported on this platform")
}

func (d *LinuxTUN) WritePacket([]byte) error {
	return fmt.Errorf("linux tun is not supported on this platform")
}

func (d *LinuxTUN) Close() error {
	return nil
}
