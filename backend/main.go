package main

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"os"
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

	// SQLite database.
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "shop.db"
	}
	db, err = sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatal("Failed to open database: ", err)
	}

	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		log.Fatal("Failed to connect to database: ", err)
	}

	if _, err := db.Exec(schema); err != nil {
		log.Fatal("Failed to initialize database schema: ", err)
	}

	// Existing database migrations.
	migrate()
	migrateAuth()
	migrateExpenses()

	// Register API routes.
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ShopManager API is running"))
	})

	mux.HandleFunc("GET /products", listProductsV2)
	mux.HandleFunc("POST /products", createProduct)
	mux.HandleFunc("PUT /products/{id}", updateProduct)
	mux.HandleFunc("POST /products/{id}/restock", restockProduct)

	mux.HandleFunc("POST /sales", createSaleV3)
	mux.HandleFunc("GET /sales/recent", recentSales2)
	mux.HandleFunc("GET /sales/history", salesHistoryV2)

	mux.HandleFunc("GET /expenses", listExpenses)
	mux.HandleFunc("POST /expenses", createExpense)
	mux.HandleFunc("POST /expenses/{id}/review", reviewExpense)
	mux.HandleFunc("POST /expenses/{id}/reimburse", reimburseExpense)

	mux.HandleFunc("GET /reports/today", todayReport2)
	mux.HandleFunc("POST /sales/{id}/void", voidSaleV2)
	mux.HandleFunc("GET /reports/range", rangeReport)
	mux.HandleFunc("GET /reports/export", exportCSV)

	registerAuthRoutes(mux)

	// CORS and authentication middleware.
	handler := cors(stripAPI(withAuth(mux)))

	// Vercel may supply PORT. Use 8080 for local development.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * 1000000000, // 10 seconds
	}

	log.Printf("ShopManager API listening on port %s", port)

	if err := server.ListenAndServe(); err != nil &&
		err != http.ErrServerClosed {
		log.Fatal("API server failed: ", err)
	}
}

// Accepts both /auth/status and /api/auth/status, whichever path the host forwards.
func stripAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" {
			r.URL.Path = "/"
			r.URL.RawPath = ""
		} else if strings.HasPrefix(r.URL.Path, "/api/") {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			r.URL.RawPath = ""
		}
		next.ServeHTTP(w, r)
	})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set(
			"Access-Control-Allow-Headers",
			"Content-Type, Authorization",
		)
		w.Header().Set(
			"Access-Control-Allow-Methods",
			"GET, POST, PUT, OPTIONS",
		)

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

	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

