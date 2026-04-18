# 构建和安装 GhostNet

## 系统要求

| 要求 | 版本 | 说明 |
|---|---|---|
| 操作系统 | Ubuntu 20.04+ / Debian 11+ | Linux 内核 5.4+ |
| Go | 1.22+ | 从源码构建时需要 |
| 能力 | `CAP_NET_RAW` | 原始套接字注入所需 |

---

## 获取二进制文件（推荐）

```bash
wget https://github.com/4m1rali/ghostnet/releases/latest/download/ghostnet-linux-amd64
chmod +x ghostnet-linux-amd64
sudo setcap cap_net_raw+ep ghostnet-linux-amd64
```

---

## 从源码构建

```bash
git clone https://github.com/4m1rali/ghostnet
cd ghostnet
go mod download
go build -ldflags="-s -w" -o ghostnet ./cmd/ghostnet/
sudo setcap cap_net_raw+ep ./ghostnet
```

---

## 从 Windows 交叉编译

在 `ghostnet` 文件夹中打开 PowerShell：

```powershell
# Linux amd64（大多数 VPS 服务器）
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -ldflags="-s -w" -o ghostnet-linux-amd64 ./cmd/ghostnet/

# Linux arm64
$env:GOOS="linux"; $env:GOARCH="arm64"; $env:CGO_ENABLED="0"
go build -ldflags="-s -w" -o ghostnet-linux-arm64 ./cmd/ghostnet/
```

传输到服务器：
```powershell
scp ghostnet-linux-amd64 root@服务器IP:/root/ghostnet
```

---

## 服务器首次设置

```bash
chmod +x /root/ghostnet
sudo setcap cap_net_raw+ep /root/ghostnet

# 完整自动设置
sudo /root/ghostnet setup
```

`setup` 命令完成所有工作：调优内核、扫描 SNI 域名、创建 config.json 并启动代理。

---

## 手动设置（逐步）

```bash
# 步骤 1：调优内核
sudo ./ghostnet tune

# 步骤 2：扫描 SNI 域名并写入 config
./ghostnet scan -w -c config.json

# 步骤 3：运行
./ghostnet run -c config.json
```

---

## systemd 服务

```bash
cat > /etc/systemd/system/ghostnet.service << 'EOF'
[Unit]
Description=GhostNet DPI Bypass Proxy
After=network.target

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

systemctl daemon-reload
systemctl enable ghostnet
systemctl start ghostnet
journalctl -u ghostnet -f
```

---

## 命令参考

| 命令 | 说明 |
|---|---|
| `tune` | 优化 Linux 内核设置 |
| `scan` | 扫描 SNI 域名 |
| `scan -f sni.txt` | 从文件扫描 |
| `scan -w -c config.json` | 扫描并写入 config |
| `setup` | 完整自动设置 |
| `run -c config.json` | 启动代理 |

---

## 故障排除

| 错误 | 原因 | 解决方案 |
|---|---|---|
| `operation not permitted` | 缺少 `CAP_NET_RAW` | `sudo setcap cap_net_raw+ep ./ghostnet` |
| `address already in use` | 端口被占用 | 修改 config.json 中的 `listen_port` |
| `connect_ip required` | config 中没有 IP | 运行 `scan -w` |
| `invalid character '\n'` | Windows 行尾 | `sed -i 's/\r//' config.json` |
