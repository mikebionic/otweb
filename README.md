# OTAPI Hub

Коннектор OT Commerce (Taobao/JD/Poizon) -> CS-Cart (wabrum.com).
Забирает товары из китайских маркетплейсов, переводит на 3 языка через DeepSeek, публикует в интернет-магазин с вариациями (размер+цвет), ценами и остатками.

## Быстрый старт

```bash
# 1. Поднять MySQL (Docker)
docker-compose up -d

# 2. Скопировать конфигурацию
cp config.yaml.example config.yaml
# Заполнить API ключи в config.yaml

# 3. Запустить
make run
# или: go run .

# 4. Открыть http://localhost:5500/otweb/
```

### Деплой на сервер

```bash
# Сборка Linux бинарника
make build
# -> otapi-hub-linux

# Загрузка (через gzip для надёжности)
gzip -k otapi-hub-linux
scp otapi-hub-linux.gz root@server:/tmp/
ssh root@server "gunzip /tmp/otapi-hub-linux.gz && cp /tmp/otapi-hub-linux /opt/otapi-hub-src/otapi-hub && chmod +x /opt/otapi-hub-src/otapi-hub && systemctl restart otapi-hub"
```

### Серверная конфигурация

```
Путь:    /opt/otapi-hub-src/
Сервис:  systemctl status otapi-hub
Логи:    journalctl -u otapi-hub -f
Конфиг:  /opt/otapi-hub-src/config.yaml
Nginx:   /etc/nginx/vhosts-resources/wabrum.com/otweb.conf
URL:     https://wabrum.com/otweb/
```

---

## Архитектура

```
OT Commerce API (otapi.net/service-json)
  |
  | 1. BatchSearchItemsFrame (100 товаров/запрос, фильтры)
  | 2. GetItemFullInfo (1 товар/запрос, кеш 7 дней)
  v
OTAPI Hub (Go, порт 5500) -- MySQL otapi_hub (локальная БД)
  |
  | 3. DeepSeek: перевод RU/EN/TK + описания + характеристики
  |
  | 4. CS-Cart REST API:
  |    POST /api/products       (товар + фото, status=D)
  |    POST /api/options         (размер/цвет с картинками и ценами)
  |    PUT  /api/products/{id}   (характеристики, обновление)
  |    INSERT cscart_product_options_inventory (комбинации SKU)
  v
CS-Cart (wabrum.com) -- MySQL wabrum_mv (mirror)
  Продавец: WABRUM Commerce (company_id=376)
  Все товары: status=D (Hidden), avail_since=+7 дней
```

---

## Структура файлов

```
otapi-hub/
├── main.go                     # HTTP сервер, 30+ маршрутов, все хэндлеры
├── config/
│   ├── config.go               # YAML конфигурация, структуры, Default()
│   └── config_test.go
├── otapi/                      # OT Commerce Legacy API клиент
│   ├── client.go               # GetCatalog, SearchProducts, GetProduct
│   ├── models.go               # SearchItem, ProductItem, SKU, Location, Price
│   └── client_test.go
├── cscart/                     # CS-Cart REST API клиент
│   ├── client.go               # CreateProduct, UpdateProduct, CreateOptionAdvanced
│   ├── models.go               # ProductInput, ProductUpdate, NormalizeSize
│   └── client_test.go
├── db/                         # MySQL (две базы: otapi_hub + wabrum_mv)
│   ├── db.go                   # Store{Hub, Mirror} - два подключения
│   ├── otapi_repo.go           # Product CRUD, фильтры, маппинги, наценки
│   └── migrations/
│       ├── 001_init.sql        # Полная схема (12 таблиц)
│       ├── 002_detail_sync.sql # detail_fetched_at
│       ├── 003_v4.sql          # enabled, settings, cs_categories
│       ├── 004_descriptions.sql # description_ru/en/tk, weight_estimated
│       └── 005_location.sql    # location_city, location_state
├── sync/                       # Синхронизация OTAPI -> Hub DB
│   ├── importer.go             # SyncProducts (2 фазы), SyncPricesOnly
│   └── importer_test.go
├── push/                       # Push из Hub DB -> CS-Cart
│   └── api_pusher.go           # PushSingleProduct, комбинации, price modifiers
├── translate/                  # DeepSeek AI перевод
│   ├── deepseek.go             # API клиент, Normalize()
│   └── prompt.go               # Промпт с enum-значениями характеристик
├── web/templates/              # HTML шаблоны (Bootstrap 5, embedded)
│   ├── layout.html             # Sidebar + topbar
│   ├── dashboard.html          # Статистика
│   ├── categories.html         # Категории OT Commerce
│   ├── products.html           # Список товаров (сетка, фильтры, bulk actions)
│   ├── product_detail.html     # Карточка товара (SKU, фото, переводы)
│   ├── sync.html               # Синхронизация с фильтрами и live-логом
│   ├── push.html               # Отправленные / готовые к отправке
│   ├── mapping.html            # Маппинг категорий OT -> CS-Cart
│   └── settings.html           # API ключи, цены, доставка, cron
├── config.yaml.example         # Шаблон конфигурации (без ключей)
├── docker-compose.yml          # MySQL 8.4 + Adminer (dev)
├── Makefile                    # run, build, test, deploy
├── start.sh / stop.sh          # Скрипты запуска
└── otapi-hub.postman_collection.json  # 33 запроса Postman
```

