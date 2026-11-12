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
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const pbkdf2Iter = 600000

type authUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	StoreName string `json:"store_name"`
	StoreID   int64  `json:"-"`
}

func configuredSuperAdminUsername() string {
	return strings.TrimSpace(os.Getenv("SUPER_ADMIN_USERNAME"))
}

func applyConfiguredRole(u authUser) authUser {
	superAdminUsername := configuredSuperAdminUsername()
	if superAdminUsername != "" && strings.EqualFold(u.Username, superAdminUsername) {
		u.Role = "super_admin"
	}
	return u
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
		`CREATE TABLE IF NOT EXISTS store_settings (
			id         INTEGER PRIMARY KEY CHECK (id = 1),
			store_name TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS stores (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			log.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO store_settings (id, store_name) VALUES (1, 'ShopManager')`); err != nil {
		log.Fatal(err)
	}
	for _, table := range []string{"users", "products", "sales", "expenses"} {
		if !hasColumn(table, "store_id") {
			if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN store_id INTEGER REFERENCES stores(id)`); err != nil {
				log.Fatal(err)
			}
		}
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO stores (id, name)
		SELECT 1, store_name FROM store_settings WHERE id = 1 AND EXISTS (SELECT 1 FROM users)`); err != nil {
		log.Fatal(err)
	}
	for _, table := range []string{"users", "products", "sales", "expenses"} {
		if _, err := db.Exec(`UPDATE ` + table + ` SET store_id = 1 WHERE store_id IS NULL AND EXISTS (SELECT 1 FROM stores WHERE id = 1)`); err != nil {
			log.Fatal(err)
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS users_store_idx ON users(store_id);
		CREATE INDEX IF NOT EXISTS products_store_idx ON products(store_id);
		CREATE INDEX IF NOT EXISTS sales_store_idx ON sales(store_id);
		CREATE INDEX IF NOT EXISTS expenses_store_idx ON expenses(store_id)`); err != nil {
		log.Fatal(err)
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
	u, err := lookupSession(hashToken(t))
	if err != nil {
		log.Println("session lookup failed:", err)
	}
	if err == nil {
		u = applyConfiguredRole(u)
	}
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
	case m == "POST" && p == "/auth/signup":
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
	case m == "GET" && p == "/sales/my-summary":
		return "any"
	case m == "POST" && p == "/assistant/chat":
		return "any"
	case m == "POST" && p == "/assistant/transcribe":
		return "any"
	case m == "GET" && p == "/expenses":
		return "any"
	case m == "POST" && p == "/expenses":
		return "any"
	case m == "POST" && strings.HasPrefix(p, "/sales/") && strings.HasSuffix(p, "/void"):
		return "any"
	case strings.HasPrefix(p, "/superadmin/"):
		return "super_admin"
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
		if need == "super_admin" && u.Role != "super_admin" {
			http.Error(w, "super admins only", 403)
			return
		}
		if need == "admin" && u.Role != "admin" && u.Role != "super_admin" {
			http.Error(w, "managers only", 403)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

// ---------- login, setup, accounts ----------

type credsReq struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Role      string `json:"role"`
	StoreName string `json:"store_name"`
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
	writeJSON(w, 200, map[string]any{"setup_needed": n == 0, "signup_enabled": true})
}

func signup(w http.ResponseWriter, r *http.Request) {
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
	if configuredSuperAdminUsername() != "" &&
		strings.EqualFold(in.Username, configuredSuperAdminUsername()) {
		http.Error(w, "that username is reserved for the configured super admin", http.StatusConflict)
		return
	}
	in.StoreName = strings.TrimSpace(in.StoreName)
	if in.StoreName == "" || len([]rune(in.StoreName)) > 80 {
		http.Error(w, "store name must be 1 to 80 characters", 400)
		return
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()
	storeResult, err := tx.Exec(`INSERT INTO stores (name) VALUES (?)`, in.StoreName)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	storeID, err := storeResult.LastInsertId()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	res, err := tx.Exec(`INSERT INTO users (username, password_hash, role, store_id)
		VALUES (?, ?, 'admin', ?)`, in.Username, hash, storeID)
	if err != nil {
		if strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
			http.Error(w, "that username is already taken; choose another username", 409)
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	id, _ := res.LastInsertId()
	token, err := issueSession(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 201, map[string]any{"token": token, "user": authUser{ID: id, Username: in.Username, Role: "admin", StoreName: in.StoreName, StoreID: storeID}})
}

func setup(w http.ResponseWriter, r *http.Request) {
	signup(w, r)
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
	err := db.QueryRow(`SELECT u.id, u.username, u.role, u.password_hash, u.active, u.store_id, s.name
		FROM users u JOIN stores s ON s.id = u.store_id WHERE u.username = ?`,
		name).Scan(&u.ID, &u.Username, &u.Role, &hash, &active, &u.StoreID, &u.StoreName)
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
	u = applyConfiguredRole(u)
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

func getStore(w http.ResponseWriter, r *http.Request) {
	var storeName string
	if err := db.QueryRow(`SELECT name FROM stores WHERE id = ?`, currentUser(r).StoreID).Scan(&storeName); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]string{"store_name": storeName})
}

func updateStore(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var in struct {
		StoreName string `json:"store_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	in.StoreName = strings.TrimSpace(in.StoreName)
	if in.StoreName == "" || len([]rune(in.StoreName)) > 80 {
		http.Error(w, "store name must be 1 to 80 characters", 400)
		return
	}
	if _, err := db.Exec(`UPDATE stores SET name = ? WHERE id = ?`, in.StoreName, currentUser(r).StoreID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]string{"store_name": in.StoreName})
}

type UserRow struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
}

func listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT id, username, role, active, created_at FROM users WHERE store_id = ? ORDER BY id`, currentUser(r).StoreID)
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
	if configuredSuperAdminUsername() != "" &&
		strings.EqualFold(in.Username, configuredSuperAdminUsername()) {
		http.Error(w, "that username is reserved for the configured super admin", http.StatusConflict)
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
	u := currentUser(r)
	res, err := db.Exec(`INSERT INTO users (username, password_hash, role, store_id) VALUES (?, ?, ?, ?)`,
		in.Username, hash, in.Role, u.StoreID)
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
	if id != currentUser(r).ID && configuredSuperAdminUsername() != "" {
		var username string
		err := db.QueryRow(`SELECT username FROM users WHERE id = ? AND store_id = ?`,
			id, currentUser(r).StoreID).Scan(&username)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "user not found", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to verify user", http.StatusInternalServerError)
			return
		}
		if strings.EqualFold(username, configuredSuperAdminUsername()) {
			http.Error(w, "only the configured super admin can change their password", http.StatusBadRequest)
			return
		}
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	res, err := db.Exec(`UPDATE users SET password_hash = ? WHERE id = ? AND store_id = ?`, hash, id, currentUser(r).StoreID)
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
	if !in.Active && configuredSuperAdminUsername() != "" {
		var username string
		err := db.QueryRow(`SELECT username FROM users WHERE id = ? AND store_id = ?`,
			id, currentUser(r).StoreID).Scan(&username)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "user not found", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to verify user", http.StatusInternalServerError)
			return
		}
		if strings.EqualFold(username, configuredSuperAdminUsername()) {
			http.Error(w, "the configured super admin cannot be disabled", http.StatusBadRequest)
			return
		}
	}
	flag := 0
	if in.Active {
		flag = 1
	}
	res, err := db.Exec(`UPDATE users SET active = ? WHERE id = ? AND store_id = ?`, flag, id, currentUser(r).StoreID)
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

type SuperAdminUserRow struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	StoreID   int64  `json:"store_id"`
	StoreName string `json:"store_name"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
}

func listAllUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT u.id, u.username, u.role, s.id, s.name, u.active, u.created_at
		FROM users u JOIN stores s ON s.id = u.store_id
		ORDER BY s.name, u.username`)
	if err != nil {
		http.Error(w, "failed to load users", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	users := []SuperAdminUserRow{}
	for rows.Next() {
		var user SuperAdminUserRow
		var active int
		if err := rows.Scan(&user.ID, &user.Username, &user.Role, &user.StoreID, &user.StoreName, &active, &user.CreatedAt); err != nil {
			http.Error(w, "failed to load users", http.StatusInternalServerError)
			return
		}
		user.Active = active == 1
		if strings.EqualFold(user.Username, configuredSuperAdminUsername()) &&
			configuredSuperAdminUsername() != "" {
			user.Role = "super_admin"
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to load users", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func setGlobalUserActive(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "bad user id", http.StatusBadRequest)
		return
	}

	var in struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if id == currentUser(r).ID {
		http.Error(w, "you cannot change your own account status", http.StatusBadRequest)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "failed to update user status", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var username string
	if err := tx.QueryRow(`SELECT username FROM users WHERE id = ?`, id).Scan(&username); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to update user status", http.StatusInternalServerError)
		return
	}
	if strings.EqualFold(username, configuredSuperAdminUsername()) {
		http.Error(w, "you cannot change the configured super admin's account status", http.StatusBadRequest)
		return
	}

	active := 0
	if in.Active {
		active = 1
	}
	if _, err := tx.Exec(`UPDATE users SET active = ? WHERE id = ?`, active, id); err != nil {
		http.Error(w, "failed to update user status", http.StatusInternalServerError)
		return
	}
	if !in.Active {
		if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
			http.Error(w, "failed to revoke user sessions", http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "failed to update user status", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func registerAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/status", authStatus)
	mux.HandleFunc("POST /auth/signup", signup)
	mux.HandleFunc("POST /auth/setup", setup)
	mux.HandleFunc("POST /auth/login", login)
	mux.HandleFunc("POST /auth/logout", logout)
	mux.HandleFunc("GET /auth/me", me)
	mux.HandleFunc("GET /store", getStore)
	mux.HandleFunc("PUT /store", updateStore)
	mux.HandleFunc("GET /sales/my-summary", mySalesSummary)
	mux.HandleFunc("GET /users", listUsers)
	mux.HandleFunc("POST /users", createUser)
	mux.HandleFunc("POST /users/{id}/password", resetPassword)
	mux.HandleFunc("POST /users/{id}/active", setActive)
	mux.HandleFunc("GET /superadmin/users", listAllUsers)
	mux.HandleFunc("POST /superadmin/users/{id}/active", setGlobalUserActive)
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
	products, err := queryProducts(currentUser(r).StoreID, "")
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
	if req.Payments == nil && method == "" {
		method = "cash"
	}
	if req.Payments == nil && !validPayment[method] {
		http.Error(w, "payment must be cash, transfer or pos", 400)
		return
	}
	req.CustomerName = strings.TrimSpace(req.CustomerName)
	if len(req.CustomerName) > 120 {
		http.Error(w, "customer name must be 120 characters or fewer", 400)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO sales (total, user_id, store_id) VALUES (0, ?, ?)`, u.ID, u.StoreID)
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
		err := tx.QueryRow(
			`SELECT name, selling_price, cost_price, stock FROM products WHERE id = ? AND store_id = ?`,
			it.ProductID, u.StoreID,
		).Scan(&name, &price, &cost, &stock)

		if err == sql.ErrNoRows {
			http.Error(w, "product not found", 404)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// Only admins can override the saved selling price.
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

		if _, err := tx.Exec(
			`INSERT INTO sale_items (sale_id, product_id, qty, unit_price, unit_cost)
			 VALUES (?, ?, ?, ?, ?)`,
			saleID, it.ProductID, it.Qty, price, cost,
		); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if _, err := tx.Exec(
			`UPDATE products SET stock = stock - ? WHERE id = ? AND store_id = ?`,
			it.Qty, it.ProductID, u.StoreID,
		); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		total += price * int64(it.Qty)
	}

	tenders := req.Payments
	var received int64
	if tenders != nil {
		method = ""
		var cashReceived int64
		for _, tender := range tenders {
			if tender.Amount <= 0 {
				http.Error(w, "payment amounts must be greater than zero", 400)
				return
			}
			if !validPayment[tender.PaymentMethod] {
				http.Error(w, "payment must be cash, transfer or pos", 400)
				return
			}
			if tender.Amount > int64(^uint64(0)>>1)-received {
				http.Error(w, "total payment amount is too large", 400)
				return
			}
			received += tender.Amount
			if tender.PaymentMethod == "cash" {
				cashReceived += tender.Amount
			}
			if method == "" {
				method = tender.PaymentMethod
			}
		}
		if req.AmountReceived != nil && *req.AmountReceived != received {
			http.Error(w, "amount received must equal the sum of payment amounts", 400)
			return
		}
		if method == "" {
			method = "cash"
		}
		if received > total && cashReceived < received-total {
			http.Error(w, "cash received must cover any change given", 400)
			return
		}
	} else {
		received = total
		if req.AmountReceived != nil {
			received = *req.AmountReceived
			// Preserve legacy zero-as-default clients unless this is a named credit sale.
			if received == 0 && req.CustomerName == "" {
				received = total
			}
		}
	}
	if received < 0 {
		http.Error(w, "amount received cannot be negative", 400)
		return
	}
	if received < total && req.CustomerName == "" {
		http.Error(w, "customer name is required when the sale is not paid in full", 400)
		return
	}
	change := received - total
	if change < 0 {
		change = 0
	}

	if _, err := tx.Exec(
		`UPDATE sales
		 SET total = ?, payment_method = ?, amount_received = ?, change_given = ?, customer_name = ?
		 WHERE id = ? AND store_id = ?`,
		total, method, received, change, req.CustomerName, saleID, u.StoreID,
	); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if tenders != nil {
		changeRemaining := change
		for _, tender := range tenders {
			tenderChange := int64(0)
			if tender.PaymentMethod == "cash" {
				tenderChange = changeRemaining
				if tenderChange > tender.Amount {
					tenderChange = tender.Amount
				}
				changeRemaining -= tenderChange
			}
			if _, err := tx.Exec(`INSERT INTO sale_payments
				(sale_id, amount, change_given, payment_method, recorded_by)
				VALUES (?, ?, ?, ?, ?)`, saleID, tender.Amount, tenderChange, tender.PaymentMethod, u.ID); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		}
	} else if received > 0 {
		if _, err := tx.Exec(`INSERT INTO sale_payments
			(sale_id, amount, change_given, payment_method, recorded_by)
			VALUES (?, ?, ?, ?, ?)`, saleID, received, change, method, u.ID); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 201, map[string]any{
		"sale_id":         saleID,
		"total":           total,
		"amount_received": received,
		"change_given":    change,
		"balance":         total - received + change,
	})
}

