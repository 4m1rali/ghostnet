package domain

import (
	"context"
	"math"
	"math/rand"
	"net"
	"sort"
	"sync"
	"time"
)

type Entry struct {
	Domain    string
	Latency   time.Duration
	Alive     bool
	LastCheck time.Time

	mu           sync.Mutex
	failStreak   int
	nextProbeAt  time.Time
}

func (e *Entry) backoffDelay() time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failStreak == 0 {
		return 0
	}
	base := 10.0
	delay := base * math.Pow(2, float64(e.failStreak-1))
	if delay > 600 {
		delay = 600
	}
	jitter := rand.Float64() * base
	return time.Duration((delay+jitter)*float64(time.Second))
}

func (e *Entry) recordResult(lat time.Duration, alive bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.LastCheck = time.Now()
	if alive {
		e.failStreak = 0
		e.nextProbeAt = time.Time{}
	} else {
		e.failStreak++
		base := 10.0
		delay := base * math.Pow(2, float64(e.failStreak-1))
		if delay > 600 {
			delay = 600
		}
		jitter := rand.Float64() * base
		e.nextProbeAt = time.Now().Add(time.Duration((delay+jitter)*float64(time.Second)))
	}
	e.Latency = lat
	e.Alive = alive
}

func (e *Entry) dueForProbe() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.nextProbeAt.IsZero() || time.Now().After(e.nextProbeAt)
}

type Pool struct {
	mu            sync.RWMutex
	entries       []*Entry
	best          string
	probePort     int
	probeTimeout  time.Duration
	probeInterval time.Duration
}

func NewPool(domains []string, probePort int, probeInterval int) *Pool {
	entries := make([]*Entry, 0, len(domains))
	for _, d := range domains {
		entries = append(entries, &Entry{Domain: d, Alive: true})
	}
	p := &Pool{
		entries:       entries,
		probePort:     probePort,
		probeTimeout:  3 * time.Second,
		probeInterval: time.Duration(probeInterval) * time.Second,
	}
	if len(entries) > 0 {
		p.best = entries[0].Domain
	}
	return p
}

func (p *Pool) Best() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.best
}

func (p *Pool) All() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, len(p.entries))
	for i, e := range p.entries {
		out[i] = e.Domain
	}
	return out
}

func (p *Pool) Entries() []Entry {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]Entry, len(p.entries))
	for i, e := range p.entries {
		out[i] = *e
	}
	return out
}

func (p *Pool) StartProbing(ctx context.Context) {
	go p.loop(ctx)
}

func (p *Pool) loop(ctx context.Context) {
	p.probeAll()
	if p.probeInterval <= 0 {
		return
	}
	ticker := time.NewTicker(p.probeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.probeAll()
		}
	}
}

func (p *Pool) probeAll() {
	p.mu.RLock()
	snapshot := make([]*Entry, len(p.entries))
	copy(snapshot, p.entries)
	p.mu.RUnlock()

	var wg sync.WaitGroup
	for _, e := range snapshot {
		if !e.dueForProbe() {
			continue
		}
		wg.Add(1)
		go func(entry *Entry) {
			defer wg.Done()
			lat, alive := p.probe(entry.Domain)
			entry.recordResult(lat, alive)
		}(e)
	}
	wg.Wait()

	p.mu.Lock()
	sort.Slice(p.entries, func(i, j int) bool {
		a, b := p.entries[i], p.entries[j]
		if a.Alive != b.Alive {
			return a.Alive
		}
		if a.Latency == 0 && b.Latency == 0 {
			return false
		}
		if a.Latency == 0 {
			return false
		}
		if b.Latency == 0 {
			return true
		}
		return a.Latency < b.Latency
	})
	for _, e := range p.entries {
		if e.Alive {
			p.best = e.Domain
			break
		}
	}
	p.mu.Unlock()
}

func (p *Pool) probe(domain string) (time.Duration, bool) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(domain, "443"), p.probeTimeout)
	if err != nil {
		return 0, false
	}
	conn.Close()
	return time.Since(start), true
}

func (p *Pool) Add(domain string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.entries {
		if e.Domain == domain {
			return
		}
	}
	p.entries = append(p.entries, &Entry{Domain: domain, Alive: true})
}

func (p *Pool) Remove(domain string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, e := range p.entries {
		if e.Domain == domain {
			p.entries = append(p.entries[:i], p.entries[i+1:]...)
			return
		}
	}
}
