package wopi

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Athenavi/Stora/pkg/storage"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	db      *sql.DB
	storage storage.Driver
}

func NewHandler(db *sql.DB, store storage.Driver) *Handler {
	return &Handler{db: db, storage: store}
}

// CheckFileInfo returns file metadata for Collabora Online.
// GET /api/v2/wopi/files/{fileId}
func (h *Handler) CheckFileInfo(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "fileId"), 10, 64)

	var filename, mimeType string
	var fileSize int64
	var isFolder bool
	err := h.db.QueryRow(
		`SELECT COALESCE(filename, ''), COALESCE(mime_type, 'application/octet-stream'),
		        COALESCE(file_size, 0), is_folder
		 FROM file_items WHERE id = $1 AND deleted_at IS NULL`,
		fileID,
	).Scan(&filename, &mimeType, &fileSize, &isFolder)
	if err != nil {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}

	info := map[string]interface{}{
		"BaseFileName":      filename,
		"OwnerId":           "stora",
		"Size":              fileSize,
		"Version":           "1.0",
		"SupportsUpdate":    true,
		"SupportsRename":    false,
		"UserCanWrite":      true,
		"UserFriendlyName":  "Stora User",
		"BreadcrumbDocName": filename,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// GetFile serves file content to Collabora.
// GET /api/v2/wopi/files/{fileId}/contents
func (h *Handler) GetFile(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "fileId"), 10, 64)

	var filePath, mimeType, filename string
	err := h.db.QueryRow(
		`SELECT file_path, COALESCE(mime_type, 'application/octet-stream'),
		        COALESCE(filename, '') FROM file_items WHERE id = $1 AND deleted_at IS NULL`,
		fileID,
	).Scan(&filePath, &mimeType, &filename)
	if err != nil {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}

	reader, err := h.storage.Retrieve(filePath)
	if err != nil {
		http.Error(w, `{"error":"storage error"}`, http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	io.Copy(w, reader)
}

// PutFile saves edited file content from Collabora.
// POST /api/v2/wopi/files/{fileId}/contents
func (h *Handler) PutFile(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "fileId"), 10, 64)

	// Save new version
	fileHash, storagePath, err := h.storage.StoreHash(r.Body)
	if err != nil {
		http.Error(w, `{"error":"storage failed"}`, http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	now := time.Now().Format(time.RFC3339)
	var oldPath string
	h.db.QueryRow(`SELECT file_path FROM file_items WHERE id = $1`, fileID).Scan(&oldPath)
	if oldPath != "" {
		h.db.Exec(`INSERT INTO file_versions (file_id, version_num, file_path, file_size, created_by, created_at)
			SELECT $1, COALESCE((SELECT MAX(version_num) FROM file_versions WHERE file_id = $1), 0) + 1,
			       file_path, file_size, 0, $2 FROM file_items WHERE id = $1`,
			fileID, now)
	}

	_, err = h.db.Exec(
		`UPDATE file_items SET file_path = $1, file_hash = $2, updated_at = $3 WHERE id = $4`,
		storagePath, fileHash, now, fileID,
	)
	if err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "saved"})
}
