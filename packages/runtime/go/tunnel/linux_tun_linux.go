//go:build linux

package tunnel

import (
	"fmt"
	"os"
	"unsafe"

	"syscall"
)

const (
	tunDevicePath = "/dev/net/tun"
	iffTUN        = 0x0001
	iffNoPI       = 0x1000
	tunsetIFF     = 0x400454ca
)

type LinuxTUN struct {
	name string
	file *os.File
}

func OpenLinux(name string) (*LinuxTUN, error) {
	file, err := os.OpenFile(tunDevicePath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", tunDevicePath, err)
	}

	ifr := ifreq{
		Flags: iffTUN | iffNoPI,
	}
	copy(ifr.Name[:], []byte(name))

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), uintptr(tunsetIFF), uintptr(unsafe.Pointer(&ifr))); errno != 0 {
		_ = file.Close()
		return nil, fmt.Errorf("configure tun interface %s: %w", name, errno)
	}

	return &LinuxTUN{
		name: zeroTerminatedString(ifr.Name[:]),
		file: file,
	}, nil
}

func (d *LinuxTUN) Name() string {
	if d == nil {
		return ""
	}
	return d.name
}

func (d *LinuxTUN) ReadPacket() ([]byte, error) {
	buffer := make([]byte, 64*1024)
	n, err := d.file.Read(buffer)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), buffer[:n]...), nil
}

func (d *LinuxTUN) WritePacket(packet []byte) error {
	_, err := d.file.Write(packet)
	return err
}

func (d *LinuxTUN) Close() error {
	if d == nil || d.file == nil {
		return nil
	}
	return d.file.Close()
}

type ifreq struct {
	Name  [16]byte
	Flags uint16
	_     [22]byte
}

func zeroTerminatedString(raw []byte) string {
	for i, b := range raw {
		if b == 0 {
			return string(raw[:i])
		}
	}
	return string(raw)
}
