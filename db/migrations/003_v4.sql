-- OTAPI Hub v4: Product enable/disable, settings, cs_categories
-- Run: mysql otapi_hub < 003_v4.sql

ALTER TABLE products ADD COLUMN IF NOT EXISTS enabled TINYINT(1) NOT NULL DEFAULT 1 AFTER cs_product_id;
ALTER TABLE products ADD COLUMN IF NOT EXISTS hidden_at BIGINT DEFAULT NULL AFTER enabled;

CREATE TABLE IF NOT EXISTS settings (
    key_name VARCHAR(128) NOT NULL PRIMARY KEY,
    value TEXT NOT NULL DEFAULT '',
    updated_at BIGINT NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS cs_categories (
    category_id INT NOT NULL PRIMARY KEY,
    parent_id INT NOT NULL DEFAULT 0,
    name VARCHAR(255) NOT NULL DEFAULT '',
    fetched_at BIGINT NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
