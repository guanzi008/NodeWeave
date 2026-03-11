package tunnel

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"nodeweave/packages/runtime/go/dataplane"
)

type linkedTransport struct {
	addr     string
	peerAddr string
	incoming chan dataplane.Frame
	peer     *linkedTransport
	mu       sync.RWMutex
}

func newLinkedPair() (*linkedTransport, *linkedTransport) {
	left := &linkedTransport{
		addr:     "left",
		peerAddr: "right",
		incoming: make(chan dataplane.Frame, 16),
	}
	right := &linkedTransport{
		addr:     "right",
		peerAddr: "left",
		incoming: make(chan dataplane.Frame, 16),
	}
	left.peer = right
	right.peer = left
	return left, right
}

func (t *linkedTransport) Address() string {
	return t.addr
}

func (t *linkedTransport) Send(_ context.Context, address string, frame dataplane.Frame) error {
	if address != t.peerAddr {
		return fmt.Errorf("unexpected address %s", address)
	}
	t.peer.incoming <- frame
	return nil
}

func (t *linkedTransport) Serve(ctx context.Context, handler func(context.Context, dataplane.Frame, net.Addr) error) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case frame, ok := <-t.incoming:
			if !ok {
				return nil
			}
			if err := handler(ctx, frame, nil); err != nil {
				return err
			}
		}
	}
}

func (t *linkedTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	select {
	case <-t.incoming:
	default:
	}
	close(t.incoming)
	return nil
}

func TestDestinationIP(t *testing.T) {
	packet := []byte{
		0x45, 0x00, 0x00, 0x14,
		0x00, 0x00, 0x00, 0x00,
		0x40, 0x11, 0x00, 0x00,
		100, 64, 0, 10,
		100, 64, 0, 11,
	}
	got, err := DestinationIP(packet)
	if err != nil {
		t.Fatalf("destination ip: %v", err)
	}
	if got != "100.64.0.11" {
		t.Fatalf("unexpected destination ip %q", got)
	}
}

func TestPumpTransfersPacketsBetweenDevices(t *testing.T) {
	deviceA := NewMemoryDevice("tun-a", 8)
	deviceB := NewMemoryDevice("tun-b", 8)

	transportA, transportB := newLinkedPair()

	pumpA := NewPump(deviceA)
	engineA := dataplane.NewEngine(dataplane.Spec{
		NodeID: "node-a",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.11/32",
				PrefixBits:       32,
				PeerNodeID:       "node-b",
				CandidateAddress: transportA.peerAddr,
			},
		},
	}, transportA, pumpA)
	pumpA.AttachEngine(engineA)

	pumpB := NewPump(deviceB)
	engineB := dataplane.NewEngine(dataplane.Spec{
		NodeID: "node-b",
		Routes: []dataplane.Route{
			{
				NetworkCIDR:      "100.64.0.10/32",
				PrefixBits:       32,
				PeerNodeID:       "node-a",
				CandidateAddress: transportB.peerAddr,
			},
		},
	}, transportB, pumpB)
	pumpB.AttachEngine(engineB)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 2)
	go func() { errCh <- pumpA.Run(ctx) }()
	go func() { errCh <- pumpB.Run(ctx) }()

	packet := []byte{
		0x45, 0x00, 0x00, 0x14,
		0x00, 0x00, 0x00, 0x00,
		0x40, 0x11, 0x00, 0x00,
		100, 64, 0, 10,
		100, 64, 0, 11,
	}
	if err := deviceA.Inject(packet); err != nil {
		t.Fatalf("inject packet: %v", err)
	}

	select {
	case received := <-deviceB.outbound:
		if got, err := DestinationIP(received); err != nil || got != "100.64.0.11" {
			t.Fatalf("unexpected received packet dst=%q err=%v", got, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for packet on device B")
	}

	cancel()
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("pump exited with error: %v", err)
		}
	}
}
