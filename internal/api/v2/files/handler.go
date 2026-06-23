package files

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/Athenavi/Stora/pkg/cache"
	"github.com/Athenavi/Stora/pkg/storage"
	"github.com/Athenavi/Stora/pkg/utils"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const (
	maxPathDepth = 50
	maxPathLen   = 4096
	maxSegLen    = 255
	cacheTTL     = 10 * time.Second
	maxCacheSize = 10000 // max in-memory path cache entries before eviction
)

type Handler struct {
	db      *sql.DB
	storage storage.Driver
	tempDir string

	pathCache   map[string]pathCacheEntry
	pathCacheMu sync.RWMutex
	redisCache  *cache.PathCache // optional L2 cache (nil = disabled)

	limiter  *middleware.SpeedLimiter // upload/download speed control (nil = no limit)
	vaultDir string
}

type pathCacheEntry struct {
	folderID  int64
	expiresAt time.Time
}

func NewHandler(db *sql.DB, store storage.Driver, tempDir string, vaultDir string, redisCache *cache.PathCache, limiter *middleware.SpeedLimiter) *Handler {
	h := &Handler{
		db:        db,
		storage:   store,
		tempDir:   tempDir,
		pathCache: make(map[string]pathCacheEntry),
		redisCache: redisCache,
		limiter:   limiter,
		vaultDir:  vaultDir,
	}
	// Periodic cache cleanup: removes expired entries and evicts oldest if over maxCacheSize
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			h.pathCacheMu.Lock()
			now := time.Now()
			for k, v := range h.pathCache {
				if now.After(v.expiresAt) {
					delete(h.pathCache, k)
				}
			}
			// If still over limit, delete oldest entries
			if len(h.pathCache) > maxCacheSize {
				toDelete := len(h.pathCache) - maxCacheSize
				// Simple approach: delete first N entries (iteration order is stable-ish in practice)
				for k := range h.pathCache {
					if toDelete <= 0 {
						break
					}
					delete(h.pathCache, k)
					toDelete--
				}
			}
			h.pathCacheMu.Unlock()
		}
	}()
	return h
}

// ---------- File CRUD ----------

// ListFiles returns paginated files for the current user.
func (h *Handler) ListFiles(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	folderID := r.URL.Query().Get("folder_id")
	fileType := r.URL.Query().Get("file_type")
	sortBy := r.URL.Query().Get("sort_by")
	sortOrder := r.URL.Query().Get("sort_order")

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}
	offset := (page - 1) * perPage

	// Build query
	where := []string{"user_id = $1", "deleted_at IS NULL"}
	args := []interface{}{userID}
	argIdx := 2

	if folderID != "" {
		if fid, err := strconv.ParseInt(folderID, 10, 64); err == nil {
			where = append(where, fmt.Sprintf("folder_id = $%d", argIdx))
			args = append(args, fid)
			argIdx++
		}
	} else {
		where = append(where, "folder_id IS NULL")
	}

	if fileType != "" && fileType != "all" {
		where = append(where, fmt.Sprintf("file_type = $%d", argIdx))
		args = append(args, fileType)
		argIdx++
	}

	// is_favorite filter
	favStr := r.URL.Query().Get("is_favorite")
	if favStr == "true" || favStr == "1" {
		where = append(where, "is_favorite = true")
	} else if favStr == "false" || favStr == "0" {
		where = append(where, "is_favorite = false")
	}

	// category filter
	if cat := r.URL.Query().Get("category"); cat != "" {
		where = append(where, fmt.Sprintf("category = $%d", argIdx))
		args = append(args, cat)
		argIdx++
	}

	// tag_id filter
	if tagIDStr := r.URL.Query().Get("tag_id"); tagIDStr != "" {
		if tid, err := strconv.ParseInt(tagIDStr, 10, 64); err == nil {
			where = append(where, fmt.Sprintf("id IN (SELECT file_id FROM file_tag_assignments WHERE tag_id = $%d)", argIdx))
			args = append(args, tid)
			argIdx++
		}
	}

	// Sort
	if sortBy == "" {
		sortBy = "created_at"
	}
	if sortOrder == "" {
		sortOrder = "desc"
	}
	allowedSorts := map[string]bool{"created_at": true, "filename": true, "file_size": true, "updated_at": true, "sort_order": true}
	if !allowedSorts[sortBy] {
		sortBy = "created_at"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	whereClause := strings.Join(where, " AND ")

	// Count
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM file_items WHERE %s", whereClause)
	h.db.QueryRow(countQuery, args...).Scan(&total)

	// Query
	query := fmt.Sprintf(
		`SELECT id, filename, original_filename, file_size, mime_type, file_type, is_folder, is_favorite,
		        thumbnail_url, width, height, duration, folder_id, category, sort_order,
		        description, file_hash, is_encrypted, download_count, created_at, updated_at
		 FROM file_items WHERE %s ORDER BY is_folder DESC, %s %s LIMIT $%d OFFSET $%d`,
		whereClause, sortBy, sortOrder, argIdx, argIdx+1)
	args = append(args, perPage, offset)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type FileItem struct {
		ID           int64   `json:"id"`
		Filename     *string `json:"filename"`
		OrigName     *string `json:"original_filename"`
		FileSize     *int64  `json:"file_size"`
		MimeType     *string `json:"mime_type"`
		FileType     *string `json:"file_type"`
		IsFolder     *bool   `json:"is_folder"`
		IsFav        *bool   `json:"is_favorite"`
		ThumbURL     *string `json:"thumbnail_url"`
		Width        *int    `json:"width"`
		Height       *int    `json:"height"`
		Duration     *int    `json:"duration"`
		FolderID     *int64  `json:"folder_id"`
		Category     *string `json:"category"`
		SortOrder    *int64  `json:"sort_order"`
		Description  *string `json:"description"`
		FileHash     *string `json:"file_hash"`
		IsEncrypted  *bool   `json:"is_encrypted"`
		DownloadCnt  *int64  `json:"download_count"`
		CreatedAt    *string `json:"created_at"`
		UpdatedAt    *string `json:"updated_at"`
	}

	var items = make([]FileItem, 0)
	for rows.Next() {
		var item FileItem
		rows.Scan(&item.ID, &item.Filename, &item.OrigName, &item.FileSize, &item.MimeType,
			&item.FileType, &item.IsFolder, &item.IsFav, &item.ThumbURL, &item.Width,
			&item.Height, &item.Duration, &item.FolderID, &item.Category, &item.SortOrder,
			&item.Description, &item.FileHash, &item.IsEncrypted, &item.DownloadCnt,
			&item.CreatedAt, &item.UpdatedAt)
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
		"total": total,
		"page":  page,
		"per_page": perPage,
	})
}

