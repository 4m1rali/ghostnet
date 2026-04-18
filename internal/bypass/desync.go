package bypass

import (
	"context"
	"fmt"
	"net"
	"time"
)

type DesyncMethod int

const (
	DesyncBadChecksum DesyncMethod = iota
	DesyncRST
	DesyncTTLLimited
)

type DesyncBypass struct {
	injector *RawInjector
	stats    *BypassStats
}

func NewDesyncBypass() (*DesyncBypass, error) {
	inj, err := NewRawInjector()
	if err != nil {
		return nil, fmt.Errorf("desync: %w", err)
	}
	return &DesyncBypass{injector: inj, stats: &BypassStats{}}, nil
}

func (b *DesyncBypass) Close() error        { return b.injector.Close() }
func (b *DesyncBypass) Stats() *BypassStats { return b.stats }

type DesyncParams struct {
	SrcIP      net.IP
	DstIP      net.IP
	SrcPort    uint16
	DstPort    uint16
	SynSeq     uint32
	SynAckSeq  uint32
	SynTime    time.Time
	Method     DesyncMethod
	Payload    []byte
	DelayMs    float64
	MinDelayMs float64
	MaxDelayMs float64
	JitterMs   float64
}

func (b *DesyncBypass) Inject(ctx context.Context, p DesyncParams) error {
	delay := AdaptiveDelay(p.SynTime, p.DelayMs, p.MinDelayMs, p.MaxDelayMs)
	select {
	case <-ctx.Done():
		b.stats.Fail.Add(1)
		return ctx.Err()
	case <-time.After(delay):
	}

	select {
	case <-ctx.Done():
		b.stats.Fail.Add(1)
		return ctx.Err()
	default:
	}

	var err error
	switch p.Method {
	case DesyncBadChecksum:
		err = b.badChecksum(p)
	case DesyncRST:
		err = b.rst(p)
	case DesyncTTLLimited:
		err = b.ttlLimited(p)
	default:
		err = b.badChecksum(p)
	}

	if err != nil {
		b.stats.Fail.Add(1)
		return err
	}
	b.stats.OK.Add(1)
	return nil
}

func (b *DesyncBypass) badChecksum(p DesyncParams) error {
	pkt, err := CraftPacket(PacketParams{
		SrcIP:   p.SrcIP,
		DstIP:   p.DstIP,
		SrcPort: p.SrcPort,
		DstPort: p.DstPort,
		SeqNum:  p.SynSeq + 1,
		AckNum:  p.SynAckSeq + 1,
		Flags:   TCPFlagPSH | TCPFlagACK,
		TTL:     RandomTTL(),
		Payload: p.Payload,
	})
	if err != nil {
		return fmt.Errorf("desync bad-cs: craft: %w", err)
	}
	if len(pkt) >= 38 {
		pkt[36] ^= 0xFF
		pkt[37] ^= 0xFF
	}
	return b.injector.Send(pkt, p.DstIP, p.DstPort)
}

func (b *DesyncBypass) rst(p DesyncParams) error {
	rstSeq := (p.SynSeq + 1 - 65536) & 0xFFFFFFFF
	pkt, err := CraftPacket(PacketParams{
		SrcIP:   p.SrcIP,
		DstIP:   p.DstIP,
		SrcPort: p.SrcPort,
		DstPort: p.DstPort,
		SeqNum:  rstSeq,
		AckNum:  0,
		Flags:   TCPFlagRST,
		TTL:     RandomTTL(),
	})
	if err != nil {
		return fmt.Errorf("desync rst: craft: %w", err)
	}
	return b.injector.Send(pkt, p.DstIP, p.DstPort)
}

func (b *DesyncBypass) ttlLimited(p DesyncParams) error {
	fakeSeq := (p.SynSeq + 1 - uint32(len(p.Payload))) & 0xFFFFFFFF
	pkt, err := CraftPacket(PacketParams{
		SrcIP:   p.SrcIP,
		DstIP:   p.DstIP,
		SrcPort: p.SrcPort,
		DstPort: p.DstPort,
		SeqNum:  fakeSeq,
		AckNum:  p.SynAckSeq + 1,
		Flags:   TCPFlagPSH | TCPFlagACK,
		TTL:     1,
		Payload: p.Payload,
	})
	if err != nil {
		return fmt.Errorf("desync ttl: craft: %w", err)
	}
	return b.injector.Send(pkt, p.DstIP, p.DstPort)
}
