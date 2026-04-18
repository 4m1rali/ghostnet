package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/4m1rali/ghostnet/internal/bypass"
	"github.com/4m1rali/ghostnet/internal/config"
	"github.com/4m1rali/ghostnet/internal/domain"
	"github.com/4m1rali/ghostnet/internal/log"
	"github.com/4m1rali/ghostnet/internal/metrics"
	"github.com/4m1rali/ghostnet/internal/network"
	"github.com/4m1rali/ghostnet/internal/preflight"
	"github.com/4m1rali/ghostnet/internal/proxy"
	"github.com/4m1rali/ghostnet/internal/routing"
	"github.com/4m1rali/ghostnet/internal/tuner"
	tlspkg "github.com/4m1rali/ghostnet/internal/tls"
)

const (
	version = "2.1.0"

	banner = "\033[36m" + `
  ██████╗  ██╗  ██╗ ██████╗ ███████╗████████╗███╗   ██╗███████╗████████╗
 ██╔════╝  ██║  ██║██╔═══██╗██╔════╝╚══██╔══╝████╗  ██║██╔════╝╚══██╔══╝
 ██║  ███╗ ███████║██║   ██║███████╗   ██║   ██╔██╗ ██║█████╗     ██║   
 ██║   ██║ ██╔══██║██║   ██║╚════██║   ██║   ██║╚██╗██║██╔══╝     ██║   
 ╚██████╔╝ ██║  ██║╚██████╔╝███████║   ██║   ██║ ╚████║███████╗   ██║   
  ╚═════╝  ╚═╝  ╚═╝ ╚═════╝ ╚══════╝   ╚═╝   ╚═╝  ╚═══╝╚══════╝   ╚═╝   
  				████  G H O S T N E T  ████
` + "\033[0m" + "\033[90m" + `
  ┌─────────────────────────────────────────────────────────────┐
  │  GhostNet  ·  DPI Bypass Proxy  ·  v` + version + `                  │
  │  High-performance  ·  Stealth  ·  Linux  ·  Go             │
  └─────────────────────────────────────────────────────────────┘
` + "\033[0m"
)

var (
	cfgFile string
	dbg     bool
	stealth bool
)

func main() {
	root := &cobra.Command{
		Use:          "ghostnet",
		Short:        "GhostNet — DPI bypass proxy",
		Long:         banner,
		SilenceUsage: true,
	}

	root.PersistentFlags().StringVarP(&cfgFile, "config", "c", "config.json", "config file (JSON or YAML)")
	root.PersistentFlags().BoolVar(&dbg, "debug", false, "enable debug logging")
	root.PersistentFlags().BoolVar(&stealth, "stealth", false, "minimal logging (stealth mode)")

	root.AddCommand(
		tuneCmd(),
		scanCmd(),
		setupCmd(),
		runCmd(),
		versionCmd(),
		benchCmd(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the proxy — reads config.json, no scan, no tuning",
		Long: `Starts the proxy using config.json as-is.
No SNI scan, no kernel tuning. Config must already have connect_ip and fake_sni set.
Use 'setup' for first-time automatic configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return startProxy(false)
		},
	}
}

func tuneCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "tune",
		Short: "Optimize Linux kernel settings for high-performance operation",
		Long: `Applies sysctl and ulimit settings for maximum connection capacity.
Requires root (sudo). Settings are persisted to /etc/sysctl.d/99-ghostnet.conf.

Tunes: file descriptors, TCP buffers, connection backlog, TIME_WAIT reuse,
       BBR congestion control, rp_filter (required for raw socket injection).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTune(dryRun)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be changed without applying")
	return cmd
}

func scanCmd() *cobra.Command {
	var (
		sniFile   string
		writeBack bool
		port      int
		timeout   int
	)
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan SNI domains and find the best reachable one",
		Long: `Probes SNI domains concurrently and ranks them by latency.

Sources (in order of priority):
  --file sni.txt   one domain per line
  built-in list    49 known hcaptcha + vercel domains

Use -w to write the best SNI and IP directly into config.json.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(sniFile, writeBack, port, time.Duration(timeout)*time.Millisecond)
		},
	}
	cmd.Flags().StringVarP(&sniFile, "file", "f", "", "file with domains to scan (one per line)")
	cmd.Flags().BoolVarP(&writeBack, "write", "w", false, "write best SNI and IP to config file")
	cmd.Flags().IntVarP(&port, "port", "p", 443, "port to probe")
	cmd.Flags().IntVarP(&timeout, "timeout", "t", 4000, "probe timeout per domain in milliseconds")
	return cmd
}

func setupCmd() *cobra.Command {
	var listenPort int
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Full automatic setup: tune → scan → create config → run",
		Long: `Runs the complete setup sequence in one command:

  1. Apply Linux kernel tuning (requires root)
  2. Scan all known SNI domains and pick the fastest
  3. Generate config.json with the best SNI and IP
  4. Start the proxy

Run this once on a fresh server. After setup, use 'run' for subsequent starts.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup(listenPort)
		},
	}
	cmd.Flags().IntVarP(&listenPort, "port", "p", 40443, "listen port for the proxy")
	return cmd
}