// GetFile returns a single file's details.
func (h *Handler) GetFile(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	var item struct {
		ID        int64    `json:"id"`
		Filename  *string  `json:"filename"`
		OrigName  *string  `json:"original_filename"`
		FileSize  *int64   `json:"file_size"`
		MimeType  *string  `json:"mime_type"`
		FileType  *string  `json:"file_type"`
		IsFolder  *bool    `json:"is_folder"`
		IsFav     *bool    `json:"is_favorite"`
		FilePath  *string  `json:"-"`
		FileURL   *string  `json:"file_url"`
		ThumbURL  *string  `json:"thumbnail_url"`
		Width     *int     `json:"width"`
		Height    *int     `json:"height"`
		Duration  *int     `json:"duration"`
		FolderID  *int64   `json:"folder_id"`
		CreatedAt *string  `json:"created_at"`
		UpdatedAt *string  `json:"updated_at"`
		Desc      *string  `json:"description"`
	}

	err := h.db.QueryRow(
		`SELECT id, filename, original_filename, file_size, mime_type, file_type, is_folder,
		        is_favorite, file_path, file_url, thumbnail_url, width, height, duration,
		        folder_id, created_at, updated_at, description
		 FROM file_items WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		fileID, userID,
	).Scan(&item.ID, &item.Filename, &item.OrigName, &item.FileSize, &item.MimeType,
		&item.FileType, &item.IsFolder, &item.IsFav, &item.FilePath, &item.FileURL,
		&item.ThumbURL, &item.Width, &item.Height, &item.Duration, &item.FolderID,
		&item.CreatedAt, &item.UpdatedAt, &item.Desc)

	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if err != nil {
		log.Printf("[GetFile] Scan error for file %d: %v", fileID, err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	writeJSON(w, http.StatusOK, item)
}

// UploadFile handles file upload.
func (h *Handler) UploadFile(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "file too large or invalid form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "file field required")
		return
	}
	defer file.Close()

	folderIDStr := r.FormValue("folder_id")
	var folderID *int64
	if folderIDStr != "" {
		if fid, err := strconv.ParseInt(folderIDStr, 10, 64); err == nil {
			folderID = &fid
		}
	}

	filename := header.Filename
	mimeType := header.Header.Get("Content-Type")
	fileType := detectFileType(mimeType, filename)
	fileSize := header.Size

	// Storage quota check — BEFORE writing to disk
	var totalStorage, usedStorage int64
	h.db.QueryRow(`SELECT total_storage, used_storage FROM users WHERE id = $1`, userID).Scan(&totalStorage, &usedStorage)
	if totalStorage > 0 && usedStorage+fileSize > totalStorage {
		utils.WriteError(w, http.StatusInsufficientStorage, "storage quota exceeded")
		return
	}
	// Warn at 90%
	if totalStorage > 0 && usedStorage+fileSize > int64(float64(totalStorage)*0.9) {
		h.db.Exec(`INSERT INTO notifications (user_id, type, title, body, created_at)
			VALUES ($1, 'quota', '存储空间预警', $2, $3)
			ON CONFLICT DO NOTHING`,
			userID,
			fmt.Sprintf("您的存储空间使用已超过 90%%（%d MB / %d MB）。请及时清理或升级。",
				(usedStorage+fileSize)/(1024*1024), totalStorage/(1024*1024)),
			time.Now().Format(time.RFC3339))
	}

	// Apply upload speed limit if configured
	uploadSrc := io.Reader(file)
	if h.limiter != nil {
		uploadSrc = h.limiter.WrapUploadReader(userID, file)
	}

	// Compute SHA256 hash and store via content-addressable path (objects/{hash[:2]}/{hash[2:]})
	fileHash, storagePath, err := h.storage.StoreHash(uploadSrc)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "storage failed")
		return
	}

	// Dedup: check file_fingerprints table
	now := time.Now().Format(time.RFC3339)
	var existingID int64
	if err := h.db.QueryRow(`SELECT id FROM file_fingerprints WHERE hash = $1`, fileHash).Scan(&existingID); err == nil {
		// File content already exists — increment reference count
		h.db.Exec(`UPDATE file_fingerprints SET reference_count = reference_count + 1, updated_at = $1 WHERE id = $2`, now, existingID)
	} else {
		// New content — create fingerprint record
		h.db.Exec(
			`INSERT INTO file_fingerprints (hash, file_size, mime_type, storage_path, reference_count, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, 1, $5, $5)`,
			fileHash, fileSize, mimeType, storagePath, now,
		)
	}

	// Save file item
	var fileID int64
	err = h.db.QueryRow(
		`INSERT INTO file_items (user_id, folder_id, filename, original_filename, file_path, file_size,
		                         mime_type, file_type, storage_driver, file_hash, is_folder, deleted_at, description, file_url, duration, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'local', $9, false, NULL, NULL, '', 0, $10, $10) RETURNING id`,
		userID, folderID, filename, filename, storagePath, fileSize, mimeType, fileType, fileHash, now,
	).Scan(&fileID)

	if err != nil {
		log.Printf("[UploadFile] INSERT error: %v", err)
		utils.WriteError(w, http.StatusInternalServerError, "database insert failed")
		return
	}

	// Update user storage quota
	h.db.Exec(`UPDATE users SET used_storage = used_storage + $1 WHERE id = $2`, fileSize, userID)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":        fileID,
		"filename":  filename,
		"file_size": fileSize,
		"file_type": fileType,
	})
	// Trigger webhook
	TriggerWebhooks(h.db, "file.create", map[string]interface{}{
		"event": "file.create", "file_id": fileID,
		"user_id": userID, "filename": filename,
	})
}

// DeleteFile soft-deletes a file.
func (h *Handler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	now := time.Now().Format(time.RFC3339)
	// Get file size before soft-delete so we can subtract from quota
	var deletedSize int64
	h.db.QueryRow(`SELECT file_size FROM file_items WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, fileID, userID).Scan(&deletedSize)
	result, err := h.db.Exec(
		`UPDATE file_items SET deleted_at = $1 WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL`,
		now, fileID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	// Subtract from user storage quota
	if deletedSize > 0 {
		h.db.Exec(`UPDATE users SET used_storage = GREATEST(0, used_storage - $1) WHERE id = $2`, deletedSize, userID)
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// CopyFileToVault copies or moves a file from the file area to a vault (private space).
// POST /files/{id}/to-vault  { vault_id, action: "copy"|"move" }
func (h *Handler) CopyFileToVault(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		VaultID int64  `json:"vault_id"`
		Action  string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.VaultID == 0 {
		writeError(w, http.StatusBadRequest, "vault_id required")
		return
	}
	if req.Action != "copy" && req.Action != "move" {
		writeError(w, http.StatusBadRequest, "action must be 'copy' or 'move'")
		return
	}

	var filename, filePath, mimeType string
	var fileSize int64
	err := h.db.QueryRow(
		`SELECT filename, file_path, file_size, COALESCE(mime_type, '') FROM file_items WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		fileID, userID,
	).Scan(&filename, &filePath, &fileSize, &mimeType)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	var vaultOwnerID int64
	err = h.db.QueryRow(`SELECT user_id FROM vaults WHERE id = $1`, req.VaultID).Scan(&vaultOwnerID)
	if err != nil || vaultOwnerID != userID {
		writeError(w, http.StatusNotFound, "vault not found")
		return
	}

	reader, err := h.storage.Retrieve(filePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read file")
		return
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read file content")
		return
	}

	encrypted, err := encrypt(base64.StdEncoding.EncodeToString(content))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encryption failed")
		return
	}

	vaultStorageDir := filepath.Join(h.vaultDir, fmt.Sprintf("%d", req.VaultID))
	if err := os.MkdirAll(vaultStorageDir, 0700); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create vault dir")
		return
	}
	itemUUID := uuid.New().String()
	vaultFilePath := filepath.Join(vaultStorageDir, itemUUID+".enc")
	if err := os.WriteFile(vaultFilePath, []byte(encrypted), 0600); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write vault file")
		return
	}

	now := time.Now().Format(time.RFC3339)
	var vaultItemID int64
	err = h.db.QueryRow(
		`INSERT INTO vault_items (vault_id, user_id, name, filename, file_size, mime_type, file_path, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8) RETURNING id`,
		req.VaultID, userID, filename, filename, fileSize, mimeType, vaultFilePath, now,
	).Scan(&vaultItemID)
	if err != nil {
		os.Remove(vaultFilePath)
		writeError(w, http.StatusInternalServerError, "failed to save vault item")
		return
	}

	if req.Action == "move" {
		h.db.Exec(`UPDATE file_items SET deleted_at = $1 WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL`, now, fileID, userID)
		if fileSize > 0 {
			h.db.Exec(`UPDATE users SET used_storage = GREATEST(0, used_storage - $1) WHERE id = $2`, fileSize, userID)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"vault_item_id": vaultItemID,
		"action":        req.Action,
	})
}

// RenameFile renames a file.
func (h *Handler) RenameFile(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename == "" {
		writeError(w, http.StatusBadRequest, "filename required")
		return
	}

	result, err := h.db.Exec(
		`UPDATE file_items SET filename = $1, original_filename = $1, updated_at = $2
		 WHERE id = $3 AND user_id = $4 AND deleted_at IS NULL`,
		req.Filename, time.Now().Format(time.RFC3339), fileID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "rename failed")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "renamed"})
}

// ToggleFavorite toggles the favorite status.
func (h *Handler) ToggleFavorite(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		Favorite bool `json:"favorite"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Default to false, no error needed
		req.Favorite = false
	}

	_, err := h.db.Exec(
		`UPDATE file_items SET is_favorite = $1 WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL`,
		req.Favorite, fileID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"favorite": req.Favorite})
}

// MoveFile moves a file to another folder.
func (h *Handler) MoveFile(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		FolderID *int64 `json:"folder_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Default to nil, no error needed
		req.FolderID = nil
	}

	_, err := h.db.Exec(
		`UPDATE file_items SET folder_id = $1, updated_at = $2 WHERE id = $3 AND user_id = $4 AND deleted_at IS NULL`,
		req.FolderID, time.Now().Format(time.RFC3339), fileID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "move failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "moved"})
}

// UpdateFile partially updates a file (filename, description, is_favorite).
func (h *Handler) UpdateFile(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		Filename    *string `json:"filename"`
		Description *string `json:"description"`
		IsFavorite  *bool   `json:"is_favorite"`
		FileType    *string `json:"file_type"`
		Category    *string `json:"category"`
		SortOrder   *int64  `json:"sort_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	var sets []string
	var args []interface{}
	argIdx := 1

	if req.Filename != nil {
		sets = append(sets, fmt.Sprintf("filename = $%d, original_filename = $%d", argIdx, argIdx))
		args = append(args, *req.Filename)
		argIdx++
	}
	if req.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.IsFavorite != nil {
		sets = append(sets, fmt.Sprintf("is_favorite = $%d", argIdx))
		args = append(args, *req.IsFavorite)
		argIdx++
	}
	if req.FileType != nil {
		sets = append(sets, fmt.Sprintf("file_type = $%d", argIdx))
		args = append(args, *req.FileType)
		argIdx++
	}
	if req.Category != nil {
		sets = append(sets, fmt.Sprintf("category = $%d", argIdx))
		args = append(args, *req.Category)
		argIdx++
	}
	if req.SortOrder != nil {
		sets = append(sets, fmt.Sprintf("sort_order = $%d", argIdx))
		args = append(args, *req.SortOrder)
		argIdx++
	}
	if len(sets) == 0 {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	sets = append(sets, fmt.Sprintf("updated_at = $%d", argIdx))
	args = append(args, time.Now().Format(time.RFC3339))
	argIdx++

	args = append(args, fileID, userID)
	q := fmt.Sprintf("UPDATE file_items SET %s WHERE id = $%d AND user_id = $%d AND deleted_at IS NULL",
		strings.Join(sets, ", "), argIdx, argIdx+1)

	result, err := h.db.Exec(q, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

// UpdateFileContent replaces file content (used by ImageEditor save).
// PUT /files/{id}/content (multipart: content)
func (h *Handler) UpdateFileContent(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	if err := r.ParseMultipartForm(100 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "file too large")
		return
	}
	content, header, err := r.FormFile("content")
	if err != nil {
		writeError(w, http.StatusBadRequest, "content required")
		return
	}
	defer content.Close()

	// Store new version
	fileHash, storagePath, err := h.storage.StoreHash(content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage failed")
		return
	}

	now := time.Now().Format(time.RFC3339)

	// Save old version
	var oldPath string
	h.db.QueryRow(`SELECT file_path FROM file_items WHERE id = $1 AND user_id = $2`, fileID, userID).Scan(&oldPath)
	if oldPath != "" {
		h.db.Exec(`INSERT INTO file_versions (file_id, version_num, file_path, file_size, created_by, created_at)
			SELECT $1, COALESCE((SELECT MAX(version_num) FROM file_versions WHERE file_id = $1), 0) + 1,
			       file_path, file_size, $2, $3 FROM file_items WHERE id = $1`,
			fileID, userID, now)
	}

	// Update file item
	newFilename := header.Filename
	mimeType := header.Header.Get("Content-Type")
	fileType := detectFileType(mimeType, newFilename)
	_, err = h.db.Exec(
		`UPDATE file_items SET file_path = $1, file_hash = $2, file_size = $3,
		 mime_type = $4, file_type = $5, original_filename = $6, updated_at = $7
		 WHERE id = $8 AND user_id = $9`,
		storagePath, fileHash, header.Size, mimeType, fileType, newFilename, now, fileID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "saved"})
}

// ---------- File Comments ----------

func (h *Handler) ListComments(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	rows, err := h.db.Query(
		`SELECT c.id, c.content, c.user_id, COALESCE(u.username, ''), c.created_at
		 FROM file_comments c LEFT JOIN users u ON c.user_id = u.id
		 WHERE c.file_id = $1 AND c.file_id IN (SELECT id FROM file_items WHERE user_id = $2)
		 ORDER BY c.created_at ASC`,
		fileID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	type Comment struct {
		ID        int64  `json:"id"`
		Content   string `json:"content"`
		UserID    int64  `json:"user_id"`
		Username  string `json:"username"`
		CreatedAt string `json:"created_at"`
	}
	var comments = make([]Comment, 0)
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.Content, &c.UserID, &c.Username, &c.CreatedAt); err == nil {
			comments = append(comments, c)
		}
	}
	writeJSON(w, http.StatusOK, comments)
}

func (h *Handler) AddComment(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Content == "" {
		writeError(w, http.StatusBadRequest, "content required")
		return
	}
	now := time.Now().Format(time.RFC3339)
	var commentID int64
	err := h.db.QueryRow(
		`INSERT INTO file_comments (file_id, user_id, content, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $4) RETURNING id`,
		fileID, userID, req.Content, now,
	).Scan(&commentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "add comment failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": commentID})
}

func (h *Handler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	commentID, _ := strconv.ParseInt(chi.URLParam(r, "commentId"), 10, 64)
	result, err := h.db.Exec(
		`DELETE FROM file_comments WHERE id = $1 AND user_id = $2`, commentID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "comment not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// ---------- Folder operations ----------

// ListFolders returns the folder tree.
func (h *Handler) ListFolders(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	rows, err := h.db.Query(
		`SELECT id, parent_id, name FROM folders WHERE user_id = $1 ORDER BY name`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type Folder struct {
		ID       int64  `json:"id"`
		ParentID *int64 `json:"parent_id"`
		Name     string `json:"name"`
		Children []Folder `json:"children,omitempty"`
	}

	var folders = make([]Folder, 0)
	folderMap := make(map[int64]*Folder)

	for rows.Next() {
		var f Folder
		rows.Scan(&f.ID, &f.ParentID, &f.Name)
		folderMap[f.ID] = &f
		if f.ParentID == nil {
			folders = append(folders, f)
		}
	}

	// Build tree (second pass)
	for i := range folderMap {
		f := folderMap[i]
		if f.ParentID != nil {
			if parent, ok := folderMap[*f.ParentID]; ok {
				parent.Children = append(parent.Children, *f)
			}
		}
	}

	writeJSON(w, http.StatusOK, folders)
}

// CreateFolder creates a new folder.
func (h *Handler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		Name     string `json:"name"`
		ParentID *int64 `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}

	now := time.Now().Format(time.RFC3339)
	var folderID int64
	err := h.db.QueryRow(
		`INSERT INTO folders (user_id, parent_id, name, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $4) RETURNING id`,
		userID, req.ParentID, req.Name, now,
	).Scan(&folderID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create failed: %v", err))
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":   folderID,
		"name": req.Name,
	})
}

// DeleteFolder deletes a folder.
func (h *Handler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	folderID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	result, err := h.db.Exec(
		`DELETE FROM folders WHERE id = $1 AND user_id = $2`,
		folderID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "folder not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// UpdateFolder renames a folder
func (h *Handler) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	folderID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}

	result, err := h.db.Exec(
		`UPDATE folders SET name = $1 WHERE id = $2 AND user_id = $3`,
		req.Name, folderID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "folder not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

// GetFolderChildren returns folders and files inside a specific folder, plus breadcrumb path.
func (h *Handler) GetFolderChildren(w http.ResponseWriter, r *http.Request) {
	folderID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	// Build breadcrumb path
	type PathItem struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	path := []PathItem{{ID: 0, Name: "我的文件"}}
	var walkID int64 = folderID
	var names = make(map[int64]string)
	var parents = make(map[int64]int64)

	rows, err := h.db.Query(`SELECT id, COALESCE(parent_id,0), name FROM folders WHERE user_id = $1`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, pid int64
		var name string
		rows.Scan(&id, &pid, &name)
		names[id] = name
		parents[id] = pid
	}

	// Walk up to build path (reverse order)
	var rev []PathItem
	for walkID > 0 {
		if n, ok := names[walkID]; ok {
			rev = append(rev, PathItem{ID: walkID, Name: n})
			walkID = parents[walkID]
		} else {
			break
		}
	}
	for i := len(rev) - 1; i >= 0; i-- {
		path = append(path, rev[i])
	}

	// Fetch child folders
	type FolderItem struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		ParentID *int64 `json:"parent_id"`
	}
	var folders []FolderItem
	frows, err := h.db.Query(`SELECT id, parent_id, name FROM folders WHERE user_id = $1 AND parent_id = $2 ORDER BY name`, userID, folderID)
	if err == nil {
		defer frows.Close()
		for frows.Next() {
			var f FolderItem
			frows.Scan(&f.ID, &f.ParentID, &f.Name)
			folders = append(folders, f)
		}
	}

	// Fetch child files
	type FileItem struct {
		ID        int64   `json:"id"`
		Filename  *string `json:"filename"`
		FileSize  *int64  `json:"file_size"`
		MimeType  *string `json:"mime_type"`
		FileType  *string `json:"file_type"`
		IsFav     *bool   `json:"is_favorite"`
		ThumbURL  *string `json:"thumbnail_url"`
		FolderID  *int64  `json:"folder_id"`
		CreatedAt *string `json:"created_at"`
		UpdatedAt *string `json:"updated_at"`
	}
	var files = make([]FileItem, 0)
	frows2, err := h.db.Query(
		`SELECT id, filename, file_size, mime_type, file_type, is_favorite, thumbnail_url, folder_id, created_at, updated_at
		 FROM file_items WHERE user_id = $1 AND folder_id = $2 AND deleted_at IS NULL
		 ORDER BY is_folder DESC, created_at DESC`, userID, folderID)
	if err == nil {
		defer frows2.Close()
		for frows2.Next() {
			var f FileItem
			frows2.Scan(&f.ID, &f.Filename, &f.FileSize, &f.MimeType, &f.FileType, &f.IsFav, &f.ThumbURL, &f.FolderID, &f.CreatedAt, &f.UpdatedAt)
			files = append(files, f)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"folders": folders,
		"files":   files,
		"path":    path,
	})
}

// ---------- Tags ----------

func (h *Handler) ListTags(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	// Ensure file_tag_assignments table exists
	h.db.Exec(`CREATE TABLE IF NOT EXISTS file_tag_assignments (
		id SERIAL PRIMARY KEY, file_id INTEGER NOT NULL, tag_id INTEGER NOT NULL,
		UNIQUE(file_id, tag_id)
	)`)

	rows, err := h.db.Query(
		`SELECT t.id, t.name, t.color,
		        COALESCE((SELECT COUNT(*) FROM file_tag_assignments a WHERE a.tag_id = t.id), 0) AS file_count
		 FROM file_tags t WHERE t.user_id = $1 ORDER BY t.name`, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type Tag struct {
		ID        int64   `json:"id"`
		Name      string  `json:"name"`
		Color     *string `json:"color"`
		FileCount int     `json:"file_count"`
	}
	var tags = make([]Tag, 0)
	for rows.Next() {
		var t Tag
		rows.Scan(&t.ID, &t.Name, &t.Color, &t.FileCount)
		tags = append(tags, t)
	}
	writeJSON(w, http.StatusOK, tags)
}

func (h *Handler) CreateTag(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		Name  string  `json:"name"`
		Color *string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}

	var tagID int64
	h.db.QueryRow(
		`INSERT INTO file_tags (user_id, name, color) VALUES ($1, $2, $3) RETURNING id`,
		userID, req.Name, req.Color,
	).Scan(&tagID)

	writeJSON(w, http.StatusCreated, map[string]int64{"id": tagID})
}

func (h *Handler) UpdateTag(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	tagID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	var req struct {
		Name  string  `json:"name"`
		Color *string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	if req.Name != "" {
		_, err := h.db.Exec(`UPDATE file_tags SET name = $1 WHERE id = $2 AND user_id = $3`,
			req.Name, tagID, userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
	}
	if req.Color != nil {
		h.db.Exec(`UPDATE file_tags SET color = $1 WHERE id = $2 AND user_id = $3`,
			*req.Color, tagID, userID)
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

func (h *Handler) DeleteTag(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	tagID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	// Remove assignments first
	h.db.Exec(`DELETE FROM file_tag_assignments WHERE tag_id = $1`, tagID)

	result, err := h.db.Exec(`DELETE FROM file_tags WHERE id = $1 AND user_id = $2`, tagID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// ListFileTags lists tags assigned to a file
func (h *Handler) ListFileTags(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	rows, err := h.db.Query(
		`SELECT t.id, t.name, t.color FROM file_tags t
		 JOIN file_tag_assignments a ON a.tag_id = t.id
		 WHERE a.file_id = $1 AND t.user_id = $2 ORDER BY t.name`,
		fileID, userID,
	)
	if err != nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	defer rows.Close()

	var tags []map[string]interface{}
	for rows.Next() {
		var id int64
		var name string
		var color *string
		rows.Scan(&id, &name, &color)
		tags = append(tags, map[string]interface{}{
			"id": id, "name": name, "color": color,
		})
	}
	if tags == nil {
		tags = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, tags)
}

// AssignFileTags assigns tags to a file (replaces all)
func (h *Handler) AssignFileTags(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		TagIDs []int64 `json:"tag_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	// Verify file belongs to user
	var ownerID int64
	err := h.db.QueryRow(`SELECT user_id FROM file_items WHERE id = $1 AND deleted_at IS NULL`, fileID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	// Replace all tag assignments in a transaction
	tx, err := h.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction failed")
		return
	}
	defer tx.Rollback()

	tx.Exec(`DELETE FROM file_tag_assignments WHERE file_id = $1`, fileID)
	for _, tagID := range req.TagIDs {
		// Verify tag belongs to user
		var tid int64
		if err := tx.QueryRow(`SELECT id FROM file_tags WHERE id = $1 AND user_id = $2`, tagID, userID).Scan(&tid); err != nil {
			continue
		}
		tx.Exec(`INSERT INTO file_tag_assignments (file_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, fileID, tagID)
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "tags updated"})
}

// RemoveFileTag removes a single tag from a file
func (h *Handler) RemoveFileTag(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	tagID, _ := strconv.ParseInt(chi.URLParam(r, "tagId"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	result, err := h.db.Exec(
		`DELETE FROM file_tag_assignments
		 WHERE file_id = $1 AND tag_id = $2
		 AND $1 IN (SELECT id FROM file_items WHERE user_id = $3 AND deleted_at IS NULL)`,
		fileID, tagID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "assignment not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "tag removed"})
}

// ---------- Search ----------

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "query required")
		return
	}

	rows, err := h.db.Query(
		`SELECT id, filename, file_type, file_size, created_at FROM file_items
		 WHERE user_id = $1 AND deleted_at IS NULL AND
		       (filename ILIKE $2 OR original_filename ILIKE $2)
		 ORDER BY created_at DESC LIMIT 50`,
		userID, "%"+q+"%",
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	defer rows.Close()

	type Hit struct {
		ID        int64   `json:"id"`
		Filename  *string `json:"filename"`
		FileType  string  `json:"file_type"`
		FileSize  int64   `json:"file_size"`
		CreatedAt *string `json:"created_at"`
	}

	var hits []Hit
	for rows.Next() {
		var h Hit
		rows.Scan(&h.ID, &h.Filename, &h.FileType, &h.FileSize, &h.CreatedAt)
		hits = append(hits, h)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": hits,
		"query": q,
	})
}

// ---------- Download ----------

func (h *Handler) DownloadFile(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	var filePath, mimeType, filename string
	err := h.db.QueryRow(
		`SELECT file_path, mime_type, COALESCE(original_filename, filename)
		 FROM file_items WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		fileID, userID,
	).Scan(&filePath, &mimeType, &filename)

	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "file not found")
		} else {
			writeError(w, http.StatusInternalServerError, "query failed")
		}
		return
	}

	reader, err := h.storage.Retrieve(filePath)
	if err != nil {
		utils.WriteError(w, http.StatusNotFound, "file not found on storage")
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	limited := h.wrapDownloadReader(userID, reader)
	io.Copy(w, limited)
}

// PreviewFile serves a file for inline preview (optional auth, works for <img> tags).
func (h *Handler) PreviewFile(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	var filePath, mimeType, filename string
	var err error

	// If user is authenticated, check ownership; otherwise serve by ID only
	if userID, ok := middleware.GetUserID(r.Context()); ok {
		err = h.db.QueryRow(
			`SELECT file_path, mime_type, COALESCE(original_filename, filename)
			 FROM file_items WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
			fileID, userID,
		).Scan(&filePath, &mimeType, &filename)
	} else {
		err = h.db.QueryRow(
			`SELECT file_path, mime_type, COALESCE(original_filename, filename)
			 FROM file_items WHERE id = $1 AND deleted_at IS NULL AND is_folder = false`,
			fileID,
		).Scan(&filePath, &mimeType, &filename)
	}

	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	reader, err := h.storage.Retrieve(filePath)
	if err != nil {
		// Debug: log the path we tried
		localDrv, ok := h.storage.(*storage.LocalDriver)
		if ok {
			fullPath := filepath.Join(localDrv.ObjectsPath, filePath)
			utils.WriteError(w, http.StatusNotFound, fmt.Sprintf("file not found at %s", fullPath))
		} else {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found on storage"})
		}
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	// Speed limit if user is authenticated
	if uid, ok := middleware.GetUserID(r.Context()); ok {
		io.Copy(w, h.wrapDownloadReader(uid, reader))
	} else {
		io.Copy(w, reader)
	}
}

// wrapDownloadReader 如果启用了限速，包装 reader 为限速版本
func (h *Handler) wrapDownloadReader(userID int64, reader io.Reader) io.Reader {
	if h.limiter != nil {
		return h.limiter.WrapDownloadReader(userID, reader)
	}
	return reader
}

// ---------- Utilities ----------

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

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	utils.WriteJSON(w, status, data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	utils.WriteError(w, status, msg)
}

// ─── Path Helpers ───

func validateFolderPath(raw string) ([]string, error) {
	cleaned := strings.Trim(raw, "/")
	if len(cleaned) > maxPathLen {
		return nil, fmt.Errorf("path too long")
	}
	if !utf8.ValidString(cleaned) {
		return nil, fmt.Errorf("invalid utf-8")
	}
	if cleaned == "" {
		return nil, nil
	}
	segs := strings.Split(cleaned, "/")
	if len(segs) > maxPathDepth {
		return nil, fmt.Errorf("path too deep")
	}
	for _, s := range segs {
		if s == "" {
			continue
		}
		if s == "." || s == ".." {
			return nil, fmt.Errorf("invalid path segment")
		}
		if len(s) > maxSegLen {
			return nil, fmt.Errorf("segment too long")
		}
		if strings.ContainsAny(s, "\x00/\\") {
			return nil, fmt.Errorf("invalid characters in segment")
		}
	}
	return segs, nil
}

func validateFolderName(name string) error {
	if name == "" || len(name) > maxSegLen {
		return fmt.Errorf("name length invalid")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid folder name")
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("name contains invalid characters")
	}
	if !utf8.ValidString(name) {
		return fmt.Errorf("invalid utf-8")
	}
	return nil
}

// --- Path Cache ---

func (h *Handler) cacheKey(userID int64, path string) string {
	return fmt.Sprintf("%d:%s", userID, path)
}

func (h *Handler) getCachedPath(userID int64, path string) (int64, bool) {
	// L1: in-memory cache
	h.pathCacheMu.RLock()
	entry, ok := h.pathCache[h.cacheKey(userID, path)]
	h.pathCacheMu.RUnlock()
	if ok && !time.Now().After(entry.expiresAt) {
		return entry.folderID, true
	}

	// L2: Redis cache (optional)
	if h.redisCache != nil {
		if fid, ok := h.redisCache.Get(userID, path); ok {
			// Backfill L1
			h.setCachedPath(userID, path, fid)
			return fid, true
		}
	}

	return 0, false
}

func (h *Handler) setCachedPath(userID int64, path string, folderID int64) {
	h.pathCacheMu.Lock()
	h.pathCache[h.cacheKey(userID, path)] = pathCacheEntry{
		folderID:  folderID,
		expiresAt: time.Now().Add(cacheTTL),
	}
	h.pathCacheMu.Unlock()

	// L2: Redis
	if h.redisCache != nil {
		h.redisCache.Set(userID, path, folderID)
	}
}

func (h *Handler) invalidateUserCache(userID int64) {
	h.pathCacheMu.Lock()
	prefix := fmt.Sprintf("%d:", userID)
	for k := range h.pathCache {
		if strings.HasPrefix(k, prefix) {
			delete(h.pathCache, k)
		}
	}
	h.pathCacheMu.Unlock()

	// L2: Redis
	if h.redisCache != nil {
		h.redisCache.InvalidateUser(userID)
	}
}

// --- In-memory folder tree builder ---

type folderNode struct {
	ID       int64
	Name     string
	ParentID int64
}

// loadFolderTree loads all user folders and builds lookup maps.
// Returns: nameOf[id], childrenOf[parentID], roots, error
func (h *Handler) loadFolderTree(userID int64) (map[int64]string, map[int64][]int64, []int64, error) {
	rows, err := h.db.Query(
		`SELECT id, COALESCE(parent_id,0), name FROM folders WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()

	nameOf := make(map[int64]string)
	childrenOf := make(map[int64][]int64)
	var roots []int64

	for rows.Next() {
		var n folderNode
		if err := rows.Scan(&n.ID, &n.ParentID, &n.Name); err != nil {
			return nil, nil, nil, err
		}
		nameOf[n.ID] = n.Name
		if n.ParentID == 0 {
			roots = append(roots, n.ID)
		} else {
			childrenOf[n.ParentID] = append(childrenOf[n.ParentID], n.ID)
		}
	}
	return nameOf, childrenOf, roots, rows.Err()
}

// resolvePath resolves a validated path to a folder ID, using cache when available.
func (h *Handler) resolvePath(userID int64, segments []string, nameOf map[int64]string, childrenOf map[int64][]int64, roots []int64) (int64, error) {
	if len(segments) == 0 {
		return 0, nil // root
	}
	pathStr := strings.Join(segments, "/")
	if cached, ok := h.getCachedPath(userID, pathStr); ok {
		return cached, nil
	}
	curIDs := roots
	var resolvedID int64
	for _, seg := range segments {
		if len(curIDs) == 0 {
			return 0, fmt.Errorf("path not found")
		}
		found := false
		for _, cid := range curIDs {
			if nameOf[cid] == seg {
				resolvedID = cid
				curIDs = childrenOf[cid]
				found = true
				break
			}
		}
		if !found {
			return 0, fmt.Errorf("path not found")
		}
	}
	h.setCachedPath(userID, pathStr, resolvedID)
	return resolvedID, nil
}

// buildChildPath constructs the child's full path from parent path + child name.
func buildChildPath(parentPath, childName string) string {
	if parentPath == "" || parentPath == "/" {
		return "/" + childName
	}
	return strings.TrimRight(parentPath, "/") + "/" + childName
}

// ─── Path-based Handlers ───

// GetFolderChildrenByPath returns folder children resolved by path instead of ID.
// GET /files/folders/by-path?path=文档/设计
func (h *Handler) GetFolderChildrenByPath(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	rawPath := r.URL.Query().Get("path")

	segments, err := validateFolderPath(rawPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	nameOf, childrenOf, roots, err := h.loadFolderTree(userID)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "query failed")
		return
	}

	folderID, err := h.resolvePath(userID, segments, nameOf, childrenOf, roots)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "path not found"})
		return
	}

	// Build breadcrumb from segments
	breadcrumb := []string{"我的文件"}
	for _, seg := range segments {
		breadcrumb = append(breadcrumb, seg)
	}

	// Fetch child folders
	type FolderItem struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Path string `json:"path"`
	}
	var folders = make([]FolderItem, 0)

	if folderID == 0 {
		// Root level
		for _, rid := range roots {
			folders = append(folders, FolderItem{
				ID:   rid,
				Name: nameOf[rid],
				Path: "/" + nameOf[rid],
			})
		}
	} else {
		// Non-root: look up children of this folderID from the tree
		for _, cid := range childrenOf[folderID] {
			folders = append(folders, FolderItem{
				ID:   cid,
				Name: nameOf[cid],
				Path: buildChildPath(strings.Join(segments, "/"), nameOf[cid]),
			})
		}
	}

	// Fetch child files
	type FileItem struct {
		ID        int64   `json:"id"`
		Filename  *string `json:"filename"`
		FileSize  *int64  `json:"file_size"`
		MimeType  *string `json:"mime_type"`
		FileType  *string `json:"file_type"`
		IsFav     *bool   `json:"is_favorite"`
		ThumbURL  *string `json:"thumbnail_url"`
		FolderID  *int64  `json:"folder_id"`
		CreatedAt *string `json:"created_at"`
		UpdatedAt *string `json:"updated_at"`
	}
	var files = make([]FileItem, 0)
	if folderID == 0 {
		rows, err := h.db.Query(
			`SELECT id, filename, file_size, mime_type, file_type, is_favorite, thumbnail_url, folder_id, created_at, updated_at
			 FROM file_items WHERE user_id = $1 AND folder_id IS NULL AND deleted_at IS NULL
			 ORDER BY created_at DESC`, userID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var f FileItem
				rows.Scan(&f.ID, &f.Filename, &f.FileSize, &f.MimeType, &f.FileType, &f.IsFav, &f.ThumbURL, &f.FolderID, &f.CreatedAt, &f.UpdatedAt)
				files = append(files, f)
			}
		}
	} else {
		rows, err := h.db.Query(
			`SELECT id, filename, file_size, mime_type, file_type, is_favorite, thumbnail_url, folder_id, created_at, updated_at
			 FROM file_items WHERE user_id = $1 AND folder_id = $2 AND deleted_at IS NULL
			 ORDER BY created_at DESC`, userID, folderID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var f FileItem
				rows.Scan(&f.ID, &f.Filename, &f.FileSize, &f.MimeType, &f.FileType, &f.IsFav, &f.ThumbURL, &f.FolderID, &f.CreatedAt, &f.UpdatedAt)
				files = append(files, f)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"folders":    folders,
		"files":      files,
		"path":       breadcrumb,
		"folder_id":  folderID,
	})
}

