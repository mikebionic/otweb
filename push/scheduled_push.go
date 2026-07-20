package push

import (
	"fmt"
)

// resolveCSForProduct — определяет CS-категорию для товара по его OT-категории и маппингу.
// Возвращает (csCategoryID, ok). Если маппинга нет — ok=false (товар пропускаем).
func (p *APIPusher) resolveCSForProduct(hubProductID int64) (int, bool) {
	var otCat, titleRu, titleOrig, gender string
	err := p.store.Hub.QueryRow(
		`SELECT category_id, IFNULL(title_ru,''), IFNULL(title_original,''), IFNULL(gender,'')
		 FROM products WHERE id=?`, hubProductID).Scan(&otCat, &titleRu, &titleOrig, &gender)
	if err != nil {
		return 0, false
	}
	mapping, err := p.store.GetCategoryMappingByOT(otCat)
	if err != nil || mapping == nil || mapping.CSCategoryID == 0 {
		return 0, false
	}
	return mapping.ResolveCategoryID(titleRu, titleOrig, gender), true
}

// PushProductsAuto — push списка товаров; CS-категория каждого определяется
// автоматически по его OT-категории и маппингу (товары могут быть из разных категорий).
func (p *APIPusher) PushProductsAuto(productIDs []int64) *PushResult {
	res := &PushResult{}
	res.logMsg(fmt.Sprintf("Push выбранных товаров: %d шт", len(productIDs)))
	for i, id := range productIDs {
		csCat, ok := p.resolveCSForProduct(id)
		if !ok {
			res.logMsg(fmt.Sprintf("[%d/%d] product_id=%d — нет маппинга категории, пропуск", i+1, len(productIDs), id))
			res.Errors++
			continue
		}
		res.logMsg(fmt.Sprintf("[%d/%d] product_id=%d -> CS cat %d...", i+1, len(productIDs), id, csCat))
		csID, err := p.PushSingleProduct(id, csCat)
		if err != nil {
			res.logMsg(fmt.Sprintf("  ERROR: %v", err))
			res.Errors++
			continue
		}
		res.logMsg(fmt.Sprintf("  OK -> cs_product_id=%d", csID))
		res.Pushed++
	}
	res.logMsg(fmt.Sprintf("Готово: %d pushed, %d errors", res.Pushed, res.Errors))
	return res
}

// PushAllUnpushed — push ВСЕХ ещё не отправленных товаров (cs_product_id IS NULL,
// enabled, в наличии). CS-категория определяется по маппингу каждого товара.
func (p *APIPusher) PushAllUnpushed() *PushResult {
	rows, err := p.store.Hub.Query(
		`SELECT id FROM products
		 WHERE cs_product_id IS NULL AND enabled=1
		   AND is_sell_allowed=1 AND is_expired=0 AND master_quantity>0
		 ORDER BY id ASC`)
	if err != nil {
		res := &PushResult{}
		res.logMsg(fmt.Sprintf("ERROR query: %v", err))
		return res
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	return p.PushProductsAuto(ids)
}
