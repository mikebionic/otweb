-- Глобальная таблица переводов атрибутов (pid:vid → ru)
-- Дедуплицирует переводы: один pid:vid переводится один раз для всех товаров
CREATE TABLE IF NOT EXISTS attr_translations (
    pid              VARCHAR(128)  NOT NULL,
    vid              VARCHAR(128)  NOT NULL,
    property_name_zh VARCHAR(512)  NOT NULL DEFAULT '',
    value_zh         VARCHAR(1024) NOT NULL DEFAULT '',
    property_name_ru VARCHAR(512)  NOT NULL DEFAULT '',
    value_ru         VARCHAR(1024) NOT NULL DEFAULT '',
    translated_at    BIGINT        DEFAULT NULL,
    PRIMARY KEY (pid, vid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
