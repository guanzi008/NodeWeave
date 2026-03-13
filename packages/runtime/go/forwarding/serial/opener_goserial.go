package serial

import (
	"fmt"
	"io"
	"strings"
	"time"

	goserial "go.bug.st/serial"
)

type GoSerialPortOpener struct{}

func (GoSerialPortOpener) Open(cfg PortConfig) (io.ReadWriteCloser, error) {
	cfg = NormalizePortConfig(cfg)
	mode := &goserial.Mode{
		BaudRate: cfg.BaudRate,
		DataBits: cfg.DataBits,
		Parity:   goSerialParity(cfg.Parity),
		StopBits: goSerialStopBits(cfg.StopBits),
	}
	port, err := goserial.Open(cfg.Name, mode)
	if err != nil {
		return nil, fmt.Errorf("open serial port %s: %w", cfg.Name, err)
	}
	_ = port.SetReadTimeout(time.Duration(cfg.ReadTimeoutMillis) * time.Millisecond)
	return port, nil
}

func goSerialParity(value string) goserial.Parity {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "odd":
		return goserial.OddParity
	case "even":
		return goserial.EvenParity
	default:
		return goserial.NoParity
	}
}

func goSerialStopBits(value int) goserial.StopBits {
	if value >= 2 {
		return goserial.TwoStopBits
	}
	return goserial.OneStopBit
}
