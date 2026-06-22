package share

import (
	"archive/zip"
	"crypto/rand"
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
	"github.com/Athenavi/Stora/pkg/auth"
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

func hashPassword(pw string) (string, error) {
	return auth.HashPassword(pw)
}

func checkSharePassword(password, hash string) bool {
	return auth.CheckPassword(password, hash)
}

// nullIfZero returns nil for 0, allowing DB NULL for optional FK columns.
func nullIfZero(v int64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

// ─── Shared types ───

type ShareLinkJSON struct {
	ID                int64   `json:"id"`
	ShortCode         string  `json:"short_code"`
	FileID            int64   `json:"file_id"`
	FolderID          *int64  `json:"folder_id,omitempty"`
	Filename          string  `json:"filename"`
	Permission        string  `json:"permission"`
	IsActive          bool    `json:"is_active"`
	IsFolder          bool    `json:"is_folder"`
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
	var folderID int64
	var permission string
	var password *string
	var expiresInHours int
	maxDownloads := 0

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") || strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "invalid form")
			return
		}
		if fid := r.FormValue("file_id"); fid != "" {
			if v, err := strconv.ParseInt(fid, 10, 64); err == nil {
				fileID = v
			}
		}
		if fid := r.FormValue("folder_id"); fid != "" {
			if v, err := strconv.ParseInt(fid, 10, 64); err == nil {
				folderID = v
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
			FolderID       int64   `json:"folder_id"`
			Permission     string  `json:"permission"`
			Password       *string `json:"password"`
			ExpiresInHours int     `json:"expires_in_hours"`
			MaxDownloads   int     `json:"max_downloads"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		fileID = req.FileID
		folderID = req.FolderID
		permission = req.Permission
		password = req.Password
		expiresInHours = req.ExpiresInHours
		maxDownloads = req.MaxDownloads
	}

	if fileID == 0 && folderID == 0 {
		writeError(w, http.StatusBadRequest, "file_id or folder_id required")
		return
	}
	if fileID != 0 && folderID != 0 {
		writeError(w, http.StatusBadRequest, "provide either file_id or folder_id, not both")
		return
	}
	if permission == "" {
		permission = "read"
	}
	allowedPerms := map[string]bool{"read": true, "download": true, "edit": true}
	if !allowedPerms[permission] {
		writeError(w, http.StatusBadRequest, "invalid permission")
		return
	}

	// Verify ownership
	if folderID > 0 {
		var ownerID int64
		err := h.db.QueryRow(`SELECT user_id FROM folders WHERE id = $1`, folderID).Scan(&ownerID)
		if err != nil || ownerID != userID {
			writeError(w, http.StatusNotFound, "folder not found")
			return
		}
	} else if fileID > 0 {
		var ownerID int64
		err := h.db.QueryRow(`SELECT user_id FROM file_items WHERE id = $1 AND deleted_at IS NULL`, fileID).Scan(&ownerID)
		if err != nil || ownerID != userID {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
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
		v, err := hashPassword(*password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "password hash failed")
			return
		}
		hashedPw = sql.NullString{String: v, Valid: true}
	}

	EnsureCompat(h.db)

	var linkID int64
	isFolder := folderID > 0

	err := h.db.QueryRow(
		`INSERT INTO share_links (file_id, folder_id, user_id, short_code, permission, password, expires_at, max_downloads, is_active, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true, $9) RETURNING id`,
		nullIfZero(fileID), nullIfZero(folderID), userID, shortCode, permission, hashedPw, expiresAt, maxDownloads, nowStr,
	).Scan(&linkID)

	if err != nil {
		log.Printf("[share] CreateShareLink insert failed: %v (file_id=%d, folder_id=%d, user_id=%d)", err, fileID, folderID, userID)
		utils.WriteError(w, http.StatusInternalServerError, "create failed")
		return
	}

	itemName := ""
	if isFolder {
		h.db.QueryRow(`SELECT name FROM folders WHERE id = $1`, folderID).Scan(&itemName)
	} else {
		h.db.QueryRow(`SELECT COALESCE(filename, '') FROM file_items WHERE id = $1`, fileID).Scan(&itemName)
	}

	utils.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":                 linkID,
		"short_code":         shortCode,
		"permission":         permission,
		"password_protected": hashedPw.Valid,
		"is_folder":          isFolder,
		"filename":           itemName,
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
		 AND (s.max_downloads IS NULL OR s.max_downloads = 0 OR s.download_count < s.max_downloads)`,
		code, time.Now().Format(time.RFC3339),
	).Scan(&linkID, &fileID, &hashedPw)

	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "link invalid or expired")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
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
		if !checkSharePassword(pw, *hashedPw) {
			writeError(w, http.StatusForbidden, "invalid or expired share link")
			return
		}
	}

	// Increment view count
	h.db.Exec(`UPDATE share_links SET view_count = view_count + 1 WHERE id = $1`, linkID)

	// Determine if this is a file or folder share
	var isFolder bool
	var folderID int64
	h.db.QueryRow(`SELECT folder_id IS NOT NULL, COALESCE(folder_id, 0) FROM share_links WHERE id = $1`, linkID).Scan(&isFolder, &folderID)

	if isFolder {
		// Return folder info + file listing
		var folderName string
		h.db.QueryRow(`SELECT name FROM folders WHERE id = $1`, folderID).Scan(&folderName)

		type FolderFileItem struct {
			ID       int64  `json:"id"`
			Filename string `json:"filename"`
			FileSize int64  `json:"file_size"`
			FileType string `json:"file_type"`
		}
		items := make([]FolderFileItem, 0)

		// Get child files
		frows, err := h.db.Query(
			`SELECT id, COALESCE(filename, ''), COALESCE(file_size, 0), COALESCE(file_type, 'other')
			 FROM file_items WHERE folder_id = $1 AND deleted_at IS NULL ORDER BY filename`,
			folderID,
		)
		if err == nil {
			defer frows.Close()
			for frows.Next() {
				var fi FolderFileItem
				frows.Scan(&fi.ID, &fi.Filename, &fi.FileSize, &fi.FileType)
				items = append(items, fi)
			}
		}

		// Get child folders
		type SubFolder struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		}
		subFolders := make([]SubFolder, 0)
		drows, err := h.db.Query(`SELECT id, name FROM folders WHERE parent_id = $1 ORDER BY name`, folderID)
		if err == nil {
			defer drows.Close()
			for drows.Next() {
				var sf SubFolder
				drows.Scan(&sf.ID, &sf.Name)
				subFolders = append(subFolders, sf)
			}
		}

		utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"share_info": map[string]interface{}{
				"id":                 linkID,
				"short_code":         code,
				"permission":         "read",
				"password_protected": hashedPw != nil && *hashedPw != "",
				"is_folder":          true,
			},
			"item": map[string]interface{}{
				"id":       folderID,
				"filename": folderName,
				"is_folder": true,
			},
			"folders": subFolders,
			"items":   items,
		})
		return
	}

	// Return share info + single file details
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
// For folder shares, streams a ZIP of all files in the folder.
func (h *Handler) ShareFileDownload(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var fileID, folderID int64
	var hashedPw *string
	var isFolder bool
	err := h.db.QueryRow(
		`SELECT COALESCE(s.file_id, 0), COALESCE(s.folder_id, 0), s.password,
		        CASE WHEN s.folder_id IS NOT NULL THEN true ELSE false END
		 FROM share_links s
		 WHERE (s.short_code = $1 OR s.token = $1) AND s.is_active = true
		 AND (s.expires_at IS NULL OR s.expires_at > $2)
		 AND (s.max_downloads IS NULL OR s.max_downloads = 0 OR s.download_count < s.max_downloads)`,
		code, time.Now().Format(time.RFC3339),
	).Scan(&fileID, &folderID, &hashedPw, &isFolder)

	if err != nil {
		writeError(w, http.StatusNotFound, "link invalid or expired")
		return
	}

	// Check password
	pw := r.URL.Query().Get("password")
	if hashedPw != nil && *hashedPw != "" {
		if pw == "" {
			writeError(w, http.StatusForbidden, "password required")
			return
		}
		if !checkSharePassword(pw, *hashedPw) {
			writeError(w, http.StatusForbidden, "invalid or expired share link")
			return
		}
	}

	// Increment download count
	h.db.Exec(`UPDATE share_links SET download_count = download_count + 1 WHERE (short_code = $1 OR token = $1)`, code)

	if isFolder {
		// Query all files in the folder (one level, non-recursive for simplicity)
		type fileRef struct {
			FilePath string
			Filename string
		}
		rows, err := h.db.Query(
			`SELECT file_path, COALESCE(original_filename, filename) FROM file_items
			 WHERE folder_id = $1 AND deleted_at IS NULL`, folderID,
		)
		if err != nil || rows == nil {
			writeError(w, http.StatusInternalServerError, "query failed")
			return
		}
		var files []fileRef
		for rows.Next() {
			var fr fileRef
			rows.Scan(&fr.FilePath, &fr.Filename)
			files = append(files, fr)
		}
		rows.Close()

		// Stream ZIP
		var folderName string
		h.db.QueryRow(`SELECT name FROM folders WHERE id = $1`, folderID).Scan(&folderName)

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, folderName))
		w.WriteHeader(http.StatusOK)

		zw := zip.NewWriter(w)
		for _, f := range files {
			reader, rErr := h.storage.Retrieve(f.FilePath)
			if rErr != nil {
				continue
			}
			hdr := &zip.FileHeader{Name: f.Filename, Method: zip.Deflate}
			writer, wErr := zw.CreateHeader(hdr)
			if wErr != nil {
				reader.Close()
				continue
			}
			io.Copy(writer, reader)
			reader.Close()
		}
		zw.Close()
		return
	}

	// Get file details
	var filePath, mimeType, filename string
	err = h.db.QueryRow(
		`SELECT file_path, mime_type, COALESCE(original_filename, filename) FROM file_items WHERE id = $1 AND deleted_at IS NULL`,
		fileID,
	).Scan(&filePath, &mimeType, &filename)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	// Stream the file
	reader, err := h.storage.Retrieve(filePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found on storage")
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
		 AND (s.max_downloads IS NULL OR s.max_downloads = 0 OR s.download_count < s.max_downloads)`,
		code, time.Now().Format(time.RFC3339),
	).Scan(&fileID, &hashedPw)

	if err != nil {
		writeError(w, http.StatusNotFound, "link invalid or expired")
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
		if !checkSharePassword(pw, *hashedPw) {
			writeError(w, http.StatusForbidden, "invalid or expired share link")
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
		`SELECT s.id, COALESCE(s.short_code, s.token), COALESCE(s.file_id, 0), COALESCE(s.folder_id, 0),
		        CASE WHEN s.file_id IS NOT NULL THEN COALESCE(f.filename, '') ELSE COALESCE(d.name, '') END,
		        COALESCE(s.permission, 'read'), s.is_active,
		        CASE WHEN s.password IS NOT NULL AND s.password != '' THEN true ELSE false END,
		        s.view_count, s.download_count, COALESCE(s.max_downloads, 0),
		        s.expires_at, s.created_at,
		        CASE WHEN s.folder_id IS NOT NULL THEN true ELSE false END
		 FROM share_links s
		 LEFT JOIN file_items f ON s.file_id = f.id
		 LEFT JOIN folders d ON s.folder_id = d.id
		 WHERE s.user_id = $1 ORDER BY s.created_at DESC LIMIT $2 OFFSET $3`,
		userID, pageSize, offset,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	items := make([]ShareLinkJSON, 0)
	for rows.Next() {
		var item ShareLinkJSON
		var folderID int64
		rows.Scan(&item.ID, &item.ShortCode, &item.FileID, &folderID,
			&item.Filename, &item.Permission, &item.IsActive, &item.PasswordProtected,
			&item.ViewCount, &item.DownloadCount, &item.MaxDownloads,
			&item.ExpiresAt, &item.CreatedAt, &item.IsFolder)
		if folderID > 0 {
			item.FolderID = &folderID
		}
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
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "not found")
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
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.Permission == "" {
		req.Permission = "view"
	}
	if req.FileID == 0 || req.SharedWith == 0 {
		writeError(w, http.StatusBadRequest, "file_id and shared_with required")
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
		writeError(w, http.StatusInternalServerError, "share failed")
		return
	}
	utils.WriteJSON(w, http.StatusCreated, map[string]int64{"id": shareID})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	utils.WriteJSON(w, status, data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	utils.WriteError(w, status, msg)
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
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	fmt.Fprintf(w, `{"success":true,"data":{"id":%d,"short_code":"%s","file_id":%d,"password_protected":%t}}`,
		item.ID, item.ShortCode, item.FileID, item.PasswordProtected)
}

// EnsureCompat runs migration to add missing columns.
func EnsureCompat(db *sql.DB) {
	for _, m := range []string{
		`ALTER TABLE share_links ADD COLUMN IF NOT EXISTS folder_id BIGINT REFERENCES folders(id) ON DELETE SET NULL`,
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
