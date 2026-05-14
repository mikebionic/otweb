#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PID_FILE="$SCRIPT_DIR/.otapi-hub.pid"
LOG_FILE="$SCRIPT_DIR/.otapi-hub.log"

# Проверяем не запущен ли уже
if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
  echo "OTAPI Hub уже запущен (PID: $(cat "$PID_FILE"))"
  echo "Открыть: http://localhost:5500"
  exit 0
fi

echo "==> Запускаем MySQL контейнер..."
cd "$SCRIPT_DIR"
docker compose up -d mysql

echo "==> Ждём готовности MySQL..."
for i in $(seq 1 30); do
  if docker exec otapi_mysql mysqladmin ping -u otapi -potapi_pass --silent 2>/dev/null; then
    echo "    MySQL готов"
    break
  fi
  printf "."
  sleep 1
done

echo "==> Запускаем OTAPI Hub..."
cd "$SCRIPT_DIR"
go run . >> "$LOG_FILE" 2>&1 &
echo $! > "$PID_FILE"

sleep 1

if kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
  echo ""
  echo "OTAPI Hub запущен!"
  echo "  URL:     http://localhost:5500"
  echo "  Adminer: http://localhost:8082"
  echo "  Лог:     $LOG_FILE"
  echo "  PID:     $(cat "$PID_FILE")"
else
  echo "ОШИБКА: не удалось запустить. Смотри лог: $LOG_FILE"
  tail -20 "$LOG_FILE"
  exit 1
fi
