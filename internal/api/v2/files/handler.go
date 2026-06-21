package files

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	tempDir string
}

func NewHandler(db *sql.DB, store storage.Driver, tempDir string) *Handler {
	return &Handler{db: db, storage: store, tempDir: tempDir}
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

	// Sort
	if sortBy == "" {
		sortBy = "created_at"
	}
	if sortOrder == "" {
		sortOrder = "desc"
	}
	allowedSorts := map[string]bool{"created_at": true, "filename": true, "file_size": true, "updated_at": true}
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
		        thumbnail_url, width, height, duration, folder_id, created_at, updated_at
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
		ID        int64   `json:"id"`
		Filename  *string `json:"filename"`
		OrigName  *string `json:"original_filename"`
		FileSize  int64   `json:"file_size"`
		MimeType  *string `json:"mime_type"`
		FileType  string  `json:"file_type"`
		IsFolder  bool    `json:"is_folder"`
		IsFav     bool    `json:"is_favorite"`
		ThumbURL  *string `json:"thumbnail_url"`
		Width     *int    `json:"width"`
		Height    *int    `json:"height"`
		Duration  *int    `json:"duration"`
		FolderID  *int64  `json:"folder_id"`
		CreatedAt *string `json:"created_at"`
		UpdatedAt *string `json:"updated_at"`
	}

	var items []FileItem
	for rows.Next() {
		var item FileItem
		rows.Scan(&item.ID, &item.Filename, &item.OrigName, &item.FileSize, &item.MimeType,
			&item.FileType, &item.IsFolder, &item.IsFav, &item.ThumbURL, &item.Width,
			&item.Height, &item.Duration, &item.FolderID, &item.CreatedAt, &item.UpdatedAt)
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
		ID        int64   `json:"id"`
		Filename  *string `json:"filename"`
		OrigName  *string `json:"original_filename"`
		FileSize  int64   `json:"file_size"`
		MimeType  *string `json:"mime_type"`
		FileType  string  `json:"file_type"`
		IsFolder  bool    `json:"is_folder"`
		IsFav     bool    `json:"is_favorite"`
		FilePath  *string `json:"-"`
		FileURL   *string `json:"file_url"`
		ThumbURL  *string `json:"thumbnail_url"`
		Width     *int    `json:"width"`
		Height    *int    `json:"height"`
		Duration  *int    `json:"duration"`
		FolderID  *int64  `json:"folder_id"`
		CreatedAt *string `json:"created_at"`
		UpdatedAt *string `json:"updated_at"`
		Desc      *string `json:"description"`
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
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
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
	fileSize := header.Size
	mimeType := header.Header.Get("Content-Type")
	fileType := detectFileType(mimeType, filename)

	// Generate storage path
	ext := filepath.Ext(filename)
	storedName := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), strings.TrimSuffix(filename, ext), ext)
	storagePath := fmt.Sprintf("uploads/%d/%s/%s", userID, time.Now().Format("2006/01"), storedName)

	// Store file
	if err := h.storage.Store(storagePath, file); err != nil {
		http.Error(w, `{"error":"storage failed"}`, http.StatusInternalServerError)
		return
	}

	// Save to database
	now := time.Now().Format(time.RFC3339)
	var fileID int64
	err = h.db.QueryRow(
		`INSERT INTO file_items (user_id, folder_id, filename, original_filename, file_path, file_size,
		                         mime_type, file_type, storage_driver, is_folder, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'local', false, $9, $9) RETURNING id`,
		userID, folderID, filename, filename, storagePath, fileSize, mimeType, fileType, now,
	).Scan(&fileID)

	if err != nil {
		h.storage.Delete(storagePath)
		http.Error(w, `{"error":"database insert failed"}`, http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":       fileID,
		"filename": filename,
		"file_size": fileSize,
		"file_type": fileType,
	})
}

// DeleteFile soft-deletes a file.
func (h *Handler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	now := time.Now().Format(time.RFC3339)
	result, err := h.db.Exec(
		`UPDATE file_items SET deleted_at = $1 WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL`,
		now, fileID, userID,
	)
	if err != nil {
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// RenameFile renames a file.
func (h *Handler) RenameFile(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename == "" {
		http.Error(w, `{"error":"filename required"}`, http.StatusBadRequest)
		return
	}

	result, err := h.db.Exec(
		`UPDATE file_items SET filename = $1, original_filename = $1, updated_at = $2
		 WHERE id = $3 AND user_id = $4 AND deleted_at IS NULL`,
		req.Filename, time.Now().Format(time.RFC3339), fileID, userID,
	)
	if err != nil {
		http.Error(w, `{"error":"rename failed"}`, http.StatusInternalServerError)
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
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
	json.NewDecoder(r.Body).Decode(&req)

	_, err := h.db.Exec(
		`UPDATE file_items SET is_favorite = $1 WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL`,
		req.Favorite, fileID, userID,
	)
	if err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
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
	json.NewDecoder(r.Body).Decode(&req)

	_, err := h.db.Exec(
		`UPDATE file_items SET folder_id = $1, updated_at = $2 WHERE id = $3 AND user_id = $4 AND deleted_at IS NULL`,
		req.FolderID, time.Now().Format(time.RFC3339), fileID, userID,
	)
	if err != nil {
		http.Error(w, `{"error":"move failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "moved"})
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
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Folder struct {
		ID       int64  `json:"id"`
		ParentID *int64 `json:"parent_id"`
		Name     string `json:"name"`
		Children []Folder `json:"children,omitempty"`
	}

	var folders []Folder
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
		http.Error(w, `{"error":"name required"}`, http.StatusBadRequest)
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
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
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
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		http.Error(w, `{"error":"folder not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// ---------- Tags ----------

func (h *Handler) ListTags(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	rows, err := h.db.Query(
		`SELECT id, name, color FROM file_tags WHERE user_id = $1 ORDER BY name`, userID,
	)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Tag struct {
		ID    int64   `json:"id"`
		Name  string  `json:"name"`
		Color *string `json:"color"`
	}
	var tags []Tag
	for rows.Next() {
		var t Tag
		rows.Scan(&t.ID, &t.Name, &t.Color)
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
	json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		http.Error(w, `{"error":"name required"}`, http.StatusBadRequest)
		return
	}

	var tagID int64
	h.db.QueryRow(
		`INSERT INTO file_tags (user_id, name, color) VALUES ($1, $2, $3) RETURNING id`,
		userID, req.Name, req.Color,
	).Scan(&tagID)

	writeJSON(w, http.StatusCreated, map[string]int64{"id": tagID})
}

// ---------- Search ----------

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, `{"error":"query required"}`, http.StatusBadRequest)
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
		http.Error(w, `{"error":"search failed"}`, http.StatusInternalServerError)
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
			http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		} else {
			http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		}
		return
	}

	reader, err := h.storage.Retrieve(filePath)
	if err != nil {
		utils.WriteError(w, http.StatusNotFound, "file not found on storage")
		return
	}
	defer reader.Close()

	// Get file info for Content-Length
	info, err := os.Stat(filepath.Join("storage", filePath))
	if err == nil {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	io.Copy(w, reader)
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
