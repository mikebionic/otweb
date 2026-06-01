-- Маппинг (pid, vid) → CS-Cart feature_id + variant_id
-- Строится один раз через DeepSeek, используется при каждом пуше
CREATE TABLE IF NOT EXISTS attr_cs_mapping (
    pid               VARCHAR(128) NOT NULL,
    vid               VARCHAR(128) NOT NULL,
    cs_feature_id     INT          DEFAULT NULL,  -- NULL = нет совпадения в CS-Cart
    cs_variant_id     INT          DEFAULT NULL,  -- NULL = свободный текст
    canonical_value   VARCHAR(512) NOT NULL DEFAULT '', -- нормализованное RU значение
    mapped_at         BIGINT       NOT NULL DEFAULT 0,
    PRIMARY KEY (pid, vid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Маппинг сырых значений вариаций (цвет, размер) → стандарт CS-Cart
CREATE TABLE IF NOT EXISTS sku_option_mapping (
    raw_value         VARCHAR(512) NOT NULL,
    option_type       VARCHAR(64)  NOT NULL,  -- 'color', 'size'
    canonical_value   VARCHAR(512) NOT NULL,
    cs_variant_id     INT          DEFAULT NULL,
    mapped_at         BIGINT       NOT NULL DEFAULT 0,
    PRIMARY KEY (raw_value(200), option_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Кеш CS-Cart features (характеристики) - обновляется при синхронизации
CREATE TABLE IF NOT EXISTS cs_features_cache (
    feature_id     INT          NOT NULL,
    feature_name   VARCHAR(512) NOT NULL,
    feature_type   VARCHAR(32)  NOT NULL DEFAULT 'S',  -- S=select, T=text, N=number
    fetched_at     BIGINT       NOT NULL DEFAULT 0,
    PRIMARY KEY (feature_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS cs_feature_variants_cache (
    variant_id     INT          NOT NULL,
    feature_id     INT          NOT NULL,
    variant_value  VARCHAR(512) NOT NULL,
    PRIMARY KEY (variant_id),
    KEY (feature_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
