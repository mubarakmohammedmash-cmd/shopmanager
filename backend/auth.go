package main

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const pbkdf2Iter = 600000

type authUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type ctxKey struct{}

func currentUser(r *http.Request) authUser {
	u, _ := r.Context().Value(ctxKey{}).(authUser)
	return u
}

// ---------- database setup ----------

func hasColumn(table, col string) bool {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			log.Fatal(err)
		}
		if name == col {
			return true
		}
	}
	return false
}

func migrateAuth() {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
			password_hash TEXT NOT NULL,
			role          TEXT NOT NULL CHECK (role IN ('admin', 'rep')),
			active        INTEGER NOT NULL DEFAULT 1,
			created_at    TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token_hash TEXT PRIMARY KEY,
			user_id    INTEGER NOT NULL REFERENCES users(id),
			expires_at TEXT NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			log.Fatal(err)
		}
	}
	for _, c := range []string{"user_id", "voided_by"} {
		if !hasColumn("sales", c) {
			if _, err := db.Exec(`ALTER TABLE sales ADD COLUMN ` + c + ` INTEGER`); err != nil {
				log.Fatal(err)
			}
		}
	}
}

// ---------- passwords and tokens ----------

func hashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, pw, salt, pbkdf2Iter, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2$%d$%s$%s", pbkdf2Iter, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

func checkPassword(stored, pw string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// Used so a wrong username takes as long to reject as a wrong password.
var dummyHash, _ = hashPassword("not-a-real-password")

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

func issueSession(userID int64) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	db.Exec(`DELETE FROM sessions WHERE expires_at <= datetime('now')`)
	_, err := db.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at)
		VALUES (?, ?, datetime('now', '+12 hours'))`, hashToken(token), userID)
	return token, err
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func userFromRequest(r *http.Request) (authUser, bool) {
	t := bearer(r)
	if t == "" {
		return authUser{}, false
	}
	var u authUser
	err := db.QueryRow(`SELECT u.id, u.username, u.role
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > datetime('now') AND u.active = 1`,
		hashToken(t)).Scan(&u.ID, &u.Username, &u.Role)
	return u, err == nil
}

// ---------- wrong-password limiter ----------

var (
	failMu sync.Mutex
	fails  = map[string][]time.Time{}
)

func tooManyFails(key string) bool {
	failMu.Lock()
	defer failMu.Unlock()
	cutoff := time.Now().Add(-5 * time.Minute)
	var kept []time.Time
	for _, t := range fails[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	fails[key] = kept
	return len(kept) >= 5
}

func recordFail(key string) {
	failMu.Lock()
	defer failMu.Unlock()
	if len(fails) > 10000 {
		fails = map[string][]time.Time{}
	}
	fails[key] = append(fails[key], time.Now())
}

func clearFails(key string) {
	failMu.Lock()
	defer failMu.Unlock()
	delete(fails, key)
}

// ---------- who may call what ----------

// Anything not listed here is manager-only (safe by default).
func requiredRole(r *http.Request) string {
	p, m := r.URL.Path, r.Method
	switch {
	case m == "GET" && p == "/":
		return "public"
	case m == "GET" && p == "/auth/status":
		return "public"
	case m == "POST" && p == "/auth/setup":
		return "public"
	case m == "POST" && p == "/auth/login":
		return "public"
	case m == "GET" && p == "/auth/me":
		return "any"
	case m == "POST" && p == "/auth/logout":
		return "any"
	case m == "GET" && p == "/products":
		return "any"
	case m == "POST" && p == "/sales":
		return "any"
	case m == "GET" && p == "/sales/history":
		return "any"
	case m == "POST" && strings.HasPrefix(p, "/sales/") && strings.HasSuffix(p, "/void"):
		return "any"
	}
	return "admin"
}

func withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		need := requiredRole(r)
		if need == "public" {
			next.ServeHTTP(w, r)
			return
		}
		u, ok := userFromRequest(r)
		if !ok {
			http.Error(w, "please log in", 401)
			return
		}
		if need == "admin" && u.Role != "admin" {
			http.Error(w, "managers only", 403)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

// ---------- login, setup, accounts ----------

type credsReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{3,30}$`)

