# GhostNet

**High-performance DPI bypass proxy for Linux servers (Go 1.22+).**

GhostNet defeats SNI-based blocking by injecting a crafted fake TLS `ClientHello` before the real handshake. DPI sees an allowed hostname, while the destination server ignores the fake packet and accepts the real flow.

---

## Language / زبان / Язык / 语言

| | Language | Technical | Building | System Tuning | Contributing |
|---|---|---|---|---|---|
| 🇬🇧 | English | [technical](docs/en/technical.md) | [building](docs/en/building.md) | [tuning](docs/en/system_tuning.md) | [contributing](docs/en/contributing.md) |
| 🇮🇷 | فارسی | [فنی](docs/fa/technical.md) | [ساخت](docs/fa/building.md) | [بهینه‌سازی](docs/fa/system_tuning.md) | [مشارکت](docs/fa/contributing.md) |
| 🇷🇺 | Русский | [техническое](docs/ru/technical.md) | [сборка](docs/ru/building.md) | [настройка](docs/ru/system_tuning.md) | [участие](docs/ru/contributing.md) |
| 🇨🇳 | 中文 | [技术](docs/zh/technical.md) | [构建](docs/zh/building.md) | [调优](docs/zh/system_tuning.md) | [贡献](docs/zh/contributing.md) |

---

## Why GhostNet

- Bypasses SNI-oriented DPI with raw-packet desynchronization strategies.
- Optimized for high concurrency on commodity VPS instances.
- Adaptive endpoint routing with health checks and circuit breaker logic.
- Includes observability (`/metrics`, optional pprof, interval stats loop).
- Single static binary with minimal dependencies.

---

## How DPI bypass works

Most censorship systems inspect TLS `ClientHello` packets and block forbidden `SNI` values before encryption starts.

GhostNet pipeline:

1. Establish TCP session toward target endpoint.
2. Read connection metadata and craft a fake browser-like `ClientHello`.
3. Inject fake packet using a bypass strategy (`wrong_seq`, `fragment`, `desync`, or `adaptive`).
4. Relay real traffic normally once desync succeeds.

The fake packet is intentionally designed so middleboxes parse it, but the target server does not accept it as valid application payload.

For the full low-level explanation, packet layout details, and strategy internals, see [docs/en/technical.md](docs/en/technical.md).

---

## Quick Start (Linux VPS)

```bash
# 1) Download latest binary
wget https://github.com/4m1rali/ghostnet/releases/latest/download/ghostnet-linux-amd64
chmod +x ghostnet-linux-amd64

# 2) Full automatic bootstrap (kernel tune + SNI scan + config + run)
sudo ./ghostnet-linux-amd64 setup
```

After first setup, use:

```bash
./ghostnet-linux-amd64 run -c config.json
```

---

## Command Reference

```
ghostnet tune      Optimize Linux kernel and limits for high-load operation
ghostnet scan      Probe SNI domains, rank by latency, optionally write config
ghostnet setup     tune -> scan -> config -> run (first-time bootstrap)
ghostnet run       Start proxy from existing config (no scan, no tuning)
ghostnet version   Print version/runtime info
ghostnet bench     Benchmark ClientHello build throughput
```

### `tune`

```bash
sudo ./ghostnet tune
./ghostnet tune --dry-run
```

Applies persistent tuning (`/etc/sysctl.d/99-ghostnet.conf`, `/etc/security/limits.d/99-ghostnet.conf`) including TCP queues, buffer sizing, BBR, and options required for raw injection.

### `scan`

```bash
./ghostnet scan
./ghostnet scan -f sni.txt
./ghostnet scan -w -c config.json
./ghostnet scan -p 443 -t 3000
```

Scans domains concurrently, resolves IPs, and ranks by observed latency. With `-w`, updates `fake_sni`, endpoint IP, and SNI pool in config.

### `setup`

```bash
sudo ./ghostnet setup -p 40443
```

Performs one-pass bootstrap suitable for fresh servers. If not root, tuning is skipped and setup continues with scan/config/run.

### `run`