---

## Конфигурация

### config.yaml

```yaml
server:
  port: "5500"

database:
  hub_dsn: "user:pass@tcp(host:port)/otapi_hub?collation=utf8mb4_unicode_ci&parseTime=true"
  mirror_dsn: "user:pass@tcp(host:port)/wabrum_mv?collation=utf8mb4_unicode_ci&parseTime=true"

otapi:
  instance_key: "YOUR_KEY"
  base_url: "https://rest.otapi.net"
  legacy_url: "https://otapi.net/service-json"

cscart:
  base_url: "https://wabrum.com"
  email: "api@wabrum.com"
  api_key: "YOUR_KEY"
  company_id: 376

deepseek:
  api_key: "YOUR_KEY"
  base_url: "https://api.deepseek.com"

pricing:
  default_markup_pct: 35.0
  exchange_rate_cny: 0.57
```

### Настройки через веб-интерфейс (/otweb/settings)

Сохраняются в таблицу `settings`, не требуют рестарта:

| Настройка | Описание |
|-----------|----------|
| Курс CNY -> TMT | Обменный курс (по умолчанию 2.74) |
| Наценка % | Процент наценки (по умолчанию 35%) |
| Фиксированная надбавка | Плоская сумма к цене (TMT) |
| Доставка $/кг | Стоимость авиадоставки из Китая ($7/кг) |
| Курс USD/CNY | Для конвертации стоимости доставки (7.3) |
| Включить доставку в цену | Вкл/выкл |
| AI промпт | Кастомный промпт для DeepSeek |
| Провайдеры | Вкл/выкл Taobao, JD, Poizon |
| Cron расписание | Автосинк цен и товаров |

---

## HTTP маршруты

Все маршруты под префиксом `/otweb/`.

### Просмотр

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/` | Dashboard - статистика, последние синки |
| GET | `/categories` | Список категорий OT Commerce |
| GET | `/products` | Список товаров (фильтры, пагинация, bulk actions) |
| GET | `/products/{id}` | Детальная карточка товара |
| GET | `/sync` | Страница синхронизации + история |
| GET | `/push` | Отправленные и готовые к push товары |
| GET | `/mapping` | Маппинг категорий OT -> CS-Cart |
| GET | `/settings` | Все настройки |

### Действия с товарами

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/products/{id}/push` | Push/update одного товара в CS-Cart |
| POST | `/products/{id}/translate` | AI-перевод или ручной ввод (action=deepseek/manual) |
| POST | `/products/{id}/toggle-enabled` | Включить/скрыть товар |
| POST | `/products/bulk-action` | Массовое действие (push/translate/enable/disable) |
| POST | `/products/bulk-translate` | AI-перевод всех непереведённых (фон) |

### Синхронизация

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/sync/run` | Запуск синка категории (фон) |
| GET | `/sync/log/{id}` | JSON лог синка в реальном времени |
| POST | `/sync/prices` | Обновление только цен (без деталей) |

### Маппинг и Push

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/mapping/add` | Добавить маппинг OT -> CS-Cart категория |
| POST | `/mapping/delete` | Удалить маппинг |
| POST | `/mapping/refresh-cscart` | Обновить список CS-Cart категорий |
| POST | `/push/api` | Push всех товаров категории в CS-Cart |

