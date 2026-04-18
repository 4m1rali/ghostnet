package bypass

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/4m1rali/ghostnet/internal/tls"
)

type BypassStats struct {
	OK   atomic.Int64
	Fail atomic.Int64
}

type WrongSeqBypass struct {
	injector *RawInjector
	stats    *BypassStats
}

func NewWrongSeqBypass() (*WrongSeqBypass, error) {
	inj, err := NewRawInjector()
	if err != nil {
		return nil, fmt.Errorf("wrong_seq: %w", err)
	}
	return &WrongSeqBypass{injector: inj, stats: &BypassStats{}}, nil
}

func (b *WrongSeqBypass) Close() error        { return b.injector.Close() }
func (b *WrongSeqBypass) Stats() *BypassStats { return b.stats }

type InjectParams struct {
	SrcIP      net.IP
	DstIP      net.IP
	SrcPort    uint16
	DstPort    uint16
	SynSeq     uint32
	SynTime    time.Time
	FakeSNI    string
	Profile    string
	TTLSpoof   bool
	DelayMs    float64
	MinDelayMs float64
	MaxDelayMs float64
	JitterMs   float64
}

func (b *WrongSeqBypass) Inject(ctx context.Context, p InjectParams) error {
	builder := tls.NewBuilder(p.Profile)
	fakeHello, err := builder.Build(p.FakeSNI)
	if err != nil {
		b.stats.Fail.Add(1)
		return fmt.Errorf("wrong_seq: build: %w", err)
	}

	delay := AdaptiveDelay(p.SynTime, p.DelayMs, p.MinDelayMs, p.MaxDelayMs)
	if p.JitterMs > 0 {
		extra := time.Duration((tRandFloat64()*2-1)*p.JitterMs*float64(time.Millisecond))
		delay += extra
		if delay < 0 {
			delay = 0
		}
	}

	if delay > 0 {
		select {
		case <-ctx.Done():
			b.stats.Fail.Add(1)
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	select {
	case <-ctx.Done():
		b.stats.Fail.Add(1)
		return ctx.Err()
	default:
	}

	fakeSeq := (p.SynSeq + 1 - uint32(len(fakeHello))) & 0xFFFFFFFF

	var ttl uint8
	if p.TTLSpoof {
		ttl = RandomTTL()
	}

	pkt, err := CraftPacket(PacketParams{
		SrcIP:   p.SrcIP,
		DstIP:   p.DstIP,
		SrcPort: p.SrcPort,
		DstPort: p.DstPort,
		SeqNum:  fakeSeq,
		AckNum:  0,
		Flags:   TCPFlagPSH | TCPFlagACK,
		TTL:     ttl,
		Payload: fakeHello,
	})
	if err != nil {
		b.stats.Fail.Add(1)
		return fmt.Errorf("wrong_seq: craft: %w", err)
	}

	if err := b.injector.Send(pkt, p.DstIP, p.DstPort); err != nil {
		b.stats.Fail.Add(1)
		return fmt.Errorf("wrong_seq: send: %w", err)
	}

	b.stats.OK.Add(1)
	return nil
}
