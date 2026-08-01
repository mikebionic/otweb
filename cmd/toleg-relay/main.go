// Toleg Payment Relay — QR-платёж Toleg/Belgi для WBRM.
//
// Держит секреты (Bearer-токен, merchantId), проксирует вызовы к Toleg,
// верифицирует статус и (при ENABLED) помечает заказ CS-Cart оплаченным.
// Обслуживает и веб-витрину, и мобильное приложение через одни эндпоинты.
//
// Безопасность: секреты только здесь (env), заказ = оплачен ТОЛЬКО после
// серверной проверки статуса=1 у Toleg. Клиенту не верим.
//
// Конфиг — переменные окружения (см. toleg-relay.env / systemd unit):
//   TOLEG_LISTEN           адрес прослушивания (":5600")
//   TOLEG_ENABLED          "true" чтобы разрешить intent и пометку оплаты (по умолч. false)
//   TOLEG_BASE             https://ta.belgi.com.tm
//   TOLEG_QR_BASE          https://tolegapp.com/qrpay/merchant
//   TOLEG_TOKEN            Bearer-токен Toleg (СЕКРЕТ)
//   TOLEG_MERCHANT         encrypted merchantId для QR (пусто = intent отдаёт 503)
//   TOLEG_AMOUNT_MULT      множитель суммы: 1 (манаты) или 100 (субъединица). ОБЯЗАТЕЛЬНО уточнить.
//   TOLEG_DSN              MySQL DSN для таблицы toleg_payments
//   TOLEG_INTERNAL_SECRET  общий секрет; заголовок X-Relay-Secret на /pay/intent и /pay/refund
//   CSCART_API_URL         https://wabrum.com/api (для пометки заказа оплаченным)
//   CSCART_API_EMAIL       e-mail API-пользователя CS-Cart
//   CSCART_API_KEY         API-ключ CS-Cart
//   CSCART_PAID_STATUS     код статуса «оплачен/в обработке» в CS-Cart (напр. "P")
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type config struct {
	listen, base, qrBase, token, merchant, dsn, secret string
	amountMult                                         int
	enabled                                            bool
	csURL, csEmail, csKey, csPaidStatus                string
}

var (
	cfg config
	db  *sql.DB
	hc  = &http.Client{Timeout: 30 * time.Second}
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	cfg = config{
		listen:       env("TOLEG_LISTEN", ":5600"),
		base:         strings.TrimRight(env("TOLEG_BASE", "https://ta.belgi.com.tm"), "/"),
		qrBase:       strings.TrimRight(env("TOLEG_QR_BASE", "https://tolegapp.com/qrpay/merchant"), "/"),
		token:        env("TOLEG_TOKEN", ""),
		merchant:     env("TOLEG_MERCHANT", ""),
		dsn:          env("TOLEG_DSN", ""),
		secret:       env("TOLEG_INTERNAL_SECRET", ""),
		enabled:      env("TOLEG_ENABLED", "false") == "true",
		csURL:        strings.TrimRight(env("CSCART_API_URL", ""), "/"),
		csEmail:      env("CSCART_API_EMAIL", ""),
		csKey:        env("CSCART_API_KEY", ""),
		csPaidStatus: env("CSCART_PAID_STATUS", "P"),
	}
	cfg.amountMult = 1
	if env("TOLEG_AMOUNT_MULT", "1") == "100" {
		cfg.amountMult = 100
	}
	if cfg.token == "" || cfg.dsn == "" {
		log.Fatal("[toleg] TOLEG_TOKEN и TOLEG_DSN обязательны")
	}

	var err error
	db, err = sql.Open("mysql", cfg.dsn)
	if err != nil {
		log.Fatalf("[toleg] db open: %v", err)
	}
	if err = db.Ping(); err != nil {
		log.Fatalf("[toleg] db ping: %v", err)
	}
	ensureSchema()

	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/pay/intent", handleIntent)
	http.HandleFunc("/pay/status/", handleStatus)
	http.HandleFunc("/pay/refund", handleRefund)

	log.Printf("[toleg] relay слушает %s (enabled=%v, merchant_set=%v, amount_mult=%d)",
		cfg.listen, cfg.enabled, cfg.merchant != "", cfg.amountMult)
	log.Fatal(http.ListenAndServe(cfg.listen, nil))
}

