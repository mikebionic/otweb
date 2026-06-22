// migrate-categories — реструктуризация дерева категорий CS-Cart.
// Переходим от структуры "по полу/возрасту" к структуре "по типу товара".
//
// Запуск:
//   ./migrate-categories -dsn "user:pass@tcp(host:port)/db" -cs-url "https://..." -dry
//   ./migrate-categories -dsn "..." -cs-url "..." (реальная миграция)
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var (
	dsn   string
	csURL string
	dry   bool
)

func main() {
	flag.StringVar(&dsn, "dsn", "root@tcp(localhost:3306)/shop_mirror?charset=utf8mb4", "MySQL DSN для shop_mirror")
	flag.StringVar(&csURL, "cs-url", "https://example.com", "CS-Cart base URL (не используется, для справки)")
	flag.BoolVar(&dry, "dry", false, "Dry run — показать план без изменений")
	flag.Parse()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	m := &Migrator{db: db, dry: dry}
	m.run()
}

type Migrator struct {
	db  *sql.DB
	dry bool
}

func (m *Migrator) run() {
	log.Println("=== Миграция категорий CS-Cart ===")
	if m.dry {
		log.Println(">>> DRY RUN — изменения не применяются <<<")
	}

	// Шаг 1: переименовать существующие top-level категории
	log.Println("\n--- Шаг 1: Переименование ---")
	renames := []struct {
		id   int
		name string
	}{
		{322, "Женская одежда"},
		{323, "Мужская одежда"},
		{329, "Аксессуары и украшения"},
		{1272, "Текстиль и домашний декор"},
		{1282, "Посуда и кухонные принадлежности"},
	}
	for _, r := range renames {
		m.renameCategory(r.id, r.name)
	}

	// Шаг 2: создать новые top-level категории
	log.Println("\n--- Шаг 2: Создание новых категорий ---")
	newCats := []string{
		"Детская одежда",
		"Нижнее бельё",
		"Сумки, рюкзаки и чемоданы",
		"Автоаксессуары",
		"Бытовая техника",
		"Игрушки и развивающие игры",
		"Освещение",
		"Офис, канцтовары и мероприятия",
		"Спорт, туризм и отдых",
		"Хранение и уборка дома",
		"Электроника",
	}
	newIDs := make(map[string]int)
	for _, name := range newCats {
		id := m.createTopLevel(name)
		newIDs[name] = id
		log.Printf("  Создана: %q → ID=%d", name, id)
	}

	// Шаг 3: переместить Девочкам/Мальчикам/Малышкам/Малышам → Детская одежда
	log.Println("\n--- Шаг 3: Детская одежда ---")
	childCatID := newIDs["Детская одежда"]
	for _, id := range []int{324, 325, 326, 327} {
		m.moveCategory(id, childCatID)
	}

	// Шаг 4: Нижнее бельё — переместить из каждой одежды + переименовать
	log.Println("\n--- Шаг 4: Нижнее бельё ---")
	underWearParentID := newIDs["Нижнее бельё"]
	underWearMoves := []struct {
		id      int
		newName string
	}{
		{355, "Женское"},  // из Женской одежды
		{377, "Мужское"},  // из Мужской одежды
		{405, "Девочкам"}, // из Девочкам
		{430, "Мальчикам"},// из Мальчикам
		{459, "Малышкам"}, // из Малышкам
		{488, "Малышам"},  // из Малышам
	}
	for _, uw := range underWearMoves {
		m.moveCategory(uw.id, underWearParentID)
		m.renameCategory(uw.id, uw.newName)
	}

	// Шаг 5: Сумки, рюкзаки и чемоданы
	log.Println("\n--- Шаг 5: Сумки, рюкзаки и чемоданы ---")
	bagsParentID := newIDs["Сумки, рюкзаки и чемоданы"]
	bagsMoves := []struct {
		id      int
		newName string
	}{
		{545, "Женские"},  // из Аксессуары → Для женщин
		{559, "Мужские"},  // из Аксессуары → Для мужчин
		{570, "Детские"},  // из Аксессуары → Для девочек
		{579, "Детские (мальчики)"}, // из Аксессуары → Для мальчиков
	}
	for _, bm := range bagsMoves {
		m.moveCategory(bm.id, bagsParentID)
		m.renameCategory(bm.id, bm.newName)
	}

	// Шаг 6: обновить category_map в otapi_hub
	log.Println("\n--- Шаг 6: Обновление category_map (TODO вручную или отдельным шагом) ---")
	log.Println("  Новые категории созданы. category_map нужно дополнить вручную через UI Hub.")

	log.Println("\n=== Миграция завершена ===")
}

