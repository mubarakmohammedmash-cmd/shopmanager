package main

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"time"
)

type PaymentTotal struct {
	Method string `json:"method"`
	Amount int64  `json:"amount"`
}

type DayTotal struct {
	Date    string `json:"date"`
	Revenue int64  `json:"revenue"`
	Profit  int64  `json:"profit"`
}

// Dates are Lagos dates (UTC+1), written like 2026-10-24.
func lagosDate(t time.Time) string { return t.UTC().Add(time.Hour).Format("2006-01-02") }

func rangeParams(r *http.Request) (string, string, bool) {
	today := lagosDate(time.Now())
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if from == "" {
		from = today
	}
	if to == "" {
		to = today
	}
	_, e1 := time.Parse("2006-01-02", from)
	_, e2 := time.Parse("2006-01-02", to)
	return from, to, e1 == nil && e2 == nil && from <= to
}

func topInRange(from, to string) ([]TopProduct, error) {
	rows, err := db.Query(`SELECT p.name, SUM(si.qty), SUM(si.qty * (si.unit_price - si.unit_cost))
		FROM sale_items si
		JOIN products p ON p.id = si.product_id
		JOIN sales s ON s.id = si.sale_id
		WHERE s.status = 'completed' AND date(s.created_at, '+1 hour') BETWEEN ? AND ?
		GROUP BY p.id ORDER BY SUM(si.qty) DESC LIMIT 10`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TopProduct{}
	for rows.Next() {
		var t TopProduct
		if err := rows.Scan(&t.Name, &t.Units, &t.Profit); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func paymentsInRange(from, to string) ([]PaymentTotal, error) {
	rows, err := db.Query(`SELECT s.payment_method, SUM(si.qty * si.unit_price)
		FROM sales s JOIN sale_items si ON si.sale_id = s.id
		WHERE s.status = 'completed' AND date(s.created_at, '+1 hour') BETWEEN ? AND ?
		GROUP BY s.payment_method ORDER BY SUM(si.qty * si.unit_price) DESC`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PaymentTotal{}
	for rows.Next() {
		var p PaymentTotal
		if err := rows.Scan(&p.Method, &p.Amount); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func daysInRange(from, to string) ([]DayTotal, error) {
	rows, err := db.Query(`SELECT date(s.created_at, '+1 hour'),
			SUM(si.qty * si.unit_price),
			SUM(si.qty * (si.unit_price - si.unit_cost))
		FROM sales s JOIN sale_items si ON si.sale_id = s.id
		WHERE s.status = 'completed' AND date(s.created_at, '+1 hour') BETWEEN ? AND ?
		GROUP BY date(s.created_at, '+1 hour') ORDER BY 1 DESC`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DayTotal{}
	for rows.Next() {
		var d DayTotal
		if err := rows.Scan(&d.Date, &d.Revenue, &d.Profit); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func rangeReport(w http.ResponseWriter, r *http.Request) {
	from, to, ok := rangeParams(r)
	if !ok {
		http.Error(w, "dates must look like 2026-10-01, and from must not be after to", 400)
		return
	}

	var revenue, profit, expenses int64
	var count, voided int
	err := db.QueryRow(`SELECT
			COALESCE(SUM(si.qty * si.unit_price), 0),
			COALESCE(SUM(si.qty * (si.unit_price - si.unit_cost)), 0),
			COUNT(DISTINCT s.id)
		FROM sales s JOIN sale_items si ON si.sale_id = s.id
		WHERE s.status = 'completed' AND date(s.created_at, '+1 hour') BETWEEN ? AND ?`,
		from, to).Scan(&revenue, &profit, &count)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	err = db.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM expenses
		WHERE date(created_at, '+1 hour') BETWEEN ? AND ?`, from, to).Scan(&expenses)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	err = db.QueryRow(`SELECT COUNT(*) FROM sales
		WHERE status = 'voided' AND date(created_at, '+1 hour') BETWEEN ? AND ?`, from, to).Scan(&voided)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	top, err := topInRange(from, to)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	pays, err := paymentsInRange(from, to)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	days, err := daysInRange(from, to)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	writeJSON(w, 200, map[string]any{
		"from":         from,
		"to":           to,
		"revenue":      revenue,
		"profit":       profit,
		"expenses":     expenses,
		"net_profit":   profit - expenses,
		"sales_count":  count,
		"voided_count": voided,
		"top_products": top,
		"payments":     pays,
		"days":         days,
	})
}

// Stops spreadsheet programs from running a cell that starts with = + - or @.
func safeCell(s string) string {
	if s != "" && (s[0] == '=' || s[0] == '+' || s[0] == '-' || s[0] == '@') {
		return "'" + s
	}
	return s
}

func exportCSV(w http.ResponseWriter, r *http.Request) {
	from, to, ok := rangeParams(r)
	if !ok {
		http.Error(w, "dates must look like 2026-10-01, and from must not be after to", 400)
		return
	}
	rows, err := db.Query(`SELECT s.id, s.created_at, s.total,
			SUM(si.qty * (si.unit_price - si.unit_cost)),
			GROUP_CONCAT(p.name || ' x' || si.qty, ', '),
			s.payment_method, s.status
		FROM sales s
		JOIN sale_items si ON si.sale_id = s.id
		JOIN products p ON p.id = si.product_id
		WHERE date(s.created_at, '+1 hour') BETWEEN ? AND ?
		GROUP BY s.id ORDER BY s.id`, from, to)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="sales-%s-to-%s.csv"`, from, to))
	cw := csv.NewWriter(w)
	cw.Write([]string{"Receipt", "Date and time (Lagos)", "Items", "Paid by", "Status", "Total (NGN)", "Profit (NGN)"})
	for rows.Next() {
		var id, total, profit int64
		var created, items, method, status string
		if err := rows.Scan(&id, &created, &total, &profit, &items, &method, &status); err != nil {
			break
		}
		shown := created
		if t, err := time.Parse("2006-01-02 15:04:05", created); err == nil {
			shown = t.Add(time.Hour).Format("2006-01-02 15:04")
		}
		cw.Write([]string{
			fmt.Sprint(id), shown, safeCell(items), method, status,
			fmt.Sprintf("%.2f", float64(total)/100),
			fmt.Sprintf("%.2f", float64(profit)/100),
		})
	}
	cw.Flush()
}