func startProxy(withPreFlight bool) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	if dbg {
		cfg.LogLevel = config.LogDebug
	} else if stealth {
		cfg.LogLevel = config.LogStealth
	}

	if cfg.InterfaceIP == "" {
		if ip, err := network.LocalIP(); err == nil {
			cfg.InterfaceIP = ip
		}
	}

	logger, err := log.New(log.ParseLevel(string(cfg.LogLevel)), cfg.LogFile)
	if err != nil {
		return fmt.Errorf("logger: %w", err)
	}
	defer logger.Close()
	logger.SetNoColor(cfg.LogNoColor)

	if !stealth {
		fmt.Print(banner)
	}

	logger.Startup("server", "GhostNet v"+version)
	logger.Startup("go", runtime.Version())
	logger.Startup("os/arch", runtime.GOOS+"/"+runtime.GOARCH)
	logger.Startup("cpus", fmt.Sprintf("%d", runtime.NumCPU()))

	if withPreFlight && (cfg.ConnectIP == "" || cfg.FakeSNI == "" || cfg.FakeSNI == "hcaptcha.com") {
		logger.Info("running pre-flight SNI check (%d domains)...", len(preflight.KnownSNIDomains))
		report := preflight.Run(cfg.ConnectPort, 3*time.Second, func(line string) {
			logger.Debug("%s", line)
		})
		if len(report.Reachable) > 0 {
			logger.Info("pre-flight: %d/%d reachable  best=%s  ip=%s  latency=%s",
				len(report.Reachable), len(preflight.KnownSNIDomains),
				report.Best.Domain, report.Best.IP,
				report.Best.Latency.Round(time.Millisecond))
			if cfg.FakeSNI == "" || cfg.FakeSNI == "hcaptcha.com" {
				cfg.FakeSNI = report.Best.Domain
			}
			pool := make([]string, 0, len(report.Reachable))
			for _, r := range report.Reachable {
				pool = append(pool, r.Domain)
			}
			cfg.FakeSNIPool = pool
			if cfg.ConnectIP == "" && report.Best.IP != "" {
				cfg.ConnectIP = report.Best.IP
				cfg.Endpoints = []config.Endpoint{{IP: cfg.ConnectIP, Port: cfg.ConnectPort, Weight: 1}}
			}
			if len(cfg.DomainPool) == 0 {
				cfg.DomainPool = pool
			}
		} else {
			logger.Warn("pre-flight: no reachable domains found — using config values")
		}
	}

	logger.Startup("listen", fmt.Sprintf("%s:%d", cfg.ListenHost, cfg.ListenPort))
	logger.Startup("connect", fmt.Sprintf("%s:%d", cfg.ConnectIP, cfg.ConnectPort))
	logger.Startup("fake_sni", cfg.FakeSNI)
	logger.Startup("bypass", string(cfg.BypassMethod))
	logger.Startup("profile", string(cfg.BrowserProfile))

	m := metrics.New(cfg.PrometheusEnabled, cfg.PrometheusAddr)
	if cfg.PrometheusEnabled {
		if err := m.StartPrometheus(); err != nil {
			logger.Warn("prometheus start failed: %v", err)
		} else {
			logger.Startup("prometheus", cfg.PrometheusAddr+"/metrics")
		}
	}

	if cfg.PprofEnabled {
		go func() {
			logger.Startup("pprof", cfg.PprofAddr+"/debug/pprof")
			http.ListenAndServe(cfg.PprofAddr, nil)
		}()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := domain.NewPool(cfg.DomainPool, cfg.ConnectPort, cfg.DomainProbeInterval)
	if cfg.DomainProbeEnabled {
		pool.StartProbing(ctx)
		logger.Startup("domain_pool", fmt.Sprintf("%d domains", len(cfg.DomainPool)))
	}

	router := routing.New(cfg, logger)
	router.StartHealthChecks(ctx)
	logger.Startup("router", router.Status())

	adaptive, err := bypass.NewAdaptiveBypass()
	if err != nil {
		logger.Warn("raw socket bypass unavailable: %v — relay-only mode", err)
		adaptive = nil
	} else {
		adaptive.SetRecorder(m)
		defer adaptive.Close()
		logger.Startup("bypass_engine", "adaptive (wrong_seq/fragment/desync)")
	}

	if cfg.StatsInterval > 0 {
		go statsLoop(ctx, m, router, logger, time.Duration(cfg.StatsInterval)*time.Second)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		for sig := range sigCh {
			switch sig {
			case syscall.SIGHUP:
				logger.Info("SIGHUP — reloading config from %s", cfgFile)
				newCfg, err := config.Load(cfgFile)
				if err != nil {
					logger.Error("config reload failed: %v", err)
					continue
				}
				safe := newCfg.SafeCopy()
				cfg.Lock()
				oldLevel := cfg.LogLevel
				*cfg = safe
				cfg.Unlock()
				if cfg.LogLevel != oldLevel {
					logger.SetLevel(log.ParseLevel(string(cfg.LogLevel)))
					logger.Info("log level changed to %s", cfg.LogLevel)
				}
				logger.Info("config reloaded")
			default:
				logger.Info("signal %s — shutting down", sig)
				cancel()
				return
			}
		}
	}()

	srv := proxy.NewServer(cfg, adaptive, pool, router, m, logger)
	return srv.ListenAndServe(ctx)
}

