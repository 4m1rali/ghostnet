package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const latencyWindowSize = 1024

type latencyWindow struct {
	mu      sync.Mutex
	samples []int64
	pos     int
	full    bool
}

func newLatencyWindow() *latencyWindow {
	return &latencyWindow{samples: make([]int64, latencyWindowSize)}
}

func (w *latencyWindow) record(ms int64) {
	w.mu.Lock()
	w.samples[w.pos] = ms
	w.pos = (w.pos + 1) % latencyWindowSize
	if w.pos == 0 {
		w.full = true
	}
	w.mu.Unlock()
}

func (w *latencyWindow) percentiles() (p50, p95, p99 float64) {
	w.mu.Lock()
	n := latencyWindowSize
	if !w.full {
		n = w.pos
	}
	if n == 0 {
		w.mu.Unlock()
		return 0, 0, 0
	}
	tmp := make([]int64, n)
	copy(tmp, w.samples[:n])
	w.mu.Unlock()

	sort.Slice(tmp, func(i, j int) bool { return tmp[i] < tmp[j] })
	p50 = float64(tmp[int(float64(n)*0.50)])
	p95 = float64(tmp[int(float64(n)*0.95)])
	p99 = float64(tmp[min(int(float64(n)*0.99), n-1)])
	return
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type strategyCounters struct {
	ok   atomic.Int64
	fail atomic.Int64
}

type Collector struct {
	activeConns atomic.Int64
	totalConns  atomic.Int64
	failedConns atomic.Int64
	bypassOK    atomic.Int64
	bypassFail  atomic.Int64
	relayActive atomic.Int64
	bytesIn     atomic.Int64
	bytesOut    atomic.Int64

	latency *latencyWindow

	strategyMu sync.RWMutex
	strategies map[string]*strategyCounters

	sniMu  sync.Mutex
	sniMap map[string]int64

	startTime   time.Time
	promEnabled bool
	promAddr    string
}

type SNICount struct {
	SNI   string
	Count int64
}

type StrategyStats struct {
	Name string
	OK   int64
	Fail int64
}

type Snapshot struct {
	ActiveConns int64
	TotalConns  int64
	FailedConns int64
	BypassOK    int64
	BypassFail  int64
	RelayActive int64
	BytesIn     int64
	BytesOut    int64
	LatP50Ms    float64
	LatP95Ms    float64
	LatP99Ms    float64
	Uptime      time.Duration
	TopSNIs     []SNICount
	Strategies  []StrategyStats
}

func New(promEnabled bool, promAddr string) *Collector {
	return &Collector{
		sniMap:      make(map[string]int64),
		strategies:  make(map[string]*strategyCounters),
		latency:     newLatencyWindow(),
		startTime:   time.Now(),
		promEnabled: promEnabled,
		promAddr:    promAddr,
	}
}

func (c *Collector) StartPrometheus() error {
	if !c.promEnabled {
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", c.promHandler)
	go http.ListenAndServe(c.promAddr, mux)
	return nil
}

func (c *Collector) ConnOpened() {
	c.activeConns.Add(1)
	c.totalConns.Add(1)
}

func (c *Collector) ConnClosed()   { c.activeConns.Add(-1) }
func (c *Collector) RelayDone()    { c.relayActive.Add(-1) }
func (c *Collector) RelayStarted() { c.relayActive.Add(1) }
func (c *Collector) BypassOK()     { c.bypassOK.Add(1) }
func (c *Collector) BypassFailed() { c.bypassFail.Add(1) }

func (c *Collector) ConnFailed() {
	c.failedConns.Add(1)
	c.activeConns.Add(-1)
}

func (c *Collector) AddBytesIn(n int64)  { c.bytesIn.Add(n) }
func (c *Collector) AddBytesOut(n int64) { c.bytesOut.Add(n) }

func (c *Collector) RecordLatency(d time.Duration) {
	c.latency.record(d.Milliseconds())
}

func (c *Collector) RecordBypassStrategy(strategy string, ok bool) {
	c.strategyMu.RLock()
	sc, exists := c.strategies[strategy]
	c.strategyMu.RUnlock()

	if !exists {
		c.strategyMu.Lock()
		sc, exists = c.strategies[strategy]
		if !exists {
			sc = &strategyCounters{}
			c.strategies[strategy] = sc
		}
		c.strategyMu.Unlock()
	}

	if ok {
		sc.ok.Add(1)
	} else {
		sc.fail.Add(1)
	}
}

func (c *Collector) RecordSNI(sni string) {
	c.sniMu.Lock()
	c.sniMap[sni]++
	c.sniMu.Unlock()
}

func (c *Collector) TopSNIs(n int) []SNICount {
	c.sniMu.Lock()
	counts := make([]SNICount, 0, len(c.sniMap))
	for sni, cnt := range c.sniMap {
		counts = append(counts, SNICount{SNI: sni, Count: cnt})
	}
	c.sniMu.Unlock()
	sort.Slice(counts, func(i, j int) bool { return counts[i].Count > counts[j].Count })
	if n > 0 && len(counts) > n {
		counts = counts[:n]
	}
	return counts
}

func (c *Collector) strategyStats() []StrategyStats {
	c.strategyMu.RLock()
	out := make([]StrategyStats, 0, len(c.strategies))
	for name, sc := range c.strategies {
		out = append(out, StrategyStats{
			Name: name,
			OK:   sc.ok.Load(),
			Fail: sc.fail.Load(),
		})
	}
	c.strategyMu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (c *Collector) Snapshot() Snapshot {
	p50, p95, p99 := c.latency.percentiles()
	return Snapshot{
		ActiveConns: c.activeConns.Load(),
		TotalConns:  c.totalConns.Load(),
		FailedConns: c.failedConns.Load(),
		BypassOK:    c.bypassOK.Load(),
		BypassFail:  c.bypassFail.Load(),
		RelayActive: c.relayActive.Load(),
		BytesIn:     c.bytesIn.Load(),
		BytesOut:    c.bytesOut.Load(),
		LatP50Ms:    p50,
		LatP95Ms:    p95,
		LatP99Ms:    p99,
		Uptime:      time.Since(c.startTime),
		TopSNIs:     c.TopSNIs(10),
		Strategies:  c.strategyStats(),
	}
}

func (s Snapshot) String() string {
	return fmt.Sprintf(
		"uptime=%-10s  total=%-6d  active=%-4d  failed=%-4d  bypass_ok=%-6d  bypass_fail=%-4d  p50=%.0fms  p95=%.0fms  p99=%.0fms  in=%s  out=%s",
		fmtDur(s.Uptime),
		s.TotalConns, s.ActiveConns, s.FailedConns,
		s.BypassOK, s.BypassFail,
		s.LatP50Ms, s.LatP95Ms, s.LatP99Ms,
		fmtBytes(s.BytesIn), fmtBytes(s.BytesOut),
	)
}

func (c *Collector) ActiveConns() int64 { return c.activeConns.Load() }
func (c *Collector) TotalConns() int64  { return c.totalConns.Load() }

func (c *Collector) promHandler(w http.ResponseWriter, r *http.Request) {
	s := c.Snapshot()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "ghostnet_active_connections %d\n", s.ActiveConns)
	fmt.Fprintf(w, "ghostnet_total_connections_total %d\n", s.TotalConns)
	fmt.Fprintf(w, "ghostnet_failed_connections_total %d\n", s.FailedConns)
	fmt.Fprintf(w, "ghostnet_bypass_ok_total %d\n", s.BypassOK)
	fmt.Fprintf(w, "ghostnet_bypass_fail_total %d\n", s.BypassFail)
	fmt.Fprintf(w, "ghostnet_bytes_in_total %d\n", s.BytesIn)
	fmt.Fprintf(w, "ghostnet_bytes_out_total %d\n", s.BytesOut)
	fmt.Fprintf(w, "ghostnet_latency_p50_ms %.2f\n", s.LatP50Ms)
	fmt.Fprintf(w, "ghostnet_latency_p95_ms %.2f\n", s.LatP95Ms)
	fmt.Fprintf(w, "ghostnet_latency_p99_ms %.2f\n", s.LatP99Ms)
	fmt.Fprintf(w, "ghostnet_uptime_seconds %.0f\n", s.Uptime.Seconds())
	for _, sc := range s.TopSNIs {
		fmt.Fprintf(w, "ghostnet_sni_connections_total{sni=%q} %d\n", sc.SNI, sc.Count)
	}
	for _, st := range s.Strategies {
		fmt.Fprintf(w, "ghostnet_bypass_strategy_ok_total{strategy=%q} %d\n", st.Name, st.OK)
		fmt.Fprintf(w, "ghostnet_bypass_strategy_fail_total{strategy=%q} %d\n", st.Name, st.Fail)
	}
}

func fmtBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2fGB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.2fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.2fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func fmtDur(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