func validCreds(username, password string) string {
	if !usernameRe.MatchString(username) {
		return "username must be 3 to 30 letters, numbers, dots, dashes or underscores"
	}
	if len(password) < 6 || len(password) > 200 {
		return "password must be 6 to 200 characters"
	}
	return ""
}

func userCount() (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func authStatus(w http.ResponseWriter, r *http.Request) {
	n, err := userCount()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]any{"setup_needed": n == 0})
}

// Creates the first manager account. Only works while there are no users at all.
func setup(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var in credsReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if msg := validCreds(in.Username, in.Password); msg != "" {
		http.Error(w, msg, 400)
		return
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	res, err := db.Exec(`INSERT INTO users (username, password_hash, role)
		SELECT ?, ?, 'admin' WHERE NOT EXISTS (SELECT 1 FROM users)`, in.Username, hash)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, "setup is already done", 409)
		return
	}
	id, _ := res.LastInsertId()
	token, err := issueSession(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 201, map[string]any{"token": token, "user": authUser{ID: id, Username: in.Username, Role: "admin"}})
}

func login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var in credsReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	name := strings.TrimSpace(in.Username)
	key := strings.ToLower(name)
	if tooManyFails(key) {
		http.Error(w, "too many wrong attempts, try again in 5 minutes", 429)
		return
	}

	var u authUser
	var hash string
	var active int
	err := db.QueryRow(`SELECT id, username, role, password_hash, active FROM users WHERE username = ?`,
		name).Scan(&u.ID, &u.Username, &u.Role, &hash, &active)
	if err != nil {
		checkPassword(dummyHash, in.Password)
		recordFail(key)
		http.Error(w, "wrong username or password", 401)
		return
	}
	if active != 1 || !checkPassword(hash, in.Password) {
		recordFail(key)
		http.Error(w, "wrong username or password", 401)
		return
	}
	clearFails(key)
	token, err := issueSession(u.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]any{"token": token, "user": u})
}

func logout(w http.ResponseWriter, r *http.Request) {
	db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashToken(bearer(r)))
	writeJSON(w, 200, map[string]any{"ok": true})
}

func me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, currentUser(r))
}

type UserRow struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
}

func listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT id, username, role, active, created_at FROM users ORDER BY id`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []UserRow{}
	for rows.Next() {
		var u UserRow
		var active int
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &active, &u.CreatedAt); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		u.Active = active == 1
		out = append(out, u)
	}
	writeJSON(w, 200, out)
}

func createUser(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var in credsReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if msg := validCreds(in.Username, in.Password); msg != "" {
		http.Error(w, msg, 400)
		return
	}
	if in.Role == "" {
		in.Role = "rep"
	}
	if in.Role != "rep" && in.Role != "admin" {
		http.Error(w, "role must be rep or admin", 400)
		return
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	res, err := db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`,
		in.Username, hash, in.Role)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			http.Error(w, "that username is already taken", 409)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, authUser{ID: id, Username: in.Username, Role: in.Role})
}

func resetPassword(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad user id", 400)
		return
	}
	var in credsReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if len(in.Password) < 6 || len(in.Password) > 200 {
		http.Error(w, "password must be 6 to 200 characters", 400)
		return
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	res, err := db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, hash, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, "user not found", 404)
		return
	}
	db.Exec(`DELETE FROM sessions WHERE user_id = ?`, id)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func setActive(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad user id", 400)
		return
	}
	var in struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if !in.Active && id == currentUser(r).ID {
		http.Error(w, "you cannot deactivate your own account", 400)
		return
	}
	flag := 0
	if in.Active {
		flag = 1
	}
	res, err := db.Exec(`UPDATE users SET active = ? WHERE id = ?`, flag, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.Error(w, "user not found", 404)
		return
	}
	if !in.Active {
		db.Exec(`DELETE FROM sessions WHERE user_id = ?`, id)
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func registerAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/status", authStatus)
	mux.HandleFunc("POST /auth/setup", setup)
	mux.HandleFunc("POST /auth/login", login)
	mux.HandleFunc("POST /auth/logout", logout)
	mux.HandleFunc("GET /auth/me", me)
	mux.HandleFunc("GET /users", listUsers)
	mux.HandleFunc("POST /users", createUser)
	mux.HandleFunc("POST /users/{id}/password", resetPassword)
	mux.HandleFunc("POST /users/{id}/active", setActive)
}

