# Contributing to GhostNet

Thanks for helping improve GhostNet. This guide covers development setup, workflow expectations, and project-specific design rules so contributions can be reviewed and merged quickly.

---

## What contributions are most useful

- Bypass strategy improvements (`internal/bypass`) with real-world test evidence.
- TLS fingerprint/profile parity updates (`internal/tls`).
- Routing reliability and endpoint health behavior improvements (`internal/routing`).
- Performance and memory optimizations with benchmark results.
- Documentation and translation updates across `docs/en`, `docs/fa`, `docs/ru`, `docs/zh`.

---

## Development environment

Required:

- Go `1.22+`
- Linux runtime for full raw-socket validation
- `CAP_NET_RAW` (or root) for bypass-path tests

Recommended:

- A disposable Linux VPS for network-level behavior validation
- Two test networks (or VPN + non-VPN) for comparative behavior

---

## Getting started locally

```bash
git clone https://github.com/4m1rali/ghostnet
cd ghostnet
go mod download
go build ./...
```

---

## Project structure

```
ghostnet/
├── cmd/ghostnet/          Entry point — CLI commands
├── internal/
│   ├── bypass/            DPI bypass strategies
│   ├── config/            Configuration
│   ├── domain/            SNI domain pool
│   ├── log/               Logger
│   ├── metrics/           Metrics collection
│   ├── network/           Network utilities
│   ├── preflight/         SNI domain scanner
│   ├── proxy/             TCP proxy core
│   ├── routing/           Endpoint routing
│   ├── tls/               TLS packet construction
│   └── tuner/             Kernel tuner
└── docs/
    ├── en/                English docs
    ├── fa/                Persian docs
    ├── ru/                Russian docs
    └── zh/                Chinese docs
```

---

## Workflow

1. Create a focused branch per change.
2. Keep each PR scoped to one theme (strategy, routing, docs, etc.).
3. Include rationale and expected behavior in PR description.
4. Add before/after logs, metrics, or benchmark snippets when relevant.
5. Update docs in the same PR for user-facing behavior changes.

---

## Build matrix examples

```bash
# Linux amd64 (standard VPS)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/ghostnet-linux-amd64 ./cmd/ghostnet/

# Linux arm64 (AWS Graviton, Oracle ARM)
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/ghostnet-linux-arm64 ./cmd/ghostnet/

# Linux armv7 (Raspberry Pi)
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/ghostnet-linux-armv7 ./cmd/ghostnet/
```

### Build script

```bash
#!/bin/bash
mkdir -p dist
targets=(
    "linux/amd64"
    "linux/arm64"
    "linux/arm/7"
)
for target in "${targets[@]}"; do
    IFS='/' read -r os arch arm <<< "$target"
    name="ghostnet-${os}-${arch}"
    [ -n "$arm" ] && name="${name}v${arm}"
    GOOS=$os GOARCH=$arch GOARM=$arm CGO_ENABLED=0 \
        go build -ldflags="-s -w" -o "dist/$name" ./cmd/ghostnet/
    echo "built: dist/$name ($(du -sh dist/$name | cut -f1))"
done
```

---

## Validation checklist before opening a PR

Core checks:

```bash
go build ./...
go test ./...
```

Runtime checks:

```bash
# Validate CLI and config load
./ghostnet version
./ghostnet run -c config.json --debug

# Validate scanner behavior
./ghostnet scan -t 3000
```

Linux raw path check:

```bash
sudo setcap cap_net_raw+ep ./ghostnet
./ghostnet run -c config.json
```

If your change touches tuning, routing, bypass selection, or relay internals, include reproducible evidence (logs, p95/p99 changes, failure-rate changes, or packet-level notes).

---

## Adding a bypass strategy

1. Create `internal/bypass/mymethod.go`
2. Implement the injection function
3. Add a new `Strategy` constant in `adaptive.go`
4. Add the case to `AdaptiveBypass.run()`
5. Add it to the fallback chain in `AdaptiveBypass.next()`

### Minimal bypass implementation

