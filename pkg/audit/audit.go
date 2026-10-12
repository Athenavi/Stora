// Package audit records security- and compliance-relevant actions.
//
// The audit_logs table already existed but had no writer at all, so the admin UI
// showed an empty list forever. Writes here are best-effort and asynchronous:
// auditing must never fail, or slow down, the request it describes.
package audit

import (
	"database/sql"
	"log"
	"time"
)

// Entry describes one recorded action.
type Entry struct {
	UserID    int64  // 0 for anonymous or system actions
	Action    string // e.g. "auth.login", "file.destroy", "share.create"
	Resource  string // e.g. "file:123", "user:7"
	Detail    string // extra context — never secrets or tokens
	IPAddress string
}

// Log writes an entry in the background. Safe to call from a request handler:
// it returns immediately and never panics into the caller.
func Log(db *sql.DB, e Entry) {
	if db == nil || e.Action == "" {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Audit] panic while recording %s: %v", e.Action, r)
			}
		}()
		_, err := db.Exec(
			`INSERT INTO audit_logs (user_id, action, resource, detail, ip_address, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			nullIfZero(e.UserID),
			e.Action,
			nullIfEmpty(e.Resource),
			nullIfEmpty(e.Detail),
			nullIfEmpty(e.IPAddress),
			time.Now().Format(time.RFC3339),
		)
		if err != nil {
			log.Printf("[Audit] failed to record %s: %v", e.Action, err)
		}
	}()
}

// nullIfZero keeps user_id NULL for anonymous actions. The column is a nullable
// FK (ON DELETE SET NULL), so 0 must not be written literally.
func nullIfZero(v int64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
