package bypass

import (
	"container/list"
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/4m1rali/ghostnet/internal/tls"
)

type Strategy int

const (
	StrategyWrongSeq    Strategy = iota
	StrategyFragment
	StrategyDesyncBadCS
	StrategyDesyncTTL
	StrategyUnknown
)

func (s Strategy) String() string {
	switch s {
	case StrategyWrongSeq:
		return "wrong_seq"
	case StrategyFragment:
		return "fragment"
	case StrategyDesyncBadCS:
		return "desync_bad_cs"
	case StrategyDesyncTTL:
		return "desync_ttl"
	default:
		return "unknown"
	}
}

type cacheEntry struct {
	key       string
	strategy  Strategy
	expiresAt time.Time
}

type lruCache struct {
	mu      sync.Mutex
	cap     int
	ttl     time.Duration
	items   map[string]*list.Element
	order   *list.List
}

func newLRUCache(cap int, ttl time.Duration) *lruCache {
	return &lruCache{
		cap:   cap,
		ttl:   ttl,
		items: make(map[string]*list.Element, cap),
		order: list.New(),
	}
}

func (c *lruCache) get(key string) (Strategy, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return StrategyUnknown, false
	}
	entry := el.Value.(*cacheEntry)
	if time.Now().After(entry.expiresAt) {
		c.order.Remove(el)
		delete(c.items, key)
		return StrategyUnknown, false
	}
	c.order.MoveToFront(el)
	return entry.strategy, true
}

func (c *lruCache) set(key string, s Strategy) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		el.Value.(*cacheEntry).strategy = s
		el.Value.(*cacheEntry).expiresAt = time.Now().Add(c.ttl)
		return
	}

	if c.order.Len() >= c.cap {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			delete(c.items, oldest.Value.(*cacheEntry).key)
		}
	}

	entry := &cacheEntry{key: key, strategy: s, expiresAt: time.Now().Add(c.ttl)}
	el := c.order.PushFront(entry)
	c.items[key] = el
}

type StrategyRecorder interface {
	RecordBypassStrategy(strategy string, ok bool)
}

type AdaptiveBypass struct {
	wrongSeq *WrongSeqBypass
	fragment *FragmentBypass
	desync   *DesyncBypass
	cache    *lruCache
	recorder StrategyRecorder
}

func NewAdaptiveBypass() (*AdaptiveBypass, error) {
	ws, err := NewWrongSeqBypass()
	if err != nil {
		return nil, fmt.Errorf("adaptive: wrong_seq: %w", err)
	}
	frag, err := NewFragmentBypass()
	if err != nil {
		ws.Close()
		return nil, fmt.Errorf("adaptive: fragment: %w", err)
	}
	desync, err := NewDesyncBypass()
	if err != nil {
		ws.Close()
		frag.Close()
		return nil, fmt.Errorf("adaptive: desync: %w", err)
	}
	return &AdaptiveBypass{
		wrongSeq: ws,
		fragment: frag,
		desync:   desync,
		cache:    newLRUCache(4096, 10*time.Minute),
	}, nil
}

func (a *AdaptiveBypass) SetRecorder(r StrategyRecorder) {
	a.recorder = r
}

func (a *AdaptiveBypass) Close() {
	a.wrongSeq.Close()
	a.fragment.Close()
	a.desync.Close()
}

type AdaptiveParams struct {
	SrcIP      net.IP
	DstIP      net.IP
	SrcPort    uint16
	DstPort    uint16
	SynSeq     uint32
	SynAckSeq  uint32
	SynTime    time.Time
	FakeSNI    string
	Profile    string
	TTLSpoof   bool
	DelayMs    float64
	MinDelayMs float64
	MaxDelayMs float64
	JitterMs   float64
	FragSize   int
}

func (a *AdaptiveBypass) Inject(ctx context.Context, p AdaptiveParams) (Strategy, error) {
	key := fmt.Sprintf("%s:%d", p.DstIP, p.DstPort)

	strategy := StrategyWrongSeq
	if cached, ok := a.cache.get(key); ok {
		strategy = cached
	}

	err := a.run(ctx, strategy, p)
	if err != nil {
		if a.recorder != nil {
			a.recorder.RecordBypassStrategy(strategy.String(), false)
		}
		next := a.next(strategy)
		if next != strategy {
			if err2 := a.run(ctx, next, p); err2 == nil {
				a.cache.set(key, next)
				if a.recorder != nil {
					a.recorder.RecordBypassStrategy(next.String(), true)
				}
				return next, nil
			}
		}
		return strategy, err
	}

	a.cache.set(key, strategy)
	if a.recorder != nil {
		a.recorder.RecordBypassStrategy(strategy.String(), true)
	}
	return strategy, nil
}

