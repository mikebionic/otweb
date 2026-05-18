-- Город и провинция продавца в Китае (из API Location)
ALTER TABLE products ADD COLUMN location_city VARCHAR(100) NOT NULL DEFAULT '' AFTER brand_name;
ALTER TABLE products ADD COLUMN location_state VARCHAR(100) NOT NULL DEFAULT '' AFTER location_city;
