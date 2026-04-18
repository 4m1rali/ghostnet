package routing

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/4m1rali/ghostnet/internal/config"
	"github.com/4m1rali/ghostnet/internal/log"
)

type EndpointState int32

const (
	StateHealthy   EndpointState = iota
	StateUnhealthy
	StateOpen
)

func (s EndpointState) String() string {
	switch s {
	case StateHealthy:
		return "healthy"
	case StateUnhealthy:
		return "unhealthy"
	case StateOpen:
		return "open"
	default:
		return "unknown"
	}
}

type Endpoint struct {
	Addr string

	mu        sync.Mutex
	state     EndpointState
	failures  int
	openUntil time.Time

	ewmaLatencyNs atomic.Int64
	totalConns    atomic.Int64
	failedConns   atomic.Int64
}

func (e *Endpoint) State() EndpointState {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state == StateOpen && time.Now().After(e.openUntil) {
		e.state = StateUnhealthy
		e.failures = 0
	}
	return e.state
}

func (e *Endpoint) Latency() time.Duration {
	return time.Duration(e.ewmaLatencyNs.Load())
}

func (e *Endpoint) updateEWMA(sample time.Duration) {
	const alpha = 8
	prev := e.ewmaLatencyNs.Load()
	var next int64
	if prev == 0 {
		next = int64(sample)
	} else {
		next = (prev*(alpha-1) + int64(sample)) / alpha
	}
	e.ewmaLatencyNs.Store(next)
}

func (e *Endpoint) RecordSuccess(lat time.Duration) {
	e.updateEWMA(lat)
	e.totalConns.Add(1)
	e.mu.Lock()
	e.state = StateHealthy
	e.failures = 0
	e.mu.Unlock()
}

func (e *Endpoint) RecordFailure(threshold int, cooldown time.Duration) (tripped bool) {
	e.failedConns.Add(1)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.failures++
	if e.failures >= threshold {
		e.state = StateOpen
		e.openUntil = time.Now().Add(cooldown)
		return true
	}
	e.state = StateUnhealthy
	return false
}

func (e *Endpoint) Stats() (total, failed int64, lat time.Duration, state EndpointState) {
	return e.totalConns.Load(), e.failedConns.Load(), e.Latency(), e.State()
}

type Router struct {
	endpoints []*Endpoint
	mu        sync.RWMutex
	cfg       *config.Config
	logger    *log.Logger
}

func New(cfg *config.Config, logger *log.Logger) *Router {
	r := &Router{cfg: cfg, logger: logger}
	for _, ep := range cfg.Endpoints {
		r.endpoints = append(r.endpoints, &Endpoint{
			Addr: fmt.Sprintf("%s:%d", ep.IP, ep.Port),
		})
	}
	return r
}

func (r *Router) Pick() (*Endpoint, error) {
	r.mu.RLock()
	eps := make([]*Endpoint, len(r.endpoints))
	copy(eps, r.endpoints)
	r.mu.RUnlock()

	var candidates []*Endpoint
	for _, ep := range eps {
		if ep.State() != StateOpen {
			candidates = append(candidates, ep)
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("router: all %d endpoints circuit-open", len(eps))
	}

	sort.Slice(candidates, func(i, j int) bool {
		li := candidates[i].Latency()
		lj := candidates[j].Latency()
		if li == 0 && lj == 0 {
			return false
		}
		if li == 0 {
			return false
		}
		if lj == 0 {
			return true
		}
		return li < lj
	})

	top := len(candidates)
	if top > 3 {
		top = 3
	}
	return candidates[rand.Intn(top)], nil
}

func (r *Router) StartHealthChecks(ctx context.Context) {
	go r.healthLoop(ctx)
}

func (r *Router) healthLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	r.checkAll()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.checkAll()
		}
	}
}

func (r *Router) checkAll() {
	r.mu.RLock()
	eps := make([]*Endpoint, len(r.endpoints))
	copy(eps, r.endpoints)
	r.mu.RUnlock()

	var wg sync.WaitGroup
	for _, ep := range eps {
		if ep.State() == StateOpen {
			continue
		}
		wg.Add(1)
		go func(e *Endpoint) {
			defer wg.Done()
			lat, alive := r.probe(e.Addr)
			if alive {
				e.updateEWMA(lat)
				e.mu.Lock()
				if e.state == StateUnhealthy {
					e.state = StateHealthy
					e.failures = 0
				}
				e.mu.Unlock()
				r.logger.Health(e.Addr, true, lat, 0)
			} else {
				e.mu.Lock()
				e.failures++
				fails := e.failures
				tripped := false
				if e.failures >= r.cfg.CircuitBreakerThreshold {
					e.state = StateOpen
					e.openUntil = time.Now().Add(r.cfg.CircuitBreakerCooldown())
					tripped = true
				} else {
					e.state = StateUnhealthy
				}
				e.mu.Unlock()
				r.logger.Health(e.Addr, false, 0, fails)
				if tripped {
					r.logger.Circuit(e.Addr, "open")
				}
			}
		}(ep)
	}
	wg.Wait()
}

func (r *Router) probe(addr string) (time.Duration, bool) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return 0, false
	}
	conn.Close()
	return time.Since(start), true
}

func (r *Router) Endpoints() []*Endpoint {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Endpoint, len(r.endpoints))
	copy(out, r.endpoints)
	return out
}

func (r *Router) Status() string {
	r.mu.RLock()
	eps := make([]*Endpoint, len(r.endpoints))
	copy(eps, r.endpoints)
	r.mu.RUnlock()

	healthy, unhealthy, open := 0, 0, 0
	for _, ep := range eps {
		switch ep.State() {
		case StateHealthy:
			healthy++
		case StateUnhealthy:
			unhealthy++
		case StateOpen:
			open++
		}
	}
	return fmt.Sprintf("total=%d healthy=%d unhealthy=%d open=%d", len(eps), healthy, unhealthy, open)
}
