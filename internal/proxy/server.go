package proxy

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/4m1rali/ghostnet/internal/bypass"
	"github.com/4m1rali/ghostnet/internal/config"
	"github.com/4m1rali/ghostnet/internal/domain"
	"github.com/4m1rali/ghostnet/internal/log"
	"github.com/4m1rali/ghostnet/internal/metrics"
	"github.com/4m1rali/ghostnet/internal/routing"
)

type Server struct {
	cfg      *config.Config
	adaptive *bypass.AdaptiveBypass
	domains  *domain.Pool
	router   *routing.Router
	metrics  *metrics.Collector
	logger   *log.Logger
	pool     *workerPool
	rl       *rateLimiter
	connSeq  atomic.Uint64
}

func NewServer(
	cfg *config.Config,
	adaptive *bypass.AdaptiveBypass,
	domains *domain.Pool,
	router *routing.Router,
	m *metrics.Collector,
	logger *log.Logger,
) *Server {
	size := cfg.WorkerPoolSize
	if size <= 0 {
		size = runtime.NumCPU() * 256
		if size < 512 {
			size = 512
		}
		if size > 16384 {
			size = 16384
		}
	}
	return &Server{
		cfg:      cfg,
		adaptive: adaptive,
		domains:  domains,
		router:   router,
		metrics:  m,
		logger:   logger,
		pool:     newWorkerPool(size),
		rl:       newRateLimiter(cfg.RateLimit),
	}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.ListenHost, s.cfg.ListenPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("server: listen %s: %w", addr, err)
	}
	s.logger.Info("listening on %s  workers=%d", addr, s.pool.size)

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	go s.rl.cleanupLoop(ctx)

	handler := NewHandler(s.cfg, s.adaptive, s.domains, s.router, s.metrics, s.logger)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				s.logger.Info("shutting down — draining %d active connections", s.metrics.ActiveConns())
				s.pool.drain()
				return nil
			default:
				s.logger.Error("accept: %v", err)
				time.Sleep(5 * time.Millisecond)
				continue
			}
		}

		if s.cfg.MaxConnections > 0 && s.metrics.ActiveConns() >= int64(s.cfg.MaxConnections) {
			s.logger.Warn("max connections reached (%d) — rejecting %s", s.cfg.MaxConnections, conn.RemoteAddr())
			conn.Close()
			continue
		}

		if s.cfg.RateLimit > 0 {
			ip := conn.RemoteAddr().(*net.TCPAddr).IP.String()
			if !s.rl.allow(ip) {
				s.logger.RateLimit(ip, s.cfg.RateLimit)
				conn.Close()
				continue
			}
		}

		connID := fmt.Sprintf("%08x", s.connSeq.Add(1))
		c := conn
		id := connID
		s.pool.submit(func() {
			handler.Handle(ctx, c, id)
		})
	}
}

type workerPool struct {
	work     chan func()
	wg       sync.WaitGroup
	overflow sync.WaitGroup
	size     int
	queued   atomic.Int64
	overflow_ atomic.Int64
}

func newWorkerPool(size int) *workerPool {
	p := &workerPool{
		work: make(chan func(), size*4),
		size: size,
	}
	for i := 0; i < size; i++ {
		p.wg.Add(1)
		go p.run()
	}
	return p
}

func (p *workerPool) run() {
	defer p.wg.Done()
	for fn := range p.work {
		p.queued.Add(-1)
		fn()
	}
}

func (p *workerPool) submit(fn func()) {
	p.queued.Add(1)
	select {
	case p.work <- fn:
	default:
		p.queued.Add(-1)
		p.overflow.Add(1)
		p.overflow_.Add(1)
		go func() {
			defer p.overflow.Done()
			defer p.overflow_.Add(-1)
			fn()
		}()
	}
}

func (p *workerPool) drain() {
	close(p.work)
	p.wg.Wait()
	p.overflow.Wait()
}

func (p *workerPool) Queued() int64   { return p.queued.Load() }
func (p *workerPool) Overflow() int64 { return p.overflow_.Load() }

type ipBucket struct {
	mu         sync.Mutex
	timestamps []int64
	lastSeen   time.Time
}

type rateLimiter struct {
	limit   int
	mu      sync.RWMutex
	buckets map[string]*ipBucket
}

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{
		limit:   limit,
		buckets: make(map[string]*ipBucket, 64),
	}
}

func (r *rateLimiter) allow(ip string) bool {
	r.mu.RLock()
	b, ok := r.buckets[ip]
	r.mu.RUnlock()

	if !ok {
		r.mu.Lock()
		b, ok = r.buckets[ip]
		if !ok {
			b = &ipBucket{timestamps: make([]int64, 0, r.limit+1)}
			r.buckets[ip] = b
		}
		r.mu.Unlock()
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	b.lastSeen = now
	cutoff := now.Add(-time.Second).UnixNano()

	j := 0
	for _, t := range b.timestamps {
		if t >= cutoff {
			b.timestamps[j] = t
			j++
		}
	}
	b.timestamps = b.timestamps[:j]

	if len(b.timestamps) >= r.limit {
		return false
	}
	b.timestamps = append(b.timestamps, now.UnixNano())
	return true
}

func (r *rateLimiter) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.cleanup()
		}
	}
}

func (r *rateLimiter) cleanup() {
	cutoff := time.Now().Add(-2 * time.Minute)
	r.mu.Lock()
	for ip, b := range r.buckets {
		b.mu.Lock()
		stale := b.lastSeen.Before(cutoff)
		b.mu.Unlock()
		if stale {
			delete(r.buckets, ip)
		}
	}
	r.mu.Unlock()
}
