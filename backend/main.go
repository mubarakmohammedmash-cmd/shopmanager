package main

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
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

var (
	instanceID, _ = os.Hostname()
	startedAt     = time.Now().UTC().Format(time.RFC3339)
)

func configuredDBPath() (string, error) {
	dbPath := strings.TrimSpace(os.Getenv("DB_PATH"))
	if dbPath == "" {
		if strings.EqualFold(os.Getenv("RENDER"), "true") {
			return "", errors.New("DB_PATH must point to a file under the persistent disk mount /var/data (for example, /var/data/shop.db)")
		}
		return "shop.db", nil
	}
	if strings.EqualFold(os.Getenv("RENDER"), "true") {
		cleanPath := filepath.Clean(dbPath)
		relativePath, err := filepath.Rel("/var/data", cleanPath)
		if err != nil || relativePath == "." || relativePath == ".." ||
			strings.HasPrefix(relativePath, ".."+string(os.PathSeparator)) {
			return "", errors.New("DB_PATH must point to a database file under the persistent disk mount /var/data")
		}
		dbPath = cleanPath
	}
	return dbPath, nil
}

func main() {
	var err error

	// Database: Turso (libSQL) when configured, otherwise local SQLite.
	tursoURL := strings.TrimSpace(os.Getenv("TURSO_DATABASE_URL"))
	if tursoURL != "" {
		db, err = sql.Open("libsql", tursoURL+"?authToken="+os.Getenv("TURSO_AUTH_TOKEN"))
	} else {
		dbPath, pathErr := configuredDBPath()
		if pathErr != nil {
			log.Fatal("Invalid database configuration: ", pathErr)
		}
		db, err = sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	}
	if err != nil {
		log.Fatal("Failed to open database: ", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		log.Fatal("Failed to connect to database: ", err)
	}

	if _, err := db.Exec(schema); err != nil {
		log.Fatal("Failed to initialize database schema: ", err)
	}

	// Existing database migrations.
	migrate()
	migrateAuth()
	migratePayments()
	migrateExpenses()

	// Register API routes.
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ShopManager API is running\ninstance: " + instanceID + "\nstarted: " + startedAt + "\n"))
	})

	mux.HandleFunc("GET /products", listProductsV2)
	mux.HandleFunc("POST /products", createProduct)
	mux.HandleFunc("PUT /products/{id}", updateProduct)
	mux.HandleFunc("POST /products/{id}/restock", restockProduct)

	mux.HandleFunc("POST /sales", createSaleV3)
	mux.HandleFunc("GET /sales/recent", recentSales2)
	mux.HandleFunc("GET /sales/history", salesHistoryV2)
	mux.HandleFunc("GET /sales/outstanding", outstandingSales)
	mux.HandleFunc("GET /sales/{id}/payments", salePayments)
	mux.HandleFunc("POST /sales/{id}/payments", recordSalePayment)

	mux.HandleFunc("GET /expenses", listExpenses)
	mux.HandleFunc("POST /expenses", createExpense)
	mux.HandleFunc("POST /expenses/{id}/review", reviewExpense)
	mux.HandleFunc("POST /expenses/{id}/reimburse", reimburseExpense)

	mux.HandleFunc("GET /reports/today", todayReport2)
	mux.HandleFunc("POST /sales/{id}/void", voidSaleV2)
	mux.HandleFunc("GET /reports/range", rangeReport)
	mux.HandleFunc("GET /reports/export", exportCSV)
	mux.HandleFunc("POST /assistant/chat", assistantChat)
	mux.HandleFunc("POST /assistant/transcribe", assistantTranscribe)

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
		w.Header().Set("Cache-Control", "no-store")
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

func queryProducts(storeID int64, condition string) ([]Product, error) {
	query := `
		SELECT id, name, cost_price, selling_price, stock, low_stock_level
		FROM products WHERE store_id = ?`
	if condition != "" {
		query += " AND " + strings.TrimPrefix(condition, "WHERE ")
	}
	query += `
		ORDER BY name
	`
	rows, err := db.Query(query, storeID)
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
			(name, cost_price, selling_price, stock, low_stock_level, store_id)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		p.Name,
		p.CostPrice,
		p.SellingPrice,
		p.Stock,
		p.LowStockLevel,
		currentUser(r).StoreID,
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
		WHERE id = ? AND store_id = ?
	`,
		p.Name,
		p.CostPrice,
		p.SellingPrice,
		p.LowStockLevel,
		id,
		currentUser(r).StoreID,
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
		`UPDATE products SET stock = stock + ? WHERE id = ? AND store_id = ?`,
		req.Qty,
		id,
		currentUser(r).StoreID,
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

type RecentSale struct {
	ID        int64  `json:"id"`
	CreatedAt string `json:"created_at"`
	Total     int64  `json:"total"`
	Profit    int64  `json:"profit"`
	Units     int    `json:"units"`
}

type TopProduct struct {
	Name   string `json:"name"`
	Units  int    `json:"units"`
	Profit int64  `json:"profit"`
}
