package push

import (
	"fmt"
	"log"
	"otapi-hub/db"
	"time"
)

type Pusher struct {
	store *db.Store
}

func New(store *db.Store) *Pusher {
	return &Pusher{store: store}
}

type PushResult struct {
	Pushed int
	Errors int
	Log    []string
}

func (r *PushResult) logMsg(msg string) {
	r.Log = append(r.Log, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg))
	log.Println(msg)
}

func (p *Pusher) ExecuteQueue() *PushResult {
	res := &PushResult{}

	rows, err := p.store.Hub.Query(`
		SELECT q.id, q.product_id, q.action,
		       pr.otapi_id, pr.title_ru, pr.title_original,
		       pr.price_tmt, pr.master_quantity,
		       IFNULL(pr.main_image_url,''),
		       pr.category_id,
		       IFNULL(cc.cs_category_id, 0)
		FROM push_queue q
		JOIN products pr ON pr.id = q.product_id
		LEFT JOIN category_config cc ON cc.category_id = pr.category_id
		WHERE q.status = 'pending'
		ORDER BY q.created_at ASC
		LIMIT 50`)
	if err != nil {
		res.logMsg(fmt.Sprintf("ERROR query queue: %v", err))
		return res
	}
	defer rows.Close()

	type queueRow struct {
		QueueID      int
		ProductID    int64
		Action       string
		OtapiID      string
		TitleRu      string
		TitleOrig    string
		PriceTMT     float64
		Quantity     int
		ImageURL     string
		CategoryID   string
		CSCategoryID int
	}

	var items []queueRow
	for rows.Next() {
		var item queueRow
		if err := rows.Scan(
			&item.QueueID, &item.ProductID, &item.Action,
			&item.OtapiID, &item.TitleRu, &item.TitleOrig,
			&item.PriceTMT, &item.Quantity,
			&item.ImageURL,
			&item.CategoryID, &item.CSCategoryID,
		); err != nil {
			res.logMsg(fmt.Sprintf("ERROR scan: %v", err))
			continue
		}
		items = append(items, item)
	}
	rows.Close()

	for _, item := range items {
		csProductID, err := p.pushProduct(item.OtapiID, item.TitleRu, item.TitleOrig,
			item.PriceTMT, item.Quantity, item.CSCategoryID)
		if err != nil {
			res.logMsg(fmt.Sprintf("ERROR push %s: %v", item.OtapiID, err))
			p.store.Hub.Exec(`UPDATE push_queue SET status='error', error_msg=?, pushed_at=?
				WHERE id=?`, err.Error(), time.Now().Unix(), item.QueueID)
			res.Errors++
			continue
		}

		p.pushImages(item.ProductID, csProductID)

		p.store.Hub.Exec(`UPDATE push_queue SET status='done', cs_product_id=?, pushed_at=?
			WHERE id=?`, csProductID, time.Now().Unix(), item.QueueID)
		p.store.Hub.Exec(`UPDATE products SET cs_product_id=?, pushed_to_cs_at=? WHERE id=?`,
			csProductID, time.Now().Unix(), item.ProductID)

		res.logMsg(fmt.Sprintf("OK %s -> cs_product_id=%d", item.OtapiID, csProductID))
		res.Pushed++
	}

	res.logMsg(fmt.Sprintf("Готово: %d запушено, %d ошибок", res.Pushed, res.Errors))
	return res
}

// PushCategoryDirect пушит все товары категории напрямую (без очереди).
func (p *Pusher) PushCategoryDirect(categoryID string, csCategoryID int) *PushResult {
	res := &PushResult{}

	rows, err := p.store.Hub.Query(`
		SELECT id, otapi_id, title_ru, title_original,
		       price_tmt, master_quantity, IFNULL(main_image_url,'')
		FROM products
		WHERE category_id = ? AND is_sell_allowed = 1 AND is_expired = 0
		ORDER BY id ASC`, categoryID)
	if err != nil {
		res.logMsg(fmt.Sprintf("ERROR query products: %v", err))
		return res
	}
	defer rows.Close()

	type prodRow struct {
		ID        int64
		OtapiID   string
		TitleRu   string
		TitleOrig string
		PriceTMT  float64
		Quantity  int
		ImageURL  string
	}

	var products []prodRow
	for rows.Next() {
		var pr prodRow
		rows.Scan(&pr.ID, &pr.OtapiID, &pr.TitleRu, &pr.TitleOrig,
			&pr.PriceTMT, &pr.Quantity, &pr.ImageURL)
		products = append(products, pr)
	}
	rows.Close()

	for _, pr := range products {
		csProductID, err := p.pushProduct(pr.OtapiID, pr.TitleRu, pr.TitleOrig,
			pr.PriceTMT, pr.Quantity, csCategoryID)
		if err != nil {
			res.logMsg(fmt.Sprintf("ERROR push %s: %v", pr.OtapiID, err))
			res.Errors++
			continue
		}

		p.pushImages(pr.ID, csProductID)

		now := time.Now().Unix()
		p.store.Hub.Exec(`UPDATE products SET cs_product_id=?, pushed_to_cs_at=? WHERE id=?`,
			csProductID, now, pr.ID)

		res.logMsg(fmt.Sprintf("OK %s -> cs_product_id=%d", pr.OtapiID, csProductID))
		res.Pushed++
	}

	res.logMsg(fmt.Sprintf("Готово: %d запушено, %d ошибок", res.Pushed, res.Errors))
	return res
}