type SalePayment struct {
	ID            int64  `json:"id"`
	Amount        int64  `json:"amount"`
	ChangeGiven   int64  `json:"change_given"`
	PaymentMethod string `json:"payment_method"`
	CreatedAt     string `json:"created_at"`
	RecordedBy    string `json:"recorded_by"`
}

type OutstandingSale struct {
	ID        int64  `json:"id"`
	CreatedAt string `json:"created_at"`
	Customer  string `json:"customer_name"`
	Total     int64  `json:"total"`
	Paid      int64  `json:"paid"`
	Balance   int64  `json:"balance"`
	SoldBy    string `json:"sold_by"`
}

var errSaleForbidden = errors.New("you can only access your own sales")

func saleAccess(r *http.Request, tx *sql.Tx, id int64) error {
	u := currentUser(r)
	var owner sql.NullInt64
	err := tx.QueryRow(`SELECT user_id FROM sales WHERE id = ? AND store_id = ?`,
		id, u.StoreID).Scan(&owner)
	if err != nil {
		return err
	}
	if u.Role != "admin" && (!owner.Valid || owner.Int64 != u.ID) {
		return errSaleForbidden
	}
	return nil
}

func saleAccessError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		http.Error(w, "sale not found", 404)
	case errors.Is(err, errSaleForbidden):
		http.Error(w, err.Error(), 403)
	default:
		http.Error(w, err.Error(), 500)
	}
}

