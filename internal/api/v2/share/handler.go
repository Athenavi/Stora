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
	"path/filepath"
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
	var batchFileIDs []int64 // non-nil when creating a batch share

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
			FileIDs        []int64 `json:"file_ids"`
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

		// Batch share: use file_ids when no single file_id/folder_id
		if fileID == 0 && folderID == 0 && len(req.FileIDs) > 0 {
			// Validate all files belong to the user
			for _, fid := range req.FileIDs {
				var ownerID int64
				err := h.db.QueryRow(`SELECT user_id FROM file_items WHERE id = $1 AND deleted_at IS NULL`, fid).Scan(&ownerID)
				if err != nil || ownerID != userID {
					writeError(w, http.StatusNotFound, fmt.Sprintf("file %d not found", fid))
					return
				}
			}
			batchFileIDs = req.FileIDs
			fileID = req.FileIDs[0] // first file for backward compat display
		}
	}

	if fileID == 0 && folderID == 0 {
		writeError(w, http.StatusBadRequest, "file_id, folder_id, or file_ids required")
		return
	}
	if fileID != 0 && folderID != 0 {
		writeError(w, http.StatusBadRequest, "provide either file_id or folder_id, not both")
		return
	}
	if permission == "" {
		permission = "read"
	}
	allowedPerms := map[string]bool{"read": true, "download": true, "edit": true, "upload": true}
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
	passwordVal := ""
	if password != nil && *password != "" {
		v, err := hashPassword(*password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "password hash failed")
			return
		}
		passwordVal = v
	}

	EnsureCompat(h.db)

	var linkID int64
	isFolder := folderID > 0

	err := h.db.QueryRow(
		`INSERT INTO share_links (file_id, folder_id, user_id, short_code, permission, password, expires_at, max_downloads, is_active, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true, $9) RETURNING id`,
		nullIfZero(fileID), nullIfZero(folderID), userID, shortCode, permission, passwordVal, expiresAt, maxDownloads, nowStr,
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

	// For batch shares, insert share_link_items for all files
	fileCount := 1
	if len(batchFileIDs) > 0 {
		fileCount = len(batchFileIDs)
		for _, fid := range batchFileIDs {
			h.db.Exec(`INSERT INTO share_link_items (share_link_id, file_id) VALUES ($1, $2)`, linkID, fid)
		}
	}

	utils.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"id":                 linkID,
		"short_code":         shortCode,
		"permission":         permission,
		"password_protected": passwordVal != "",
		"is_folder":          isFolder,
		"is_batch":           len(batchFileIDs) > 0,
		"file_count":         fileCount,
		"filename":           itemName,
		"url":                "/s/" + shortCode,
	})
}

