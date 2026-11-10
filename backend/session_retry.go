package main

import (
	"database/sql"
	"errors"
	"log"
	"time"
)

// lookupSession retries transient database errors (e.g. a dropped Turso
// connection) instead of treating them as "not logged in".
func lookupSession(hash string) (authUser, error) {
	var u authUser
	var err error
	for i := 0; i < 3; i++ {
		err = db.QueryRow(`SELECT u.id, u.username, u.role, st.name, u.store_id
			FROM sessions s JOIN users u ON u.id = s.user_id
			JOIN stores st ON st.id = u.store_id
			WHERE s.token_hash = ? AND s.expires_at > datetime('now') AND u.active = 1`,
			hash).Scan(&u.ID, &u.Username, &u.Role, &u.StoreName, &u.StoreID)
		if err == nil || errors.Is(err, sql.ErrNoRows) {
			return u, err
		}
		log.Printf("session lookup retry %d: %v", i+1, err)
		time.Sleep(time.Duration(i+1) * 200 * time.Millisecond)
	}
	return u, err
}