func salePayments(w http.ResponseWriter, r *http.Request) {
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
	if err := saleAccess(r, tx, id); err != nil {
		saleAccessError(w, err)
		return
	}
	rows, err := tx.Query(`SELECT p.id, p.amount, p.change_given, p.payment_method,
			p.created_at, COALESCE(u.username, '')
		FROM sale_payments p LEFT JOIN users u ON u.id = p.recorded_by
		WHERE p.sale_id = ? ORDER BY p.id`, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []SalePayment{}
	for rows.Next() {
		var p SalePayment
		if err := rows.Scan(&p.ID, &p.Amount, &p.ChangeGiven, &p.PaymentMethod,
			&p.CreatedAt, &p.RecordedBy); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, out)
}

func recordSalePayment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad sale id", 400)
		return
	}
	var req struct {
		Amount        int64  `json:"amount"`
		PaymentMethod string `json:"payment_method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if req.Amount <= 0 {
		http.Error(w, "payment amount must be greater than zero", 400)
		return
	}
	if !validPayment[req.PaymentMethod] {
		http.Error(w, "payment must be cash, transfer or pos", 400)
		return
	}

	u := currentUser(r)
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()
	if err := saleAccess(r, tx, id); err != nil {
		saleAccessError(w, err)
		return
	}

	var status string
	var total, received, change int64
	if err := tx.QueryRow(`SELECT status, total, amount_received, change_given
		FROM sales WHERE id = ? AND store_id = ?`, id, u.StoreID).
		Scan(&status, &total, &received, &change); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if status != "completed" {
		http.Error(w, "payments can only be recorded for completed sales", 409)
		return
	}
	balance := total - (received - change)
	if req.Amount > balance {
		http.Error(w, "payment cannot exceed the outstanding balance", 400)
		return
	}
	if _, err := tx.Exec(`INSERT INTO sale_payments
		(sale_id, amount, payment_method, recorded_by) VALUES (?, ?, ?, ?)`,
		id, req.Amount, req.PaymentMethod, u.ID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if _, err := tx.Exec(`UPDATE sales SET amount_received = amount_received + ?
		WHERE id = ? AND store_id = ?`, req.Amount, id, u.StoreID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]int64{
		"paid":    received - change + req.Amount,
		"balance": balance - req.Amount,
	})
}

func outstandingSales(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	q := `SELECT s.id, s.created_at, s.customer_name, s.total,
			s.amount_received - s.change_given,
			s.total - (s.amount_received - s.change_given),
			COALESCE(u.username, '')
		FROM sales s LEFT JOIN users u ON u.id = s.user_id
		WHERE s.store_id = ? AND s.status = 'completed'
			AND s.total > (s.amount_received - s.change_given)`
	args := []any{u.StoreID}
	if u.Role != "admin" {
		q += ` AND s.user_id = ?`
		args = append(args, u.ID)
	}
	q += ` ORDER BY s.created_at DESC, s.id DESC`
	rows, err := db.Query(q, args...)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []OutstandingSale{}
	for rows.Next() {
		var s OutstandingSale
		if err := rows.Scan(&s.ID, &s.CreatedAt, &s.Customer, &s.Total,
			&s.Paid, &s.Balance, &s.SoldBy); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, out)
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
	err = tx.QueryRow(`SELECT status, user_id FROM sales WHERE id = ? AND store_id = ?`, id, u.StoreID).Scan(&status, &owner)
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
		if _, err := tx.Exec(`UPDATE products SET stock = stock + ? WHERE id = ? AND store_id = ?`, l.qty, l.productID, u.StoreID); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	if _, err := tx.Exec(`UPDATE sales SET status = 'voided', voided_at = datetime('now'), voided_by = ? WHERE id = ? AND store_id = ?`,
		u.ID, id, u.StoreID); err != nil {
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
	CustomerName  string `json:"customer_name"`
	Paid          int64  `json:"paid"`
	Balance       int64  `json:"balance"`
	Profit        *int64 `json:"profit,omitempty"`
	Items         string `json:"items"`
	PaymentMethod string `json:"payment_method"`
	Status        string `json:"status"`
	SoldBy        string `json:"sold_by"`
	VoidedBy      string `json:"voided_by,omitempty"`
}

// Managers see every sale with profit. Reps see only their own sales, with no profit.

func mySalesSummary(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)

	var revenue, transactions, units int64
	var cashReceived, transferReceived, posReceived int64
	var changePaid int64

	err := db.QueryRow(`
        SELECT
            COALESCE(SUM(CASE WHEN s.status = 'completed'
                THEN si.qty * si.unit_price ELSE 0 END), 0),
            COUNT(DISTINCT CASE WHEN s.status = 'completed' THEN s.id END),
            COALESCE(SUM(CASE WHEN s.status = 'completed'
                THEN si.qty ELSE 0 END), 0)
        FROM sales s
        LEFT JOIN sale_items si ON si.sale_id = s.id
        WHERE s.user_id = ? AND s.store_id = ?
          AND date(s.created_at, '+1 hour') = date('now', '+1 hour')
    `, u.ID, u.StoreID).Scan(&revenue, &transactions, &units)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = db.QueryRow(`
        SELECT
            COALESCE(SUM(CASE WHEN p.payment_method = 'cash'
                THEN p.amount ELSE 0 END), 0),
            COALESCE(SUM(CASE WHEN p.payment_method = 'transfer'
                THEN p.amount ELSE 0 END), 0),
            COALESCE(SUM(CASE WHEN p.payment_method = 'pos'
                THEN p.amount ELSE 0 END), 0),
            COALESCE(SUM(p.change_given), 0)
        FROM sale_payments p
        JOIN sales s ON s.id = p.sale_id
        WHERE s.user_id = ? AND s.store_id = ?
          AND s.status = 'completed'
          AND date(p.created_at, '+1 hour') = date('now', '+1 hour')
    `, u.ID, u.StoreID).Scan(
		&cashReceived, &transferReceived, &posReceived, &changePaid,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]int64{
		"revenue":           revenue,
		"transactions":      transactions,
		"items_sold":        units,
		"cash_received":     cashReceived,
		"transfer_received": transferReceived,
		"pos_received":      posReceived,
		"cash_change_paid":  changePaid,
		"cash_drawer_net":   cashReceived - changePaid,
	})
}

func salesHistoryV2(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	q := `SELECT s.id, s.created_at, s.total,
			SUM(si.qty * (si.unit_price - si.unit_cost)),
			GROUP_CONCAT(p.name || ' x' || si.qty, ', '),
			COALESCE((SELECT GROUP_CONCAT(DISTINCT sp.payment_method)
				FROM sale_payments sp WHERE sp.sale_id = s.id), ''),
			s.status,
			COALESCE(su.username, ''), COALESCE(vu.username, ''),
			COALESCE(s.customer_name, ''),
			s.amount_received - s.change_given,
			CASE WHEN s.status = 'completed'
				THEN MAX(0, s.total - (s.amount_received - s.change_given))
				ELSE 0 END
		FROM sales s
		JOIN sale_items si ON si.sale_id = s.id
		JOIN products p ON p.id = si.product_id
		LEFT JOIN users su ON su.id = s.user_id
		LEFT JOIN users vu ON vu.id = s.voided_by`
	q += ` WHERE s.store_id = ?`
	args := []any{u.StoreID}
	if u.Role != "admin" {
		q += ` AND s.user_id = ?`
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
			&h.PaymentMethod, &h.Status, &h.SoldBy, &h.VoidedBy,
			&h.CustomerName, &h.Paid, &h.Balance); err != nil {
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
