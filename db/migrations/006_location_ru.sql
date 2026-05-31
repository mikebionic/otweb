-- Переведённые названия региона/города продавца (Chinese → Russian)
-- Оригинал сохраняется в location_city / location_state
ALTER TABLE products ADD COLUMN location_city_ru  VARCHAR(100) NOT NULL DEFAULT '' AFTER location_city;
ALTER TABLE products ADD COLUMN location_state_ru VARCHAR(100) NOT NULL DEFAULT '' AFTER location_state;
