package share

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	db *sql.DB
}

func NewHandler(db *sql.DB) *Handler {
	return &Handler{db: db}
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// CreateShareLink creates a new share link.
func (h *Handler) CreateShareLink(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		FileID       int64   `json:"file_id"`
		Password     *string `json:"password"`
		ExpiresAt    *string `json:"expires_at"`
		MaxDownloads *int    `json:"max_downloads"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FileID == 0 {
		http.Error(w, `{"error":"file_id required"}`, http.StatusBadRequest)
		return
	}

	token := generateToken()
	now := time.Now().Format(time.RFC3339)

	var linkID int64
	err := h.db.QueryRow(
		`INSERT INTO share_links (file_id, user_id, token, password, expires_at, max_downloads, is_active, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, true, $7) RETURNING id`,
		req.FileID, userID, token, req.Password, req.ExpiresAt, req.MaxDownloads, now,
	).Scan(&linkID)

	if err != nil {
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":    linkID,
		"token": token,
		"url":   "/api/v2/share/" + token,
	})
}

// AccessShareLink accesses a shared file via token.
func (h *Handler) AccessShareLink(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	var fileID int64
	err := h.db.QueryRow(
		`SELECT s.file_id FROM share_links s
		 WHERE s.token = $1 AND s.is_active = true
		 AND (s.expires_at IS NULL OR s.expires_at > $2)
		 AND (s.max_downloads IS NULL OR s.download_count < s.max_downloads)`,
		token, time.Now().Format(time.RFC3339),
	).Scan(&fileID)

	if err != nil {
		http.Error(w, `{"error":"link invalid or expired"}`, http.StatusNotFound)
		return
	}

	// Increment download count
	h.db.Exec(`UPDATE share_links SET download_count = download_count + 1 WHERE token = $1`, token)

	writeJSON(w, http.StatusOK, map[string]int64{"file_id": fileID})
}

// ListShareLinks lists the user's share links.
func (h *Handler) ListShareLinks(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	rows, err := h.db.Query(
		`SELECT s.id, s.token, s.file_id, s.download_count, s.max_downloads, s.expires_at, s.is_active, s.created_at,
		        COALESCE(f.filename, '') as filename
		 FROM share_links s LEFT JOIN file_items f ON s.file_id = f.id
		 WHERE s.user_id = $1 ORDER BY s.created_at DESC`, userID,
	)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Link struct {
		ID            int64   `json:"id"`
		Token         string  `json:"token"`
		FileID        int64   `json:"file_id"`
		Filename      string  `json:"filename"`
		DownloadCount int     `json:"download_count"`
		MaxDownloads  *int    `json:"max_downloads"`
		ExpiresAt     *string `json:"expires_at"`
		IsActive      bool    `json:"is_active"`
		CreatedAt     *string `json:"created_at"`
	}

	var links []Link
	for rows.Next() {
		var l Link
		rows.Scan(&l.ID, &l.Token, &l.FileID, &l.DownloadCount, &l.MaxDownloads, &l.ExpiresAt, &l.IsActive, &l.CreatedAt, &l.Filename)
		links = append(links, l)
	}
	writeJSON(w, http.StatusOK, links)
}

// DeleteShareLink deletes a share link.
func (h *Handler) DeleteShareLink(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	linkID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	result, err := h.db.Exec(`DELETE FROM share_links WHERE id = $1 AND user_id = $2`, linkID, userID)
	if err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// ---------- File Sharing (between users) ----------

func (h *Handler) ShareWithUser(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := middleware.GetUserID(r.Context())

	var req struct {
		FileID     int64  `json:"file_id"`
		SharedWith int64  `json:"shared_with"`
		Permission string `json:"permission"` // view/download/edit
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	now := time.Now().Format(time.RFC3339)
	var shareID int64
	err := h.db.QueryRow(
		`INSERT INTO file_shares (file_id, owner_id, shared_with, permission, created_at)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		req.FileID, ownerID, req.SharedWith, req.Permission, now,
	).Scan(&shareID)

	if err != nil {
		http.Error(w, `{"error":"share failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": shareID})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
