package wopi

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	fileapi "github.com/Athenavi/Stora/internal/api/v2/files"
	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/Athenavi/Stora/pkg/auth"
	"github.com/Athenavi/Stora/pkg/storage"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	db         *sql.DB
	storage    storage.Driver
	jwtManager *auth.JWTManager
}

func NewHandler(db *sql.DB, store storage.Driver, jwtManager *auth.JWTManager) *Handler {
	return &Handler{db: db, storage: store, jwtManager: jwtManager}
}

// wopiUserID resolves the caller identity from the WOPI access token.
// Collabora passes it as ?access_token= (the WOPI standard) or as a bearer
// header. The token is an ordinary Stora JWT minted by GetWopiAccess, so it
// carries the same signature and expiry guarantees as every other API call.
func (h *Handler) wopiUserID(r *http.Request) (int64, bool) {
	tok := r.URL.Query().Get("access_token")
	if tok == "" {
		if hdr := r.Header.Get("Authorization"); hdr != "" {
			if parts := strings.SplitN(hdr, " ", 2); len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
				tok = parts[1]
			}
		}
	}
	if tok == "" {
		return 0, false
	}
	claims, err := h.jwtManager.ValidateToken(tok)
	if err != nil {
		return 0, false
	}
	return claims.UserID, true
}

// CheckFileInfo returns file metadata for Collabora Online.
// GET /api/v2/wopi/files/{fileId}
func (h *Handler) CheckFileInfo(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.wopiUserID(r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "fileId"), 10, 64)

	var filename, mimeType string
	var fileSize int64
	var isFolder bool
	err := h.db.QueryRow(
		`SELECT COALESCE(filename, ''), COALESCE(mime_type, 'application/octet-stream'),
		        COALESCE(file_size, 0), is_folder
		 FROM file_items WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		fileID, userID,
	).Scan(&filename, &mimeType, &fileSize, &isFolder)
	if err != nil {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}

	info := map[string]interface{}{
		"BaseFileName":      filename,
		"OwnerId":           strconv.FormatInt(userID, 10),
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
	userID, ok := h.wopiUserID(r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "fileId"), 10, 64)

	var filePath, mimeType, filename string
	err := h.db.QueryRow(
		`SELECT file_path, COALESCE(mime_type, 'application/octet-stream'),
		        COALESCE(filename, '') FROM file_items
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		fileID, userID,
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
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.WriteHeader(http.StatusOK)
	io.Copy(w, reader)
}

// PutFile saves edited file content from Collabora.
// POST /api/v2/wopi/files/{fileId}/contents
func (h *Handler) PutFile(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.wopiUserID(r)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "fileId"), 10, 64)

	// The token must be for this very file's owner, otherwise anyone holding any
	// valid token could overwrite arbitrary files.
	var ownerID int64
	if err := h.db.QueryRow(
		`SELECT user_id FROM file_items WHERE id = $1 AND is_folder = false`,
		fileID,
	).Scan(&ownerID); err != nil || ownerID != userID {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}

	// Save new version
	fileHash, storagePath, err := h.storage.StoreHash(r.Body)
	if err != nil {
		http.Error(w, `{"error":"storage failed"}`, http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	now := time.Now().Format(time.RFC3339)
	var oldPath, oldHash, oldDriver string
	h.db.QueryRow(`SELECT file_path, file_hash, storage_driver FROM file_items WHERE id = $1`, fileID).Scan(&oldPath, &oldHash, &oldDriver)
	if oldPath != "" {
		h.db.Exec(`INSERT INTO file_versions (file_id, version_num, file_path, file_size, file_hash, storage_driver, created_by, created_at)
			SELECT $1, COALESCE((SELECT MAX(version_num) FROM file_versions WHERE file_id = $1), 0) + 1,
			       file_path, file_size, file_hash, storage_driver, created_by, $2 FROM file_items WHERE id = $1`,
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

	// Edited in Office — the thumbnail no longer reflects the content.
	fileapi.EnqueueThumbnail(h.db, fileID)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "saved"})
}

// GetWopiAccess returns the Collabora Online editor URL for a file.
// GET /api/v2/wopi/access/{fileId} — requires an authenticated Stora session.
func (h *Handler) GetWopiAccess(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "fileId"), 10, 64)
	if fileID == 0 {
		http.Error(w, `{"error":"invalid file id"}`, http.StatusBadRequest)
		return
	}

	// Only the owner may ask for an editor token for a file.
	var username string
	if err := h.db.QueryRow(
		`SELECT COALESCE(u.username, '') FROM file_items f JOIN users u ON u.id = f.user_id
		 WHERE f.id = $1 AND f.user_id = $2 AND f.deleted_at IS NULL AND f.is_folder = false`,
		fileID, userID,
	).Scan(&username); err != nil {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}

	tokens, err := h.jwtManager.GenerateTokens(userID, username, false)
	if err != nil {
		http.Error(w, `{"error":"token generation failed"}`, http.StatusInternalServerError)
		return
	}

	// Build Collabora WOPI URL
	// Collabora expects: http://<collabora>/cool/wopi/files/<fileId>?access_token=<token>
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	wopiSrc := fmt.Sprintf("%s://%s/api/v2/wopi/files/%d", scheme, r.Host, fileID)

	// Default Collabora URL; users can override via env
	collaboraURL := "http://localhost:9980"
	if env := os.Getenv("COLLABORA_URL"); env != "" {
		collaboraURL = env
	}

	editorURL := fmt.Sprintf("%s/cool/wopi/files/%d?access_token=%s&WOPISrc=%s",
		collaboraURL, fileID, url.QueryEscape(tokens.AccessToken), url.QueryEscape(wopiSrc))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"editor_url": editorURL,
	})
}
