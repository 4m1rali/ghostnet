# Сборка и установка GhostNet

## Требования

| Требование | Версия | Примечания |
|---|---|---|
| ОС | Ubuntu 20.04+ / Debian 11+ | Ядро Linux 5.4+ |
| Go | 1.22+ | Для сборки из исходников |
| Возможность | `CAP_NET_RAW` | Для внедрения через raw-сокет |

---

## Получение бинарного файла (рекомендуется)

```bash
wget https://github.com/4m1rali/ghostnet/releases/latest/download/ghostnet-linux-amd64
chmod +x ghostnet-linux-amd64
sudo setcap cap_net_raw+ep ghostnet-linux-amd64
```

---

## Сборка из исходников

```bash
git clone https://github.com/4m1rali/ghostnet
cd ghostnet
go mod download
go build -ldflags="-s -w" -o ghostnet ./cmd/ghostnet/
sudo setcap cap_net_raw+ep ./ghostnet
```

---

## Кросс-компиляция из Windows

В PowerShell в папке `ghostnet`:

```powershell
# Linux amd64 (большинство VPS-серверов)
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -ldflags="-s -w" -o ghostnet-linux-amd64 ./cmd/ghostnet/

# Linux arm64
$env:GOOS="linux"; $env:GOARCH="arm64"; $env:CGO_ENABLED="0"
go build -ldflags="-s -w" -o ghostnet-linux-arm64 ./cmd/ghostnet/
```

Передача на сервер:
```powershell
scp ghostnet-linux-amd64 root@IP_СЕРВЕРА:/root/ghostnet
```

---

## Первоначальная настройка на сервере

```bash
chmod +x /root/ghostnet
sudo setcap cap_net_raw+ep /root/ghostnet

# Полная автоматическая настройка
sudo /root/ghostnet setup
```

Команда `setup` делает всё: настраивает ядро, сканирует SNI-домены, создаёт config.json и запускает прокси.

---

## Ручная настройка (шаг за шагом)

```bash
# Шаг 1: Настройка ядра
sudo ./ghostnet tune

# Шаг 2: Сканирование SNI-доменов и запись в config
./ghostnet scan -w -c config.json

# Шаг 3: Запуск
./ghostnet run -c config.json
```

---

## Сервис systemd

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

## Справочник команд

| Команда | Описание |
|---|---|
| `tune` | Оптимизация настроек ядра Linux |
| `scan` | Сканирование SNI-доменов |
| `scan -f sni.txt` | Сканирование из файла |
| `scan -w -c config.json` | Сканирование и запись в config |
| `setup` | Полная автоматическая настройка |
| `run -c config.json` | Запуск прокси |

---

## Устранение неполадок

| Ошибка | Причина | Решение |
|---|---|---|
| `operation not permitted` | Нет `CAP_NET_RAW` | `sudo setcap cap_net_raw+ep ./ghostnet` |
| `address already in use` | Порт занят | Измените `listen_port` в config.json |
| `connect_ip required` | Нет IP в config | Запустите `scan -w` |
| `invalid character '\n'` | Окончания строк Windows | `sed -i 's/\r//' config.json` |
