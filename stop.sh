#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PID_FILE="$SCRIPT_DIR/.otapi-hub.pid"

# Останавливаем Go сервер
if [ -f "$PID_FILE" ]; then
  PID=$(cat "$PID_FILE")
  if kill -0 "$PID" 2>/dev/null; then
    echo "==> Останавливаем OTAPI Hub (PID: $PID)..."
    kill "$PID"
    sleep 1
    # Убиваем дочерние go run процессы если остались
    pkill -f "otapi-hub" 2>/dev/null || true
    echo "    Остановлен"
  else
    echo "    Процесс $PID уже не запущен"
  fi
  rm -f "$PID_FILE"
else
  # На всякий случай убиваем по порту
  PID=$(lsof -ti:5500 2>/dev/null)
  if [ -n "$PID" ]; then
    echo "==> Останавливаем процесс на порту 5500 (PID: $PID)..."
    kill "$PID"
    echo "    Остановлен"
  else
    echo "OTAPI Hub не запущен"
  fi
fi

# Останавливаем Docker контейнер
read -p "Остановить MySQL контейнер? (y/N) " confirm
if [[ "$confirm" =~ ^[Yy]$ ]]; then
  cd "$SCRIPT_DIR"
  docker compose stop mysql
  echo "MySQL контейнер остановлен"
fi