// ---------- role-aware versions of products and sales ----------

type RepProduct struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	SellingPrice  int64  `json:"selling_price"`
	Stock         int    `json:"stock"`
	LowStockLevel int    `json:"low_stock_level"`
}

// Managers get cost prices. Reps never receive them.
func listProductsV2(w http.ResponseWriter, r *http.Request) {
	products, err := queryProducts("")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if currentUser(r).Role == "admin" {
		writeJSON(w, 200, products)
		return
	}
	out := make([]RepProduct, 0, len(products))
	for _, p := range products {
		out = append(out, RepProduct{p.ID, p.Name, p.SellingPrice, p.Stock, p.LowStockLevel})
	}
	writeJSON(w, 200, out)
}

func createSaleV3(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
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

	res, err := tx.Exec(`INSERT INTO sales (total, user_id) VALUES (0, ?)`, u.ID)
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
		// Only managers may change the price. For reps the saved price is always used.
		if it.Price != nil && u.Role == "admin" {
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

// Reps can void only their own sales. The manager can void any sale. Who voided is recorded.
func voidSaleV2(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
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
	var owner sql.NullInt64
	err = tx.QueryRow(`SELECT status, user_id FROM sales WHERE id = ?`, id).Scan(&status, &owner)
	if err == sql.ErrNoRows {
		http.Error(w, "sale not found", 404)
		return
	} else if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if u.Role != "admin" && (!owner.Valid || owner.Int64 != u.ID) {
		http.Error(w, "you can only void your own sales", 403)
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
	if _, err := tx.Exec(`UPDATE sales SET status = 'voided', voided_at = datetime('now'), voided_by = ? WHERE id = ?`,
		u.ID, id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

type HistoryRow struct {
	ID            int64  `json:"id"`
	CreatedAt     string `json:"created_at"`
	Total         int64  `json:"total"`
	Profit        *int64 `json:"profit,omitempty"`
	Items         string `json:"items"`
	PaymentMethod string `json:"payment_method"`
	Status        string `json:"status"`
	SoldBy        string `json:"sold_by"`
	VoidedBy      string `json:"voided_by,omitempty"`
}

// Managers see every sale with profit. Reps see only their own sales, with no profit.
func salesHistoryV2(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	q := `SELECT s.id, s.created_at, s.total,
			SUM(si.qty * (si.unit_price - si.unit_cost)),
			GROUP_CONCAT(p.name || ' x' || si.qty, ', '),
			s.payment_method, s.status,
			COALESCE(su.username, ''), COALESCE(vu.username, '')
		FROM sales s
		JOIN sale_items si ON si.sale_id = s.id
		JOIN products p ON p.id = si.product_id
		LEFT JOIN users su ON su.id = s.user_id
		LEFT JOIN users vu ON vu.id = s.voided_by`
	args := []any{}
	if u.Role != "admin" {
		q += ` WHERE s.user_id = ?`
		args = append(args, u.ID)
	}
	q += ` GROUP BY s.id ORDER BY s.id DESC LIMIT 100`

	rows, err := db.Query(q, args...)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []HistoryRow{}
	for rows.Next() {
		var h HistoryRow
		var profit int64
		if err := rows.Scan(&h.ID, &h.CreatedAt, &h.Total, &profit, &h.Items,
			&h.PaymentMethod, &h.Status, &h.SoldBy, &h.VoidedBy); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if u.Role == "admin" {
			p := profit
			h.Profit = &p
		}
		out = append(out, h)
	}
	writeJSON(w, 200, out)
}
