#!/bin/bash
# Deploy script for otapi-hub with price filter

set -e

echo "[*] Building otapi-hub..."
go build -o otapi-hub

echo "[*] Copying to /opt/otapi-hub-src/..."
sudo cp otapi-hub /opt/otapi-hub-src/otapi-hub

echo "[*] Restarting systemd service..."
sudo systemctl restart otapi-hub

echo "[*] Checking status..."
sleep 2
sudo systemctl status otapi-hub --no-pager

echo ""
echo "✓ Deploy successful!"
echo ""
echo "Changes deployed:"
echo "  - Added MaxPriceLimit filter to SyncOptions"
echo "  - Default limit: 1000 CNY (configurable via form)"
echo "  - Items exceeding limit are skipped with WARNING log"
echo ""
