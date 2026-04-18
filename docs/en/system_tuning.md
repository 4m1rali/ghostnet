# Linux System Tuning

GhostNet's `tune` command applies all of these automatically. This document explains what each setting does and why it matters.

---

## Apply automatically

```bash
sudo ./ghostnet tune
```

To preview without applying:
```bash
./ghostnet tune --dry-run
```

---

## sysctl settings

### File descriptors

```
fs.file-max = 2097152
fs.nr_open  = 2097152
```

Each TCP connection uses one file descriptor. The default Linux limit is 65536. At 10,000 concurrent connections you need at least 10,000 + overhead. Setting to 2M gives headroom for any load.

### TCP connection backlog

```
net.core.somaxconn            = 65535
net.ipv4.tcp_max_syn_backlog  = 65535
```

`somaxconn` limits the number of connections waiting to be accepted. `tcp_max_syn_backlog` limits the SYN queue. Both must be large enough to handle connection bursts without dropping.

### Port range

```
net.ipv4.ip_local_port_range = 1024 65535
```

GhostNet opens one outbound connection per client connection. The default range (32768–60999) gives ~28,000 simultaneous outbound connections. Expanding to 1024–65535 gives ~64,000.

### Socket buffers

```
net.core.rmem_default = 262144
net.core.wmem_default = 262144
net.core.rmem_max     = 134217728
net.core.wmem_max     = 134217728
net.ipv4.tcp_rmem     = 4096 87380 134217728
net.ipv4.tcp_wmem     = 4096 65536 134217728
```

Larger buffers improve throughput on high-latency or high-bandwidth connections. The kernel auto-tunes within the min/default/max range.

### TIME_WAIT and connection reuse

```
net.ipv4.tcp_tw_reuse     = 1
net.ipv4.tcp_fin_timeout  = 15
net.ipv4.tcp_max_tw_buckets = 1440000
```

After a connection closes, it enters TIME_WAIT for 2× MSL (typically 60 seconds). Under high connection rates, TIME_WAIT sockets accumulate and exhaust the port range. `tcp_tw_reuse` allows reusing TIME_WAIT sockets for new outbound connections. `tcp_fin_timeout` reduces the FIN_WAIT_2 timeout.

### TCP Fast Open

```
net.ipv4.tcp_fastopen = 3
```

Allows data to be sent in the SYN packet, reducing connection latency by one round trip. Value 3 enables both client and server mode.

### BBR congestion control

```
net.core.default_qdisc              = fq
net.ipv4.tcp_congestion_control     = bbr
```

BBR (Bottleneck Bandwidth and Round-trip propagation time) is Google's congestion control algorithm. It achieves higher throughput and lower latency than the default CUBIC, especially on lossy or high-latency links.

### rp_filter (required for raw socket injection)

```
net.ipv4.conf.all.rp_filter     = 0
net.ipv4.conf.default.rp_filter = 0
```

Reverse path filtering drops packets where the source IP does not match the expected interface. GhostNet injects packets with the local machine's IP as source — the kernel would drop these if rp_filter is enabled. **This setting is required for bypass injection to work.**

### Virtual memory

```
vm.max_map_count = 262144
```

Required for large Go heaps. The Go runtime uses memory-mapped regions for goroutine stacks.

---

## ulimit settings

Written to `/etc/security/limits.d/99-ghostnet.conf`:

```
*    soft nofile 1048576
*    hard nofile 1048576
root soft nofile 1048576
root hard nofile 1048576
```

These take effect on next login. For the current session:
```bash
ulimit -n 1048576
```

---

## Persistence

Settings are written to `/etc/sysctl.d/99-ghostnet.conf` and applied with `sysctl -p`. They persist across reboots.

---

## Verify

```bash
# Check congestion control
sysctl net.ipv4.tcp_congestion_control

# Check file descriptor limit
ulimit -n

# Check connection backlog
cat /proc/sys/net/core/somaxconn

# Check rp_filter
sysctl net.ipv4.conf.all.rp_filter

# Check raw socket capability
getcap ./ghostnet
```

---

## systemd service limits

The systemd service file includes:
```ini
LimitNOFILE=1048576
AmbientCapabilities=CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_RAW
NoNewPrivileges=yes
```

This grants `CAP_NET_RAW` to the process without running as root, and sets the file descriptor limit independently of the system ulimit.