func ensureSchema() {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS toleg_payments (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		order_id VARCHAR(64) NOT NULL UNIQUE,
		amount_tmt DECIMAL(12,2) NOT NULL DEFAULT 0,
		status TINYINT NOT NULL DEFAULT 0,   -- 0 нов/ожид,1 успех,2 неуспех,3 неизв,4 возврат
		internal_txid BIGINT DEFAULT NULL,   -- id транзакции Toleg (для рефанда)
		card VARCHAR(32) DEFAULT NULL,
		card_holder VARCHAR(128) DEFAULT NULL,
		bank_type VARCHAR(32) DEFAULT NULL,
		marked_paid TINYINT NOT NULL DEFAULT 0,
		created_at BIGINT NOT NULL,
		updated_at BIGINT NOT NULL,
		INDEX idx_status (status)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		log.Fatalf("[toleg] schema: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]interface{}{"ok": true, "enabled": cfg.enabled, "merchant_set": cfg.merchant != ""})
}

// POST /pay/intent {order_id, amount_tmt} -> {qr_url, order_id}
func handleIntent(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Relay-Secret") != cfg.secret || cfg.secret == "" {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	if !cfg.enabled || cfg.merchant == "" {
		writeJSON(w, 503, map[string]string{"error": "toleg disabled or merchant not configured"})
		return
	}
	var body struct {
		OrderID   string  `json:"order_id"`
		AmountTMT float64 `json:"amount_tmt"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.OrderID == "" || body.AmountTMT <= 0 {
		writeJSON(w, 400, map[string]string{"error": "order_id and amount_tmt required"})
		return
	}
	now := time.Now().Unix()
	_, err := db.Exec(`INSERT INTO toleg_payments (order_id, amount_tmt, status, created_at, updated_at)
		VALUES (?,?,0,?,?) ON DUPLICATE KEY UPDATE amount_tmt=VALUES(amount_tmt), updated_at=VALUES(updated_at)`,
		body.OrderID, body.AmountTMT, now, now)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "db"})
		return
	}
	amount := int(body.AmountTMT * float64(cfg.amountMult))
	qr := fmt.Sprintf("%s/%s?amount=%d&pos_txid=%s", cfg.qrBase, cfg.merchant, amount, body.OrderID)
	log.Printf("[toleg] intent order=%s amount_tmt=%.2f -> amount=%d", body.OrderID, body.AmountTMT, amount)
	writeJSON(w, 200, map[string]string{"order_id": body.OrderID, "qr_url": qr})
}

type tolegStatusResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		ID        int64  `json:"id"`
		ExtID     string `json:"ext_id"`
		Status    int    `json:"status"`
		CreatedAt string `json:"created_at"`
	} `json:"data"`
}

// GET /pay/status/{order_id}
func handleStatus(w http.ResponseWriter, r *http.Request) {
	orderID := strings.TrimPrefix(r.URL.Path, "/pay/status/")
	if orderID == "" {
		writeJSON(w, 400, map[string]string{"error": "order_id required"})
		return
	}
	st, err := tolegStatus(orderID)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	now := time.Now().Unix()
	db.Exec(`UPDATE toleg_payments SET status=?, internal_txid=?, updated_at=? WHERE order_id=?`,
		st.Data.Status, st.Data.ID, now, orderID)

	// Верифицированный успех -> пометить заказ оплаченным (идемпотентно, только при ENABLED)
	if st.Data.Status == 1 && cfg.enabled {
		var marked int
		db.QueryRow(`SELECT marked_paid FROM toleg_payments WHERE order_id=?`, orderID).Scan(&marked)
		if marked == 0 {
			if err := markOrderPaid(orderID); err != nil {
				log.Printf("[toleg] markOrderPaid order=%s ОШИБКА: %v", orderID, err)
			} else {
				db.Exec(`UPDATE toleg_payments SET marked_paid=1, updated_at=? WHERE order_id=?`, now, orderID)
				log.Printf("[toleg] order=%s ОПЛАЧЕН, помечен в CS-Cart", orderID)
			}
		}
	}
	writeJSON(w, 200, map[string]interface{}{"order_id": orderID, "status": st.Data.Status, "toleg_code": st.Code})
}

func tolegStatus(orderID string) (*tolegStatusResp, error) {
	url := fmt.Sprintf("%s/qrpay/v1/admin/transactions/%s/status", cfg.base, orderID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+cfg.token)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var st tolegStatusResp
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("parse status: %s", string(b))
	}
	return &st, nil
}

// POST /pay/refund {order_id, amount?}
func handleRefund(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Relay-Secret") != cfg.secret || cfg.secret == "" {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	var body struct {
		OrderID string  `json:"order_id"`
		Amount  float64 `json:"amount"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.OrderID == "" {
		writeJSON(w, 400, map[string]string{"error": "order_id required"})
		return
	}
	var internalID int64
	if db.QueryRow(`SELECT internal_txid FROM toleg_payments WHERE order_id=?`, body.OrderID).Scan(&internalID) != nil || internalID == 0 {
		writeJSON(w, 404, map[string]string{"error": "transaction id not known; call status first"})
		return
	}
	payload := map[string]interface{}{}
	if body.Amount > 0 {
		payload["amount"] = int(body.Amount * float64(cfg.amountMult))
	}
	pb, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/qrpay/v1/admin/transactions/%d/refund", cfg.base, internalID)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(pb))
	req.Header.Set("Authorization", "Bearer "+cfg.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	log.Printf("[toleg] refund order=%s txid=%d -> %s", body.OrderID, internalID, string(b))
	db.Exec(`UPDATE toleg_payments SET status=4, updated_at=? WHERE order_id=?`, time.Now().Unix(), body.OrderID)
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

// markOrderPaid — помечает заказ CS-Cart оплаченным через REST API (basic auth email:api_key).
func markOrderPaid(orderID string) error {
	if cfg.csURL == "" || cfg.csKey == "" {
		return fmt.Errorf("cs-cart api не настроен")
	}
	payload, _ := json.Marshal(map[string]string{"status": cfg.csPaidStatus})
	url := fmt.Sprintf("%s/orders/%s", cfg.csURL, orderID)
	req, _ := http.NewRequest("PUT", url, bytes.NewReader(payload))
	req.SetBasicAuth(cfg.csEmail, cfg.csKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cs-cart status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
