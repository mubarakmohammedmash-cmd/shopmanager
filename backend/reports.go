package main

import "net/http"

type HistorySale struct {
	ID            int64  `json:"id"`
	CreatedAt     string `json:"created_at"`
	Total         int64  `json:"total"`
	Profit        int64  `json:"profit"`
	Items         string `json:"items"`
	PaymentMethod string `json:"payment_method"`
	Status        string `json:"status"`
}

func salesHistory(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT s.id, s.created_at, s.total,
			SUM(si.qty * (si.unit_price - si.unit_cost)),
			GROUP_CONCAT(p.name || ' x' || si.qty, ', '),
			s.payment_method, s.status
		FROM sales s
		JOIN sale_items si ON si.sale_id = s.id
		JOIN products p ON p.id = si.product_id
		GROUP BY s.id ORDER BY s.id DESC LIMIT 100`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []HistorySale{}
	for rows.Next() {
		var s HistorySale
		if err := rows.Scan(&s.ID, &s.CreatedAt, &s.Total, &s.Profit, &s.Items, &s.PaymentMethod, &s.Status); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		out = append(out, s)
	}
	writeJSON(w, 200, out)
}

// Same as the old today report, but voided sales are left out.
func todayReport2(w http.ResponseWriter, r *http.Request) {
	var revenue, profit, expenses int64
	var count int
	err := db.QueryRow(`SELECT
			COALESCE(SUM(si.qty * si.unit_price), 0),
			COALESCE(SUM(si.qty * (si.unit_price - si.unit_cost)), 0),
			COUNT(DISTINCT s.id)
		FROM sales s JOIN sale_items si ON si.sale_id = s.id
		WHERE s.status = 'completed'
		AND date(s.created_at, '+1 hour') = date('now', '+1 hour')`).Scan(&revenue, &profit, &count)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	err = db.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM expenses
		WHERE status = 'approved'
		AND date(created_at, '+1 hour') = date('now', '+1 hour')`).Scan(&expenses)
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
		WHERE s.status = 'completed' AND s.created_at >= datetime('now', '-30 days')
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