func runTune(dryRun bool) error {
	fmt.Print(banner)
	if !tuner.IsRoot() && !dryRun {
		return fmt.Errorf("tune requires root — run with: sudo ./ghostnet tune")
	}

	fmt.Printf("\033[36m\nApplying Linux kernel optimizations...\033[0m\n\n")
	fmt.Printf("  %-45s  %-15s  %s\n", "SETTING", "BEFORE", "AFTER")
	fmt.Printf("  %-45s  %-15s  %s\n", strings.Repeat("-", 45), strings.Repeat("-", 15), strings.Repeat("-", 15))

	results, err := tuner.Apply(dryRun)
	changed := 0
	for _, r := range results {
		if r.Error != "" {
			fmt.Printf("  \033[31m✗\033[0m %-44s  %-15s  ERROR: %s\n", r.Key, r.Before, r.Error)
			continue
		}
		marker := "  "
		if r.Changed {
			marker = "  \033[32m✓\033[0m"
			changed++
		} else {
			marker = "  \033[90m·\033[0m"
		}
		fmt.Printf("%s %-44s  %-15s  %s\n", marker, r.Key, truncate(r.Before, 15), truncate(r.After, 15))
	}

	ulimit := tuner.CurrentUlimit()
	fmt.Printf("\n\033[36mSummary:\033[0m\n")
	fmt.Printf("  settings changed : %d / %d\n", changed, len(results))
	if ulimit > 0 {
		fmt.Printf("  current ulimit   : %d\n", ulimit)
	}

	if err != nil {
		fmt.Printf("  \033[31mwarning: %v\033[0m\n", err)
	}

	if dryRun {
		fmt.Printf("\n  \033[33m[dry-run] no changes applied — run without --dry-run to apply\033[0m\n")
	} else {
		fmt.Printf("\n  \033[32m✓ Kernel settings applied and persisted to /etc/sysctl.d/99-ghostnet.conf\033[0m\n")
		fmt.Printf("  \033[32m✓ ulimit settings written to /etc/security/limits.d/99-ghostnet.conf\033[0m\n")
	}
	return nil
}

