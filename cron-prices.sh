#!/bin/bash
# Запускает обновление цен через HTTP API
# Использовать в cron: каждые 6 часов
# 0 */6 * * * /path/to/otapi-hub/cron-prices.sh

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HUB_URL="http://localhost:5500"
LOG_FILE="$SCRIPT_DIR/.cron-prices.log"

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Запуск обновления цен..." >> "$LOG_FILE"

# Получаем включённые категории из БД
CATS=$(docker exec otapi_mysql mysql -u otapi -potapi_pass otapi_hub -sN \
  -e "SELECT category_id FROM category_config WHERE enabled=1;" 2>/dev/null)

if [ -z "$CATS" ]; then
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] Нет включённых категорий" >> "$LOG_FILE"
  exit 0
fi

while IFS= read -r cat_id; do
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] Обновляем цены: $cat_id" >> "$LOG_FILE"
  curl -s -X POST -d "category_id=$cat_id" "$HUB_URL/sync/prices" > /dev/null
  sleep 5
done <<< "$CATS"

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Готово" >> "$LOG_FILE"
