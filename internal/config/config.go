package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type BypassMethod string

const (
	BypassWrongSeq   BypassMethod = "wrong_seq"
	BypassFragment   BypassMethod = "fragment"
	BypassDesync     BypassMethod = "desync"
	BypassMultiLayer BypassMethod = "multi_layer"
	BypassAdaptive   BypassMethod = "adaptive"
)

type LogLevel string

const (
	LogDebug   LogLevel = "debug"
	LogInfo    LogLevel = "info"
	LogStealth LogLevel = "stealth"
	LogWarn    LogLevel = "warn"
	LogError   LogLevel = "error"
)

type BrowserProfile string

const (
	ProfileChrome  BrowserProfile = "chrome"
	ProfileFirefox BrowserProfile = "firefox"
	ProfileSafari  BrowserProfile = "safari"
	ProfileEdge    BrowserProfile = "edge"
	ProfileRandom  BrowserProfile = "random"
)

type Endpoint struct {
	IP     string `json:"ip"     yaml:"ip"`
	Port   int    `json:"port"   yaml:"port"`
	Weight int    `json:"weight" yaml:"weight"`
}

type PluginConfig struct {
	Name    string            `json:"name"    yaml:"name"`
	Path    string            `json:"path"    yaml:"path"`
	Options map[string]string `json:"options" yaml:"options"`
}

type Config struct {
	ListenHost string `json:"listen_host" yaml:"listen_host"`
	ListenPort int    `json:"listen_port" yaml:"listen_port"`

	ConnectIP   string     `json:"connect_ip"   yaml:"connect_ip"`
	ConnectPort int        `json:"connect_port" yaml:"connect_port"`
	Endpoints   []Endpoint `json:"endpoints"    yaml:"endpoints"`

	BypassMethod   BypassMethod   `json:"bypass_method"   yaml:"bypass_method"`
	FakeSNI        string         `json:"fake_sni"        yaml:"fake_sni"`
	FakeSNIPool    []string       `json:"fake_sni_pool"   yaml:"fake_sni_pool"`
	BrowserProfile BrowserProfile `json:"browser_profile" yaml:"browser_profile"`
	TTLSpoof       bool           `json:"ttl_spoof"       yaml:"ttl_spoof"`
	FragmentSize   int            `json:"fragment_size"   yaml:"fragment_size"`

	FakeDelayMs    float64 `json:"fake_delay_ms"    yaml:"fake_delay_ms"`
	MinDelayMs     float64 `json:"min_delay_ms"     yaml:"min_delay_ms"`
	MaxDelayMs     float64 `json:"max_delay_ms"     yaml:"max_delay_ms"`
	BypassTimeout  float64 `json:"bypass_timeout"   yaml:"bypass_timeout"`
	ConnectTimeout float64 `json:"connect_timeout"  yaml:"connect_timeout"`
	ReadTimeout    float64 `json:"read_timeout"     yaml:"read_timeout"`
	WriteTimeout   float64 `json:"write_timeout"    yaml:"write_timeout"`
	JitterMs       float64 `json:"jitter_ms"        yaml:"jitter_ms"`

	RetryLimit  int     `json:"retry_limit"   yaml:"retry_limit"`
	RetryBaseMs float64 `json:"retry_base_ms" yaml:"retry_base_ms"`
	RetryMaxMs  float64 `json:"retry_max_ms"  yaml:"retry_max_ms"`

	CircuitBreakerThreshold int     `json:"circuit_breaker_threshold" yaml:"circuit_breaker_threshold"`
	CircuitBreakerCooldownS float64 `json:"circuit_breaker_cooldown"  yaml:"circuit_breaker_cooldown"`

	RecvBuffer     int `json:"recv_buffer"      yaml:"recv_buffer"`
	MaxConnections int `json:"max_connections"  yaml:"max_connections"`
	IdleTimeout    int `json:"idle_timeout"     yaml:"idle_timeout"`
	RateLimit      int `json:"rate_limit"       yaml:"rate_limit"`
	WorkerPoolSize int `json:"worker_pool_size" yaml:"worker_pool_size"`

	LogLevel     LogLevel `json:"log_level"      yaml:"log_level"`
	LogClientSNI bool     `json:"log_client_sni" yaml:"log_client_sni"`
	LogFile      string   `json:"log_file"       yaml:"log_file"`
	LogNoColor   bool     `json:"log_no_color"   yaml:"log_no_color"`
	StatsInterval int     `json:"stats_interval" yaml:"stats_interval"`

	PrometheusEnabled bool   `json:"prometheus_enabled" yaml:"prometheus_enabled"`
	PrometheusAddr    string `json:"prometheus_addr"    yaml:"prometheus_addr"`
	PprofEnabled      bool   `json:"pprof_enabled"      yaml:"pprof_enabled"`
	PprofAddr         string `json:"pprof_addr"         yaml:"pprof_addr"`

	InterfaceIP string `json:"interface_ip" yaml:"interface_ip"`

	DomainProbeEnabled  bool     `json:"domain_probe_enabled"  yaml:"domain_probe_enabled"`
	DomainProbeInterval int      `json:"domain_probe_interval" yaml:"domain_probe_interval"`
	DomainPool          []string `json:"domain_pool"           yaml:"domain_pool"`

	Plugins []PluginConfig `json:"plugins" yaml:"plugins"`

	mu sync.RWMutex
}