func runScan(sniFile string, writeBack bool, port int, timeout time.Duration) error {
	fmt.Print(banner)

	var domains []string
	if sniFile != "" {
		loaded, err := loadDomainsFromFile(sniFile)
		if err != nil {
			return fmt.Errorf("scan: read file %q: %w", sniFile, err)
		}
		domains = loaded
		fmt.Printf("\033[36m\nScanning %d domains from %s...\033[0m\n\n", len(domains), sniFile)
	} else {
		domains = preflight.KnownSNIDomains
		fmt.Printf("\033[36m\nScanning %d built-in SNI domains (port %d)...\033[0m\n\n", len(domains), port)
	}

	fmt.Printf("  %-3s  %-45s  %-10s  %s\n", "#", "DOMAIN", "LATENCY", "IP")
	fmt.Printf("  %-3s  %-45s  %-10s  %s\n",
		"---", strings.Repeat("-", 45), "----------", "---------------")

	report := preflight.RunDomains(domains, port, timeout, func(line string) {})

	for i, r := range report.Reachable {
		marker := "  "
		if i == 0 {
			marker = "\033[32m★ \033[0m"
		}
		fmt.Printf("%s%-3d  %-45s  %-10s  %s\n",
			marker, i+1, r.Domain, r.Latency.Round(time.Millisecond), r.IP)
	}

	if len(report.Dead) > 0 {
		fmt.Printf("\n\033[90m  Unreachable (%d):\033[0m\n", len(report.Dead))
		for _, r := range report.Dead {
			fmt.Printf("\033[90m  ✗  %s\033[0m\n", r.Domain)
		}
	}

	fmt.Printf("\n\033[36mSummary:\033[0m\n")
	fmt.Printf("  total     : %d\n", len(domains))
	fmt.Printf("  reachable : %d\n", len(report.Reachable))
	fmt.Printf("  dead      : %d\n", len(report.Dead))

	if len(report.Reachable) == 0 {
		fmt.Printf("\n  \033[31mNo reachable domains found.\033[0m\n")
		return nil
	}

	fmt.Printf("  best SNI  : \033[32m%s\033[0m\n", report.Best.Domain)
	fmt.Printf("  best IP   : \033[32m%s\033[0m\n", report.Best.IP)
	fmt.Printf("  latency   : %s\n\n", report.Best.Latency.Round(time.Millisecond))

	if writeBack {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			cfg = config.Default()
		}
		cfg.FakeSNI = report.Best.Domain
		pool := make([]string, 0, len(report.Reachable))
		for _, r := range report.Reachable {
			pool = append(pool, r.Domain)
		}
		cfg.FakeSNIPool = pool
		cfg.DomainPool = pool
		if report.Best.IP != "" {
			cfg.ConnectIP = report.Best.IP
			cfg.Endpoints = []config.Endpoint{{IP: cfg.ConnectIP, Port: cfg.ConnectPort, Weight: 1}}
		}
		if err := writeCfg(cfgFile, cfg); err != nil {
			return fmt.Errorf("scan: write config: %w", err)
		}
		fmt.Printf("\033[32m✓ Config updated: %s\033[0m\n", cfgFile)
		fmt.Printf("  fake_sni   = %s\n", cfg.FakeSNI)
		fmt.Printf("  connect_ip = %s\n", cfg.ConnectIP)
		fmt.Printf("  pool size  = %d domains\n", len(pool))
	} else {
		fmt.Printf("  To write results to config:\n")
		fmt.Printf("  \033[33m./ghostnet scan -w -c config.json\033[0m\n")
	}
	return nil
}

