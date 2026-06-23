package main

import "otapi-hub/db"

// computeCategoryAliases возвращает map: id-тёзки → id-канонической категории.
// Тёзки (одинаковое имя) скрываются в интерфейсе, их товары/счётчики показываются под
// канонической. Объединяем в один кластер только «связанные» тёзки (чтобы не слить лишнее):
//   - дочерняя категория с именем как у родителя (parent-child одно имя);
//   - братья с одинаковым именем (один parent_id).
// Канонической в кластере становится та, где БОЛЬШЕ своих товаров (при равенстве — меньший id),
// чтобы фильтр по канонической всегда показывал реальные товары.
func computeCategoryAliases(cats []db.CategoryWithConfig) map[string]string {
	byID := make(map[string]*db.CategoryWithConfig, len(cats))
	for i := range cats {
		byID[cats[i].ID] = &cats[i]
	}

	// union-find
	parent := make(map[string]string, len(cats))
	for i := range cats {
		parent[cats[i].ID] = cats[i].ID
	}
	var find func(string) string
	find = func(x string) string {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}

	// связываем тёзок
	siblings := make(map[[2]string][]string)
	for i := range cats {
		c := &cats[i]
		if c.ParentID != "" {
			if p, ok := byID[c.ParentID]; ok && p.Name == c.Name {
				union(c.ID, c.ParentID) // дочерняя = имя родителя
			}
		}
		siblings[[2]string{c.ParentID, c.Name}] = append(siblings[[2]string{c.ParentID, c.Name}], c.ID)
	}
	for _, ids := range siblings {
		for i := 1; i < len(ids); i++ {
			union(ids[0], ids[i]) // братья-тёзки
		}
	}

	// по кластерам выбираем каноник (макс. товаров; при равенстве — меньший id)
	comp := make(map[string][]string)
	for i := range cats {
		r := find(cats[i].ID)
		comp[r] = append(comp[r], cats[i].ID)
	}
	alias := make(map[string]string)
	for _, ids := range comp {
		if len(ids) < 2 {
			continue
		}
		canon := ids[0]
		for _, id := range ids[1:] {
			ci, cc := byID[id], byID[canon]
			if ci.LocalCount > cc.LocalCount || (ci.LocalCount == cc.LocalCount && id < canon) {
				canon = id
			}
		}
		for _, id := range ids {
			if id != canon {
				alias[id] = canon
			}
		}
	}
	return alias
}