```go
package bypass

import (
    "context"
    "fmt"
    "time"
)

type MyBypass struct {
    injector *RawInjector
    stats    *BypassStats
}

func NewMyBypass() (*MyBypass, error) {
    inj, err := NewRawInjector()
    if err != nil {
        return nil, fmt.Errorf("mybypass: %w", err)
    }
    return &MyBypass{injector: inj, stats: &BypassStats{}}, nil
}

func (b *MyBypass) Close() error { return b.injector.Close() }

type MyParams struct {
    // ... connection parameters
}

func (b *MyBypass) Inject(ctx context.Context, p MyParams) error {
    // Wait for appropriate delay
    select {
    case <-ctx.Done():
        b.stats.Fail.Add(1)
        return ctx.Err()
    case <-time.After(HumanDelay(p.DelayMs, p.MinDelayMs, p.MaxDelayMs)):
    }

    // Craft and send packet
    pkt, err := CraftPacket(PacketParams{
        SrcIP:   p.SrcIP,
        DstIP:   p.DstIP,
        SrcPort: p.SrcPort,
        DstPort: p.DstPort,
        SeqNum:  /* your sequence number logic */,
        Flags:   TCPFlagPSH | TCPFlagACK,
        Payload: /* your payload */,
    })
    if err != nil {
        b.stats.Fail.Add(1)
        return err
    }

    if err := b.injector.Send(pkt, p.DstIP, p.DstPort); err != nil {
        b.stats.Fail.Add(1)
        return err
    }

    b.stats.OK.Add(1)
    return nil
}
```

---

## Adding a browser profile

Edit `internal/tls/fingerprint.go`:

```go
var MyBrowser = &Profile{
    Name: "MyBrowser/1.0",
    CipherSuites: []uint16{
        0x1301, 0x1302, 0x1303,
        // ... cipher suites in exact browser order
    },
    SupportedGroups: []uint16{0x001D, 0x0017, 0x0018},
    SigAlgs: []uint16{
        0x0403, 0x0804, 0x0401,
        // ... signature algorithms
    },
    ALPNProtos:     []string{"h2", "http/1.1"},
    TLSVersions:    []uint16{0x0304, 0x0303},
    PSKModes:       []byte{0x01},
    UseGREASE:      true,
    ExtensionOrder: []uint16{
        extSNI, extExtendedMasterSec, /* ... in browser order */
    },
}
```

Then add it to `profileMap` and `allProfiles`.

---

## Adding SNI domains to the built-in list

Edit `internal/preflight/checker.go` — add domains to `KnownSNIDomains`:

```go
var KnownSNIDomains = []string{
    "hcaptcha.com",
    // ... add your domains here
}
```

Domains should be:
- Reachable on port 443 from most networks
- On common DPI allowlists (CDN domains, major services)
- Not blocked in the target regions

---

## Code style

- Keep code simple and explicit; avoid hidden behavior.
- No comments in code (project convention), except when absolutely needed for non-obvious protocol logic.
- No external dependencies beyond `cobra` and `yaml.v3`
- All platform-specific code in `_linux.go` / `_other.go` files with build tags
- Errors wrapped with `fmt.Errorf("package: %w", err)`
- Atomic operations for all shared counters
- Prefer bounded retries/timeouts; avoid unbounded loops in network paths.
- Keep logging informative but avoid noisy per-packet logs in normal mode.

---

## Pull request checklist

- [ ] `go build ./...` passes
- [ ] `go test ./...` passes (if tests are present/added)
- [ ] No new external dependencies
- [ ] Platform-specific code has build tags
- [ ] New bypass strategies added to adaptive fallback chain
- [ ] New config fields have defaults in `config.Default()`
- [ ] Runtime behavior change documented in `README.md` or docs
- [ ] Documentation updated in all 4 languages (or note that translation help is needed)

---

## Reporting issues

Include:
- OS and kernel version (`uname -a`)
- GhostNet version (`./ghostnet version`)
- Full error output
- Config file (remove sensitive IPs)
- Network environment (ISP, country, VPS provider)
- Whether `CAP_NET_RAW` is granted (`getcap ./ghostnet`)
- Whether issue occurs in relay-only mode vs bypass-enabled mode
