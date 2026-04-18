package proxy

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/4m1rali/ghostnet/internal/bypass"
	"github.com/4m1rali/ghostnet/internal/config"
	"github.com/4m1rali/ghostnet/internal/domain"
	"github.com/4m1rali/ghostnet/internal/log"
	"github.com/4m1rali/ghostnet/internal/metrics"
	"github.com/4m1rali/ghostnet/internal/routing"
)

type Handler struct {
	cfg      *config.Config
	adaptive *bypass.AdaptiveBypass
	domains  *domain.Pool
	router   *routing.Router
	metrics  *metrics.Collector
	logger   *log.Logger
}

func NewHandler(
	cfg *config.Config,
	adaptive *bypass.AdaptiveBypass,
	domains *domain.Pool,
	router *routing.Router,
	m *metrics.Collector,
	logger *log.Logger,
) *Handler {
	return &Handler{
		cfg:      cfg,
		adaptive: adaptive,
		domains:  domains,
		router:   router,
		metrics:  m,
		logger:   logger,
	}
}

func (h *Handler) Handle(ctx context.Context, client net.Conn, connID string) {
	defer client.Close()

	clog := h.logger.WithConnID(connID)
	addr := client.RemoteAddr().(*net.TCPAddr)
	start := time.Now()

	h.metrics.ConnOpened()
	defer h.metrics.ConnClosed()

	clog.ConnOpen(connID, addr.String(), h.metrics.ActiveConns(), h.metrics.TotalConns())

	server, ep, err := h.dialWithRetry(ctx, connID, clog)
	if err != nil {
		clog.ConnFail(connID, addr.String(), err.Error())
		h.metrics.ConnFailed()
		return
	}
	defer server.Close()

	applyTCPOpts(server)
	applyTCPOpts(client)

	fakeSNI := h.domains.Best()
	if fakeSNI == "" {
		fakeSNI = h.cfg.FakeSNI
	}

	localAddr := server.LocalAddr().(*net.TCPAddr)
	remoteAddr := server.RemoteAddr().(*net.TCPAddr)

	synSeq, synAckSeq, synTime, seqErr := getTCPSeqNums(server)
	if seqErr != nil {
		clog.Debug("seq nums unavailable (%v) — using pseudo-ISN", seqErr)
	}

	if h.adaptive != nil {
		injectCtx, injectCancel := context.WithTimeout(ctx, h.cfg.BypassTimeoutDuration())
		injectStart := time.Now()
		strategy, injectErr := h.adaptive.Inject(injectCtx, bypass.AdaptiveParams{
			SrcIP:      localAddr.IP.To4(),
			DstIP:      remoteAddr.IP.To4(),
			SrcPort:    uint16(localAddr.Port),
			DstPort:    uint16(remoteAddr.Port),
			SynSeq:     synSeq,
			SynAckSeq:  synAckSeq,
			SynTime:    synTime,
			FakeSNI:    fakeSNI,
			Profile:    string(h.cfg.BrowserProfile),
			TTLSpoof:   h.cfg.TTLSpoof,
			DelayMs:    h.cfg.FakeDelayMs,
			MinDelayMs: h.cfg.MinDelayMs,
			MaxDelayMs: h.cfg.MaxDelayMs,
			JitterMs:   h.cfg.JitterMs,
			FragSize:   h.cfg.FragmentSize,
		})
		injectCancel()
		if injectErr != nil {
			clog.BypassFail(connID, "adaptive", injectErr.Error())
			h.metrics.BypassFailed()
		} else {
			clog.BypassOK(connID, strategy.String(), fakeSNI, time.Since(injectStart))
			h.metrics.BypassOK()
		}
	}

	clog.Relay(connID, localAddr.String(), remoteAddr.String())
	h.metrics.RelayStarted()
	defer h.metrics.RelayDone()

	idleTimeout := h.cfg.IdleTimeoutDuration()
	var sni string
	var result RelayResult

	if h.cfg.LogClientSNI {
		sni, result = RelayWithSNI(
			ctx, client, server, idleTimeout, 16384,
			func(n int64) { h.metrics.AddBytesIn(n) },
			func(n int64) { h.metrics.AddBytesOut(n) },
		)
		if sni != "" {
			clog.SNI(connID, sni, addr.IP.String())
			h.metrics.RecordSNI(sni)
		}
	} else {
		result = Relay(
			ctx, client, server, idleTimeout,
			func(n int64) { h.metrics.AddBytesIn(n) },
			func(n int64) { h.metrics.AddBytesOut(n) },
		)
	}

	dur := time.Since(start)
	h.metrics.RecordLatency(dur)

	if ep != nil {
		ep.RecordSuccess(dur)
	}

	clog.ConnClose(connID, addr.String(), result.BytesIn, result.BytesOut, dur)
}

func (h *Handler) dialWithRetry(ctx context.Context, connID string, clog *log.Logger) (net.Conn, *routing.Endpoint, error) {
	maxAttempts := h.cfg.RetryLimit + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			delay := bypass.ExponentialBackoff(attempt-1, h.cfg.RetryBaseMs, h.cfg.RetryMaxMs)
			clog.ConnRetry(connID, attempt, maxAttempts-1, delay, lastErr.Error())
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		ep, err := h.router.Pick()
		if err != nil {
			lastErr = err
			continue
		}

		dialCtx, dialCancel := context.WithTimeout(ctx, h.cfg.ConnectTimeoutDuration())
		dialer := &net.Dialer{LocalAddr: h.localAddr()}
		conn, err := dialer.DialContext(dialCtx, "tcp", ep.Addr)
		dialCancel()

		if err != nil {
			tripped := ep.RecordFailure(h.cfg.CircuitBreakerThreshold, h.cfg.CircuitBreakerCooldown())
			if tripped {
				clog.Warn("circuit breaker tripped for %s after %d failures", ep.Addr, h.cfg.CircuitBreakerThreshold)
			}
			lastErr = fmt.Errorf("dial %s: %w", ep.Addr, err)
			clog.Debug("dial %s failed (attempt %d/%d): %v", ep.Addr, attempt+1, maxAttempts, err)
			continue
		}
		return conn, ep, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("all %d dial attempts exhausted", maxAttempts)
	}
	return nil, nil, lastErr
}

func (h *Handler) localAddr() *net.TCPAddr {
	if h.cfg.InterfaceIP == "" {
		return nil
	}
	return &net.TCPAddr{IP: net.ParseIP(h.cfg.InterfaceIP)}
}

func applyTCPOpts(conn net.Conn) {
	tc, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}
	tc.SetNoDelay(true)
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(11 * time.Second)
	tc.SetReadBuffer(65536)
	tc.SetWriteBuffer(65536)
}