func (p *Pusher) pushProduct(otapiID, titleRu, titleOrig string, priceTMT float64, qty, csCategoryID int) (int, error) {
	now := time.Now().Unix()

	title := titleRu
	if title == "" {
		title = titleOrig
	}

	var existingID int
	err := p.store.Mirror.QueryRow(
		`SELECT product_id FROM cscart_products WHERE source_import_key=?`, otapiID,
	).Scan(&existingID)

	if err != nil {
		result, err := p.store.Mirror.Exec(`
			INSERT INTO cscart_products
			  (product_code, product_type, status, company_id, amount,
			   timestamp, updated_timestamp, source_import_key,
			   usergroup_ids, tracking, free_shipping, is_returnable,
			   return_period, shipping_params, facebook_obj_type, buy_now_url,
			   zero_price_action, is_pbp, is_op, is_oper, is_edp,
			   edp_shipping, unlimited_download, age_verification,
			   options_type, exceptions_type, details_layout)
			VALUES (?, 'P', 'D', 376, ?,
			        ?, ?, ?,
			        '0', 'O', 'N', 'Y',
			        10, 'a:5:{s:16:"min_items_in_box";i:0;s:16:"max_items_in_box";i:0;s:10:"box_length";i:0;s:9:"box_width";i:0;s:10:"box_height";i:0;}', '', '',
			        'R', 'N', 'N', 'N', 'N',
			        'N', 'N', 'N',
			        'P', 'F', 'default')`,
			otapiID, qty, now, now, otapiID)
		if err != nil {
			return 0, fmt.Errorf("insert product: %w", err)
		}
		newID64, _ := result.LastInsertId()
		existingID = int(newID64)
	} else {
		p.store.Mirror.Exec(`
			UPDATE cscart_products SET amount=?, updated_timestamp=? WHERE product_id=?`,
			qty, now, existingID)
	}

	// Описание (ru)
	p.store.Mirror.Exec(`
		INSERT INTO cscart_product_descriptions (product_id, lang_code, product)
		VALUES (?, 'ru', ?)
		ON DUPLICATE KEY UPDATE product=VALUES(product)`,
		existingID, title)

	// Цена (price + percentage_discount=0, lower_limit=1, usergroup_id=0)
	p.store.Mirror.Exec(`
		INSERT INTO cscart_product_prices (product_id, price, percentage_discount, lower_limit, usergroup_id)
		VALUES (?, ?, 0, 1, 0)
		ON DUPLICATE KEY UPDATE price=VALUES(price)`,
		existingID, priceTMT)

	if csCategoryID > 0 {
		p.store.Mirror.Exec(`
			INSERT IGNORE INTO cscart_products_categories (product_id, category_id, link_type)
			VALUES (?, ?, 'M')`,
			existingID, csCategoryID)
	}

	return existingID, nil
}

func (p *Pusher) pushImages(hubProductID int64, csProductID int) {
	rows, err := p.store.Hub.Query(`
		SELECT url, is_main, position FROM product_images
		WHERE product_id = ? ORDER BY position ASC`, hubProductID)
	if err != nil {
		return
	}
	defer rows.Close()

	// Удаляем старые привязки
	p.store.Mirror.Exec(`DELETE il FROM cscart_images_links il
		WHERE il.object_id = ? AND il.object_type = 'product'`, csProductID)

	for rows.Next() {
		var imgURL string
		var isMain bool
		var position int
		rows.Scan(&imgURL, &isMain, &position)

		if imgURL == "" {
			continue
		}

		// Вставляем запись в cscart_images (detailed = полное фото)
		result, err := p.store.Mirror.Exec(`
			INSERT INTO cscart_images (image_path, image_x, image_y)
			VALUES (?, 0, 0)`, imgURL)
		if err != nil {
			continue
		}
		detailedID, _ := result.LastInsertId()

		// Тип: M = main (первое), A = additional (остальные)
		linkType := "A"
		if isMain {
			linkType = "M"
		}

		p.store.Mirror.Exec(`
			INSERT INTO cscart_images_links (object_id, object_type, image_id, detailed_id, type, position)
			VALUES (?, 'product', 0, ?, ?, ?)`,
			csProductID, detailedID, linkType, position)
	}
}

