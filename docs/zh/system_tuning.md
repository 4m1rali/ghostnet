# Linux 系统调优

GhostNet 的 `tune` 命令会自动应用所有这些设置。

## 自动应用

```bash
sudo ./ghostnet tune
```

预览而不应用：
```bash
./ghostnet tune --dry-run
```

---

## sysctl 设置

### 文件描述符

```
fs.file-max = 2097152
fs.nr_open  = 2097152
```

每个 TCP 连接使用一个文件描述符。Linux 默认限制为 65536。在 10,000 个并发连接时，至少需要 10,000 + 开销。

### TCP 连接队列

```
net.core.somaxconn            = 65535
net.ipv4.tcp_max_syn_backlog  = 65535
```

限制等待接受的连接数。必须足够大以处理连接突发而不丢失。

### 端口范围

```
net.ipv4.ip_local_port_range = 1024 65535
```

GhostNet 为每个客户端连接打开一个出站连接。将范围扩展到 1024–65535 可提供约 64,000 个同时出站连接。

### 套接字缓冲区

```
net.core.rmem_max     = 134217728
net.core.wmem_max     = 134217728
net.ipv4.tcp_rmem     = 4096 87380 134217728
net.ipv4.tcp_wmem     = 4096 65536 134217728
```

更大的缓冲区可提高高延迟或高带宽连接的吞吐量。

### TIME_WAIT 和连接重用

```
net.ipv4.tcp_tw_reuse     = 1
net.ipv4.tcp_fin_timeout  = 15
```

`tcp_tw_reuse` 允许将 TIME_WAIT 套接字重用于新的出站连接。

### BBR 拥塞控制

```
net.core.default_qdisc              = fq
net.ipv4.tcp_congestion_control     = bbr
```

BBR 是 Google 的拥塞控制算法。与默认的 CUBIC 相比，它实现了更高的吞吐量和更低的延迟。

### rp_filter（原始套接字注入所必需）

```
net.ipv4.conf.all.rp_filter     = 0
net.ipv4.conf.default.rp_filter = 0
```

反向路径过滤会丢弃源 IP 与预期接口不匹配的数据包。GhostNet 注入以本地机器 IP 为源的数据包——如果启用 rp_filter，内核会丢弃这些数据包。**此设置是绕过注入工作所必需的。**

---

## ulimit 设置

写入 `/etc/security/limits.d/99-ghostnet.conf`：

```
*    soft nofile 1048576
*    hard nofile 1048576
```

对于当前会话：
```bash
ulimit -n 1048576
```

---

## 验证

```bash
sysctl net.ipv4.tcp_congestion_control
ulimit -n
cat /proc/sys/net/core/somaxconn
sysctl net.ipv4.conf.all.rp_filter
getcap ./ghostnet
```
