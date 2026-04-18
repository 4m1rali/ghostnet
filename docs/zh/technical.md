# GhostNet 技术参考

## 什么是深度包检测（DPI）？

深度包检测（Deep Packet Inspection，DPI）是一种网络过滤技术，被互联网服务提供商、政府和企业防火墙使用。它不仅检查网络流量的源地址和目标地址，还检查网络数据包的实际内容。

### DPI 如何在 TLS 层工作

当您通过 HTTPS（端口 443）连接到网站时，您的浏览器会发送一个 TLS ClientHello 数据包。该数据包包含一个名为**服务器名称指示（SNI）**的字段——您正在连接的网站的主机名。SNI 在建立加密隧道之前以明文形式发送。

DPI 系统拦截此数据包并读取 SNI 字段。如果主机名在黑名单中，连接将被丢弃。如果在白名单（允许的域名）中，连接将被允许。

```
浏览器 → [TLS ClientHello: SNI="被封锁的网站.com"] → DPI → 封锁 ✗
浏览器 → [TLS ClientHello: SNI="hcaptcha.com"]     → DPI → 允许 ✓
```

### 为什么 SNI 是可见的

TLS 加密应用程序数据，但握手协商在建立加密之前进行。SNI 扩展（RFC 6066）的设计目的是允许单个服务器托管多个 TLS 证书——服务器需要在加密通道存在之前知道要提供哪个证书。

---

## GhostNet 如何绕过 DPI

GhostNet 利用 DPI 系统和 TCP 服务器处理数据包方式之间的根本差异。

**DPI 系统是无状态的（stateless）**——它们在不维护完整 TCP 状态的情况下检查数据包。它们从第一个携带数据的数据包中读取 SNI 并做出决定。

**TCP 服务器是有状态的（stateful）**——它们维护接收窗口，并丢弃超出窗口范围的数据包。

GhostNet 在真实 ClientHello 之前注入一个带有白名单 SNI 的伪造 TLS ClientHello，使用的序列号将数据包置于服务器接收窗口之外。DPI 系统看到白名单 SNI 并允许连接。服务器静默丢弃伪造数据包。真实 ClientHello 随后发送，连接正常建立。

### 完整数据包流程

```
客户端       GhostNet              DPI              目标服务器
  │              │                   │                    │
  │── 连接 ─────►│                   │                    │
  │              │── SYN ───────────►│───────────────────►│
  │              │◄─ SYN-ACK ────────│◄───────────────────│
  │              │── ACK ───────────►│───────────────────►│
  │              │                   │                    │
  │              │   [GhostNet 注入伪造数据包]              │
  │              │── PSH+ACK ────────►│                   │
  │              │   seq = ISN+1-N    │                   │
  │              │   SNI = "hcaptcha.com"（白名单）        │
  │              │                   │                    │
  │              │   DPI 读取 SNI:   │                    │
  │              │   "hcaptcha.com"  │                    │
  │              │   → 允许 ✓        │                    │
  │              │                   │  服务器丢弃        │
  │              │                   │  （窗口外）        │
  │              │── 真实 ClientHello ──────────────────►│
  │              │◄──────────────────────── ServerHello ──│
  │◄═════════════ 双向中继 ══════════════════════════════►│
```

---

## 绕过策略

### 1. wrong_seq（主要策略）

上述核心技术。最兼容——适用于大多数 DPI 部署。

**序列号计算：**
```
fake_seq = (ISN + 1 - len(payload)) & 0xFFFFFFFF
```

### 2. fragment（分片）

将伪造的 ClientHello 分割成多个 TCP 段。分割点是第 1 个字节——仅 TLS 记录类型字节（`0x16`）。

**为什么有效：** 许多 DPI 系统只检查第一个 TCP 段。单个字节（`0x16`）不是完整的 TLS 记录——DPI 无法从中解析 SNI。

### 3. desync — 错误校验和

发送一个 TCP 校验和故意损坏的数据包。

**为什么有效：** 一些无状态 DPI 系统不验证 TCP 校验和。目标服务器始终验证校验和并静默丢弃数据包。

### 4. desync — TTL 限制

发送 `TTL = 1` 的伪造 ClientHello。

**为什么有效：** 数据包被第一个路由器丢弃。DPI 系统通常比第一个路由器更靠近客户端，因此在数据包过期之前看到它。

### 5. adaptive（推荐）

按顺序尝试策略，并为每个 `(dstIP, dstPort)` 缓存获胜者：

```
wrong_seq → fragment → desync_bad_cs → desync_ttl
```

---

## TLS 指纹

GhostNet 从头构建完整、有效的 TLS 1.3 ClientHello 记录，与真实浏览器指纹完全匹配。

| 配置文件 | 基于 | GREASE | compress_cert |
|---|---|---|---|
| `chrome` | Chrome 124 | ✓ | ✓ |
| `firefox` | Firefox 125 | ✗ | ✗ |
| `safari` | Safari 17 | ✗ | ✗ |
| `edge` | Edge 124 | ✓ | ✓ |
| `chrome120` | Chrome 120 | ✓ | ✗ |
| `random` | 每次连接轮换 | — | — |

---

## 延迟模型

注入延迟使用双组件概率模型：

**高斯分量（92% 的连接）：**
```
延迟 = base_ms + N(0, σ)    其中 σ = base_ms × 0.30
```

**韦布尔尾部（8% 的连接）：**
```
延迟 = λ × (-ln(1-U))^(1/k) + 5ms
其中 λ=8, k=1.5, U ~ Uniform(0,1)
```

此模型模拟真实浏览器行为，使基于时序的检测更加困难。

---

## 原始套接字注入

GhostNet 手动构建完整的 IPv4 + TCP 数据包，并通过带有 `IP_HDRINCL` 的 `AF_INET SOCK_RAW` 注入。在 Linux 上需要 `CAP_NET_RAW`。

### TTL 欺骗

当 `ttl_spoof` 启用时：
```
base = 从 {64, 128} 随机选择
hops = [1, 8] 中的随机整数
TTL  = base - hops
```

这使注入的数据包看起来来自网络距离处的真实主机。
