package tunnel

import (
	"errors"
	"io"
	"sync"
)

type MemoryDevice struct {
	name     string
	inbound  chan []byte
	outbound chan []byte
	closed   chan struct{}
	once     sync.Once
}

func NewMemoryDevice(name string, queueSize int) *MemoryDevice {
	if queueSize <= 0 {
		queueSize = 16
	}
	return &MemoryDevice{
		name:     name,
		inbound:  make(chan []byte, queueSize),
		outbound: make(chan []byte, queueSize),
		closed:   make(chan struct{}),
	}
}

func (d *MemoryDevice) Name() string {
	return d.name
}

func (d *MemoryDevice) ReadPacket() ([]byte, error) {
	packet, ok := <-d.inbound
	if !ok {
		return nil, io.EOF
	}
	return append([]byte(nil), packet...), nil
}

func (d *MemoryDevice) WritePacket(packet []byte) error {
	select {
	case <-d.closed:
		return io.EOF
	case d.outbound <- append([]byte(nil), packet...):
		return nil
	default:
		return errors.New("memory device outbound queue is full")
	}
}

func (d *MemoryDevice) Close() error {
	d.once.Do(func() {
		close(d.closed)
		close(d.inbound)
		close(d.outbound)
	})
	return nil
}

func (d *MemoryDevice) Inject(packet []byte) error {
	select {
	case <-d.closed:
		return io.EOF
	case d.inbound <- append([]byte(nil), packet...):
		return nil
	default:
		return errors.New("memory device inbound queue is full")
	}
}

func (d *MemoryDevice) Receive() ([]byte, error) {
	packet, ok := <-d.outbound
	if !ok {
		return nil, io.EOF
	}
	return append([]byte(nil), packet...), nil
}
