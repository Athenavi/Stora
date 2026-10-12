// Package webdav exposes a user's drive over the WebDAV protocol so that
// desktop clients and tools can mount it directly: macOS Finder ("Connect to
// Server"), Windows Explorer ("Map network drive"), rclone, Infuse, ...
//
// This is the capability commercial cloud drives do not offer, which is why it
// is the first differentiator for a self-hosted deployment.
package webdav

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/Athenavi/Stora/pkg/auth"
	"github.com/Athenavi/Stora/pkg/storage"
	"golang.org/x/net/webdav"
)

// Handler serves the WebDAV endpoint.
type Handler struct {
	db  *sql.DB
	dav *webdav.Handler
}

// New builds the WebDAV handler.
//
// Authentication uses an application-specific password (created via
// /api/v2/auth/app-passwords), not the account password: WebDAV clients only
// speak HTTP Basic auth and typically store the credential in plain text, so it
// has to be individually revocable.
func New(db *sql.DB, store storage.Driver) *Handler {
	return &Handler{
		db: db,
		dav: &webdav.Handler{
			FileSystem: &fileSystem{db: db, storage: store},
			LockSystem: webdav.NewMemLS(),
			Logger: func(r *http.Request, err error) {
				if err != nil {
					log.Printf("[WebDAV] %s %s: %v", r.Method, r.URL.Path, err)
				}
			},
		},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.authenticate(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="Stora WebDAV"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	// Identity travels in the context: the webdav.FileSystem interface carries a
	// context but no request.
	ctx := withCaller(r.Context(), caller{userID: userID, ip: middleware.ClientIP(r)})
	h.dav.ServeHTTP(w, r.WithContext(ctx))
}

// authenticate resolves Basic credentials against the user's app passwords.
func (h *Handler) authenticate(r *http.Request) (int64, bool) {
	username, password, ok := r.BasicAuth()
	if !ok || username == "" || password == "" {
		return 0, false
	}

	var userID int64
	if err := h.db.QueryRow(
		`SELECT id FROM users WHERE username = $1 AND is_active = true`, username,
	).Scan(&userID); err != nil {
		return 0, false
	}

	rows, err := h.db.Query(`SELECT id, password_hash FROM app_passwords WHERE user_id = $1`, userID)
	if err != nil {
		log.Printf("[WebDAV] app password lookup failed: %v", err)
		return 0, false
	}
	defer rows.Close()

	// Ceiling: this is a linear scan of the user's app passwords, each bcrypt
	// comparison costing ~50ms. Fine for the handful a person creates; if it ever
	// matters, store a keyed digest of the password for lookup instead.
	for rows.Next() {
		var id int64
		var hash string
		if err := rows.Scan(&id, &hash); err != nil {
			continue
		}
		if auth.CheckPassword(password, hash) {
			h.db.Exec(`UPDATE app_passwords SET last_used_at = now() WHERE id = $1`, id)
			return userID, true
		}
	}
	return 0, false
}
