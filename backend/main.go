package main

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

var db *sql.DB

type Product struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	CostPrice     int64  `json:"cost_price"`
	SellingPrice  int64  `json:"selling_price"`
	Stock         int    `json:"stock"`
	LowStockLevel int    `json:"low_stock_level"`
}

type Expense struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	Amount      int64  `json:"amount"`
	CreatedAt   string `json:"created_at"`
}

func main() {
	var err error
	db, err = sql.Open("sqlite", "shop.db?_pragma=foreign_keys(1)")
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(schema); err != nil {
		log.Fatal(err)
	}

	migrate()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ShopManager API is running"))
	})
	mux.HandleFunc("GET /products", listProducts)
	mux.HandleFunc("POST /products", createProduct)
	mux.HandleFunc("PUT /products/{id}", updateProduct)
	mux.HandleFunc("POST /products/{id}/restock", restockProduct)
	mux.HandleFunc("POST /sales", createSaleV2)
	mux.HandleFunc("GET /sales/recent", recentSales2)
	mux.HandleFunc("GET /sales/history", salesHistory)
	mux.HandleFunc("GET /expenses", listExpenses)
	mux.HandleFunc("POST /expenses", createExpense)
	mux.HandleFunc("GET /reports/today", todayReport2)

	mux.HandleFunc("POST /sales/{id}/void", voidSale)
	log.Println("API running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", cors(mux)))
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func queryProducts(where string) ([]Product, error) {
	rows, err := db.Query(`SELECT id, name, cost_price, selling_price, stock, low_stock_level
		FROM products ` + where + ` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	products := []Product{}
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.CostPrice, &p.SellingPrice, &p.Stock, &p.LowStockLevel); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

func listProducts(w http.ResponseWriter, r *http.Request) {
	products, err := queryProducts("")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, products)
}

func createProduct(w http.ResponseWriter, r *http.Request) {
	var p Product
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || p.CostPrice < 0 || p.SellingPrice < 0 || p.Stock < 0 {
		http.Error(w, "name required; prices and stock must not be negative", 400)
		return
	}
	if p.LowStockLevel < 1 {
		p.LowStockLevel = 5
	}
	res, err := db.Exec(`INSERT INTO products (name, cost_price, selling_price, stock, low_stock_level)
		VALUES (?, ?, ?, ?, ?)`, p.Name, p.CostPrice, p.SellingPrice, p.Stock, p.LowStockLevel)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	p.ID, _ = res.LastInsertId()
	writeJSON(w, 201, p)
}

func updateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad product id", 400)
		return
	}
	var p Product
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || p.CostPrice < 0 || p.SellingPrice < 0 || p.LowStockLevel < 1 {
		http.Error(w, "name required; prices must not be negative; alert level must be at least 1", 400)
		return
	}
	res, err := db.Exec(`UPDATE products SET name = ?, cost_price = ?, selling_price = ?, low_stock_level = ?
		WHERE id = ?`, p.Name, p.CostPrice, p.SellingPrice, p.LowStockLevel, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, "product not found", 404)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func restockProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad product id", 400)
		return
	}
	var req struct {
		Qty int `json:"qty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Qty <= 0 {
		http.Error(w, "qty must be a positive number", 400)
		return
	}
	res, err := db.Exec(`UPDATE products SET stock = stock + ? WHERE id = ?`, req.Qty, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, "product not found", 404)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

type saleItemReq struct {
	ProductID int64 `json:"product_id"`
	Qty       int   `json:"qty"`
}
type saleReq struct {
	Items []saleItemReq `json:"items"`
}

func createSale(w http.ResponseWriter, r *http.Request) {
	var req saleReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Items) == 0 {
		http.Error(w, "send {\"items\":[{\"product_id\":1,\"qty\":2}]}", 400)
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

	if _, err := tx.Exec(`UPDATE sales SET total = ? WHERE id = ?`, total, saleID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 201, map[string]any{"sale_id": saleID, "total": total})
}

type RecentSale struct {
	ID        int64  `json:"id"`
	CreatedAt string `json:"created_at"`
	Total     int64  `json:"total"`
	Profit    int64  `json:"profit"`
	Units     int    `json:"units"`
}

func recentSales(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT s.id, s.created_at, s.total,
			SUM(si.qty * (si.unit_price - si.unit_cost)), SUM(si.qty)
		FROM sales s JOIN sale_items si ON si.sale_id = s.id
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

func listExpenses(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT id, description, amount, created_at FROM expenses
		ORDER BY id DESC LIMIT 50`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []Expense{}
	for rows.Next() {
		var e Expense
		if err := rows.Scan(&e.ID, &e.Description, &e.Amount, &e.CreatedAt); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		out = append(out, e)
	}
	writeJSON(w, 200, out)
}

func createExpense(w http.ResponseWriter, r *http.Request) {
	var e Expense
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	e.Description = strings.TrimSpace(e.Description)
	if e.Description == "" || e.Amount <= 0 {
		http.Error(w, "description and a positive amount are required", 400)
		return
	}
	res, err := db.Exec(`INSERT INTO expenses (description, amount) VALUES (?, ?)`, e.Description, e.Amount)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	e.ID, _ = res.LastInsertId()
	writeJSON(w, 201, e)
}

type TopProduct struct {
	Name   string `json:"name"`
	Units  int    `json:"units"`
	Profit int64  `json:"profit"`
}

func todayReport(w http.ResponseWriter, r *http.Request) {
	var revenue, profit, expenses int64
	var count int
	// "+1 hour" shifts UTC to Lagos time (WAT) so "today" matches the shop's day
	err := db.QueryRow(`SELECT
			COALESCE(SUM(si.qty * si.unit_price), 0),
			COALESCE(SUM(si.qty * (si.unit_price - si.unit_cost)), 0),
			COUNT(DISTINCT s.id)
		FROM sales s JOIN sale_items si ON si.sale_id = s.id
		WHERE date(s.created_at, '+1 hour') = date('now', '+1 hour')`).Scan(&revenue, &profit, &count)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	err = db.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM expenses
		WHERE date(created_at, '+1 hour') = date('now', '+1 hour')`).Scan(&expenses)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	low, err := queryProducts("WHERE stock <= low_stock_level")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	rows, err := db.Query(`SELECT p.name, SUM(si.qty), SUM(si.qty * (si.unit_price - si.unit_cost))
		FROM sale_items si
		JOIN products p ON p.id = si.product_id
		JOIN sales s ON s.id = si.sale_id
		WHERE s.created_at >= datetime('now', '-30 days')
		GROUP BY p.id ORDER BY SUM(si.qty) DESC LIMIT 5`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	top := []TopProduct{}
	for rows.Next() {
		var t TopProduct
		if err := rows.Scan(&t.Name, &t.Units, &t.Profit); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		top = append(top, t)
	}

	writeJSON(w, 200, map[string]any{
		"revenue":      revenue,
		"profit":       profit,
		"expenses":     expenses,
		"net_profit":   profit - expenses,
		"sales_count":  count,
		"low_stock":    low,
		"top_products": top,
	})
}