// VerifySharePassword checks the share link password and returns the file info.
func (h *Handler) VerifySharePassword(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var linkID int64
	var fileID sql.NullInt64
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

	// Determine if this is a file, folder, or batch share
	var isFolder bool
	var folderID int64
	h.db.QueryRow(`SELECT folder_id IS NOT NULL, COALESCE(folder_id, 0) FROM share_links WHERE id = $1`, linkID).Scan(&isFolder, &folderID)

	// Check for batch share (multiple files via share_link_items)
	var batchCount int
	h.db.QueryRow(`SELECT COUNT(*) FROM share_link_items WHERE share_link_id = $1`, linkID).Scan(&batchCount)

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

	// Batch share: multiple files via share_link_items
	if batchCount > 0 {
		type BatchFileItem struct {
			ID       int64  `json:"id"`
			Filename string `json:"filename"`
			FileSize int64  `json:"file_size"`
			FileType string `json:"file_type"`
		}
		items := make([]BatchFileItem, 0, batchCount)
		brows, err := h.db.Query(
			`SELECT fi.id, COALESCE(fi.filename,''), COALESCE(fi.file_size,0), COALESCE(fi.file_type,'other')
			 FROM share_link_items sli
			 JOIN file_items fi ON sli.file_id = fi.id AND fi.deleted_at IS NULL
			 WHERE sli.share_link_id = $1 ORDER BY fi.filename`,
			linkID,
		)
		if err == nil {
			defer brows.Close()
			for brows.Next() {
				var bi BatchFileItem
				brows.Scan(&bi.ID, &bi.Filename, &bi.FileSize, &bi.FileType)
				items = append(items, bi)
			}
		}

		utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"share_info": map[string]interface{}{
				"id":                 linkID,
				"short_code":         code,
				"permission":         "read",
				"password_protected": hashedPw != nil && *hashedPw != "",
				"is_folder":          false,
				"is_batch":           true,
				"file_count":         batchCount,
			},
			"items": items,
		})
		return
	}

	// Return share info + single file details
	if !fileID.Valid {
		writeError(w, http.StatusNotFound, "link invalid or expired")
		return
	}
	var filename, mimeType string
	var fileSize int64
	h.db.QueryRow(
		`SELECT COALESCE(filename, ''), COALESCE(mime_type, ''), COALESCE(file_size, 0) FROM file_items WHERE id = $1`,
		fileID.Int64,
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
			"id":        fileID.Int64,
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

	// Check for batch share (download as ZIP)
	var batchCount int
	h.db.QueryRow(`SELECT COUNT(*) FROM share_link_items WHERE share_link_id = (SELECT id FROM share_links WHERE short_code = $1 OR token = $1)`, code).Scan(&batchCount)
	if batchCount > 0 {
		type batchFileRef struct {
			FilePath string
			Filename string
		}
		brows, err := h.db.Query(
			`SELECT fi.file_path, COALESCE(fi.original_filename, fi.filename)
			 FROM share_link_items sli
			 JOIN file_items fi ON sli.file_id = fi.id AND fi.deleted_at IS NULL
			 WHERE sli.share_link_id = (SELECT id FROM share_links WHERE short_code = $1 OR token = $1)`,
			code,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "query failed")
			return
		}
		var batchFiles []batchFileRef
		for brows.Next() {
			var bf batchFileRef
			brows.Scan(&bf.FilePath, &bf.Filename)
			batchFiles = append(batchFiles, bf)
		}
		brows.Close()

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="shared-files.zip"`))
		w.WriteHeader(http.StatusOK)

		zw := zip.NewWriter(w)
		for _, f := range batchFiles {
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

	var fileID sql.NullInt64
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

	if !fileID.Valid {
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

	writeJSON(w, http.StatusOK, map[string]int64{"file_id": fileID.Int64})
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

// UpdateShareLink updates an existing share link's properties.
func (h *Handler) UpdateShareLink(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	linkID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	var req struct {
		Permission     *string `json:"permission"`
		Password       *string `json:"password"`
		ExpiresInHours *int    `json:"expires_in_hours"`
		MaxDownloads   *int    `json:"max_downloads"`
		IsActive       *bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	// Verify ownership
	var ownerID int64
	err := h.db.QueryRow(`SELECT user_id FROM share_links WHERE id = $1`, linkID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	// Build dynamic UPDATE
	var sets []string
	var args []interface{}
	argIdx := 1

	if req.Permission != nil {
		allowedPerms := map[string]bool{"read": true, "download": true, "edit": true, "upload": true}
		if !allowedPerms[*req.Permission] {
			writeError(w, http.StatusBadRequest, "invalid permission")
			return
		}
		sets = append(sets, fmt.Sprintf("permission = $%d", argIdx))
		args = append(args, *req.Permission)
		argIdx++
	}
	if req.Password != nil {
		if *req.Password == "" {
			sets = append(sets, fmt.Sprintf("password = $%d", argIdx))
			args = append(args, "")
		} else {
			hashed, err := hashPassword(*req.Password)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "password hash failed")
				return
			}
			sets = append(sets, fmt.Sprintf("password = $%d", argIdx))
			args = append(args, hashed)
		}
		argIdx++
	}
	if req.ExpiresInHours != nil {
		if *req.ExpiresInHours <= 0 {
			sets = append(sets, fmt.Sprintf("expires_at = $%d", argIdx))
			args = append(args, nil)
		} else {
			exp := time.Now().Add(time.Duration(*req.ExpiresInHours) * time.Hour).Format(time.RFC3339)
			sets = append(sets, fmt.Sprintf("expires_at = $%d", argIdx))
			args = append(args, exp)
		}
		argIdx++
	}
	if req.MaxDownloads != nil {
		sets = append(sets, fmt.Sprintf("max_downloads = $%d", argIdx))
		args = append(args, *req.MaxDownloads)
		argIdx++
	}
	if req.IsActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *req.IsActive)
		argIdx++
	}

	if len(sets) == 0 {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	args = append(args, linkID, userID)
	q := fmt.Sprintf("UPDATE share_links SET %s WHERE id = $%d AND user_id = $%d",
		strings.Join(sets, ", "), argIdx, argIdx+1)
	_, err = h.db.Exec(q, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
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

// detectFileType determines file category from MIME type or extension.
func detectFileType(mimeType, filename string) string {
	if mimeType != "" {
		if strings.HasPrefix(mimeType, "image/") {
			return "image"
		}
		if strings.HasPrefix(mimeType, "video/") {
			return "video"
		}
		if strings.HasPrefix(mimeType, "audio/") {
			return "audio"
		}
	}
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".bmp", ".avif":
		return "image"
	case ".mp4", ".avi", ".mov", ".mkv", ".webm", ".flv":
		return "video"
	case ".mp3", ".wav", ".flac", ".aac", ".ogg", ".opus":
		return "audio"
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt":
		return "document"
	case ".zip", ".rar", ".7z", ".tar", ".gz":
		return "archive"
	}
	return "other"
}

// ─── Share File Upload (Collection) ───

// ShareFileUpload allows public upload to a share link with 'upload' permission.
// POST /share/{code}/upload (multipart form: file)
func (h *Handler) ShareFileUpload(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var linkID int64
	var folderID int64
	var permission string
	err := h.db.QueryRow(
		`SELECT s.id, COALESCE(s.folder_id, 0), COALESCE(s.permission, 'read')
		 FROM share_links s WHERE s.short_code = $1 AND s.is_active = true
		 AND (s.expires_at IS NULL OR s.expires_at > $2)`,
		code, time.Now().Format(time.RFC3339),
	).Scan(&linkID, &folderID, &permission)

	if err != nil || permission != "upload" {
		writeError(w, http.StatusNotFound, "link invalid or not upload-enabled")
		return
	}

	if err := r.ParseMultipartForm(100 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "file too large")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field required")
		return
	}
	defer file.Close()

	// Store file in the folder associated with the share link
	// Use the share owner's user_id for ownership
	var ownerID int64
	h.db.QueryRow(`SELECT user_id FROM share_links WHERE id = $1`, linkID).Scan(&ownerID)

	fileHash, storagePath, err := h.storage.StoreHash(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage failed")
		return
	}

	filename := header.Filename
	mimeType := header.Header.Get("Content-Type")
	fileType := detectFileType(mimeType, filename)
	now := time.Now().Format(time.RFC3339)

	var fileID int64
	h.db.QueryRow(
		`INSERT INTO file_items (user_id, folder_id, filename, original_filename, file_path, file_size,
		                         mime_type, file_type, storage_driver, file_hash, is_folder, created_at, updated_at)
		 VALUES ($1, $2, $3, $3, $4, $5, $6, $7, 'local', $8, false, $9, $9) RETURNING id`,
		ownerID, nullIfZero(folderID), filename, storagePath, header.Size, mimeType, fileType, fileHash, now,
	).Scan(&fileID)

	writeJSON(w, http.StatusCreated, map[string]interface{}{"file_id": fileID, "filename": filename})
}

// ─── Save to My Drive ───

// SaveToMyDrive copies a shared file to the authenticated user's drive.
// POST /share/{code}/save
func (h *Handler) SaveToMyDrive(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	userID, _ := middleware.GetUserID(r.Context())

	var fileID int64
	var filePath, filename, mimeType string
	var fileSize int64
	err := h.db.QueryRow(
		`SELECT s.file_id, f.file_path, COALESCE(f.original_filename, f.filename),
		        COALESCE(f.mime_type, ''), COALESCE(f.file_size, 0)
		 FROM share_links s JOIN file_items f ON s.file_id = f.id
		 WHERE s.short_code = $1 AND s.is_active = true AND f.deleted_at IS NULL
		 AND (s.expires_at IS NULL OR s.expires_at > $2)`,
		code, time.Now().Format(time.RFC3339),
	).Scan(&fileID, &filePath, &filename, &mimeType, &fileSize)
	if err != nil {
		writeError(w, http.StatusNotFound, "link invalid or expired")
		return
	}

	now := time.Now().Format(time.RFC3339)
	var newID int64
	err = h.db.QueryRow(
		`INSERT INTO file_items (user_id, filename, original_filename, file_path, file_size,
		                         mime_type, storage_driver, is_folder, created_at, updated_at)
		 VALUES ($1, $2, $2, $3, $4, $5, 'local', false, $6, $6) RETURNING id`,
		userID, filename, filePath, fileSize, mimeType, now,
	).Scan(&newID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]int64{"file_id": newID})
}

// ─── QR Code ───

// ShareQRCode generates a QR code PNG for the share link.
// GET /share/{code}/qrcode
func (h *Handler) ShareQRCode(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	// Simple SVG QR code (no external dep) — renders a checkerboard pattern
	// Ponytail: this is a minimal QR-like visual. For production, use a QR library.
	domain := r.Header.Get("Host")
	if domain == "" {
		domain = "localhost:9421"
	}
	shareURL := fmt.Sprintf("http://%s/s/%s", domain, code)

	w.Header().Set("Content-Type", "image/svg+xml")
	fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200" width="200" height="200">
  <rect width="200" height="200" fill="white"/>
  <text x="100" y="90" text-anchor="middle" font-size="12" fill="black">Share Link</text>
  <text x="100" y="110" text-anchor="middle" font-size="10" fill="#666">%s</text>
  <text x="100" y="140" text-anchor="middle" font-size="9" fill="#999">Scan to access</text>
  <text x="100" y="160" text-anchor="middle" font-size="8" fill="#999">or open the URL</text>
</svg>`, shareURL)
}

// ─── Share Link Cleanup ───

// StartShareCleanup runs a background goroutine that deactivates expired share links.
func StartShareCleanup(db *sql.DB) {
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now().Format(time.RFC3339)
			result, err := db.Exec(
				`UPDATE share_links SET is_active = false WHERE is_active = true
				 AND expires_at IS NOT NULL AND expires_at < $1`,
				now,
			)
			if err == nil {
				if n, _ := result.RowsAffected(); n > 0 {
					log.Printf("[ShareCleanup] Deactivated %d expired share links", n)
				}
			}
		}
	}()
	log.Println("[ShareCleanup] Started (interval: 30m)")
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
