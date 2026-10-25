package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// Adds the new sales columns to an existing database (safe to run every start).
func migrate() {
	rows, err := db.Query(`PRAGMA table_info(sales)`)
	if err != nil {
		log.Fatal(err)
	}
	have := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			log.Fatal(err)
		}
		have[name] = true
	}
	rows.Close()

	add := [][2]string{
		{"status", `TEXT NOT NULL DEFAULT 'completed'`},
		{"payment_method", `TEXT NOT NULL DEFAULT 'cash'`},
		{"voided_at", `TEXT`},
		{"amount_received", `INTEGER NOT NULL DEFAULT 0`},
		{"change_given", `INTEGER NOT NULL DEFAULT 0`},
	}
	for _, c := range add {
		if !have[c[0]] {
			if _, err := db.Exec(`ALTER TABLE sales ADD COLUMN ` + c[0] + ` ` + c[1]); err != nil {
				log.Fatal(err)
			}
		}
	}
	// Existing completed sales are treated as paid in full.
	if _, err := db.Exec(`UPDATE sales SET amount_received = total
		WHERE amount_received = 0 AND total > 0`); err != nil {
		log.Fatal(err)
	}
}

type saleItemReq2 struct {
	ProductID int64  `json:"product_id"`
	Qty       int    `json:"qty"`
	Price     *int64 `json:"price"`
}

type saleReq2 struct {
	Items          []saleItemReq2 `json:"items"`
	PaymentMethod  string         `json:"payment_method"`
	AmountReceived int64          `json:"amount_received"`
}

var validPayment = map[string]bool{"cash": true, "transfer": true, "pos": true}

func createSaleV2(w http.ResponseWriter, r *http.Request) {
	var req saleReq2
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Items) == 0 {
		http.Error(w, "send at least one item", 400)
		return
	}
	method := req.PaymentMethod
	if method == "" {
		method = "cash"
	}
	if !validPayment[method] {
		http.Error(w, "payment must be cash, transfer or pos", 400)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO sales (total) VALUES (0)`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	saleID, _ := res.LastInsertId()

	var total int64
	for _, it := range req.Items {
		if it.Qty <= 0 {
			http.Error(w, "qty must be positive", 400)
			return
		}
		var name string
		var price, cost int64
		var stock int
		err := tx.QueryRow(`SELECT name, selling_price, cost_price, stock FROM products WHERE id = ?`,
			it.ProductID).Scan(&name, &price, &cost, &stock)
		if err == sql.ErrNoRows {
			http.Error(w, "product not found", 404)
			return
		} else if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if it.Price != nil {
			if *it.Price < 0 {
				http.Error(w, "price cannot be negative", 400)
				return
			}
			price = *it.Price
		}
		if stock < it.Qty {
			http.Error(w, "not enough stock for "+name, 409)
			return
		}
		if _, err := tx.Exec(`INSERT INTO sale_items (sale_id, product_id, qty, unit_price, unit_cost)
			VALUES (?, ?, ?, ?, ?)`, saleID, it.ProductID, it.Qty, price, cost); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if _, err := tx.Exec(`UPDATE products SET stock = stock - ? WHERE id = ?`, it.Qty, it.ProductID); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		total += price * int64(it.Qty)
	}

	if _, err := tx.Exec(`UPDATE sales SET total = ?, payment_method = ? WHERE id = ?`, total, method, saleID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 201, map[string]any{"sale_id": saleID, "total": total})
}

// Cancels a sale and puts its items back into stock. The sale stays in history as "voided".
func voidSale(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad sale id", 400)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	var status string
	err = tx.QueryRow(`SELECT status FROM sales WHERE id = ?`, id).Scan(&status)
	if err == sql.ErrNoRows {
		http.Error(w, "sale not found", 404)
		return
	} else if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if status != "completed" {
		http.Error(w, "this sale is already voided", 409)
		return
	}

	rows, err := tx.Query(`SELECT product_id, qty FROM sale_items WHERE sale_id = ?`, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	type line struct {
		productID int64
		qty       int
	}
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.productID, &l.qty); err != nil {
			rows.Close()
			http.Error(w, err.Error(), 500)
			return
		}
		lines = append(lines, l)
	}
	rows.Close()

	for _, l := range lines {
		if _, err := tx.Exec(`UPDATE products SET stock = stock + ? WHERE id = ?`, l.qty, l.productID); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	if _, err := tx.Exec(`UPDATE sales SET status = 'voided', voided_at = datetime('now') WHERE id = ?`, id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func recentSales2(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT s.id, s.created_at, s.total,
			SUM(si.qty * (si.unit_price - si.unit_cost)), SUM(si.qty)
		FROM sales s JOIN sale_items si ON si.sale_id = s.id
		WHERE s.status = 'completed'
		GROUP BY s.id ORDER BY s.id DESC LIMIT 10`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []RecentSale{}
	for rows.Next() {
		var s RecentSale
		if err := rows.Scan(&s.ID, &s.CreatedAt, &s.Total, &s.Profit, &s.Units); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		out = append(out, s)
	}
	writeJSON(w, 200, out)
}