func Default() *Config {
	return &Config{
		ListenHost:              "0.0.0.0",
		ListenPort:              40443,
		ConnectPort:             443,
		BypassMethod:            BypassAdaptive,
		FakeSNI:                 "hcaptcha.com",
		FakeSNIPool: []string{
			"hcaptcha.com",
			"newassets.hcaptcha.com",
			"js.hcaptcha.com",
			"imgs.hcaptcha.com",
			"assets.hcaptcha.com",
			"api.hcaptcha.com",
			"analytics.hcaptcha.com",
			"loader.hcaptcha.com",
			"challenge-tasks.hcaptcha.com",
			"assets.vercel.com",
			"cdn1.vercel.com",
			"data.vercel.com",
			"go.vercel.com",
		},
		BrowserProfile:          ProfileRandom,
		TTLSpoof:                true,
		FragmentSize:            0,
		FakeDelayMs:             1.0,
		MinDelayMs:              0.5,
		MaxDelayMs:              10.0,
		BypassTimeout:           2.0,
		ConnectTimeout:          5.0,
		ReadTimeout:             30.0,
		WriteTimeout:            30.0,
		JitterMs:                3.0,
		RetryLimit:              3,
		RetryBaseMs:             100.0,
		RetryMaxMs:              5000.0,
		CircuitBreakerThreshold: 5,
		CircuitBreakerCooldownS: 30.0,
		RecvBuffer:              65536,
		MaxConnections:          0,
		IdleTimeout:             120,
		RateLimit:               0,
		WorkerPoolSize:          0,
		LogLevel:                LogInfo,
		LogClientSNI:            true,
		StatsInterval:           60,
		PrometheusEnabled:       false,
		PrometheusAddr:          ":9090",
		PprofEnabled:            false,
		PprofAddr:               ":6060",
		DomainProbeEnabled:      true,
		DomainProbeInterval:     300,
		DomainPool: []string{
			"hcaptcha.com",
			"newassets.hcaptcha.com",
			"js.hcaptcha.com",
			"imgs.hcaptcha.com",
			"assets.hcaptcha.com",
			"api.hcaptcha.com",
			"analytics.hcaptcha.com",
			"loader.hcaptcha.com",
			"challenge-tasks.hcaptcha.com",
			"assets.vercel.com",
			"cdn1.vercel.com",
			"data.vercel.com",
			"go.vercel.com",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %q: %w", path, err)
	}
	data := bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("config: yaml: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("config: json: %w", err)
		}
	}
	applyEnvOverrides(cfg)
	return cfg, cfg.Validate()
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("GHOSTNET_CONNECT_IP"); v != "" {
		cfg.ConnectIP = v
	}
	if v := os.Getenv("GHOSTNET_LISTEN_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.ListenPort)
	}
	if v := os.Getenv("GHOSTNET_LOG_LEVEL"); v != "" {
		cfg.LogLevel = LogLevel(v)
	}
	if v := os.Getenv("GHOSTNET_FAKE_SNI"); v != "" {
		cfg.FakeSNI = v
	}
	if v := os.Getenv("GHOSTNET_MAX_CONNECTIONS"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.MaxConnections)
	}
	if v := os.Getenv("GHOSTNET_RATE_LIMIT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.RateLimit)
	}
}