func queryProducts(where string) ([]Product, error) {
	rows, err := db.Query(`
		SELECT id, name, cost_price, selling_price, stock, low_stock_level
		FROM products ` + where + `
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	products := []Product{}

	for rows.Next() {
		var p Product

		if err := rows.Scan(
			&p.ID,
			&p.Name,
			&p.CostPrice,
			&p.SellingPrice,
			&p.Stock,
			&p.LowStockLevel,
		); err != nil {
			return nil, err
		}

		products = append(products, p)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return products, nil
}

func listProducts(w http.ResponseWriter, r *http.Request) {
	products, err := queryProducts("")
	if err != nil {
		http.Error(w, "Failed to load products", http.StatusInternalServerError)
		log.Printf("List products error: %v", err)
		return
	}

	writeJSON(w, http.StatusOK, products)
}

func createProduct(w http.ResponseWriter, r *http.Request) {
	var p Product

	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	p.Name = strings.TrimSpace(p.Name)

	if p.Name == "" ||
		p.CostPrice < 0 ||
		p.SellingPrice < 0 ||
		p.Stock < 0 {
		http.Error(
			w,
			"name required; prices and stock must not be negative",
			http.StatusBadRequest,
		)
		return
	}

	if p.LowStockLevel < 1 {
		p.LowStockLevel = 5
	}

	res, err := db.Exec(`
		INSERT INTO products
			(name, cost_price, selling_price, stock, low_stock_level)
		VALUES (?, ?, ?, ?, ?)
	`,
		p.Name,
		p.CostPrice,
		p.SellingPrice,
		p.Stock,
		p.LowStockLevel,
	)
	if err != nil {
		log.Printf("Create product error: %v", err)
		http.Error(w, "Failed to create product", http.StatusInternalServerError)
		return
	}

	p.ID, err = res.LastInsertId()
	if err != nil {
		log.Printf("Get product ID error: %v", err)
		http.Error(w, "Product created but ID could not be retrieved",
			http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, p)
}

func updateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "bad product id", http.StatusBadRequest)
		return
	}

	var p Product

	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	p.Name = strings.TrimSpace(p.Name)

	if p.Name == "" ||
		p.CostPrice < 0 ||
		p.SellingPrice < 0 ||
		p.LowStockLevel < 1 {
		http.Error(
			w,
			"name required; prices must not be negative; alert level must be at least 1",
			http.StatusBadRequest,
		)
		return
	}

	res, err := db.Exec(`
		UPDATE products
		SET name = ?, cost_price = ?, selling_price = ?, low_stock_level = ?
		WHERE id = ?
	`,
		p.Name,
		p.CostPrice,
		p.SellingPrice,
		p.LowStockLevel,
		id,
	)
	if err != nil {
		log.Printf("Update product error: %v", err)
		http.Error(w, "Failed to update product", http.StatusInternalServerError)
		return
	}

	n, err := res.RowsAffected()
	if err != nil {
		log.Printf("Update product rows affected error: %v", err)
		http.Error(w, "Failed to verify update", http.StatusInternalServerError)
		return
	}

	if n == 0 {
		http.Error(w, "product not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func restockProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "bad product id", http.StatusBadRequest)
		return
	}

	var req struct {
		Qty int `json:"qty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		req.Qty <= 0 {
		http.Error(
			w,
			"qty must be a positive number",
			http.StatusBadRequest,
		)
		return
	}

	res, err := db.Exec(
		`UPDATE products SET stock = stock + ? WHERE id = ?`,
		req.Qty,
		id,
	)
	if err != nil {
		log.Printf("Restock product error: %v", err)
		http.Error(w, "Failed to restock product", http.StatusInternalServerError)
		return
	}

	n, err := res.RowsAffected()
	if err != nil {
		http.Error(w, "Failed to verify restock", http.StatusInternalServerError)
		return
	}

	if n == 0 {
		http.Error(w, "product not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		len(req.Items) == 0 {
		http.Error(
			w,
			`send {"items":[{"product_id":1,"qty":2}]}`,
			http.StatusBadRequest,
		)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("Begin sale transaction error: %v", err)
		http.Error(w, "Failed to start sale", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO sales (total) VALUES (0)`)
	if err != nil {
		log.Printf("Create sale error: %v", err)
		http.Error(w, "Failed to create sale", http.StatusInternalServerError)
		return
	}

	saleID, err := res.LastInsertId()
	if err != nil {
		http.Error(w, "Failed to get sale ID", http.StatusInternalServerError)
		return
	}

	var total int64

	for _, it := range req.Items {
		if it.ProductID <= 0 || it.Qty <= 0 {
			http.Error(
				w,
				"product_id and qty must be positive",
				http.StatusBadRequest,
			)
			return
		}

		var name string
		var price, cost int64
		var stock int

		err := tx.QueryRow(`
			SELECT name, selling_price, cost_price, stock
			FROM products
			WHERE id = ?
		`, it.ProductID).Scan(&name, &price, &cost, &stock)

		if err == sql.ErrNoRows {
			http.Error(w, "product not found", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("Find sale product error: %v", err)
			http.Error(w, "Failed to read product", http.StatusInternalServerError)
			return
		}

		if stock < it.Qty {
			http.Error(
				w,
				"not enough stock for "+name,
				http.StatusConflict,
			)
			return
		}

		_, err = tx.Exec(`
			INSERT INTO sale_items
				(sale_id, product_id, qty, unit_price, unit_cost)
			VALUES (?, ?, ?, ?, ?)
		`,
			saleID,
			it.ProductID,
			it.Qty,
			price,
			cost,
		)
		if err != nil {
			log.Printf("Insert sale item error: %v", err)
			http.Error(w, "Failed to add sale item", http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec(
			`UPDATE products SET stock = stock - ? WHERE id = ?`,
			it.Qty,
			it.ProductID,
		)
		if err != nil {
			log.Printf("Update stock error: %v", err)
			http.Error(w, "Failed to update stock", http.StatusInternalServerError)
			return
		}

		total += price * int64(it.Qty)
	}

	_, err = tx.Exec(
		`UPDATE sales SET total = ? WHERE id = ?`,
		total,
		saleID,
	)
	if err != nil {
		log.Printf("Update sale total error: %v", err)
		http.Error(w, "Failed to update sale total", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Commit sale error: %v", err)
		http.Error(w, "Failed to save sale", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"sale_id": saleID,
		"total":   total,
	})
}

type RecentSale struct {
	ID        int64  `json:"id"`
	CreatedAt string `json:"created_at"`
	Total     int64  `json:"total"`
	Profit    int64  `json:"profit"`
	Units     int    `json:"units"`
}

func recentSales(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT
			s.id,
			s.created_at,
			s.total,
			SUM(si.qty * (si.unit_price - si.unit_cost)),
			SUM(si.qty)
		FROM sales s
		JOIN sale_items si ON si.sale_id = s.id
		GROUP BY s.id
		ORDER BY s.id DESC
		LIMIT 10
	`)
	if err != nil {
		log.Printf("Recent sales query error: %v", err)
		http.Error(w, "Failed to load recent sales", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := []RecentSale{}

	for rows.Next() {
		var s RecentSale

		if err := rows.Scan(
			&s.ID,
			&s.CreatedAt,
			&s.Total,
			&s.Profit,
			&s.Units,
		); err != nil {
			log.Printf("Read recent sale error: %v", err)
			http.Error(w, "Failed to read recent sales", http.StatusInternalServerError)
			return
		}

		out = append(out, s)
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "Failed to read recent sales", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, out)
}

type TopProduct struct {
	Name   string `json:"name"`
	Units  int    `json:"units"`
	Profit int64  `json:"profit"`
}

func todayReport(w http.ResponseWriter, r *http.Request) {
	var revenue, profit, expenses int64
	var count int

	// Nigeria uses WAT (UTC+1).
	err := db.QueryRow(`
		SELECT
			COALESCE(SUM(si.qty * si.unit_price), 0),
			COALESCE(SUM(si.qty * (si.unit_price - si.unit_cost)), 0),
			COUNT(DISTINCT s.id)
		FROM sales s
		JOIN sale_items si ON si.sale_id = s.id
		WHERE date(s.created_at, '+1 hour') = date('now', '+1 hour')
	`).Scan(&revenue, &profit, &count)
	if err != nil {
		log.Printf("Today's sales report error: %v", err)
		http.Error(w, "Failed to generate sales report", http.StatusInternalServerError)
		return
	}

	err = db.QueryRow(`
		SELECT COALESCE(SUM(amount), 0)
		FROM expenses
		WHERE date(created_at, '+1 hour') = date('now', '+1 hour')
	`).Scan(&expenses)
	if err != nil {
		log.Printf("Today's expenses report error: %v", err)
		http.Error(w, "Failed to generate expense report", http.StatusInternalServerError)
		return
	}

	low, err := queryProducts("WHERE stock <= low_stock_level")
	if err != nil {
		log.Printf("Low stock report error: %v", err)
		http.Error(w, "Failed to load low-stock products", http.StatusInternalServerError)
		return
	}

	rows, err := db.Query(`
		SELECT
			p.name,
			SUM(si.qty),
			SUM(si.qty * (si.unit_price - si.unit_cost))
		FROM sale_items si
		JOIN products p ON p.id = si.product_id
		JOIN sales s ON s.id = si.sale_id
		WHERE s.created_at >= datetime('now', '-30 days')
		GROUP BY p.id
		ORDER BY SUM(si.qty) DESC
		LIMIT 5
	`)
	if err != nil {
		log.Printf("Top products query error: %v", err)
		http.Error(w, "Failed to load top products", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	top := []TopProduct{}

	for rows.Next() {
		var t TopProduct

		if err := rows.Scan(&t.Name, &t.Units, &t.Profit); err != nil {
			log.Printf("Read top product error: %v", err)
			http.Error(w, "Failed to read top products", http.StatusInternalServerError)
			return
		}

		top = append(top, t)
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "Failed to read top products", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"revenue":      revenue,
		"profit":       profit,
		"expenses":     expenses,
		"net_profit":   profit - expenses,
		"sales_count":  count,
		"low_stock":    low,
		"top_products": top,
	})
}
