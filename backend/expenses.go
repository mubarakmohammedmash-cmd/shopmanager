package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func createExpenseDated(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Description string `json:"description"`
		Amount      int64  `json:"amount"`
		Date        string `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	in.Description = strings.TrimSpace(in.Description)
	if in.Description == "" || in.Amount <= 0 {
		http.Error(w, "description and a positive amount are required", 400)
		return
	}

	createdAt := time.Now().UTC().Format("2006-01-02 15:04:05")
	if in.Date != "" {
		d, err := time.Parse("2006-01-02", in.Date)
		if err != nil {
			http.Error(w, "date must look like 2026-10-20", 400)
			return
		}
		if d.After(time.Now().UTC().AddDate(0, 0, 1)) {
			http.Error(w, "date cannot be in the future", 400)
			return
		}
		// noon UTC = 1pm Lagos, so it always lands on the right day
		createdAt = in.Date + " 12:00:00"
	}

	res, err := db.Exec(`INSERT INTO expenses (description, amount, created_at) VALUES (?, ?, ?)`,
		in.Description, in.Amount, createdAt)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, Expense{ID: id, Description: in.Description, Amount: in.Amount, CreatedAt: createdAt})
}
