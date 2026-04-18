# GhostNet — Ubuntu Server Tuning

## sysctl

```bash
sudo tee /etc/sysctl.d/99-ghostnet.conf << 'EOF'
fs.file-max = 2097152
fs.nr_open = 2097152
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.ip_local_port_range = 1024 65535
net.core.rmem_default = 262144
net.core.wmem_default = 262144
net.core.rmem_max = 134217728
net.core.wmem_max = 134217728
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
sudo sysctl -p /etc/sysctl.d/99-ghostnet.conf
```

## ulimit

```bash
sudo tee /etc/security/limits.d/99-ghostnet.conf << 'EOF'
*    soft nofile 1048576
*    hard nofile 1048576
root soft nofile 1048576
root hard nofile 1048576
EOF
```

## CAP_NET_RAW

```bash
sudo setcap cap_net_raw+ep ./ghostnet
```

## BBR

```bash
echo tcp_bbr | sudo tee -a /etc/modules-load.d/modules.conf
sudo modprobe tcp_bbr
echo "net.core.default_qdisc=fq" | sudo tee -a /etc/sysctl.d/99-ghostnet.conf
echo "net.ipv4.tcp_congestion_control=bbr" | sudo tee -a /etc/sysctl.d/99-ghostnet.conf
sudo sysctl -p /etc/sysctl.d/99-ghostnet.conf
```

## Verify

```bash
sysctl net.ipv4.tcp_congestion_control
ulimit -n
cat /proc/sys/net/core/somaxconn
getcap ./ghostnet
```

## systemd service

```ini
[Unit]
Description=GhostNet DPI Bypass Proxy
After=network.target

[Service]
Type=simple
User=ghostnet
ExecStart=/opt/ghostnet/ghostnet run -c /opt/ghostnet/config.json
Restart=on-failure
RestartSec=3
LimitNOFILE=1048576
AmbientCapabilities=CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_RAW

[Install]
WantedBy=multi-user.target
```
