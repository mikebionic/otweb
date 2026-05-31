#!/bin/zsh
set -e

SERVER="root@95.181.224.97"
REMOTE="/opt/otapi-hub-src"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "[1/4] Building frontend..."
cd "$SCRIPT_DIR/frontend"
npm run build

echo "[2/4] Building Go binary..."
cd "$SCRIPT_DIR"
GOOS=linux GOARCH=amd64 go build -o otapi-hub

echo "[3/4] Uploading to $SERVER..."
scp "$SCRIPT_DIR/otapi-hub" "$SERVER:$REMOTE/otapi-hub.new"
rsync -az --delete "$SCRIPT_DIR/frontend/dist/" "$SERVER:$REMOTE/frontend/dist/"
ssh "$SERVER" "mv $REMOTE/otapi-hub.new $REMOTE/otapi-hub && chmod +x $REMOTE/otapi-hub"

echo "[4/4] Restarting service..."
ssh "$SERVER" "systemctl restart otapi-hub && sleep 2 && systemctl status otapi-hub --no-pager | head -8"

echo "\nDone!"