func runSetup(listenPort int) error {
	fmt.Print(banner)
	fmt.Printf("\033[36m\n╔══════════════════════════════════════════════════╗\033[0m\n")
	fmt.Printf("\033[36m║         GhostNet Automatic Setup                 ║\033[0m\n")
	fmt.Printf("\033[36m╚══════════════════════════════════════════════════╝\033[0m\n\n")

	fmt.Printf("\033[36m[1/4] Applying Linux kernel optimizations...\033[0m\n")
	if tuner.IsRoot() {
		results, err := tuner.Apply(false)
		changed := 0
		for _, r := range results {
			if r.Changed {
				changed++
			}
		}
		if err != nil {
			fmt.Printf("  \033[33mwarning: %v\033[0m\n", err)
		} else {
			fmt.Printf("  \033[32m✓ %d settings applied\033[0m\n", changed)
		}
	} else {
		fmt.Printf("  \033[33m⚠ not root — skipping kernel tuning (run with sudo for full optimization)\033[0m\n")
	}

	fmt.Printf("\n\033[36m[2/4] Scanning SNI domains...\033[0m\n")
	report := preflight.Run(443, 4*time.Second, func(line string) {})

	if len(report.Reachable) == 0 {
		return fmt.Errorf("setup: no reachable SNI domains found — check network connectivity")
	}

	fmt.Printf("  \033[32m✓ %d/%d domains reachable\033[0m\n", len(report.Reachable), len(preflight.KnownSNIDomains))
	fmt.Printf("  best SNI : \033[32m%s\033[0m  (%s)\n", report.Best.Domain, report.Best.Latency.Round(time.Millisecond))
	fmt.Printf("  best IP  : \033[32m%s\033[0m\n", report.Best.IP)

	fmt.Printf("\n\033[36m[3/4] Creating config.json...\033[0m\n")
	cfg := config.Default()
	cfg.ListenPort = listenPort
	cfg.FakeSNI = report.Best.Domain
	cfg.ConnectIP = report.Best.IP
	cfg.ConnectPort = 443
	cfg.Endpoints = []config.Endpoint{{IP: cfg.ConnectIP, Port: cfg.ConnectPort, Weight: 1}}
	pool := make([]string, 0, len(report.Reachable))
	for _, r := range report.Reachable {
		pool = append(pool, r.Domain)
	}
	cfg.FakeSNIPool = pool
	cfg.DomainPool = pool

	if err := writeCfg(cfgFile, cfg); err != nil {
		return fmt.Errorf("setup: write config: %w", err)
	}
	fmt.Printf("  \033[32m✓ config.json created\033[0m\n")
	fmt.Printf("  listen     : 0.0.0.0:%d\n", listenPort)
	fmt.Printf("  connect_ip : %s\n", cfg.ConnectIP)
	fmt.Printf("  fake_sni   : %s\n", cfg.FakeSNI)
	fmt.Printf("  pool size  : %d domains\n", len(pool))

	fmt.Printf("\n\033[36m[4/4] Starting proxy...\033[0m\n\n")
	return startProxy(false)
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("GhostNet v%s  Go %s  %s/%s\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		},
	}
}

func benchCmd() *cobra.Command {
	var n, dur int
	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Benchmark ClientHello build throughput",
		RunE: func(cmd *cobra.Command, args []string) error {
			deadline := time.Now().Add(time.Duration(dur) * time.Second)
			count := 0
			start := time.Now()
			b := tlspkg.NewBuilder("random")
			for time.Now().Before(deadline) {
				for i := 0; i < n; i++ {
					if _, err := b.Build("hcaptcha.com"); err == nil {
						count++
					}
				}
			}
			elapsed := time.Since(start)
			fmt.Printf("built %d ClientHellos in %v — %.0f/sec\n", count, elapsed.Round(time.Millisecond), float64(count)/elapsed.Seconds())
			return nil
		},
	}
	cmd.Flags().IntVarP(&n, "count", "n", 1000, "builds per iteration")
	cmd.Flags().IntVarP(&dur, "duration", "d", 5, "duration in seconds")
	return cmd
}

func loadDomainsFromFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var domains []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		domains = append(domains, line)
	}
	return domains, sc.Err()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func writeCfg(path string, cfg *config.Config) error {
	safe := cfg.SafeCopy()

	type cfgOut struct {
		ListenHost              string                `json:"listen_host"`
		ListenPort              int                   `json:"listen_port"`
		ConnectIP               string                `json:"connect_ip"`
		ConnectPort             int                   `json:"connect_port"`
		BypassMethod            string                `json:"bypass_method"`
		FakeSNI                 string                `json:"fake_sni"`
		FakeSNIPool             []string              `json:"fake_sni_pool"`
		BrowserProfile          string                `json:"browser_profile"`
		TTLSpoof                bool                  `json:"ttl_spoof"`
		FragmentSize            int                   `json:"fragment_size"`
		FakeDelayMs             float64               `json:"fake_delay_ms"`
		MinDelayMs              float64               `json:"min_delay_ms"`
		MaxDelayMs              float64               `json:"max_delay_ms"`
		BypassTimeout           float64               `json:"bypass_timeout"`
		ConnectTimeout          float64               `json:"connect_timeout"`
		ReadTimeout             float64               `json:"read_timeout"`
		WriteTimeout            float64               `json:"write_timeout"`
		JitterMs                float64               `json:"jitter_ms"`
		RetryLimit              int                   `json:"retry_limit"`
		RetryBaseMs             float64               `json:"retry_base_ms"`
		RetryMaxMs              float64               `json:"retry_max_ms"`
		CircuitBreakerThreshold int                   `json:"circuit_breaker_threshold"`
		CircuitBreakerCooldown  float64               `json:"circuit_breaker_cooldown"`
		RecvBuffer              int                   `json:"recv_buffer"`
		MaxConnections          int                   `json:"max_connections"`
		IdleTimeout             int                   `json:"idle_timeout"`
		RateLimit               int                   `json:"rate_limit"`
		WorkerPoolSize          int                   `json:"worker_pool_size"`
		LogLevel                string                `json:"log_level"`
		LogClientSNI            bool                  `json:"log_client_sni"`
		LogFile                 string                `json:"log_file"`
		LogNoColor              bool                  `json:"log_no_color"`
		StatsInterval           int                   `json:"stats_interval"`
		PrometheusEnabled       bool                  `json:"prometheus_enabled"`
		PrometheusAddr          string                `json:"prometheus_addr"`
		PprofEnabled            bool                  `json:"pprof_enabled"`
		PprofAddr               string                `json:"pprof_addr"`
		DomainProbeEnabled      bool                  `json:"domain_probe_enabled"`
		DomainProbeInterval     int                   `json:"domain_probe_interval"`
		DomainPool              []string              `json:"domain_pool"`
		Plugins                 []config.PluginConfig `json:"plugins"`
	}

	o := cfgOut{
		ListenHost:              safe.ListenHost,
		ListenPort:              safe.ListenPort,
		ConnectIP:               safe.ConnectIP,
		ConnectPort:             safe.ConnectPort,
		BypassMethod:            string(safe.BypassMethod),
		FakeSNI:                 safe.FakeSNI,
		FakeSNIPool:             safe.FakeSNIPool,
		BrowserProfile:          string(safe.BrowserProfile),
		TTLSpoof:                safe.TTLSpoof,
		FragmentSize:            safe.FragmentSize,
		FakeDelayMs:             safe.FakeDelayMs,
		MinDelayMs:              safe.MinDelayMs,
		MaxDelayMs:              safe.MaxDelayMs,
		BypassTimeout:           safe.BypassTimeout,
		ConnectTimeout:          safe.ConnectTimeout,
		ReadTimeout:             safe.ReadTimeout,
		WriteTimeout:            safe.WriteTimeout,
		JitterMs:                safe.JitterMs,
		RetryLimit:              safe.RetryLimit,
		RetryBaseMs:             safe.RetryBaseMs,
		RetryMaxMs:              safe.RetryMaxMs,
		CircuitBreakerThreshold: safe.CircuitBreakerThreshold,
		CircuitBreakerCooldown:  safe.CircuitBreakerCooldownS,
		RecvBuffer:              safe.RecvBuffer,
		MaxConnections:          safe.MaxConnections,
		IdleTimeout:             safe.IdleTimeout,
		RateLimit:               safe.RateLimit,
		WorkerPoolSize:          safe.WorkerPoolSize,
		LogLevel:                string(safe.LogLevel),
		LogClientSNI:            safe.LogClientSNI,
		LogFile:                 safe.LogFile,
		LogNoColor:              safe.LogNoColor,
		StatsInterval:           safe.StatsInterval,
		PrometheusEnabled:       safe.PrometheusEnabled,
		PrometheusAddr:          safe.PrometheusAddr,
		PprofEnabled:            safe.PprofEnabled,
		PprofAddr:               safe.PprofAddr,
		DomainProbeEnabled:      safe.DomainProbeEnabled,
		DomainProbeInterval:     safe.DomainProbeInterval,
		DomainPool:              safe.DomainPool,
		Plugins:                 safe.Plugins,
	}
	if o.Plugins == nil {
		o.Plugins = []config.PluginConfig{}
	}

	data, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return fmt.Errorf("writeCfg: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func writeDefaultConfig(path string) error {
	cfg := config.Default()
	data := fmt.Sprintf(`{
  "listen_host":                "%s",
  "listen_port":                %d,
  "connect_ip":                 "YOUR_SERVER_IP",
  "connect_port":               %d,
  "endpoints":                  [],
  "bypass_method":              "%s",
  "fake_sni":                   "%s",
  "fake_sni_pool":              ["hcaptcha.com","cloudflare.com","ajax.googleapis.com"],
  "browser_profile":            "%s",
  "ttl_spoof":                  %v,
  "fragment_size":              %d,
  "fake_delay_ms":              %.1f,
  "min_delay_ms":               %.1f,
  "max_delay_ms":               %.1f,
  "bypass_timeout":             %.1f,
  "connect_timeout":            %.1f,
  "read_timeout":               %.1f,
  "write_timeout":              %.1f,
  "jitter_ms":                  %.1f,
  "retry_limit":                %d,
  "retry_base_ms":              %.1f,
  "retry_max_ms":               %.1f,
  "circuit_breaker_threshold":  %d,
  "circuit_breaker_cooldown":   %.1f,
  "recv_buffer":                %d,
  "max_connections":            %d,
  "idle_timeout":               %d,
  "rate_limit":                 %d,
  "worker_pool_size":           %d,
  "log_level":                  "%s",
  "log_client_sni":             %v,
  "log_file":                   "",
  "log_no_color":               false,
  "stats_interval":             %d,
  "prometheus_enabled":         %v,
  "prometheus_addr":            "%s",
  "pprof_enabled":              %v,
  "pprof_addr":                 "%s",
  "domain_probe_enabled":       %v,
  "domain_probe_interval":      %d,
  "domain_pool":                ["hcaptcha.com","cloudflare.com","ajax.googleapis.com","fonts.googleapis.com","www.gstatic.com"],
  "plugins":                    []
}
`,
		cfg.ListenHost, cfg.ListenPort, cfg.ConnectPort,
		cfg.BypassMethod, cfg.FakeSNI, cfg.BrowserProfile,
		cfg.TTLSpoof, cfg.FragmentSize,
		cfg.FakeDelayMs, cfg.MinDelayMs, cfg.MaxDelayMs,
		cfg.BypassTimeout, cfg.ConnectTimeout, cfg.ReadTimeout, cfg.WriteTimeout, cfg.JitterMs,
		cfg.RetryLimit, cfg.RetryBaseMs, cfg.RetryMaxMs,
		cfg.CircuitBreakerThreshold, cfg.CircuitBreakerCooldownS,
		cfg.RecvBuffer, cfg.MaxConnections, cfg.IdleTimeout, cfg.RateLimit, cfg.WorkerPoolSize,
		cfg.LogLevel, cfg.LogClientSNI,
		cfg.StatsInterval,
		cfg.PrometheusEnabled, cfg.PrometheusAddr,
		cfg.PprofEnabled, cfg.PprofAddr,
		cfg.DomainProbeEnabled, cfg.DomainProbeInterval,
	)
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		return fmt.Errorf("gen-config: %w", err)
	}
	fmt.Printf("config written to %s\nset connect_ip before running\n", path)
	return nil
}

func statsLoop(ctx context.Context, m *metrics.Collector, r *routing.Router, logger *log.Logger, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s := m.Snapshot()
			logger.Stats(s.String())

			for _, st := range s.Strategies {
				total := st.OK + st.Fail
				if total == 0 {
					continue
				}
				rate := float64(st.OK) / float64(total) * 100
				logger.Info("  strategy=%-18s ok=%-5d fail=%-5d rate=%5.1f%%",
					st.Name, st.OK, st.Fail, rate)
			}

			logger.Info("  router: %s", r.Status())

			if len(s.TopSNIs) > 0 {
				top := ""
				for i, sc := range s.TopSNIs {
					if i >= 5 {
						break
					}
					if i > 0 {
						top += "  "
					}
					top += fmt.Sprintf("%s(%d)", sc.SNI, sc.Count)
				}
				logger.Info("  top SNIs: %s", top)
			}
		}
	}
}
