#!/bin/bash
# sysctl-tune.sh — Ubuntu VPS / dedicated server tuning for GhostNet
# Run as root: sudo bash scripts/sysctl-tune.sh
#
# These settings optimize for:
#   - 10k+ concurrent TCP connections
#   - High-throughput relay
#   - Low-latency packet injection
#   - Minimal memory overhead per connection

set -euo pipefail

echo "[*] Applying GhostNet kernel tuning..."

# ─── TCP Connection Limits ────────────────────────────────────────────────────

# Maximum number of open file descriptors (connections)
sysctl -w fs.file-max=2097152
sysctl -w fs.nr_open=2097152

# TCP connection backlog
sysctl -w net.core.somaxconn=65535
sysctl -w net.ipv4.tcp_max_syn_backlog=65535

# Increase local port range for outgoing connections
sysctl -w net.ipv4.ip_local_port_range="1024 65535"

# ─── TCP Buffer Sizes ─────────────────────────────────────────────────────────

# Default and maximum socket buffer sizes
sysctl -w net.core.rmem_default=262144
sysctl -w net.core.wmem_default=262144
sysctl -w net.core.rmem_max=134217728   # 128MB
sysctl -w net.core.wmem_max=134217728   # 128MB
sysctl -w net.core.optmem_max=65536

# TCP-specific buffer sizes: min, default, max
sysctl -w net.ipv4.tcp_rmem="4096 87380 134217728"
sysctl -w net.ipv4.tcp_wmem="4096 65536 134217728"

# ─── TCP Performance ──────────────────────────────────────────────────────────

# Enable TCP Fast Open (reduces handshake latency)
sysctl -w net.ipv4.tcp_fastopen=3

# Use BBR congestion control (better throughput on lossy links)
sysctl -w net.ipv4.tcp_congestion_control=bbr 2>/dev/null || \
  sysctl -w net.ipv4.tcp_congestion_control=cubic

# Enable TCP window scaling
sysctl -w net.ipv4.tcp_window_scaling=1

# Reduce TIME_WAIT sockets
sysctl -w net.ipv4.tcp_tw_reuse=1
sysctl -w net.ipv4.tcp_fin_timeout=15
sysctl -w net.ipv4.tcp_max_tw_buckets=1440000

# Keepalive settings (match GhostNet defaults)
sysctl -w net.ipv4.tcp_keepalive_time=60
sysctl -w net.ipv4.tcp_keepalive_intvl=10
sysctl -w net.ipv4.tcp_keepalive_probes=6

# ─── Network Queue ────────────────────────────────────────────────────────────

# Increase network device receive queue
sysctl -w net.core.netdev_max_backlog=65536

# ─── Raw Socket / Packet Injection ───────────────────────────────────────────

# Allow raw socket creation without root (CAP_NET_RAW is still required)
# GhostNet uses SOCK_RAW for packet injection — ensure the binary has the capability:
#   sudo setcap cap_net_raw+ep ./ghostnet

# Disable reverse path filtering (allows injected packets with spoofed source)
sysctl -w net.ipv4.conf.all.rp_filter=0
sysctl -w net.ipv4.conf.default.rp_filter=0

# ─── Memory ───────────────────────────────────────────────────────────────────

# Increase virtual memory map count (for large Go heaps)
sysctl -w vm.max_map_count=262144

# ─── Persist settings ─────────────────────────────────────────────────────────

SYSCTL_CONF="/etc/sysctl.d/99-ghostnet.conf"
cat > "$SYSCTL_CONF" << 'EOF'
# GhostNet kernel tuning
fs.file-max = 2097152
fs.nr_open = 2097152
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.ip_local_port_range = 1024 65535
net.core.rmem_default = 262144
net.core.wmem_default = 262144
net.core.rmem_max = 134217728
net.core.wmem_max = 134217728
net.core.optmem_max = 65536
net.ipv4.tcp_rmem = 4096 87380 134217728
net.ipv4.tcp_wmem = 4096 65536 134217728
net.ipv4.tcp_fastopen = 3
net.ipv4.tcp_window_scaling = 1
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 15
net.ipv4.tcp_max_tw_buckets = 1440000
net.ipv4.tcp_keepalive_time = 60
net.ipv4.tcp_keepalive_intvl = 10
net.ipv4.tcp_keepalive_probes = 6
net.core.netdev_max_backlog = 65536
net.ipv4.conf.all.rp_filter = 0
net.ipv4.conf.default.rp_filter = 0
vm.max_map_count = 262144
EOF

echo "[*] Saved to $SYSCTL_CONF"

# ─── ulimit ───────────────────────────────────────────────────────────────────

LIMITS_CONF="/etc/security/limits.d/99-ghostnet.conf"
cat > "$LIMITS_CONF" << 'EOF'
# GhostNet file descriptor limits
*    soft nofile 1048576
*    hard nofile 1048576
root soft nofile 1048576
root hard nofile 1048576
EOF

echo "[*] ulimit config saved to $LIMITS_CONF"

# ─── CAP_NET_RAW capability ───────────────────────────────────────────────────

BINARY="./ghostnet"
if [ -f "$BINARY" ]; then
    setcap cap_net_raw+ep "$BINARY"
    echo "[*] CAP_NET_RAW granted to $BINARY"
else
    echo "[!] Binary not found at $BINARY — run after building:"
    echo "    sudo setcap cap_net_raw+ep ./ghostnet"
fi

# ─── BBR module ───────────────────────────────────────────────────────────────

modprobe tcp_bbr 2>/dev/null && echo "[*] BBR loaded" || echo "[!] BBR not available (using cubic)"

echo ""
echo "[✓] GhostNet kernel tuning applied."
echo "    Reboot or run 'sysctl -p $SYSCTL_CONF' to verify."
echo ""
echo "    Verify with:"
echo "      sysctl net.ipv4.tcp_congestion_control"
echo "      ulimit -n"
echo "      cat /proc/sys/net/core/somaxconn"
