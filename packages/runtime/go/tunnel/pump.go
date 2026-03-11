package tunnel

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"strings"

	"nodeweave/packages/runtime/go/dataplane"
)

type Pump struct {
	device Device
	engine *dataplane.Engine
}

func NewPump(device Device) *Pump {
	return &Pump{device: device}
}

func (p *Pump) AttachEngine(engine *dataplane.Engine) {
	p.engine = engine
}

func (p *Pump) Run(ctx context.Context) error {
	if p.device == nil {
		return errors.New("tunnel device is nil")
	}
	if p.engine == nil {
		return errors.New("dataplane engine is nil")
	}

	engineErrCh := make(chan error, 1)
	go func() {
		engineErrCh <- p.engine.Serve(ctx)
	}()

	go func() {
		<-ctx.Done()
		_ = p.device.Close()
	}()

	for {
		packet, err := p.device.ReadPacket()
		if err != nil {
			select {
			case engineErr := <-engineErrCh:
				if engineErr != nil && ctx.Err() == nil {
					return engineErr
				}
			default:
			}
			if ctx.Err() != nil || errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || strings.Contains(strings.ToLower(err.Error()), "closed") {
				return nil
			}
			return err
		}

		destinationIP, err := DestinationIP(packet)
		if err != nil {
			log.Printf("tunnel pump dropped packet: %v", err)
			continue
		}

		if err := p.engine.SendPacket(ctx, destinationIP, packet); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("tunnel pump failed to send packet dst=%s: %v", destinationIP, err)
		}
	}
}

func (p *Pump) HandleInbound(_ context.Context, packet dataplane.InboundPacket) error {
	if p.device == nil {
		return errors.New("tunnel device is nil")
	}
	return p.device.WritePacket(packet.Payload)
}
