package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestPartialSaleAndInstallmentPayments(t *testing.T) {
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
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	migrate()
	migrateAuth()
	if _, err := db.Exec(`INSERT INTO stores (id, name) VALUES (1, 'Test Store')`); err != nil {
		t.Fatal(err)
	}
	result, err := db.Exec(`INSERT INTO users (username, password_hash, role, store_id)
		VALUES ('rep', 'unused', 'rep', 1)`)
	if err != nil {
		t.Fatal(err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	migratePayments()
	migrateExpenses()
	if _, err := db.Exec(`INSERT INTO products
		(name, cost_price, selling_price, stock, low_stock_level, store_id)
		VALUES ('Test product', 300, 500, 5, 1, 1)`); err != nil {
		t.Fatal(err)
	}

	user := authUser{ID: userID, Role: "rep", StoreID: 1}
	saleResponse := requestAs(t, user, http.MethodPost, "/sales", map[string]any{
		"items":           []map[string]any{{"product_id": 1, "qty": 2}},
		"payment_method":  "cash",
		"amount_received": 300,
		"customer_name":   "Test Customer",
	}, createSaleV3)
	if saleResponse.Code != http.StatusCreated {
		t.Fatalf("createSaleV3 status = %d, want %d: %s", saleResponse.Code, http.StatusCreated, saleResponse.Body)
	}
	var sale struct {
		SaleID  int64 `json:"sale_id"`
		Total   int64 `json:"total"`
		Balance int64 `json:"balance"`
	}
	if err := json.Unmarshal(saleResponse.Body.Bytes(), &sale); err != nil {
		t.Fatal(err)
	}
	if sale.Total != 1000 || sale.Balance != 700 {
		t.Fatalf("sale total/balance = %d/%d, want 1000/700", sale.Total, sale.Balance)
	}

	var stock int
	if err := db.QueryRow(`SELECT stock FROM products WHERE id = 1`).Scan(&stock); err != nil {
		t.Fatal(err)
	}
	if stock != 3 {
		t.Fatalf("stock after credit sale = %d, want 3", stock)
	}
	migratePayments()
	var initialPaymentCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sale_payments WHERE sale_id = ?`, sale.SaleID).
		Scan(&initialPaymentCount); err != nil {
		t.Fatal(err)
	}
	if initialPaymentCount != 1 {
		t.Fatalf("payments after repeated migration = %d, want 1", initialPaymentCount)
	}

	outstandingResponse := requestAs(t, user, http.MethodGet, "/sales/outstanding", nil, outstandingSales)
	var outstanding []OutstandingSale
	if err := json.Unmarshal(outstandingResponse.Body.Bytes(), &outstanding); err != nil {
		t.Fatal(err)
	}
	if len(outstanding) != 1 || outstanding[0].Customer != "Test Customer" || outstanding[0].Balance != 700 {
		t.Fatalf("unexpected outstanding sales: %+v", outstanding)
	}

	addPayment := func(amount int64, method string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost,
			"/sales/"+strconv.FormatInt(sale.SaleID, 10)+"/payments",
			bytes.NewBufferString(`{"amount":`+strconv.FormatInt(amount, 10)+`,"payment_method":"`+method+`"}`))
		request.SetPathValue("id", strconv.FormatInt(sale.SaleID, 10))
		request = request.WithContext(context.WithValue(request.Context(), ctxKey{}, user))
		recordSalePayment(response, request)
		return response
	}
	if response := addPayment(400, "transfer"); response.Code != http.StatusOK {
		t.Fatalf("first installment status = %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}
	if response := addPayment(301, "pos"); response.Code != http.StatusBadRequest {
		t.Fatalf("overpayment status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if response := addPayment(300, "pos"); response.Code != http.StatusOK {
		t.Fatalf("final installment status = %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}

	outstandingResponse = requestAs(t, user, http.MethodGet, "/sales/outstanding", nil, outstandingSales)
	if err := json.Unmarshal(outstandingResponse.Body.Bytes(), &outstanding); err != nil {
		t.Fatal(err)
	}
	if len(outstanding) != 0 {
		t.Fatalf("outstanding sales after settlement = %+v, want none", outstanding)
	}

	historyResponse := requestAs(t, user, http.MethodGet, "/sales/history", nil, salesHistoryV2)
	var history []HistoryRow
	if err := json.Unmarshal(historyResponse.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Paid != 1000 || history[0].Balance != 0 {
		t.Fatalf("unexpected settled history: %+v", history)
	}
	methods := strings.Split(history[0].PaymentMethod, ",")
	if len(methods) != 3 {
		t.Fatalf("history payment methods = %q, want all three recorded methods", history[0].PaymentMethod)
	}

	summaryResponse := requestAs(t, user, http.MethodGet, "/sales/my-summary", nil, mySalesSummary)
	var summary map[string]int64
	if err := json.Unmarshal(summaryResponse.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary["cash_received"] != 300 || summary["transfer_received"] != 400 ||
		summary["pos_received"] != 300 {
		t.Fatalf("payment-method totals are incorrect: %+v", summary)
	}

	today := lagosDate(time.Now())
	reportResponse := requestAs(t, user, http.MethodGet,
		"/reports/range?from="+today+"&to="+today, nil, rangeReport)
	var report map[string]any
	if err := json.Unmarshal(reportResponse.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report["cash_received"] != float64(300) ||
		report["transfer_received"] != float64(400) ||
		report["pos_received"] != float64(300) {
		t.Fatalf("report payment-method totals are incorrect: %+v", report)
	}

	paymentsResponse := requestAs(t, user, http.MethodGet,
		"/sales/"+strconv.FormatInt(sale.SaleID, 10)+"/payments", nil, salePayments)
	var payments []SalePayment
	if err := json.Unmarshal(paymentsResponse.Body.Bytes(), &payments); err != nil {
		t.Fatal(err)
	}
	if len(payments) != 3 {
		t.Fatalf("payment history has %d entries, want 3", len(payments))
	}

	splitSaleResponse := requestAs(t, user, http.MethodPost, "/sales", map[string]any{
		"items": []map[string]any{{"product_id": 1, "qty": 1}},
		"payments": []map[string]any{
			{"amount": 200, "payment_method": "cash"},
			{"amount": 300, "payment_method": "pos"},
		},
	}, createSaleV3)
	if splitSaleResponse.Code != http.StatusCreated {
		t.Fatalf("split-tender sale status = %d, want %d: %s",
			splitSaleResponse.Code, http.StatusCreated, splitSaleResponse.Body)
	}
	var splitSale struct {
		SaleID int64 `json:"sale_id"`
		Total  int64 `json:"total"`
		Change int64 `json:"change_given"`
	}
	if err := json.Unmarshal(splitSaleResponse.Body.Bytes(), &splitSale); err != nil {
		t.Fatal(err)
	}
	if splitSale.Total != 500 || splitSale.Change != 0 {
		t.Fatalf("split sale total/change = %d/%d, want 500/0", splitSale.Total, splitSale.Change)
	}

	splitPaymentsResponse := requestAs(t, user, http.MethodGet,
		"/sales/"+strconv.FormatInt(splitSale.SaleID, 10)+"/payments",
		nil, salePayments)
	var splitPayments []SalePayment
	if err := json.Unmarshal(splitPaymentsResponse.Body.Bytes(), &splitPayments); err != nil {
		t.Fatal(err)
	}
	if len(splitPayments) != 2 ||
		splitPayments[0].Amount != 200 || splitPayments[0].PaymentMethod != "cash" ||
		splitPayments[1].Amount != 300 || splitPayments[1].PaymentMethod != "pos" {
		t.Fatalf("split payment ledger = %+v, want cash 200 and POS 300", splitPayments)
	}

	invalidChangeResponse := requestAs(t, user, http.MethodPost, "/sales", map[string]any{
		"items": []map[string]any{{"product_id": 1, "qty": 1}},
		"payments": []map[string]any{
			{"amount": 49, "payment_method": "cash"},
			{"amount": 501, "payment_method": "transfer"},
		},
	}, createSaleV3)
	if invalidChangeResponse.Code != http.StatusBadRequest {
		t.Fatalf("split payment without enough cash for change status = %d, want %d",
			invalidChangeResponse.Code, http.StatusBadRequest)
	}

	chart, err := assistantSalesChart(user, 7)
	if err != nil {
		t.Fatal(err)
	}
	var chartRevenue int64
	for _, point := range chart.Points {
		chartRevenue += point.Value
	}
	if len(chart.Points) != 7 || chartRevenue != 1500 {
		t.Fatalf("7-day sales chart has %d points and %d revenue, want 7 and 1500",
			len(chart.Points), chartRevenue)
	}
}

func requestAs(t *testing.T, user authUser, method, path string, body any, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	var content bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&content).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &content)
	if strings.HasPrefix(path, "/sales/") && strings.HasSuffix(path, "/payments") {
		request.SetPathValue("id", strings.TrimSuffix(strings.TrimPrefix(path, "/sales/"), "/payments"))
	}
	request = request.WithContext(context.WithValue(request.Context(), ctxKey{}, user))
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}
