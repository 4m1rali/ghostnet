# Building GhostNet

## Requirements

| Requirement | Version | Notes |
|---|---|---|
| OS | Ubuntu 20.04+ / Debian 11+ | Linux kernel 5.4+ required |
| Go | 1.22+ | `go version` to check |
| Capability | `CAP_NET_RAW` | For raw socket injection |

---

## Build options at a glance

- **Release binary (recommended)**: fastest path for production usage.
- **Build from source**: best for development, patching, and custom packaging.
- **Cross-compile**: build Linux binaries from Windows/macOS CI or local machines.

---

## Get the binary (recommended for production)

Download the pre-built binary from GitHub Releases:

```bash
wget https://github.com/4m1rali/ghostnet/releases/latest/download/ghostnet-linux-amd64
chmod +x ghostnet-linux-amd64
sudo setcap cap_net_raw+ep ghostnet-linux-amd64
```

---

## Build from source (Linux)

```bash
# Clone
git clone https://github.com/4m1rali/ghostnet
cd ghostnet

# Download dependencies
go mod download

# Build (stripped, optimized)
go build -ldflags="-s -w" -o ghostnet ./cmd/ghostnet/

# Grant capability
sudo setcap cap_net_raw+ep ./ghostnet
```

Optional verification:

```bash
getcap ./ghostnet
./ghostnet version
```

---

## Cross-compile from Windows

Open PowerShell in the `ghostnet` folder:

```powershell
# Linux amd64 (most VPS servers)
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -ldflags="-s -w" -o ghostnet-linux-amd64 ./cmd/ghostnet/

# Linux arm64 (AWS Graviton, Oracle ARM, Raspberry Pi 4)
$env:GOOS="linux"; $env:GOARCH="arm64"; $env:CGO_ENABLED="0"
go build -ldflags="-s -w" -o ghostnet-linux-arm64 ./cmd/ghostnet/
```

Transfer to server:
```powershell
scp ghostnet-linux-amd64 root@YOUR_SERVER_IP:/root/ghostnet
```

---

## Multi-architecture build matrix

```bash
mkdir -p dist

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/ghostnet-linux-amd64 ./cmd/ghostnet/
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/ghostnet-linux-arm64 ./cmd/ghostnet/
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/ghostnet-linux-armv7 ./cmd/ghostnet/
```

---

## Reproducible release-style build tips

- Use `CGO_ENABLED=0` for static-like binaries and predictable deployment.
- Use `-ldflags="-s -w"` to reduce binary size for VPS deployment.
- Build inside a pinned Go toolchain container for consistency.
- Keep release artifacts architecture-labeled (`ghostnet-linux-amd64`, etc.).

Example containerized build:

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.22 \
  bash -lc 'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/ghostnet-linux-amd64 ./cmd/ghostnet/'
```

---

## First-time setup on server

```bash
# 1. Make executable
chmod +x /root/ghostnet

# 2. Grant raw socket capability (no root needed at runtime)
sudo setcap cap_net_raw+ep /root/ghostnet

# 3. Full automatic setup
sudo /root/ghostnet setup
```

`setup` does everything: tunes the kernel, scans SNI domains, creates config.json, and starts the proxy.

---

## Manual setup (step by step)

```bash
# Step 1: Tune kernel
sudo ./ghostnet tune

# Step 2: Scan SNI domains and write to config
./ghostnet scan -w -c config.json

# Step 3: Run
./ghostnet run -c config.json
```

---

## Capability and privilege model

GhostNet needs raw sockets for full bypass mode.

- Preferred: grant binary capability and run as non-root process.
- Fallback: run as root (less secure operational model).
- If capability is missing, GhostNet may run in relay-only mode and log warnings.

Commands:

```bash
sudo setcap cap_net_raw+ep ./ghostnet
getcap ./ghostnet
```

---

## systemd service

```bash
# Create service file
cat > /etc/systemd/system/ghostnet.service << 'EOF'
[Unit]
Description=GhostNet DPI Bypass Proxy
After=network.target
Wants=network.target

[Service]
Type=simple
WorkingDirectory=/root
ExecStart=/root/ghostnet run -c /root/config.json
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=3
LimitNOFILE=1048576
AmbientCapabilities=CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_RAW
NoNewPrivileges=yes

[Install]
WantedBy=multi-user.target
EOF

# Enable and start
systemctl daemon-reload
systemctl enable ghostnet
systemctl start ghostnet

# Check status
systemctl status ghostnet

# View logs
journalctl -u ghostnet -f
```

### Live config reload (no restart)

```bash
systemctl reload ghostnet
# or
kill -HUP $(pidof ghostnet)
```

---

## Build and runtime verification checklist

```bash
# Build checks
go build ./...
go test ./...

# Runtime checks
./ghostnet version
./ghostnet scan -t 3000
./ghostnet run -c config.json --debug
```

For production readiness, also verify:

- `systemctl status ghostnet` remains healthy over restart cycles
- `/metrics` is reachable when Prometheus is enabled
- reload behavior via `systemctl reload ghostnet` (`SIGHUP`) works as expected

---

## CLI reference

| Command | Flags | Description |
|---|---|---|
| `tune` | `--dry-run` | Optimize Linux kernel settings |
| `scan` | `-f file`, `-w`, `-p port`, `-t timeout` | Scan SNI domains |
| `setup` | `-p port` | Full automatic setup |
| `run` | `-c config`, `--debug`, `--stealth` | Start proxy |
| `version` | — | Print version |
| `bench` | `-n count`, `-d duration` | Benchmark throughput |

---

## Packaging notes (deb/rpm/internal repos)

For internal packaging pipelines:

- Install binary under `/usr/local/bin/ghostnet` or `/opt/ghostnet/ghostnet`
- Include post-install step for `setcap cap_net_raw+ep`
- Ship a hardened systemd unit with `NoNewPrivileges=yes`
- Keep config outside package payload (`/etc/ghostnet/config.json`)
- Treat updates as replace-in-place binary swap + service restart/reload

---

## Troubleshooting

| Error | Cause | Fix |
|---|---|---|
| `operation not permitted` | Missing `CAP_NET_RAW` | `sudo setcap cap_net_raw+ep ./ghostnet` |
| `address already in use` | Port taken | Change `listen_port` in config.json |
| `connect_ip required` | No IP in config | Run `scan -w` or set `connect_ip` manually |
| `invalid character '\n'` | Windows line endings | `sed -i 's/\r//' config.json` |
| `relay-only mode` | No raw socket access | Grant `CAP_NET_RAW` or run as root |
| Binary won't start | Wrong architecture | Use `ghostnet-linux-amd64` not `.exe` |
