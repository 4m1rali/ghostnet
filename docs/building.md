# Building GhostNet

## Requirements

| Requirement | Minimum | Notes |
|---|---|---|
| OS | Ubuntu 20.04 / Debian 11 | Linux kernel 5.4+ required for raw socket support |
| Go | 1.22 | `go version` to check |
| Kernel capability | `CAP_NET_RAW` | Required for raw socket injection — see below |

---

## Clone

```bash
git clone https://github.com/4m1rali/ghostnet
cd ghostnet
```

---

## Download dependencies

```bash
go mod download
```

---

## Build

### Standard build

```bash
go build -ldflags="-s -w" -o ghostnet ./cmd/ghostnet/
```

`-s -w` strips debug symbols and DWARF info — reduces binary size by ~30%.

### Verify the binary

```bash
./ghostnet version
```

Expected output:
```
GhostNet v2.1.0  Go go1.22.x  linux/amd64
```

### Cross-compile for a remote server

```bash
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ghostnet ./cmd/ghostnet/
```

---

## Grant CAP_NET_RAW

Raw socket injection requires the `CAP_NET_RAW` Linux capability. This lets the binary open raw sockets without running as root.

```bash
sudo setcap cap_net_raw+ep ./ghostnet
```

Verify:

```bash
getcap ./ghostnet
# ghostnet = cap_net_raw+ep
```

If you skip this step, GhostNet will still run but falls back to relay-only mode — bypass injection will be disabled.

---

## Kernel tuning

Apply the recommended kernel settings for high-connection-count operation:

```bash
sudo bash scripts/sysctl-tune.sh
```

This script sets:

| Setting | Value | Effect |
|---|---|---|
| `fs.file-max` | `2097152` | 2M open file descriptors |
| `net.core.somaxconn` | `65535` | TCP accept backlog |
| `net.ipv4.tcp_max_syn_backlog` | `65535` | SYN queue depth |
| `net.ipv4.ip_local_port_range` | `1024 65535` | Outbound port range |
| `net.ipv4.tcp_tw_reuse` | `1` | Reuse TIME_WAIT sockets |
| `net.ipv4.tcp_fin_timeout` | `15` | Faster FIN cleanup |
| `net.ipv4.tcp_congestion_control` | `bbr` | BBR congestion control |
| `net.ipv4.conf.all.rp_filter` | `0` | Allow injected packets |
| `vm.max_map_count` | `262144` | Large Go heap support |

Settings are written to `/etc/sysctl.d/99-ghostnet.conf` and persist across reboots.

ulimit settings are written to `/etc/security/limits.d/99-ghostnet.conf`:

```
*    soft nofile 1048576
*    hard nofile 1048576
```

---

## systemd service

To run GhostNet as a system service:

```bash
sudo useradd -r -s /bin/false ghostnet
sudo mkdir -p /opt/ghostnet
sudo cp ghostnet /opt/ghostnet/
sudo cp config.json /opt/ghostnet/
sudo setcap cap_net_raw+ep /opt/ghostnet/ghostnet
```

Create `/etc/systemd/system/ghostnet.service`:

```ini
[Unit]
Description=GhostNet DPI Bypass Proxy
After=network.target
Wants=network.target

[Service]
Type=simple
User=ghostnet
WorkingDirectory=/opt/ghostnet
ExecStart=/opt/ghostnet/ghostnet run -c /opt/ghostnet/config.json
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=3
LimitNOFILE=1048576
AmbientCapabilities=CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_RAW
NoNewPrivileges=yes

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable ghostnet
sudo systemctl start ghostnet
sudo systemctl status ghostnet
```

Live config reload without restart:

```bash
sudo systemctl reload ghostnet
# or
sudo kill -HUP $(pidof ghostnet)
```

---

## Generate a config file

```bash
./ghostnet gen-config -o config.json
```

Edit `config.json` and set `connect_ip` to your target server IP before starting.

---

## Quick start

```bash
# 1. Build
go build -ldflags="-s -w" -o ghostnet ./cmd/ghostnet/

# 2. Grant capability
sudo setcap cap_net_raw+ep ./ghostnet

# 3. Apply kernel tuning
sudo bash scripts/sysctl-tune.sh

# 4. Generate config
./ghostnet gen-config -o config.json

# 5. Edit config — set connect_ip
nano config.json

# 6. Run
./ghostnet run -c config.json
```

---

## CLI reference

| Command | Description |
|---|---|
| `ghostnet run -c config.json` | Start the proxy |
| `ghostnet debug -c config.json` | Start with debug logging |
| `ghostnet run --stealth -c config.json` | Start with minimal logging |
| `ghostnet gen-config -o config.json` | Generate default config |
| `ghostnet probe [domain...]` | Probe domain latency |
| `ghostnet bench -n 1000 -d 10` | Benchmark ClientHello throughput |
| `ghostnet version` | Print version |

---

## Troubleshooting

**`raw socket: operation not permitted`**
The binary does not have `CAP_NET_RAW`. Run `sudo setcap cap_net_raw+ep ./ghostnet`.

**`listen tcp: bind: address already in use`**
Port `40443` is taken. Change `listen_port` in config.json.

**`config: connect_ip or endpoints required`**
You haven't set `connect_ip` in config.json.

**Bypass injection disabled, relay-only mode**
Either `CAP_NET_RAW` is missing or you are on a non-Linux platform. Injection requires Linux with raw socket access.

**High memory usage**
Each active connection uses ~2KB of state plus a 64KB relay buffer from the pool. 10,000 connections ≈ 20MB. If memory is a concern, lower `max_connections`.
