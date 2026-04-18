#!/bin/bash
# install.sh — Build and install GhostNet on Ubuntu/Debian
set -euo pipefail

echo "[*] GhostNet installer"

# Check Go
if ! command -v go &>/dev/null; then
    echo "[!] Go not found. Installing Go 1.22..."
    wget -q https://go.dev/dl/go1.22.2.linux-amd64.tar.gz
    sudo tar -C /usr/local -xzf go1.22.2.linux-amd64.tar.gz
    export PATH=$PATH:/usr/local/go/bin
    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
fi

echo "[*] Go version: $(go version)"

# Build
echo "[*] Building GhostNet..."
go mod download
go build -ldflags="-s -w" -o ghostnet ./cmd/ghostnet/

echo "[*] Binary: ./ghostnet"
echo "[*] Size: $(du -sh ghostnet | cut -f1)"

# Apply kernel tuning
echo "[*] Applying kernel tuning (requires sudo)..."
sudo bash scripts/sysctl-tune.sh

echo ""
echo "[✓] GhostNet installed."
echo ""
echo "    Quick start:"
echo "      cp config.example.json config.json"
echo "      # Edit config.json: set connect_ip"
echo "      ./ghostnet run -c config.json"
echo ""
echo "    Generate config:"
echo "      ./ghostnet gen-config -o config.json"