```bash
./ghostnet run -c config.json
./ghostnet run -c config.json --debug
./ghostnet run -c config.json --stealth
```

Runs from existing config only. Requires valid `connect_ip` and `fake_sni`.

---

## Runtime Architecture

```text
Listener -> worker pool -> connection handler
          -> endpoint router (EWMA + circuit breaker + health checks)
          -> adaptive bypass engine (raw packet strategies)
          -> relay engine (buffer pooling, deadlines, idle timeout)
          -> metrics collector (latency quantiles + counters + top SNIs)
```

Core modules:

- `internal/proxy`: listener, handler, relay, connection lifecycle.
- `internal/bypass`: raw packet injector + desync strategies and adaptive fallback.
- `internal/routing`: endpoint selection, health state, retry-aware decisions.
- `internal/tls`: fingerprint profiles, `ClientHello` builder, SNI parser.
- `internal/preflight`: known-domain probing and best SNI selection.
- `internal/tuner`: kernel and host tuning for scale.

---

## Configuration Overview

Generate or refresh config with:

```bash
./ghostnet scan -w -c config.json
```

Important settings:

| Key | Default | Description |
|---|---|---|
| `connect_ip` | empty | Upstream endpoint IP (required for run) |
| `connect_port` | `443` | Upstream TCP port |
| `bypass_method` | `adaptive` | `wrong_seq`, `fragment`, `desync`, `adaptive` |
| `fake_sni` | auto | Decoy SNI used in fake handshake |
| `fake_sni_pool` | discovered | Candidate SNIs for adaptive behavior |
| `browser_profile` | `random` | TLS fingerprint profile |
| `retry_limit` | `3` | Dial retry attempts before fail |
| `circuit_breaker_threshold` | `5` | Failures before temporary endpoint open-circuit |
| `worker_pool_size` | auto | Connection worker pool size |
| `stats_interval` | `30` | Periodic internal stats log interval (seconds) |
| `prometheus_enabled` | `false` | Expose `/metrics` endpoint |
| `pprof_enabled` | `false` | Enable runtime profiler endpoint |

Common env overrides:

- `GHOSTNET_CONNECT_IP`
- `GHOSTNET_FAKE_SNI`
- `GHOSTNET_LOG_LEVEL`

---

## Performance and Reliability Notes

- Concurrency defaults to CPU-scaled worker model.
- Relay path uses pooled `64KB` buffers and zeroes before reuse.
- Retry path uses exponential backoff with max cap.
- Router tracks endpoint health and avoids repeated failures.
- Stats include P50/P95/P99 latency plus per-strategy success/failure.

Enable metrics with `prometheus_enabled=true` and scrape `/metrics`.

---

## Build and Deploy

Build locally:

```bash
git clone https://github.com/4m1rali/ghostnet
cd ghostnet
go build -ldflags="-s -w" -o ghostnet ./cmd/ghostnet/
sudo setcap cap_net_raw+ep ./ghostnet
```

Cross-compile on Windows for Linux:

```powershell
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -ldflags="-s -w" -o ghostnet-linux-amd64 ./cmd/ghostnet/
```

Advanced build matrix, systemd setup, and troubleshooting: [docs/en/building.md](docs/en/building.md).

---

## Contributing

Contributions are welcome for:

- New bypass strategies and strategy-selection improvements.
- Better endpoint routing heuristics and health logic.
- More TLS/browser profile parity and fingerprint realism.
- Documentation translations and reproducible test reports.

Before opening a PR:

1. Run `go build ./...`.
2. Validate Linux-specific behavior where applicable.
3. Update related docs.
4. Include clear benchmark or repro notes for behavior changes.

Full guide: [docs/en/contributing.md](docs/en/contributing.md).

---

## Security, Ethics, and Scope

GhostNet is a censorship-circumvention research/operations tool. Use it only where legal and permitted. This project does not provide anonymity guarantees by itself; pair with your own network and operational security model.

---

## License

GPL-3.0 · [github.com/4m1rali/ghostnet](https://github.com/4m1rali/ghostnet)
