# OTAPI Hub

Коннектор между OT Commerce API (Taobao/JD/Poizon) и CS-Cart (wabrum.com).
Забирает товары из китайских маркетплейсов, нормализует через DeepSeek, публикует в интернет-магазин.

## Репозиторий

```
Путь: ~/Documents/projectsGit/wbrm/otapi-hub/
Git:  (локальный, не на GitHub)
```

## Быстрый старт

```bash
# Запуск (нужен Docker с MySQL на порту 3360)
go run .

# Открыть http://localhost:5500
# Dashboard -> Categories -> Sync -> Mapping -> Push
```

## Архитектура

```
OT Commerce API (otapi.net)
  |
  | 1. BatchSearchItemsFrame (20 товаров/запрос)
  | 2. GetItemFullInfo (1 товар/запрос)
  v
otapi-hub (Go сервер, порт 5500)
  |
  | 3. DeepSeek нормализация (название, цвет, ткань, талия...)
  |
  | 4. CS-Cart REST API:
  |    POST /api/products (товар + фото)
  |    POST /api/options (размеры)
  |    PUT  /api/products/{id} (характеристики)
  v
CS-Cart (wabrum.com)
  Продавец: WABRUM Commerce (company_id=376)
```

## Структура файлов

```
otapi-hub/
|
|-- main.go                    # HTTP сервер, маршруты, хэндлеры
|
|-- config/
|   config.go                  # Конфигурация (OTAPI, CS-Cart, DeepSeek, pricing)
|   config_test.go
|
|-- otapi/                     # Клиент OT Commerce Legacy API
|   client.go                  # GetCatalog, SearchProducts, GetProduct
|   models.go                  # Category, SearchItem, ProductItem, SKU, Attribute
|   client_test.go
|
|-- cscart/                    # Клиент CS-Cart REST API
|   client.go                  # CreateProduct, UpdateProduct, CreateOption, UpdateProductFeatures
|   models.go                  # ProductInput, NormalizeSize, NormalizeSizes
|   client_test.go
|
|-- db/                        # MySQL (две базы: otapi_hub + wabrum_mv)
|   db.go                      # Подключение к двум MySQL базам
|   otapi_repo.go              # CRUD: products, categories, SKUs, attrs, images, markup, mappings
|
|-- sync/                      # Синхронизация OTAPI -> Hub DB
|   importer.go                # SyncCategories, SyncProducts (2 фазы), SyncPricesOnly
|   importer_test.go
|
|-- push/                      # Push из Hub DB -> CS-Cart
|   api_pusher.go              # PushCategory через CS-Cart API + DeepSeek + Options + Features
|   pusher.go                  # (legacy) прямая запись в БД CS-Cart
|
|-- translate/                 # DeepSeek нормализация
|   deepseek.go                # DeepSeek API клиент, Normalize()
|   prompt.go                  # Промпт с допустимыми значениями (цвета, ткани, модели...)
|
|-- web/templates/             # HTML шаблоны (Bootstrap 5)
|   layout.html                # Общий layout с sidebar
|   dashboard.html             # Главная: статистика
|   categories.html            # Список OT категорий
|   products.html              # Список товаров (пагинация)
|   product_detail.html        # Карточка товара (SKU, атрибуты, фото)
|   sync.html                  # Запуск синхронизации, история
|   push.html                  # Очередь push (legacy)
|   mapping.html               # Маппинг OT -> CS-Cart категорий
|   settings.html              # Наценка, курс, cron
|
|-- otapi-hub.postman_collection.json  # 33 запроса Postman
```

## Поток данных (шаг за шагом)

### 1. Синк категорий
```
GET /categories/sync-all-meta
  -> sync.SyncCategories()
    -> otapi.GetCatalog()  [GET GetRootCategoryInfoList]
    -> db.UpsertCategory() x 122
Результат: 122 категории в таблице categories
```

### 2. Синк товаров (двухфазный)
```
POST /sync/run {category_id, max_products}
  -> sync.SyncProducts(categoryID, max, logCh)

    ФАЗА 1 - дешёвая (1 запрос = 20 товаров):
      -> otapi.SearchProducts(provider, catID, page, 20)
         [GET BatchSearchItemsFrame?xmlParameters=<SearchItemsParameters><CategoryId>...</CategoryId>]
      -> db.UpsertProduct() для каждого
      Сохраняет: title, price, qty, main_image, vendor, brand, features

    ФАЗА 2 - дорогая (1 запрос = 1 товар, только если detail_fetched_at IS NULL или > 7 дней):
      -> otapi.GetProduct(provider, itemID)
         [GET GetItemFullInfo?itemId=...]
      -> db.UpsertProduct() + UpsertSKU() + InsertImage() + InsertAttr()
      Сохраняет: description, weight, все SKU, все фото, все атрибуты

Стоимость: ceil(N/20) + N запросов (первый раз), ceil(N/20) запросов (повторно)
```

