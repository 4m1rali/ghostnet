package bypass

import (
	"context"
	"fmt"
	"net"
	"time"
)

type FragmentBypass struct {
	injector *RawInjector
	stats    *BypassStats
}

func NewFragmentBypass() (*FragmentBypass, error) {
	inj, err := NewRawInjector()
	if err != nil {
		return nil, fmt.Errorf("fragment: %w", err)
	}
	return &FragmentBypass{injector: inj, stats: &BypassStats{}}, nil
}

func (b *FragmentBypass) Close() error        { return b.injector.Close() }
func (b *FragmentBypass) Stats() *BypassStats { return b.stats }

type FragmentParams struct {
	SrcIP       net.IP
	DstIP       net.IP
	SrcPort     uint16
	DstPort     uint16
	SynSeq      uint32
	SynAckSeq   uint32
	SynTime     time.Time
	Payload     []byte
	FragSize    int
	DelayMs     float64
	MinDelayMs  float64
	MaxDelayMs  float64
	JitterMs    float64
	TTLSpoof    bool
	InterFragMs float64
}

func (b *FragmentBypass) Inject(ctx context.Context, p FragmentParams) error {
	if len(p.Payload) == 0 {
		return fmt.Errorf("fragment: empty payload")
	}

	fragSize := p.FragSize
	if fragSize <= 0 {
		fragSize = sniSplitPoint(p.Payload)
	}
	if fragSize <= 0 || fragSize >= len(p.Payload) {
		fragSize = len(p.Payload) / 2
	}

	delay := AdaptiveDelay(p.SynTime, p.DelayMs, p.MinDelayMs, p.MaxDelayMs)
	select {
	case <-ctx.Done():
		b.stats.Fail.Add(1)
		return ctx.Err()
	case <-time.After(delay):
	}

	baseSeq := (p.SynSeq + 1 - uint32(len(p.Payload))) & 0xFFFFFFFF
	cur := baseSeq

	frags := chunkBytes(p.Payload, fragSize)
	for i, frag := range frags {
		select {
		case <-ctx.Done():
			b.stats.Fail.Add(1)
			return ctx.Err()
		default:
		}

		flags := uint8(TCPFlagACK)
		if i == len(frags)-1 {
			flags |= TCPFlagPSH
		}

		var ttl uint8
		if p.TTLSpoof {
			ttl = RandomTTL()
		}

		pkt, err := CraftPacket(PacketParams{
			SrcIP:   p.SrcIP,
			DstIP:   p.DstIP,
			SrcPort: p.SrcPort,
			DstPort: p.DstPort,
			SeqNum:  cur,
			AckNum:  p.SynAckSeq + 1,
			Flags:   flags,
			TTL:     ttl,
			Payload: frag,
		})
		if err != nil {
			b.stats.Fail.Add(1)
			return fmt.Errorf("fragment: craft[%d]: %w", i, err)
		}

		if err := b.injector.Send(pkt, p.DstIP, p.DstPort); err != nil {
			b.stats.Fail.Add(1)
			return fmt.Errorf("fragment: send[%d]: %w", i, err)
		}

		cur = (cur + uint32(len(frag))) & 0xFFFFFFFF

		if p.InterFragMs > 0 && i < len(frags)-1 {
			JitterSleep(time.Duration(p.InterFragMs*float64(time.Millisecond)), p.JitterMs)
		}
	}

	b.stats.OK.Add(1)
	return nil
}

func sniSplitPoint(data []byte) int {
	if len(data) < 10 {
		return len(data) / 2
	}
	return 1
}

func chunkBytes(data []byte, size int) [][]byte {
	if size <= 0 {
		return [][]byte{data}
	}
	var out [][]byte
	for len(data) > 0 {
		n := size
		if n > len(data) {
			n = len(data)
		}
		out = append(out, data[:n])
		data = data[n:]
	}
	return out
}