### Настройки

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/settings/keys` | Сохранить API ключи |
| POST | `/settings/product` | Статус, Company ID |
| POST | `/settings/pricing` | Курс, наценка, надбавка |
| POST | `/settings/delivery` | Доставка ($/кг, вкл/выкл) |
| POST | `/settings/prompt` | Кастомный DeepSeek промпт |
| POST | `/settings/providers` | Включить/выключить провайдеров |
| POST | `/settings/cron` | Расписание автосинка |

---

## Потоки данных

### 1. Синхронизация товаров (двухфазная)

```
POST /otweb/sync/run
  category_id=otc-3  max_products=100  min_volume=500

Фаза 1 - SearchProducts (дешёвая, 100 товаров/запрос):
  -> BatchSearchItemsFrame с XML фильтрами
  -> Сохраняет: title, price_cny, qty, main_image, vendor, brand, sales, location
  -> 100 товаров = 1 API запрос

Фаза 2 - GetItemFullInfo (дорогая, 1 товар/запрос, кеш 7 дней):
  -> Только для товаров БЕЗ detail_fetched_at или старше 7 дней
  -> Сохраняет: description, weight, все SKU (qty+price), все фото, все атрибуты
  -> 100 новых товаров = 100 API запросов

Стоимость:
  Первый раз: 1 + 100 = 101 запрос
  Повторно:   1 + 0   = 1 запрос (всё в кеше)
```

### 2. Push товара в CS-Cart

```
POST /otweb/products/{id}/push

1. Загрузка данных из Hub DB
2. DeepSeek нормализация (~3 сек, ~$0.001):
   -> Названия RU/EN/TK
   -> Описания RU/EN/TK (2-3 предложения)
   -> Характеристики (цвет, ткань, модель...)
3. Создание/обновление товара в CS-Cart:
   -> POST /api/products (новый) или PUT /api/products/{id} (обновление)
   -> status=D (Hidden), company_id=376
   -> avail_since = +7 дней (предзаказ)
   -> Фото: main + до 5 дополнительных
4. Опция "Размер":
   -> CreateOptionAdvanced с price modifiers
   -> NormalizeSize: "S подходит для 85-105 фунтов" -> "S"
5. Опция "Цвет":
   -> CreateOptionAdvanced с картинками и price modifiers
   -> image_url из product_attrs
6. SKU комбинации:
   -> INSERT INTO cscart_product_options_inventory
   -> CRC32 hash для combination_hash
   -> tracking='O' (отслеживание по опциям)
7. Характеристики:
   -> featureMap: "Цвет"->567, "Ткань"->563, "Модель"->574...
   -> ResolveFeatureVariant -> UpdateProductFeatures

Время: ~10-15 сек/товар
```

### 3. AI-перевод (DeepSeek)

```
Один запрос DeepSeek = один товар:

Вход:
  - Оригинальное название (CN)
  - Машинный перевод (RU)
  - Атрибуты (is_configurator=0)
  - Цвета вариаций

Выход (JSON):
  - title_ru, title_en, title_tk
  - description_ru, description_en, description_tk
  - keywords_ru, keywords_en
  - color, fabric, material, occasion, thickness, pattern
  - leg_type, model, waist_height, country, hood

Стоимость: ~$0.001/товар, ~3 сек
```

### 4. Обновление цен (дешёвый синк)

```
POST /otweb/sync/prices

