package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSuperAdminCanManageUsersAcrossStores(t *testing.T) {
	previousDB := db
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	testDB.SetMaxOpenConns(1)
	db = testDB
	t.Cleanup(func() {
		testDB.Close()
		db = previousDB
	})
	t.Setenv("SUPER_ADMIN_USERNAME", "Mubarak")

	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	migrateAuth()
	if _, err := db.Exec(`INSERT INTO stores (id, name) VALUES (1, 'Main'), (2, 'Second')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, username, password_hash, role, store_id)
		VALUES (1, 'Mubarak', 'unused', 'admin', 1), (2, 'rep', 'unused', 'rep', 2),
		(3, 'manager', 'unused', 'admin', 2), (4, 'storemanager', 'unused', 'admin', 1)`); err != nil {
		t.Fatal(err)
	}

	superAdminToken, err := issueSession(1)
	if err != nil {
		t.Fatal(err)
	}
	repToken, err := issueSession(2)
	if err != nil {
		t.Fatal(err)
	}
	managerToken, err := issueSession(3)
	if err != nil {
		t.Fatal(err)
	}
	storeManagerToken, err := issueSession(4)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerAuthRoutes(mux)
	handler := withAuth(mux)

	doRequest := func(method, path, token string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var payload bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&payload).Encode(body); err != nil {
				t.Fatal(err)
			}
		}
		request := httptest.NewRequest(method, path, &payload)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	response := doRequest(http.MethodGet, "/superadmin/users", superAdminToken, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("super-admin users status = %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}
	var users []SuperAdminUserRow
	if err := json.Unmarshal(response.Body.Bytes(), &users); err != nil {
		t.Fatal(err)
	}
	if len(users) != 4 || users[0].StoreName == "" {
		t.Fatalf("global users response = %+v, want users from both stores", users)
	}
	authRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	authRequest.Header.Set("Authorization", "Bearer "+superAdminToken)
	if user, ok := userFromRequest(authRequest); !ok || user.Role != "super_admin" {
		t.Fatalf("configured user role = %q, authenticated = %v; want super_admin", user.Role, ok)
	}

	response = doRequest(http.MethodGet, "/superadmin/users", managerToken, nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("manager access status = %d, want %d", response.Code, http.StatusForbidden)
	}
	response = doRequest(http.MethodGet, "/superadmin/users", repToken, nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("rep access status = %d, want %d", response.Code, http.StatusForbidden)
	}

	request := httptest.NewRequest(http.MethodPost, "/users/1/active", bytes.NewBufferString(`{"active":false}`))
	request.SetPathValue("id", "1")
	request.Header.Set("Authorization", "Bearer "+storeManagerToken)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("manager disabling super admin status = %d, want %d", response.Code, http.StatusBadRequest)
	}

	request = httptest.NewRequest(http.MethodPost, "/users/1/password", bytes.NewBufferString(`{"password":"new-password"}`))
	request.SetPathValue("id", "1")
	request.Header.Set("Authorization", "Bearer "+storeManagerToken)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("manager resetting super admin password status = %d, want %d", response.Code, http.StatusBadRequest)
	}

	response = doRequest(http.MethodPost, "/auth/signup", "", map[string]any{
		"username": "Mubarak", "password": "valid-password", "store_name": "Another Store",
	})
	if response.Code != http.StatusConflict {
		t.Fatalf("signup claiming configured username status = %d, want %d", response.Code, http.StatusConflict)
	}

	request = httptest.NewRequest(http.MethodPost, "/superadmin/users/2/active", bytes.NewBufferString(`{"active":false}`))
	request.SetPathValue("id", strconv.FormatInt(2, 10))
	request.Header.Set("Authorization", "Bearer "+superAdminToken)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("disable user status = %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}

	var active int
	if err := db.QueryRow(`SELECT active FROM users WHERE id = 2`).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("disabled user active = %d, want 0", active)
	}
	var sessionCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = 2`).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 0 {
		t.Fatalf("disabled user's sessions = %d, want 0", sessionCount)
	}

	request = httptest.NewRequest(http.MethodPost, "/superadmin/users/1/active", bytes.NewBufferString(`{"active":false}`))
	request.SetPathValue("id", strconv.FormatInt(1, 10))
	request.Header.Set("Authorization", "Bearer "+superAdminToken)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("self-disable status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}