// CreateFolderByPath creates a folder using path-based parent reference.
// POST /files/folders/by-path
func (h *Handler) CreateFolderByPath(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		Name       string `json:"name"`
		ParentPath string `json:"parent_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	if err := validateFolderName(req.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	segments, err := validateFolderPath(req.ParentPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Load and resolve parent path inside a transaction
	tx, err := h.db.Begin()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "transaction failed")
		return
	}
	defer tx.Rollback()

	// Build tree from tx
	rows, err := tx.Query(
		`SELECT id, COALESCE(parent_id,0), name FROM folders WHERE user_id = $1`, userID)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("query failed: %v", err))
		return
	}
	nameOf := make(map[int64]string)
	childrenOf := make(map[int64][]int64)
	var roots []int64
	for rows.Next() {
		var n folderNode
		rows.Scan(&n.ID, &n.ParentID, &n.Name)
		nameOf[n.ID] = n.Name
		if n.ParentID == 0 {
			roots = append(roots, n.ID)
		} else {
			childrenOf[n.ParentID] = append(childrenOf[n.ParentID], n.ID)
		}
	}
	rows.Close()

	parentID, resolveErr := h.resolvePath(userID, segments, nameOf, childrenOf, roots)
	if resolveErr != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "parent path not found"})
		return
	}

	// Verify parent still exists
	var exists int
	if parentID > 0 {
		err = tx.QueryRow(`SELECT 1 FROM folders WHERE id = $1 AND user_id = $2`, parentID, userID).Scan(&exists)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "parent folder no longer exists"})
			return
		}
	}

	now := time.Now().Format(time.RFC3339)
	var newID int64
	var parentPtr *int64
	if parentID > 0 {
		parentPtr = &parentID
	}
	err = tx.QueryRow(
		`INSERT INTO folders (user_id, parent_id, name, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $4) RETURNING id`,
		userID, parentPtr, req.Name, now,
	).Scan(&newID)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("create failed: %v", err))
		return
	}

	if err := tx.Commit(); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("commit failed: %v", err))
		return
	}

	// Invalidate cache
	h.invalidateUserCache(userID)

	childPath := buildChildPath(req.ParentPath, req.Name)
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":   newID,
		"name": req.Name,
		"path": childPath,
	})
}