-> SearchProducts постранично (frameSize=20)
-> UPDATE price_cny, price_tmt WHERE otapi_id=?
-> Пропускает disabled товары
-> 1000 товаров = 50 запросов, ~2 мин
```

---

## Таблицы базы данных

### otapi_hub (основная)

| Таблица | Назначение | Ключевые поля |
|---------|-----------|---------------|
| `products` | Товары (28+ колонок) | otapi_id, provider, title_ru/en/tk, price_cny/tmt, cs_product_id, location_city/state, description_ru/en/tk |
| `product_skus` | SKU варианты | product_id, sku_id, quantity, price_cny, configurators (JSON) |
| `product_images` | Фотографии | product_id, url, is_main, position |
| `product_attrs` | Атрибуты | product_id, pid, vid, property_name, value, is_configurator, image_url |
| `categories` | OT Commerce категории | id, provider, name_ru, parent_id, item_count |
| `category_config` | Настройки синка | category_id, enabled, max_products, last_synced_at |
| `category_map` | Маппинг OT -> CS-Cart | otapi_category_id, cs_category_id |
| `markup_rules` | Наценки (3 уровня) | scope_type (global/category/product), markup_pct, exchange_rate, fixed_addon |
| `sync_jobs` | История синков | job_type, status, items_processed, log_text (real-time) |
| `settings` | Key-value настройки | key_name, value |
| `color_map` | Маппинг цветов CN->CS | otapi_value, cs_variant_id |
| `size_map` | Маппинг размеров | otapi_value (XS-5XL), cs_variant_id |

### wabrum_mv (CS-Cart mirror, read + limited write)

| Таблица | Доступ | Назначение |
|---------|--------|-----------|
| `cscart_product_options_inventory` | SELECT, INSERT, UPDATE | SKU комбинации |
| `cscart_products` | SELECT, UPDATE | tracking='O' |
| Остальные | SELECT only | Категории, описания, цены |

---

## Ценообразование

### Формула

```
price_TMT = price_CNY * exchange_rate * (1 + markup_pct / 100) + fixed_addon
```

### Трёхуровневая иерархия (markup_rules)

1. **Товар** (scope_type='product') - наивысший приоритет
2. **Категория** (scope_type='category') - средний
3. **Глобальный** (scope_type='global') - по умолчанию

### Price modifiers на вариациях

Если SKU имеют разные цены (price_cny в product_skus), для каждого размера/цвета вычисляется modifier:

```
modifier = (avg_sku_price_CNY * exchange_rate) - base_price_TMT
```

Modifier передаётся в CS-Cart через `CreateOptionAdvanced` как абсолютная надбавка к базовой цене.

### Доставка (планируется)

```
price_TMT = (price_CNY + weight_kg * delivery_per_kg_USD * usd_to_cny) * exchange_rate * (1 + markup%) + addon
```

Настройки: delivery_cost_per_kg ($7), usd_to_cny (7.3), delivery_included (вкл/выкл).

---

## OT Commerce API

### Используемые методы

| Метод | Тип | Назначение |
|-------|-----|-----------|
| `GetRootCategoryInfoList` | Бесплатный | 122 корневых категории |
| `BatchSearchItemsFrame` | Платный | Поиск товаров (до 100/запрос) |
| `GetItemFullInfo` | Платный | Полная карточка товара |
| `GetCallStatistics` | Бесплатный | Статистика вызовов |
| `GetEnabledFeatures` | Бесплатный | Доступные провайдеры |

### Фильтры поиска (XML параметры)

```xml
<SearchItemsParameters>
  <CategoryId>otc-3</CategoryId>
  <MinVolume>500</MinVolume>
  <MinPrice>50</MinPrice>
  <MaxPrice>500</MaxPrice>
  <ItemTitle>рубашка</ItemTitle>
  <VendorName>магазин</VendorName>
  <BrandName>бренд</BrandName>
  <OrderBy>Price:Asc</OrderBy>
  <StuffStatus>New</StuffStatus>
  <IsTmall>true</IsTmall>
  <PropertySearch><![CDATA[pid:value]]></PropertySearch>
</SearchItemsParameters>
```

### Провайдеры

| Провайдер | Category prefix | Доступен |
|-----------|----------------|----------|
| Taobao | otc-* (default) | Да |
| JD | otc-121 | Зависит от ключа |
| Poizon | otc-122 | Зависит от ключа |

### Лимиты

- Дневной лимит платных вызовов: ~300 (зависит от тарифа)
- frameSize: 100 работает стабильно с MinVolume фильтром
- GetItemFullInfo: ~1-2 сек/запрос
- При превышении лимита: ErrorCode=AccessDenied, SubErrorCode=CallLimit

### Данные из API

**SearchProducts (Phase 1):**
title, price, qty, main_image, vendor, brand, sales, fav_count, features (Tmall/Expired/FakeQty), location (city+state)

**GetItemFullInfo (Phase 2):**
+ description_html, weight, all SKUs (qty+price+configurators), all photos, all attributes (pid/vid/value/is_configurator/image_url), has_internal_delivery

---

## CS-Cart API

### Методы

| Метод | Endpoint | Назначение |
|-------|----------|-----------|
| CreateProduct | POST /api/products | Новый товар (status=D) |
| UpdateProduct | PUT /api/products/{id} | Обновление товара |
| CreateOptionAdvanced | POST /api/options | Опция с картинками и price modifiers |
| UpdateProductFeatures | PUT /api/products/{id} | Установка характеристик |
| ResolveFeatureVariant | GET /api/features/{id} | Поиск variant_id по значению |
| DeleteProduct | DELETE /api/products/{id} | Удаление товара |

### Маппинг характеристик (featureMap)

| Характеристика | feature_id | Допустимые значения |
|---------------|-----------|-------------------|
| Цвет | 567 | Черный, Белый, Синий, Серый, Красный... (25 вариантов) |
| Ткань | 563 | Деним, Трикотаж, Хлопок, Шифон... |
| Модель | 574 | Oversize, Skinny, Прямой, Свободный |
| Высота талии | 575 | Завышенная, Заниженная, Нормальная |
| Штанина | 573 | Прямая, Узкая, Широкая, Клёш |
| Длина | 570 | - |
| Толщина | 571 | Средний, Толстый, Тонкий |
| Повод | 566 | Повседневный, Офис, Спорт, Вечернее |
| Подкладка | 565 | - |
| Капюшон | 562 | Без капюшона, С капюшоном |
| Страна | 578 | Генерируется AI |

### Комбинации (SKU inventory)

CS-Cart не имеет API для комбинаций. Запись напрямую в MySQL:

```sql
INSERT INTO cscart_product_options_inventory
  (product_id, product_code, combination_hash, combination, amount)
