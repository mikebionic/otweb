-- Описания товаров на 3 языках (генерируются DeepSeek)
ALTER TABLE products ADD COLUMN description_ru TEXT DEFAULT NULL AFTER description_html;
ALTER TABLE products ADD COLUMN description_en TEXT DEFAULT NULL AFTER description_ru;
ALTER TABLE products ADD COLUMN description_tk TEXT DEFAULT NULL AFTER description_en;

-- Вес товара может быть оценён (DeepSeek / ручной ввод), не только из API
ALTER TABLE products ADD COLUMN weight_estimated TINYINT(1) NOT NULL DEFAULT 0 AFTER weight_kg;