func (c *Config) Validate() error {
	if c.ConnectIP == "" && len(c.Endpoints) == 0 {
		return fmt.Errorf("config: connect_ip or endpoints required")
	}

	if c.ConnectIP != "" && net.ParseIP(c.ConnectIP) == nil {
		addrs, err := net.LookupHost(c.ConnectIP)
		if err != nil || len(addrs) == 0 {
			return fmt.Errorf("config: connect_ip %q unresolvable: %w", c.ConnectIP, err)
		}
		c.ConnectIP = addrs[0]
	}

	for i, ep := range c.Endpoints {
		if net.ParseIP(ep.IP) == nil {
			addrs, err := net.LookupHost(ep.IP)
			if err != nil || len(addrs) == 0 {
				return fmt.Errorf("config: endpoints[%d].ip %q unresolvable", i, ep.IP)
			}
			c.Endpoints[i].IP = addrs[0]
		}
		if ep.Port < 1 || ep.Port > 65535 {
			return fmt.Errorf("config: endpoints[%d].port %d out of range", i, ep.Port)
		}
		if c.Endpoints[i].Weight <= 0 {
			c.Endpoints[i].Weight = 1
		}
	}

	if c.ListenPort < 1 || c.ListenPort > 65535 {
		return fmt.Errorf("config: listen_port %d out of range", c.ListenPort)
	}
	if c.ConnectPort < 1 || c.ConnectPort > 65535 {
		c.ConnectPort = 443
	}
	if c.RecvBuffer < 4096 {
		c.RecvBuffer = 65536
	}
	if c.BypassMethod == "" {
		c.BypassMethod = BypassAdaptive
	}
	if c.FakeSNI == "" && len(c.FakeSNIPool) > 0 {
		c.FakeSNI = c.FakeSNIPool[0]
	}
	if c.RetryLimit < 0 {
		c.RetryLimit = 0
	}
	if c.CircuitBreakerThreshold <= 0 {
		c.CircuitBreakerThreshold = 5
	}
	if c.MinDelayMs <= 0 {
		c.MinDelayMs = 0.1
	}
	if c.MaxDelayMs <= 0 || c.MaxDelayMs < c.MinDelayMs {
		c.MaxDelayMs = 50.0
	}
	if c.FakeDelayMs < c.MinDelayMs {
		c.FakeDelayMs = c.MinDelayMs
	}
	if c.FakeDelayMs > c.MaxDelayMs {
		c.FakeDelayMs = c.MaxDelayMs
	}

	if len(c.Endpoints) == 0 && c.ConnectIP != "" {
		c.Endpoints = []Endpoint{{IP: c.ConnectIP, Port: c.ConnectPort, Weight: 1}}
	}

	if len(c.Endpoints) == 0 {
		return fmt.Errorf("config: no valid endpoints after validation")
	}

	return nil
}

func (c *Config) BypassTimeoutDuration() time.Duration {
	return time.Duration(c.BypassTimeout * float64(time.Second))
}

func (c *Config) ConnectTimeoutDuration() time.Duration {
	return time.Duration(c.ConnectTimeout * float64(time.Second))
}

func (c *Config) ReadTimeoutDuration() time.Duration {
	if c.ReadTimeout <= 0 {
		return 0
	}
	return time.Duration(c.ReadTimeout * float64(time.Second))
}

func (c *Config) WriteTimeoutDuration() time.Duration {
	if c.WriteTimeout <= 0 {
		return 0
	}
	return time.Duration(c.WriteTimeout * float64(time.Second))
}

func (c *Config) IdleTimeoutDuration() time.Duration {
	if c.IdleTimeout <= 0 {
		return 0
	}
	return time.Duration(c.IdleTimeout) * time.Second
}

func (c *Config) CircuitBreakerCooldown() time.Duration {
	return time.Duration(c.CircuitBreakerCooldownS * float64(time.Second))
}

func (c *Config) RLock()   { c.mu.RLock() }
func (c *Config) RUnlock() { c.mu.RUnlock() }
func (c *Config) Lock()    { c.mu.Lock() }
func (c *Config) Unlock()  { c.mu.Unlock() }

func (c *Config) SafeCopy() Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cp := *c
	cp.mu = sync.RWMutex{}
	return cp
}