VALUES (?, ?, CRC32(?), ?, ?)
ON DUPLICATE KEY UPDATE amount=VALUES(amount);

UPDATE cscart_products SET tracking='O' WHERE product_id=?;
```

---

## Веб-интерфейс

### Страница товаров (/otweb/products)

**Фильтры:** категория, провайдер, сортировка (новые/продажи/цена/отзывы/избранное), поиск, не отправлены, включённые, кол-во на странице (40/80/120/200)

**Bulk actions (чекбоксы):**
- AI-перевод - DeepSeek для выбранных
- Push - отправить в CS-Cart
- Скрыть / Включить

**Бейджи на карточках:**
- CS#XXX (зелёный) - отправлен в CS-Cart
- Скрыт (тёмный)
- N продаж (жёлтый)
- Нет перевода (красный)
- Город (серый, если Location есть)

### Страница товара (/otweb/products/{id})

- Фото + галерея
- Цена CNY -> TMT
- Location продавца (город, провинция)
- Названия RU/EN/TK с бейджем статуса (AI/Ручной)
- Описания RU/EN/TK
- Доставка: через неделю (7 дней)
- SKU таблица (sku_id, qty, price, конфигурация)
- Кнопки: Push / Обновить в CS-Cart / AI-перевод / Ручной ввод

### Страница синхронизации (/otweb/sync)

**Фильтры API:**
- Провайдер, категория, макс. товаров
- Мин. продаж (50-50K)
- Цена от/до (CNY)
- Поиск по названию
- Сортировка (по умолчанию, цена, продажи)
- Состояние (новый/б/у), только Tmall
- Только цены (быстрый синк)

**Live лог:** polling каждые 2 сек, статус running/done/error

### Настройки (/otweb/settings)

- API ключи (OTAPI, CS-Cart, DeepSeek)
- Товары (статус по умолчанию, Company ID)
- Ценообразование (курс, наценка, надбавка)
- Доставка ($/кг, USD/CNY, вкл/выкл)
- AI промпт (кастомизация)
- Провайдеры (вкл/выкл)
- Cron расписание

---

## Graceful degradation

| Сбой | Поведение |
|------|----------|
| DeepSeek недоступен | Товар создаётся с оригинальным названием, без описания |
| CS-Cart timeout | Ошибка логируется, следующий товар продолжается |
| Фото 404 | CS-Cart пропускает фото, товар создаётся без него |
| Feature variant не найден | Логируется, остальные features сохраняются |
| API лимит (AccessDenied) | Phase 1 данные сохранены, Phase 2 пропущена |
| mirror DB нет прав | Ошибка логируется с деталями (не молча) |

---

## Миграции

```
001_init.sql          - Полная схема: 12 таблиц, индексы, color_map, size_map
002_detail_sync.sql   - detail_fetched_at для кеша GetItemFullInfo
003_v4.sql            - enabled, hidden_at, settings, cs_categories
004_descriptions.sql  - description_ru/en/tk, weight_estimated
005_location.sql      - location_city, location_state продавца
```

---

## Makefile

```bash
make run      # go run .
make build    # GOOS=linux GOARCH=amd64 go build -o otapi-hub-linux .
make test     # go test ./...
make stop     # kill процесс на порту 5500
make restart  # stop + run
```
