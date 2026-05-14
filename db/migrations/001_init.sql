-- =============================================================
-- OTAPI HUB — MySQL 8 Schema v2
-- =============================================================

-- Создаём вторую базу для зеркала CS-Cart и выдаём права
CREATE DATABASE IF NOT EXISTS otapi_cs CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
GRANT ALL PRIVILEGES ON otapi_cs.* TO 'otapi'@'%';
FLUSH PRIVILEGES;

-- =============================================================
-- OTAPI HUB (основная БД)
-- =============================================================
USE otapi_hub;

CREATE TABLE IF NOT EXISTS categories (
    id           VARCHAR(32)   NOT NULL,
    provider     VARCHAR(32)   NOT NULL,
    external_id  VARCHAR(64)   DEFAULT NULL,
    parent_id    VARCHAR(32)   DEFAULT NULL,
    name_ru      VARCHAR(512)  NOT NULL DEFAULT '',
    name_en      VARCHAR(512)  NOT NULL DEFAULT '',
    name_zh      VARCHAR(512)  NOT NULL DEFAULT '',
    is_parent    TINYINT(1)    NOT NULL DEFAULT 0,
    is_hidden    TINYINT(1)    NOT NULL DEFAULT 0,
    item_count   INT           NOT NULL DEFAULT 0,
    fetched_at   BIGINT        NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    INDEX idx_cat_parent (parent_id),
    INDEX idx_cat_provider (provider)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS products (
    id                    BIGINT        NOT NULL AUTO_INCREMENT,
    otapi_id              VARCHAR(64)   NOT NULL,
    provider              VARCHAR(32)   NOT NULL,
    category_id           VARCHAR(32)   NOT NULL DEFAULT '',
    external_category_id  VARCHAR(64)   NOT NULL DEFAULT '',
    vendor_id             VARCHAR(128)  NOT NULL DEFAULT '',
    vendor_name           VARCHAR(512)  NOT NULL DEFAULT '',
    vendor_score          INT           NOT NULL DEFAULT 0,
    brand_id              VARCHAR(128)  NOT NULL DEFAULT '',
    brand_name            VARCHAR(512)  NOT NULL DEFAULT '',
    title_original        VARCHAR(1024) NOT NULL DEFAULT '',
    title_ru              VARCHAR(1024) NOT NULL DEFAULT '',
    title_en              VARCHAR(1024) NOT NULL DEFAULT '',
    title_tk              VARCHAR(1024) NOT NULL DEFAULT '',
    translate_status      VARCHAR(16)   NOT NULL DEFAULT 'none',
    price_cny             DECIMAL(12,2) NOT NULL DEFAULT 0,
    price_usd             DECIMAL(12,2) NOT NULL DEFAULT 0,
    price_tmt             DECIMAL(12,2) NOT NULL DEFAULT 0,
    master_quantity       INT           NOT NULL DEFAULT 0,
    is_fake_quantity      TINYINT(1)    NOT NULL DEFAULT 0,
    min_order_qty         INT           NOT NULL DEFAULT 1,
    is_sell_allowed       TINYINT(1)    NOT NULL DEFAULT 1,
    is_expired            TINYINT(1)    NOT NULL DEFAULT 0,
    stuff_status          VARCHAR(16)   NOT NULL DEFAULT 'New',
    is_tmall              TINYINT(1)    NOT NULL DEFAULT 0,
    main_image_url        TEXT          DEFAULT NULL,
    platform_url          TEXT          DEFAULT NULL,
    description_html      TEXT          DEFAULT NULL,
    volume_sales          INT           NOT NULL DEFAULT 0,
    sales_last_30days     INT           NOT NULL DEFAULT 0,
    fav_count             INT           NOT NULL DEFAULT 0,
    reviews_count         INT           NOT NULL DEFAULT 0,
    weight_kg             DECIMAL(8,3)  NOT NULL DEFAULT 0,
    has_hierarchical_conf TINYINT(1)    NOT NULL DEFAULT 0,
    raw_json              MEDIUMTEXT    DEFAULT NULL,
    fetched_at            BIGINT        NOT NULL DEFAULT 0,
    updated_at            BIGINT        NOT NULL DEFAULT 0,
    pushed_to_cs_at       BIGINT        DEFAULT NULL,
    cs_product_id         INT           DEFAULT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_otapi_provider (otapi_id, provider),
    INDEX idx_prod_category (category_id),
    INDEX idx_prod_provider (provider),
    INDEX idx_prod_status (is_sell_allowed, is_expired),
    INDEX idx_prod_translate (translate_status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS product_skus (
    id            BIGINT        NOT NULL AUTO_INCREMENT,
    product_id    BIGINT        NOT NULL,
    sku_id        VARCHAR(64)   NOT NULL,
    quantity      INT           NOT NULL DEFAULT 0,
    sales_count   INT           NOT NULL DEFAULT 0,
    price_cny     DECIMAL(12,2) NOT NULL DEFAULT 0,
    configurators TEXT          DEFAULT NULL,
    cs_product_id INT           DEFAULT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_sku (product_id, sku_id),
    INDEX idx_sku_product (product_id),
    CONSTRAINT fk_sku_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS product_images (
    id          BIGINT        NOT NULL AUTO_INCREMENT,
    product_id  BIGINT        NOT NULL,
    url         TEXT          NOT NULL,
    url_small   TEXT          DEFAULT NULL,
    url_medium  TEXT          DEFAULT NULL,
    url_large   TEXT          DEFAULT NULL,
    is_main     TINYINT(1)    NOT NULL DEFAULT 0,
    position    SMALLINT      NOT NULL DEFAULT 0,
    width       SMALLINT      NOT NULL DEFAULT 0,
    height      SMALLINT      NOT NULL DEFAULT 0,
    local_path  VARCHAR(512)  DEFAULT NULL,
    PRIMARY KEY (id),
    INDEX idx_img_product (product_id),
    CONSTRAINT fk_img_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS product_attrs (
    id               BIGINT        NOT NULL AUTO_INCREMENT,
    product_id       BIGINT        NOT NULL,
    pid              VARCHAR(128)  NOT NULL DEFAULT '',
    vid              VARCHAR(128)  NOT NULL DEFAULT '',
    property_name    VARCHAR(512)  NOT NULL DEFAULT '',
    value            VARCHAR(1024) NOT NULL DEFAULT '',
    is_configurator  TINYINT(1)    NOT NULL DEFAULT 0,
    image_url        TEXT          DEFAULT NULL,
    PRIMARY KEY (id),
    INDEX idx_attr_product (product_id),
    CONSTRAINT fk_attr_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS category_config (
    category_id         VARCHAR(32)   NOT NULL,
    enabled             TINYINT(1)    NOT NULL DEFAULT 0,
    sync_schedule       VARCHAR(32)   NOT NULL DEFAULT 'manual',
    max_products        INT           NOT NULL DEFAULT 500,
    last_synced_at      BIGINT        DEFAULT NULL,
    products_imported   INT           NOT NULL DEFAULT 0,
    cs_category_id      INT           DEFAULT NULL,
    notes               VARCHAR(512)  NOT NULL DEFAULT '',
    created_at          BIGINT        NOT NULL DEFAULT 0,
    updated_at          BIGINT        NOT NULL DEFAULT 0,
    PRIMARY KEY (category_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS markup_rules (
    id             INT           NOT NULL AUTO_INCREMENT,
    scope_type     VARCHAR(16)   NOT NULL DEFAULT 'global',
    scope_id       VARCHAR(64)   DEFAULT NULL,
    markup_pct     DECIMAL(8,2)  NOT NULL DEFAULT 30.00,
    fixed_addon    DECIMAL(10,2) NOT NULL DEFAULT 0,
    exchange_rate  DECIMAL(10,4) DEFAULT NULL,
    is_active      TINYINT(1)    NOT NULL DEFAULT 1,
    notes          VARCHAR(255)  NOT NULL DEFAULT '',
    created_at     BIGINT        NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    UNIQUE KEY uk_scope (scope_type, scope_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO markup_rules (scope_type, scope_id, markup_pct, fixed_addon, exchange_rate, notes, created_at)
SELECT 'global', NULL, 35.00, 0, 0.5700, 'Глобальная наценка 35%, курс 1 CNY = 0.57 TMT', UNIX_TIMESTAMP()
WHERE NOT EXISTS (SELECT 1 FROM markup_rules WHERE scope_type='global' AND scope_id IS NULL);

CREATE TABLE IF NOT EXISTS category_map (
    otapi_category_id   VARCHAR(32)   NOT NULL,
    cs_category_id      INT           NOT NULL,
    cs_category_name    VARCHAR(255)  NOT NULL DEFAULT '',
    notes               VARCHAR(255)  NOT NULL DEFAULT '',
    PRIMARY KEY (otapi_category_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS color_map (
    otapi_value         VARCHAR(255)  NOT NULL,
    cs_variant_id       INT           NOT NULL,
    cs_variant_name     VARCHAR(255)  NOT NULL DEFAULT '',
    notes               VARCHAR(255)  NOT NULL DEFAULT '',
    PRIMARY KEY (otapi_value)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO color_map (otapi_value, cs_variant_id, cs_variant_name) VALUES
('black',2096,'Черный'),('Black',2096,'Черный'),('Черный',2096,'Черный'),('Чёрный',2096,'Черный'),
('white',2098,'Белый'),('White',2098,'Белый'),('Белый',2098,'Белый'),
('beige',2097,'Бежевый'),('Beige',2097,'Бежевый'),('Бежевый',2097,'Бежевый'),
('grey',2103,'Серый'),('gray',2103,'Серый'),('Gray',2103,'Серый'),('Серый',2103,'Серый'),
('red',2107,'Красный'),('Red',2107,'Красный'),('Красный',2107,'Красный'),
('blue',2110,'Синий'),('Blue',2110,'Синий'),('Синий',2110,'Синий'),
('pink',2114,'Розовый'),('Pink',2114,'Розовый'),('Розовый',2114,'Розовый'),
('purple',2112,'Пурпурный'),('Purple',2112,'Пурпурный'),
('brown',2106,'Коричневый'),('Brown',2106,'Коричневый'),
('yellow',2116,'Желтый'),('Yellow',2116,'Желтый'),
('navy',2108,'Темно-синий'),('Navy',2108,'Темно-синий'),
('burgundy',2099,'Бордовый'),('Бордовый',2099,'Бордовый'),
('multicolor',2100,'Разноцветный');

CREATE TABLE IF NOT EXISTS size_map (
    otapi_value         VARCHAR(255)  NOT NULL,
    cs_variant_id       INT           NOT NULL,
    cs_variant_name     VARCHAR(255)  NOT NULL DEFAULT '',
    notes               VARCHAR(255)  NOT NULL DEFAULT '',
    PRIMARY KEY (otapi_value)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO size_map (otapi_value, cs_variant_id, cs_variant_name) VALUES
('XS',2163,'XS'),('S',2164,'S'),('M',2165,'M'),
('L',2166,'L'),('XL',2167,'XL'),('2XL',2168,'2XL'),
('3XL',2169,'3XL'),('4XL',2170,'4XL'),('5XL',2171,'5XL');

CREATE TABLE IF NOT EXISTS sync_jobs (
    id                  INT           NOT NULL AUTO_INCREMENT,
    job_type            VARCHAR(32)   NOT NULL,
    category_id         VARCHAR(32)   DEFAULT NULL,
    status              VARCHAR(16)   NOT NULL DEFAULT 'pending',
    started_at          BIGINT        DEFAULT NULL,
    finished_at         BIGINT        DEFAULT NULL,
    items_processed     INT           NOT NULL DEFAULT 0,
    items_skipped       INT           NOT NULL DEFAULT 0,
    errors_count        INT           NOT NULL DEFAULT 0,
    api_requests_made   INT           NOT NULL DEFAULT 0,
    log_text            TEXT          DEFAULT NULL,
    triggered_by        VARCHAR(32)   NOT NULL DEFAULT 'manual',
    PRIMARY KEY (id),
    INDEX idx_job_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS push_queue (
    id              INT           NOT NULL AUTO_INCREMENT,
    product_id      BIGINT        NOT NULL,
    action          VARCHAR(16)   NOT NULL DEFAULT 'create',
    status          VARCHAR(16)   NOT NULL DEFAULT 'pending',
    cs_product_id   INT           DEFAULT NULL,
    pushed_at       BIGINT        DEFAULT NULL,
    error_msg       TEXT          DEFAULT NULL,
    created_at      BIGINT        NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    UNIQUE KEY uk_push (product_id, action),
    INDEX idx_push_status (status),
    CONSTRAINT fk_push_product FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


-- =============================================================
-- CS-CART ЗЕРКАЛО (отдельная БД)
-- =============================================================
USE otapi_cs;

CREATE TABLE IF NOT EXISTS cscart_categories (
    category_id   INT           NOT NULL AUTO_INCREMENT,
    parent_id     INT           NOT NULL DEFAULT 0,
    status        CHAR(1)       NOT NULL DEFAULT 'A',
    position      SMALLINT      NOT NULL DEFAULT 0,
    PRIMARY KEY (category_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS cscart_category_descriptions (
    category_id  INT           NOT NULL,
    lang_code    CHAR(2)       NOT NULL DEFAULT 'ru',
    category     VARCHAR(255)  NOT NULL DEFAULT '',
    PRIMARY KEY (category_id, lang_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS cscart_products (
    product_id          INT           NOT NULL AUTO_INCREMENT,
    product_code        VARCHAR(64)   NOT NULL DEFAULT '',
    source_import_key   VARCHAR(64)   NOT NULL DEFAULT '',
    status              CHAR(1)       NOT NULL DEFAULT 'A',
    company_id          INT           NOT NULL DEFAULT 0,
    amount              INT           NOT NULL DEFAULT 0,
    weight              DECIMAL(10,3) NOT NULL DEFAULT 0,
    parent_product_id   INT           NOT NULL DEFAULT 0,
    created_at          BIGINT        NOT NULL DEFAULT 0,
    updated_at          BIGINT        NOT NULL DEFAULT 0,
    PRIMARY KEY (product_id),
    INDEX idx_source (source_import_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS cscart_product_descriptions (
    product_id        INT           NOT NULL,
    lang_code         CHAR(2)       NOT NULL DEFAULT 'ru',
    product           VARCHAR(512)  NOT NULL DEFAULT '',
    full_description  TEXT          DEFAULT NULL,
    PRIMARY KEY (product_id, lang_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS cscart_product_prices (
    product_id     INT           NOT NULL,
    price          DECIMAL(12,2) NOT NULL DEFAULT 0,
    list_price     DECIMAL(12,2) NOT NULL DEFAULT 0,
    currency_code  CHAR(3)       NOT NULL DEFAULT 'TMT',
    PRIMARY KEY (product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS cscart_product_features_values (
    product_id    INT     NOT NULL,
    feature_id    INT     NOT NULL,
    variant_id    INT     NOT NULL DEFAULT 0,
    value_string  TEXT    DEFAULT NULL,
    PRIMARY KEY (product_id, feature_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS cscart_product_variation_groups (
    id         INT           NOT NULL AUTO_INCREMENT,
    code       VARCHAR(128)  NOT NULL DEFAULT '',
    created_at BIGINT        NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    UNIQUE KEY uk_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS cscart_product_variation_group_products (
    product_id        INT NOT NULL,
    parent_product_id INT NOT NULL DEFAULT 0,
    group_id          INT NOT NULL,
    PRIMARY KEY (product_id, group_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS cscart_products_categories (
    product_id  INT     NOT NULL,
    category_id INT     NOT NULL,
    link_type   CHAR(1) NOT NULL DEFAULT 'M',
    PRIMARY KEY (product_id, category_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
