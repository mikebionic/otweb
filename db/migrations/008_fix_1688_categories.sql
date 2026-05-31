-- Migration 008: Fix 1688 root category translations
-- Corrects mistranslations and sets canonical Russian names for all root categories.
-- Safe to re-run (uses UPDATE with WHERE to only fix known bad values).

-- Fix abb-2: 蛇和饮料 ("snakes and beverages") → correct Chinese is 食品饮料 (food and beverages)
UPDATE categories SET name_zh='食品饮料', name_ru='Еда и напитки' WHERE id='abb-2';

-- Fix abb-15: cryptic translation → "Товары первой необходимости"
UPDATE categories SET name_ru='Товары первой необходимости' WHERE id='abb-15';

-- Fix abb-123614001: "Металлические распределители" → "Металлопрокат"
UPDATE categories SET name_ru='Металлопрокат' WHERE id='abb-123614001';

-- Canonical names for main fashion/lifestyle root categories
UPDATE categories SET name_ru='Мужская одежда'                        WHERE id='abb-10165';
UPDATE categories SET name_ru='Женская одежда'                        WHERE id='abb-10166';
UPDATE categories SET name_ru='Детская одежда'                        WHERE id='abb-311';
UPDATE categories SET name_ru='Нижнее бельё'                          WHERE id='abb-312';
UPDATE categories SET name_ru='Обувь'                                  WHERE id='abb-1038378';
UPDATE categories SET name_ru='Сумки, рюкзаки и чемоданы'             WHERE id='abb-1042954';
UPDATE categories SET name_ru='Аксессуары и украшения'                 WHERE id='abb-54';
UPDATE categories SET name_ru='Косметика и уход за кожей'              WHERE id='abb-97';
UPDATE categories SET name_ru='Текстиль и домашний декор'              WHERE id='abb-96';
UPDATE categories SET name_ru='Спорт, туризм и отдых'                  WHERE id='abb-18';
UPDATE categories SET name_ru='Игрушки и развивающие игры'             WHERE id='abb-1813';
UPDATE categories SET name_ru='Электроника'                            WHERE id='abb-7';
UPDATE categories SET name_ru='Бытовая техника'                        WHERE id='abb-6';
UPDATE categories SET name_ru='Освещение'                              WHERE id='abb-58';
UPDATE categories SET name_ru='Посуда и кухонные принадлежности'       WHERE id='abb-201547801';
UPDATE categories SET name_ru='Хранение и уборка дома'                 WHERE id='abb-201547901';
UPDATE categories SET name_ru='Гигиена и бытовая химия'                WHERE id='abb-130822220';
UPDATE categories SET name_ru='Продукты питания'                       WHERE id='abb-130822002';
UPDATE categories SET name_ru='Товары для животных и сад'              WHERE id='abb-122916001';
UPDATE categories SET name_ru='Автоаксессуары'                         WHERE id='abb-122916002';
UPDATE categories SET name_ru='Автозапчасти и мотоаксессуары'          WHERE id='abb-71';
UPDATE categories SET name_ru='Медицина и медтехника'                  WHERE id='abb-66';
UPDATE categories SET name_ru='Офис, канцтовары и мероприятия'         WHERE id='abb-67';
UPDATE categories SET name_ru='Строительные материалы'                 WHERE id='abb-201346017';
UPDATE categories SET name_ru='Упаковка'                               WHERE id='abb-68';
UPDATE categories SET name_ru='Безопасность и защита'                  WHERE id='abb-70';
UPDATE categories SET name_ru='Химическая промышленность'              WHERE id='abb-8';
UPDATE categories SET name_ru='Металлургия'                            WHERE id='abb-9';
UPDATE categories SET name_ru='Текстиль и кожа'                        WHERE id='abb-4';
UPDATE categories SET name_ru='Машины и промышленное оборудование'     WHERE id='abb-65';
UPDATE categories SET name_ru='Машинное оборудование'                  WHERE id='abb-1426';
UPDATE categories SET name_ru='Электротехника'                         WHERE id='abb-5';
UPDATE categories SET name_ru='Радиокомпоненты и электроника'          WHERE id='abb-57';
UPDATE categories SET name_ru='Измерительное оборудование'             WHERE id='abb-10208';
UPDATE categories SET name_ru='Оборудование и инструменты'             WHERE id='abb-59';
UPDATE categories SET name_ru='Охрана окружающей среды'                WHERE id='abb-64';
UPDATE categories SET name_ru='Альтернативная энергия'                 WHERE id='abb-202052814';
UPDATE categories SET name_ru='Энергетика'                             WHERE id='abb-10';
UPDATE categories SET name_ru='Сельское хозяйство'                     WHERE id='abb-1';
UPDATE categories SET name_ru='Транспорт'                              WHERE id='abb-12';
UPDATE categories SET name_ru='Ремонт и отделка'                       WHERE id='abb-13';
UPDATE categories SET name_ru='Телекоммуникации'                       WHERE id='abb-509';
UPDATE categories SET name_ru='Медиа, радио и ТВ'                      WHERE id='abb-53';
UPDATE categories SET name_ru='Резина и пластик'                       WHERE id='abb-55';
UPDATE categories SET name_ru='Товары для взрослых'                    WHERE id='abb-130823000';
UPDATE categories SET name_ru='Образование и обучение'                 WHERE id='abb-72';
