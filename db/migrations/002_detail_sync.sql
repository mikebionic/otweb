-- Отслеживает когда последний раз был сделан полный GetProduct для товара.
-- NULL = никогда не синкался полностью (только базовые данные из SearchProducts).
ALTER TABLE products
    ADD COLUMN detail_fetched_at BIGINT NULL DEFAULT NULL
    AFTER updated_at;
