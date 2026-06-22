package share

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/Athenavi/Stora/pkg/storage"
	"github.com/Athenavi/Stora/pkg/utils"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	db      *sql.DB
	storage storage.Driver
}

func NewHandler(db *sql.DB, store storage.Driver) *Handler {
	return &Handler{db: db, storage: store}
}

func generateCode() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func hashPassword(pw string) string {
	h := sha256.Sum256([]byte(pw))
	return hex.EncodeToString(h[:])
}

// ─── Shared types ───

type ShareLinkJSON struct {
	ID                int64   `json:"id"`
	ShortCode         string  `json:"short_code"`
	FileID            int64   `json:"file_id"`
	Filename          string  `json:"filename"`
	Permission        string  `json:"permission"`
	IsActive          bool    `json:"is_active"`
	PasswordProtected bool    `json:"password_protected"`
	ViewCount         int     `json:"view_count"`
	DownloadCount     int     `json:"download_count"`
	MaxDownloads      int     `json:"max_downloads"`
	ExpiresAt         *string `json:"expires_at"`
	CreatedAt         *string `json:"created_at"`
}

// CreateShareLink creates a share link.
// Accepts FormData: file_id, permission, password (optional), expires_in_hours (optional), max_downloads
func (h *Handler) CreateShareLink(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	// Accept both JSON and FormData
	var fileID int64
	var permission string
	var password *string
	var expiresInHours int
	maxDownloads := 0

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") || strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, `{"error":"invalid form"}`, http.StatusBadRequest)
			return
		}
		if fid := r.FormValue("file_id"); fid != "" {
			if v, err := strconv.ParseInt(fid, 10, 64); err == nil {
				fileID = v
			}
		}
		permission = r.FormValue("permission")
		if p := r.FormValue("password"); p != "" {
			password = &p
		}
		if e := r.FormValue("expires_in_hours"); e != "" {
			if v, err := strconv.Atoi(e); err == nil {
				expiresInHours = v
			}
		}
		if m := r.FormValue("max_downloads"); m != "" {
			if v, err := strconv.Atoi(m); err == nil {
				maxDownloads = v
			}
		}
	} else {
		var req struct {
			FileID         int64   `json:"file_id"`
			Permission     string  `json:"permission"`
			Password       *string `json:"password"`
			ExpiresInHours int     `json:"expires_in_hours"`
			MaxDownloads   int     `json:"max_downloads"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		fileID = req.FileID
		permission = req.Permission
		password = req.Password
		expiresInHours = req.ExpiresInHours
		maxDownloads = req.MaxDownloads
	}

	if fileID == 0 {
		http.Error(w, `{"error":"file_id required"}`, http.StatusBadRequest)
		return
	}
	if permission == "" {
		permission = "read"
	}
	allowedPerms := map[string]bool{"read": true, "download": true, "edit": true}
	if !allowedPerms[permission] {
		http.Error(w, `{"error":"invalid permission"}`, http.StatusBadRequest)
		return
	}

	shortCode := generateCode()
	now := time.Now()
	nowStr := now.Format(time.RFC3339)

	// Compute expires_at
	var expiresAt *string
	if expiresInHours > 0 {
		v := now.Add(time.Duration(expiresInHours) * time.Hour).Format(time.RFC3339)
		expiresAt = &v
	}

	// Hash password if provided
	var hashedPw sql.NullString
	if password != nil && *password != "" {
		v := hashPassword(*password)
		hashedPw = sql.NullString{String: v, Valid: true}
	}

	// Ensure permission column exists in share_links
	EnsureCompat(h.db)

	var linkID int64
	err := h.db.QueryRow(
		`INSERT INTO share_links (file_id, user_id, short_code, permission, password, expires_at, max_downloads, is_active, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, true, $8) RETURNING id`,
		fileID, userID, shortCode, permission, hashedPw, expiresAt, maxDownloads, nowStr,
	).Scan(&linkID)

	if err != nil {
		log.Printf("[share] CreateShareLink insert failed: %v (file_id=%d, user_id=%d)", err, fileID, userID)
		utils.WriteError(w, http.StatusInternalServerError, "create failed")
		return
	}

	utils.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":                 linkID,
		"short_code":         shortCode,
		"permission":         permission,
		"password_protected": hashedPw.Valid,
		"url":                "/s/" + shortCode,
	})
}

// VerifySharePassword checks the share link password and returns the file info.
func (h *Handler) VerifySharePassword(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var linkID int64
	var fileID int64
	var hashedPw *string
	err := h.db.QueryRow(
		`SELECT s.id, s.file_id, s.password FROM share_links s
		 WHERE s.short_code = $1 AND s.is_active = true
		 AND (s.expires_at IS NULL OR s.expires_at > $2)
		 AND (s.max_downloads IS NULL OR s.download_count < s.max_downloads)`,
		code, time.Now().Format(time.RFC3339),
	).Scan(&linkID, &fileID, &hashedPw)

	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"link invalid or expired"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	// Check password
	pw := r.URL.Query().Get("password")
	if hashedPw != nil && *hashedPw != "" {
		if pw == "" {
			utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"need_password": true,
				"protected":     true,
				"share_info": map[string]interface{}{
					"id":                 linkID,
					"short_code":         code,
					"password_protected": true,
				},
			})
			return
		}
		if hashPassword(pw) != *hashedPw {
			http.Error(w, `{"error":"password incorrect"}`, http.StatusForbidden)
			return
		}
	}

	// Increment view count
	h.db.Exec(`UPDATE share_links SET view_count = view_count + 1 WHERE id = $1`, linkID)

	// Return share info + file details
	var filename, mimeType string
	var fileSize int64
	h.db.QueryRow(
		`SELECT COALESCE(filename, ''), COALESCE(mime_type, ''), COALESCE(file_size, 0) FROM file_items WHERE id = $1`,
		fileID,
	).Scan(&filename, &mimeType, &fileSize)

	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"share_info": map[string]interface{}{
			"id":                 linkID,
			"short_code":         code,
			"permission":         "read",
			"download_count":     0,
			"max_downloads":      0,
			"password_protected": hashedPw != nil && *hashedPw != "",
		},
		"item": map[string]interface{}{
			"id":        fileID,
			"filename":  filename,
			"file_size": fileSize,
			"mime_type": mimeType,
		},
	})
}

// ShareFileDownload streams a shared file for download (public, no auth required).
func (h *Handler) ShareFileDownload(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var fileID int64
	var hashedPw *string
	err := h.db.QueryRow(
		`SELECT s.file_id, s.password FROM share_links s
		 WHERE (s.short_code = $1 OR s.token = $1) AND s.is_active = true
		 AND (s.expires_at IS NULL OR s.expires_at > $2)
		 AND (s.max_downloads IS NULL OR s.download_count < s.max_downloads)`,
		code, time.Now().Format(time.RFC3339),
	).Scan(&fileID, &hashedPw)

	if err != nil {
		http.Error(w, `{"error":"link invalid or expired"}`, http.StatusNotFound)
		return
	}

	// Check password
	pw := r.URL.Query().Get("password")
	if hashedPw != nil && *hashedPw != "" {
		if pw == "" {
			http.Error(w, `{"error":"password required"}`, http.StatusForbidden)
			return
		}
		if hashPassword(pw) != *hashedPw {
			http.Error(w, `{"error":"password incorrect"}`, http.StatusForbidden)
			return
		}
	}

	// Get file details
	var filePath, mimeType, filename string
	err = h.db.QueryRow(
		`SELECT file_path, mime_type, COALESCE(original_filename, filename) FROM file_items WHERE id = $1 AND deleted_at IS NULL`,
		fileID,
	).Scan(&filePath, &mimeType, &filename)
	if err != nil {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}

	// Increment download count
	h.db.Exec(`UPDATE share_links SET download_count = download_count + 1 WHERE (short_code = $1 OR token = $1)`, code)

	// Stream the file
	reader, err := h.storage.Retrieve(filePath)
	if err != nil {
		http.Error(w, `{"error":"file not found on storage"}`, http.StatusNotFound)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", "")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, reader)
}

// AccessShareLink returns download info for a shared file (public endpoint).
func (h *Handler) AccessShareLink(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	_ = token // deprecated: use short_code

	// Use short_code from query param
	code := r.URL.Query().Get("code")
	if code == "" {
		code = token // fallback
	}

	var fileID int64
	var hashedPw *string
	err := h.db.QueryRow(
		`SELECT s.file_id, s.password FROM share_links s
		 WHERE (s.token = $1 OR s.short_code = $1) AND s.is_active = true
		 AND (s.expires_at IS NULL OR s.expires_at > $2)
		 AND (s.max_downloads IS NULL OR s.download_count < s.max_downloads)`,
		code, time.Now().Format(time.RFC3339),
	).Scan(&fileID, &hashedPw)

	if err != nil {
		http.Error(w, `{"error":"link invalid or expired"}`, http.StatusNotFound)
		return
	}

	// Check password if provided
	pw := r.URL.Query().Get("password")
	if hashedPw != nil && *hashedPw != "" {
		if pw == "" {
			utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"need_password": true,
				"protected":     true,
			})
			return
		}
		if hashPassword(pw) != *hashedPw {
			http.Error(w, `{"error":"password incorrect"}`, http.StatusForbidden)
			return
		}
	}

	// Increment download count
	h.db.Exec(`UPDATE share_links SET download_count = download_count + 1 WHERE (token = $1 OR short_code = $1)`, code)

	writeJSON(w, http.StatusOK, map[string]int64{"file_id": fileID})
}

// ListShareLinks lists the user's share links (paginated).
func (h *Handler) ListShareLinks(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	EnsureCompat(h.db)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	h.db.QueryRow(`SELECT COUNT(*) FROM share_links WHERE user_id = $1`, userID).Scan(&total)

	rows, err := h.db.Query(
		`SELECT s.id, COALESCE(s.short_code, s.token), s.file_id, COALESCE(f.filename, ''),
		        COALESCE(s.permission, 'read'), s.is_active,
		        CASE WHEN s.password IS NOT NULL AND s.password != '' THEN true ELSE false END,
		        s.view_count, s.download_count, COALESCE(s.max_downloads, 0),
		        s.expires_at, s.created_at
		 FROM share_links s LEFT JOIN file_items f ON s.file_id = f.id
		 WHERE s.user_id = $1 ORDER BY s.created_at DESC LIMIT $2 OFFSET $3`,
		userID, pageSize, offset,
	)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	items := make([]ShareLinkJSON, 0)
	for rows.Next() {
		var item ShareLinkJSON
		rows.Scan(&item.ID, &item.ShortCode, &item.FileID, &item.Filename,
			&item.Permission, &item.IsActive, &item.PasswordProtected,
			&item.ViewCount, &item.DownloadCount, &item.MaxDownloads,
			&item.ExpiresAt, &item.CreatedAt)
		items = append(items, item)
	}

	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// DeleteShareLink deletes (revokes) a share link.
func (h *Handler) DeleteShareLink(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	linkID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	result, err := h.db.Exec(`UPDATE share_links SET is_active = false WHERE id = $1 AND user_id = $2`, linkID, userID)
	if err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// ShareWithUser shares a file directly with another user.
func (h *Handler) ShareWithUser(w http.ResponseWriter, r *http.Request) {
	ownerID, _ := middleware.GetUserID(r.Context())

	var req struct {
		FileID     int64  `json:"file_id"`
		SharedWith int64  `json:"shared_with"`
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if req.Permission == "" {
		req.Permission = "view"
	}
	if req.FileID == 0 || req.SharedWith == 0 {
		http.Error(w, `{"error":"file_id and shared_with required"}`, http.StatusBadRequest)
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
	utils.WriteJSON(w, http.StatusCreated, map[string]int64{"id": shareID})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	utils.WriteJSON(w, status, data)
}

// ─── Helper for public share frontend ───

// GetShareInfo returns share metadata without incrementing counters.
func (h *Handler) GetShareInfo(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var item struct {
		ID                int64  `json:"id"`
		ShortCode         string `json:"short_code"`
		FileID            int64  `json:"file_id"`
		PasswordProtected bool   `json:"password_protected"`
		ExpiresAt         *string `json:"expires_at"`
	}
	err := h.db.QueryRow(
		`SELECT id, COALESCE(short_code, token), file_id,
		        CASE WHEN password IS NOT NULL AND password != '' THEN true ELSE false END,
		        expires_at
		 FROM share_links WHERE short_code = $1 OR token = $1`,
		code,
	).Scan(&item.ID, &item.ShortCode, &item.FileID, &item.PasswordProtected, &item.ExpiresAt)

	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"link not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, `{"success":true,"data":{"id":%d,"short_code":"%s","file_id":%d,"password_protected":%t}}`,
		item.ID, item.ShortCode, item.FileID, item.PasswordProtected)
}

// EnsureCompat runs migration to add missing columns.
func EnsureCompat(db *sql.DB) {
	for _, m := range []string{
		`ALTER TABLE share_links ADD COLUMN IF NOT EXISTS token VARCHAR(64) DEFAULT ''`,
		`ALTER TABLE share_links ADD COLUMN IF NOT EXISTS short_code VARCHAR(32) DEFAULT ''`,
		`ALTER TABLE share_links ADD COLUMN IF NOT EXISTS permission VARCHAR(20) DEFAULT 'read'`,
		`ALTER TABLE share_links ADD COLUMN IF NOT EXISTS password VARCHAR(128) DEFAULT ''`,
		`ALTER TABLE share_links ADD COLUMN IF NOT EXISTS is_active BOOLEAN DEFAULT true`,
		`ALTER TABLE share_links ADD COLUMN IF NOT EXISTS view_count INT DEFAULT 0`,
		`ALTER TABLE share_links ADD COLUMN IF NOT EXISTS download_count INT DEFAULT 0`,
		`ALTER TABLE share_links ADD COLUMN IF NOT EXISTS max_downloads INT DEFAULT 0`,
		`ALTER TABLE share_links ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP`,
		`ALTER TABLE share_links ADD COLUMN IF NOT EXISTS created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`,
	} {
		db.Exec(m)
	}
}
