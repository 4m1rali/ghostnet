# ساخت و نصب GhostNet

<div dir="rtl">

## پیش‌نیازها

| پیش‌نیاز | نسخه | توضیحات |
|---|---|---|
| سیستم‌عامل | Ubuntu 20.04+ / Debian 11+ | کرنل لینوکس 5.4+ |
| Go | 1.22+ | برای ساخت از سورس |
| قابلیت | `CAP_NET_RAW` | برای تزریق سوکت خام |

---

## دریافت باینری (توصیه شده)

```bash
wget https://github.com/4m1rali/ghostnet/releases/latest/download/ghostnet-linux-amd64
chmod +x ghostnet-linux-amd64
sudo setcap cap_net_raw+ep ghostnet-linux-amd64
```

---

## ساخت از سورس

```bash
git clone https://github.com/4m1rali/ghostnet
cd ghostnet
go mod download
go build -ldflags="-s -w" -o ghostnet ./cmd/ghostnet/
sudo setcap cap_net_raw+ep ./ghostnet
```

---

## کامپایل متقاطع از ویندوز

در PowerShell در پوشه `ghostnet`:

```powershell
# لینوکس amd64 (اکثر سرورهای VPS)
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -ldflags="-s -w" -o ghostnet-linux-amd64 ./cmd/ghostnet/

# لینوکس arm64
$env:GOOS="linux"; $env:GOARCH="arm64"; $env:CGO_ENABLED="0"
go build -ldflags="-s -w" -o ghostnet-linux-arm64 ./cmd/ghostnet/
```

انتقال به سرور:
```powershell
scp ghostnet-linux-amd64 root@IP_سرور:/root/ghostnet
```

---

## راه‌اندازی اولیه روی سرور

```bash
chmod +x /root/ghostnet
sudo setcap cap_net_raw+ep /root/ghostnet

# راه‌اندازی کامل خودکار
sudo /root/ghostnet setup
```

دستور `setup` همه کارها را انجام می‌دهد: بهینه‌سازی کرنل، اسکن دامنه‌های SNI، ایجاد config.json و راه‌اندازی پروکسی.

---

## راه‌اندازی دستی (مرحله به مرحله)

```bash
# مرحله ۱: بهینه‌سازی کرنل
sudo ./ghostnet tune

# مرحله ۲: اسکن دامنه‌های SNI و نوشتن در config
./ghostnet scan -w -c config.json

# مرحله ۳: اجرا
./ghostnet run -c config.json
```

---

## سرویس systemd

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

## مرجع دستورات

| دستور | توضیحات |
|---|---|
| `tune` | بهینه‌سازی تنظیمات کرنل لینوکس |
| `scan` | اسکن دامنه‌های SNI |
| `scan -f sni.txt` | اسکن از فایل |
| `scan -w -c config.json` | اسکن و نوشتن در config |
| `setup` | راه‌اندازی کامل خودکار |
| `run -c config.json` | اجرای پروکسی |

---

## رفع اشکال

| خطا | علت | راه‌حل |
|---|---|---|
| `operation not permitted` | `CAP_NET_RAW` ندارد | `sudo setcap cap_net_raw+ep ./ghostnet` |
| `address already in use` | پورت اشغال است | `listen_port` را در config.json تغییر دهید |
| `connect_ip required` | IP در config نیست | `scan -w` را اجرا کنید |
| `invalid character '\n'` | پایان خط ویندوز | `sed -i 's/\r//' config.json` |

</div>