func (a *AdaptiveBypass) run(ctx context.Context, s Strategy, p AdaptiveParams) error {
	builder := tls.NewBuilder(p.Profile)
	fakeHello, err := builder.Build(p.FakeSNI)
	if err != nil {
		return fmt.Errorf("adaptive: build: %w", err)
	}

	switch s {
	case StrategyWrongSeq:
		return a.wrongSeq.Inject(ctx, InjectParams{
			SrcIP:      p.SrcIP,
			DstIP:      p.DstIP,
			SrcPort:    p.SrcPort,
			DstPort:    p.DstPort,
			SynSeq:     p.SynSeq,
			SynTime:    p.SynTime,
			FakeSNI:    p.FakeSNI,
			Profile:    p.Profile,
			TTLSpoof:   p.TTLSpoof,
			DelayMs:    p.DelayMs,
			MinDelayMs: p.MinDelayMs,
			MaxDelayMs: p.MaxDelayMs,
			JitterMs:   p.JitterMs,
		})
	case StrategyFragment:
		return a.fragment.Inject(ctx, FragmentParams{
			SrcIP:       p.SrcIP,
			DstIP:       p.DstIP,
			SrcPort:     p.SrcPort,
			DstPort:     p.DstPort,
			SynSeq:      p.SynSeq,
			SynAckSeq:   p.SynAckSeq,
			SynTime:     p.SynTime,
			Payload:     fakeHello,
			FragSize:    p.FragSize,
			DelayMs:     p.DelayMs,
			MinDelayMs:  p.MinDelayMs,
			MaxDelayMs:  p.MaxDelayMs,
			JitterMs:    p.JitterMs,
			TTLSpoof:    p.TTLSpoof,
			InterFragMs: 0.5,
		})
	case StrategyDesyncBadCS:
		return a.desync.Inject(ctx, DesyncParams{
			SrcIP:      p.SrcIP,
			DstIP:      p.DstIP,
			SrcPort:    p.SrcPort,
			DstPort:    p.DstPort,
			SynSeq:     p.SynSeq,
			SynAckSeq:  p.SynAckSeq,
			SynTime:    p.SynTime,
			Method:     DesyncBadChecksum,
			Payload:    fakeHello,
			DelayMs:    p.DelayMs,
			MinDelayMs: p.MinDelayMs,
			MaxDelayMs: p.MaxDelayMs,
			JitterMs:   p.JitterMs,
		})
	case StrategyDesyncTTL:
		return a.desync.Inject(ctx, DesyncParams{
			SrcIP:      p.SrcIP,
			DstIP:      p.DstIP,
			SrcPort:    p.SrcPort,
			DstPort:    p.DstPort,
			SynSeq:     p.SynSeq,
			SynAckSeq:  p.SynAckSeq,
			SynTime:    p.SynTime,
			Method:     DesyncTTLLimited,
			Payload:    fakeHello,
			DelayMs:    p.DelayMs,
			MinDelayMs: p.MinDelayMs,
			MaxDelayMs: p.MaxDelayMs,
			JitterMs:   p.JitterMs,
		})
	default:
		return fmt.Errorf("adaptive: unknown strategy %d", s)
	}
}

func (a *AdaptiveBypass) next(s Strategy) Strategy {
	switch s {
	case StrategyWrongSeq:
		return StrategyFragment
	case StrategyFragment:
		return StrategyDesyncBadCS
	case StrategyDesyncBadCS:
		return StrategyDesyncTTL
	default:
		return StrategyWrongSeq
	}
}

func (a *AdaptiveBypass) GetCached(dstIP net.IP, dstPort uint16) Strategy {
	key := fmt.Sprintf("%s:%d", dstIP, dstPort)
	if s, ok := a.cache.get(key); ok {
		return s
	}
	return StrategyUnknown
}
