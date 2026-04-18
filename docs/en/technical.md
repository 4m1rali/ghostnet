# GhostNet Technical Deep Dive

## Overview

GhostNet is a high-performance TCP relay that performs DPI desynchronization before forwarding real client traffic. The objective is to pass middlebox filtering while keeping the upstream server handshake valid.

At a high level, every accepted connection follows this sequence:

1. Choose an endpoint (`internal/routing`) using latency and health data.
2. Establish TCP upstream connection with retry/backoff policy.
3. Extract socket metadata needed for packet crafting.
4. Generate fake browser-like TLS `ClientHello` payload.
5. Inject fake packet over raw socket (`CAP_NET_RAW`) using selected bypass strategy.
6. Continue normal bidirectional relay (`internal/proxy/relay`).
7. Record metrics for latency, bytes, strategy outcome, and active sessions.

---

## Threat model and assumptions

GhostNet primarily targets active blocking where DPI engines:

- inspect plaintext TLS handshake fields (especially SNI),
- apply policy before TLS is encrypted,
- make early allow/deny decisions from a narrow packet window.

It assumes upstream endpoints remain reachable at TCP level and that crafted fake packets can influence DPI state without breaking the real server flow.

It does **not** guarantee:

- anonymity or identity protection,
- resistance against full traffic correlation adversaries,
- bypass in all network topologies.

---

## Packet and handshake strategy

### Fake ClientHello generation

`internal/tls/clienthello.go` and `internal/tls/fingerprint.go` build fake handshakes using browser-style fingerprint profiles (`chrome`, `firefox`, `safari`, `edge`, `random`).

The builder controls:

- cipher suite ordering,
- extension ordering,
- ALPN values,
- supported groups and signature algorithms,
- optional GREASE behavior.

This keeps fake traffic realistic enough for many filters that rely on static or simplistic fingerprint checks.

### Bypass methods

The bypass engine (`internal/bypass`) provides multiple strategies:

- `wrong_seq`: fake packet with sequence positioning designed to be ignored by server TCP stack.
- `fragment`: crafted segmentation that causes parser divergence between DPI and endpoint.
- `desync`: desynchronization variants (e.g., malformed checksum or TTL behavior).
- `adaptive`: strategy orchestrator with fallback and per-strategy statistics.

`adaptive` is the recommended runtime mode and default in config.

---

## Runtime components

### Proxy server (`internal/proxy`)

- Listener accepts incoming TCP clients.
- Worker pool handles connection lifecycle.
- Handler coordinates endpoint selection, dial, bypass attempt, and relay startup.
- Relay path enforces read/write deadlines and idle timeout.
- Buffer pool reduces allocations and zeroes memory before reuse.

### Router (`internal/routing`)

- Maintains endpoint health and dynamic scoring.
- Uses failure tracking + cooldown (circuit breaker style behavior).
- Supports active health checks in background.
- Works with weighted endpoint definitions from config.

### Domain preflight (`internal/preflight`, `internal/domain`)

- `scan` and `setup` probe known or user-provided SNI domains concurrently.
- Best result is selected by latency among reachable targets.
- Optional runtime domain probing refreshes pool quality over time.

### Metrics and telemetry (`internal/metrics`)

- Active connections, bytes in/out, strategy success/failure, and latency quantiles.
- Optional Prometheus exporter (`/metrics`).
- Optional periodic stats loop with top observed SNIs.
- Optional runtime profiling via pprof endpoint.

---

## Concurrency and resource model

GhostNet is designed for high fan-in workloads:

- worker pool size defaults to CPU-relative scaling,
- relay buffers are pooled to lower pressure on GC,
- retries and timeouts are bounded to avoid unbounded stalls,
- endpoint selection avoids repeatedly failing routes.

Key resource controls exposed in config:

- `worker_pool_size`
- `max_connections`
- `recv_buffer`
- `idle_timeout`
- `connect_timeout`, `read_timeout`, `write_timeout`
- retry and circuit breaker thresholds/cooldowns

---

## Linux integration requirements

Raw packet injection requires Linux capabilities:

- `CAP_NET_RAW` via `setcap cap_net_raw+ep ./ghostnet`, or
- root execution (less preferred operationally).

Kernel tuning (`ghostnet tune`) writes persistent host optimizations:

- `/etc/sysctl.d/99-ghostnet.conf`
- `/etc/security/limits.d/99-ghostnet.conf`

These improve backlog capacity, socket buffers, and behavior under high connection churn.

---

## Config reload and process lifecycle

GhostNet listens for signals:

- `SIGINT`/`SIGTERM`: graceful shutdown path.
- `SIGHUP`: reload config file and apply safe runtime changes (e.g., log level and mutable runtime options).

This allows long-running deployments to update behavior without full process restart.

---

## Failure modes and fallback behavior

Expected degraded states include:

- missing `CAP_NET_RAW` -> relay-only mode with warning,
- all probe domains unreachable -> setup/scan cannot auto-populate endpoint,
- endpoint instability -> retries + route re-selection + breaker cooldown,
- invalid config -> startup failure with explicit validation errors.

In degraded mode, GhostNet attempts safe continuation where possible and emits actionable logs.

---

## Extension points for contributors

Most impactful extension surfaces:

- `internal/bypass`: add or tune strategy logic.
- `internal/tls`: add realistic fingerprint profiles and extension order presets.
- `internal/routing`: improve endpoint scoring and failover policy.
- `internal/preflight`: improve domain ranking and filtering heuristics.
- `internal/metrics`: expose more operational counters/histograms.

Contributor implementation details and PR workflow are documented in [contributing.md](contributing.md).