func (m *Migrator) renameCategory(id int, newName string) {
	var oldName string
	m.db.QueryRow(`SELECT category FROM cscart_category_descriptions WHERE category_id=? AND lang_code='ru'`, id).Scan(&oldName)
	log.Printf("  Переименовать %d: %q → %q", id, oldName, newName)
	if m.dry {
		return
	}
	// Обновляем для ru и en. tk/tm оставляем как есть (не трогаем).
	_, err := m.db.Exec(`UPDATE cscart_category_descriptions SET category=? WHERE category_id=? AND lang_code='ru'`, newName, id)
	if err != nil {
		log.Printf("  WARN rename ru %d: %v", id, err)
	}
	_, err = m.db.Exec(`UPDATE cscart_category_descriptions SET category=? WHERE category_id=? AND lang_code='en'`, newName, id)
	if err != nil {
		log.Printf("  WARN rename en %d: %v", id, err)
	}
}

func (m *Migrator) createTopLevel(name string) int {
	if m.dry {
		log.Printf("  [dry] Создать top-level: %q", name)
		return 0
	}
	ts := time.Now().Unix()
	res, err := m.db.Exec(`
		INSERT INTO cscart_categories
		  (parent_id, id_path, level, company_id, usergroup_ids, status, product_count, position, timestamp,
		   is_op, localization, age_verification, age_limit, parent_age_verification, parent_age_limit,
		   is_trash, is_default, category_type, ab__fn_category_status)
		VALUES (0, '', 1, 0, '0', 'A', 0, 0, ?, 'N', '', 'N', 0, 'N', 0, 'N', 'N', 'C', 'Y')`,
		ts)
	if err != nil {
		log.Fatalf("create category %q: %v", name, err)
	}
	id64, _ := res.LastInsertId()
	id := int(id64)

	// id_path для top-level = просто id
	m.db.Exec(`UPDATE cscart_categories SET id_path=? WHERE category_id=?`, fmt.Sprintf("%d", id), id)

	// Вставляем descriptions для всех языков
	langs := []string{"ru", "en", "tk", "tm", "tr"}
	for _, lang := range langs {
		m.db.Exec(`INSERT IGNORE INTO cscart_category_descriptions (category_id, lang_code, category) VALUES (?,?,?)`,
			id, lang, name)
	}
	return id
}

func (m *Migrator) moveCategory(id, newParentID int) {
	var oldParentID int
	var oldPath string
	m.db.QueryRow(`SELECT parent_id, id_path FROM cscart_categories WHERE category_id=?`, id).Scan(&oldParentID, &oldPath)

	var newParentPath string
	if newParentID == 0 {
		newParentPath = ""
	} else {
		m.db.QueryRow(`SELECT id_path FROM cscart_categories WHERE category_id=?`, newParentID).Scan(&newParentPath)
	}

	var newPath string
	if newParentPath == "" {
		newPath = fmt.Sprintf("%d", id)
	} else {
		newPath = fmt.Sprintf("%s/%d", newParentPath, id)
	}

	log.Printf("  Переместить %d: parent %d→%d, path %q→%q", id, oldParentID, newParentID, oldPath, newPath)
	if m.dry {
		return
	}

	// Обновляем parent_id и id_path для самой категории
	_, err := m.db.Exec(`UPDATE cscart_categories SET parent_id=?, id_path=? WHERE category_id=?`, newParentID, newPath, id)
	if err != nil {
		log.Printf("  WARN move %d: %v", id, err)
		return
	}

	// Обновляем id_path для всех потомков: заменяем старый префикс на новый
	if oldPath != "" {
		oldPrefix := oldPath + "/"
		newPrefix := newPath + "/"
		_, err = m.db.Exec(`
			UPDATE cscart_categories
			SET id_path = CONCAT(?, SUBSTRING(id_path, ?))
			WHERE id_path LIKE ?`,
			newPrefix,
			len(oldPrefix)+1,
			oldPrefix+"%")
		if err != nil {
			log.Printf("  WARN update descendants of %d: %v", id, err)
		} else {
			log.Printf("  Обновлены потомки %d (prefix %q → %q)", id, oldPrefix, newPrefix)
		}
	}

	// Обновляем level
	newLevel := 1 + strings.Count(newPath, "/")
	m.db.Exec(`UPDATE cscart_categories SET level=? WHERE category_id=?`, newLevel, id)
	// Потомки: level увеличивается/уменьшается на delta
	var oldLevel int
	m.db.QueryRow(`SELECT level FROM cscart_categories WHERE category_id=?`, id).Scan(&oldLevel)
	_ = oldLevel // потомки уже обновили id_path, level можно пересчитать отдельно
}