### 3. Push в CS-Cart (через API)
```
POST /push/api {category_id}
  -> push.APIPusher.PushCategoryAuto(categoryOT)
    -> db: SELECT products WHERE cs_product_id IS NULL

    Для каждого товара:
      a) DeepSeek нормализация:
         -> translate.Normalize(title, attrs, colors)
         -> Ответ: чистое название, цвет, ткань, модель, талия...
         -> ~3 сек, ~$0.001

      b) Создание товара:
         -> cscart.CreateProduct(title, category, price, photos)
         -> CS-Cart скачивает фото с alicdn.com (~3-5 сек)
         -> Возвращает product_id

      c) Размеры:
         -> cscart.CreateOption(productID, "Размер", ["S","M","L"...])
         -> NormalizeSize() чистит "S подходит для 85-105 фунтов" -> "S"

      d) Характеристики:
         -> cscart.ResolveFeatureVariant(567, "Синий") -> variant_id=2110
         -> cscart.UpdateProductFeatures(productID, {567: "2110", 563: "2052"...})

    Среднее: ~12 сек/товар, 100 товаров = ~20 мин
```

### 4. Обновление цен (дешёвый синк)
```
POST /sync/prices {category_id}
  -> sync.SyncPricesOnly(categoryID)
    -> otapi.SearchProducts() постранично (20 товаров/запрос)
    -> UPDATE products SET price_cny, price_tmt WHERE otapi_id=?
    Без GetProduct, без DeepSeek, без CS-Cart
    1000 товаров = 50 запросов OTAPI, ~2 мин
```

## API лимиты и оптимизация

### OT Commerce API
- BatchSearchItemsFrame: до 20 товаров за запрос (frameSize=20, 50 вызывает таймаут)
- GetItemFullInfo: 1 товар за запрос (нет batch)
- BulkOperations: НЕДОСТУПНЫ на нашем ключе
- Нельзя получить детали нескольких товаров одним запросом
- Цены и остатки доступны через SearchProducts (20 шт/запрос) - дешёво
- Полная карточка (SKU, фото, атрибуты) только через GetItemFullInfo - дорого

### Оптимизация затрат
- detail_fetched_at: кэш 7 дней - повторный синк не вызывает GetItemFullInfo
- SyncPricesOnly: только цены через SearchProducts (ceil(N/20) запросов)
- DeepSeek: ~$0.001/товар, кэшируемый результат
- CS-Cart API: бесплатно (свой сервер)

### Batch возможности
- Поиск: 20 товаров/запрос (цена + qty + название + фото)
- Детали: 1 товар/запрос (нет batch на нашем ключе)
- CS-Cart: 1 товар/запрос (POST /api/products)

## Конфигурация

### Ценообразование
```
TMT = CNY * exchange_rate * (1 + markup_pct/100) + fixed_addon

Дефолт: CNY * 0.57 * 1.35 = CNY * 0.7695
Пример: 119 CNY = 91.60 TMT
```

Приоритет правил (таблица markup_rules):
1. product (для конкретного товара)
2. category (для категории)
3. global (для всех)

### API ключи (config.go Default())
- OTAPI Instance Key: 0b3d51dd-...
- CS-Cart: api@wabrum.com / d4ec2ae6...
- DeepSeek: sk-ec2379a5...
- CS-Cart Company ID: 376 (WABRUM Commerce)

## Таблицы БД (otapi_hub)

| Таблица | Назначение |
|---------|-----------|
| categories | 122 категории из OT Commerce |
| category_config | Настройки синка (enabled, schedule, max_products) |
| category_map | Маппинг OT категория -> CS-Cart категория |
| products | Товары (title, price, qty, images, vendor, brand) |
| product_skus | SKU варианты (размер x цвет, qty, price) |
| product_images | Фотографии (url, small, medium, large, is_main) |
| product_attrs | Атрибуты (property_name, value, is_configurator) |
| push_queue | Очередь push (legacy) |
| markup_rules | Правила наценки (global/category/product) |
| sync_jobs | История синхронизаций |

## Graceful degradation

- DeepSeek недоступен -> товар создаётся с оригинальным названием OT Commerce
- CS-Cart timeout -> ошибка логируется, следующий товар продолжается
- Фото 404 -> CS-Cart пропускает фото, товар создаётся без него
- Feature variant не найден -> логируется, остальные features сохраняются

## Деплой на сервер

```bash
# Компиляция для Linux
GOOS=linux GOARCH=amd64 go build -o otapi-hub

# Загрузка на сервер
scp otapi-hub root@95.181.224.97:/opt/otapi-hub/

# Systemd сервис
# /etc/systemd/system/otapi-hub.service

# Cron (обновление цен каждые 6 часов)
# 0 */6 * * * curl -X POST http://localhost:5500/sync/prices -d 'category_id=162205'
```
