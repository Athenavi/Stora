package files

import (
	"archive/zip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/Athenavi/Stora/pkg/auth"
	"github.com/Athenavi/Stora/pkg/storage"
	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
)

type VaultHandler struct {
	db *sql.DB
}

func NewVaultHandler(db *sql.DB) *VaultHandler {
	return &VaultHandler{db: db}
}

// ─── Helpers ───

var vaultTokenStore = make(map[string]struct{ VaultID int64; UserID int64 })

func issueVaultToken(vaultID, userID int64) string {
	b := make([]byte, 16)
	rand.Read(b)
	tok := hex.EncodeToString(b)
	vaultTokenStore[tok] = struct{ VaultID int64; UserID int64 }{vaultID, userID}
	return tok
}

func validateVaultToken(tok string) (int64, int64, bool) {
	e, ok := vaultTokenStore[tok]
	return e.VaultID, e.UserID, ok
}

// requireVaultToken validates the vault session token for the given vault.
// The token is obtained via VerifyVaultPassword. This ensures vault access
// requires both JWT authentication AND vault password verification.
// Ponytail: we validate the token exists, but the caller's SQL still checks
// user_id ownership against the JWT — defense in depth.
func (h *VaultHandler) requireVaultToken(w http.ResponseWriter, r *http.Request, expectedVaultID int64) bool {
	tok := r.URL.Query().Get("vault_token")
	if tok == "" {
		tok = r.Header.Get("X-Vault-Token")
	}
	if tok == "" {
		writeError(w, http.StatusForbidden, "vault_token required — call /vaults/{id}/verify-password first")
		return false
	}
	vaultID, _, ok := validateVaultToken(tok)
	if !ok || vaultID != expectedVaultID {
		writeError(w, http.StatusForbidden, "invalid or expired vault token")
		return false
	}
	return true
}

// ---------- Vault ----------

func (h *VaultHandler) ListVaults(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	rows, err := h.db.Query(
		`SELECT v.id, v.name, v.description, v.created_at,
		        COALESCE((SELECT COUNT(*) FROM vault_items WHERE vault_id = v.id), 0) AS file_count,
		        COALESCE((SELECT COALESCE(SUM(file_size), 0) FROM vault_items WHERE vault_id = v.id), 0) AS total_size,
		        CASE WHEN v.password_hash IS NOT NULL AND v.password_hash != '' THEN true ELSE false END AS has_password
		 FROM vaults v WHERE v.user_id = $1 ORDER BY v.name`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type Vault struct {
		ID          int64   `json:"id"`
		Name        string  `json:"name"`
		Description *string `json:"description"`
		FileCount   int     `json:"file_count"`
		TotalSize   int64   `json:"total_size"`
		LockTimeout int     `json:"lock_timeout"`
		HasPassword bool    `json:"has_password"`
		CreatedAt   *string `json:"created_at"`
	}
	var vaults = make([]Vault, 0)
	for rows.Next() {
		var v Vault
		v.LockTimeout = 30 // default auto-lock minutes
		if err := rows.Scan(&v.ID, &v.Name, &v.Description, &v.FileCount, &v.TotalSize, &v.HasPassword, &v.CreatedAt); err != nil {
			continue
		}
		vaults = append(vaults, v)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[Vault] ListVaults row iteration error: %v", err)
	}
	writeJSON(w, http.StatusOK, vaults)
}

func (h *VaultHandler) CreateVault(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	// Accept both FormData (frontend) and JSON
	var name string
	var password string

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "invalid form")
			return
		}
		name = r.FormValue("name")
		password = r.FormValue("password")
	} else {
		var req struct {
			Name     string `json:"name"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		name = req.Name
		password = req.Password
	}

	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if password == "" {
		writeError(w, http.StatusBadRequest, "password required")
		return
	}

	// Ensure columns exist
	EnsureVaultCompat(h.db)

	pwHash, err := hashPassword(password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password hash failed")
		return
	}
	now := time.Now().Format(time.RFC3339)
	var vaultID int64
	err = h.db.QueryRow(
		`INSERT INTO vaults (user_id, name, password_hash, created_at, updated_at) VALUES ($1,$2,$3,$4,$4) RETURNING id`,
		userID, name, pwHash, now,
	).Scan(&vaultID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	token := issueVaultToken(vaultID, userID)
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":    vaultID,
		"name":  name,
		"token": token,
	})
}

func (h *VaultHandler) VerifyVaultPassword(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	vaultID, _ := strconv.ParseInt(chi.URLParam(r, "vaultId"), 10, 64)

	// Accept both FormData and query param
	pw := r.URL.Query().Get("password")
	if pw == "" {
		ct := r.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "multipart/form-data") {
			r.ParseMultipartForm(32 << 20)
			pw = r.FormValue("password")
		} else if strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
			r.ParseForm()
			pw = r.FormValue("password")
		}
	}

	if pw == "" {
		writeError(w, http.StatusBadRequest, "password required")
		return
	}

	var storedHash string
	err := h.db.QueryRow(
		`SELECT COALESCE(password_hash, '') FROM vaults WHERE id = $1 AND user_id = $2`,
		vaultID, userID,
	).Scan(&storedHash)

	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "vault not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	if storedHash == "" || !checkPassword(pw, storedHash) {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	token := issueVaultToken(vaultID, userID)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":      token,
		"vault_id":   vaultID,
	})
}

func (h *VaultHandler) DeleteVault(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	vaultID, _ := strconv.ParseInt(chi.URLParam(r, "vaultId"), 10, 64)

	// Delete items first
	h.db.Exec(`DELETE FROM vault_items WHERE vault_id = $1`, vaultID)
	result, err := h.db.Exec(`DELETE FROM vaults WHERE id = $1 AND user_id = $2`, vaultID, userID)
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

// ---------- Vault Items ----------

func (h *VaultHandler) ListVaultItems(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	vaultID, _ := strconv.ParseInt(chi.URLParam(r, "vaultId"), 10, 64)

	if !h.requireVaultToken(w, r, vaultID) {
		return
	}

	rows, err := h.db.Query(
		`SELECT id, COALESCE(filename, name), COALESCE(file_size, 0), COALESCE(mime_type, 'application/octet-stream'), created_at
		 FROM vault_items WHERE vault_id = $1 AND user_id = $2 ORDER BY created_at DESC`,
		vaultID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type Item struct {
		ID        int64  `json:"id"`
		Filename  string `json:"filename"`
		FileSize  int64  `json:"file_size"`
		MimeType  string `json:"mime_type"`
		CreatedAt string `json:"created_at"`
	}
	var items = make([]Item, 0)
	for rows.Next() {
		var it Item
		rows.Scan(&it.ID, &it.Filename, &it.FileSize, &it.MimeType, &it.CreatedAt)
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *VaultHandler) UploadVaultItem(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	vaultID, _ := strconv.ParseInt(chi.URLParam(r, "vaultId"), 10, 64)

	if !h.requireVaultToken(w, r, vaultID) {
		return
	}

	if err := r.ParseMultipartForm(100 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "form parse failed")
		return
	}

	filename := r.FormValue("filename")
	fileSizeStr := r.FormValue("file_size")
	mimeType := r.FormValue("mime_type")
	fileContentB64 := r.FormValue("file_content")

	if filename == "" || fileContentB64 == "" {
		writeError(w, http.StatusBadRequest, "filename and file_content required")
		return
	}

	fileSize, _ := strconv.ParseInt(fileSizeStr, 10, 64)
	if fileSize <= 0 {
		fileSize = int64(len(fileContentB64))
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// Encrypt the base64 content
	encrypted, err := encrypt(fileContentB64)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encryption failed")
		return
	}

	EnsureVaultCompat(h.db)

	now := time.Now().Format(time.RFC3339)
	var itemID int64
	err = h.db.QueryRow(
		`INSERT INTO vault_items (vault_id, user_id, name, filename, file_size, mime_type, content, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8) RETURNING id`,
		vaultID, userID, filename, filename, fileSize, mimeType, encrypted, now,
	).Scan(&itemID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, "insert failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]int64{"id": itemID})
}

func (h *VaultHandler) DownloadVaultItem(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	vaultID, _ := strconv.ParseInt(chi.URLParam(r, "vaultId"), 10, 64)
	itemID, _ := strconv.ParseInt(chi.URLParam(r, "itemId"), 10, 64)

	if !h.requireVaultToken(w, r, vaultID) {
		return
	}

	var filename, mimeType, encrypted string
	var fileSize int64
	err := h.db.QueryRow(
		`SELECT COALESCE(filename, name), COALESCE(mime_type, 'application/octet-stream'),
		        COALESCE(content, ''), COALESCE(file_size, 0)
		 FROM vault_items WHERE id = $1 AND vault_id = $2 AND user_id = $3`,
		itemID, vaultID, userID,
	).Scan(&filename, &mimeType, &encrypted, &fileSize)

	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	// Decrypt
	decoded, err := decrypt(encrypted)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "decryption failed")
		return
	}

	raw, err := base64.StdEncoding.DecodeString(decoded)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "base64 decode failed")
		return
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	w.WriteHeader(http.StatusOK)
	w.Write(raw)
}

func (h *VaultHandler) DeleteVaultItem(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	vaultID, _ := strconv.ParseInt(chi.URLParam(r, "vaultId"), 10, 64)
	itemID, _ := strconv.ParseInt(chi.URLParam(r, "itemId"), 10, 64)

	if !h.requireVaultToken(w, r, vaultID) {
		return
	}

	result, err := h.db.Exec(
		`DELETE FROM vault_items WHERE id = $1 AND vault_id = $2 AND user_id = $3`, itemID, vaultID, userID,
	)
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

// EnsureVaultCompat adds missing columns for the vault implementation.
func EnsureVaultCompat(db *sql.DB) {
	for _, m := range []string{
		`ALTER TABLE vaults ADD COLUMN IF NOT EXISTS password_hash VARCHAR(128) DEFAULT ''`,
		`ALTER TABLE vault_items ADD COLUMN IF NOT EXISTS filename VARCHAR(512) DEFAULT ''`,
		`ALTER TABLE vault_items ADD COLUMN IF NOT EXISTS file_size BIGINT DEFAULT 0`,
		`ALTER TABLE vault_items ADD COLUMN IF NOT EXISTS mime_type VARCHAR(128) DEFAULT ''`,
		`ALTER TABLE vault_items ADD COLUMN IF NOT EXISTS content_type VARCHAR(64) DEFAULT 'file'`,
	} {
		db.Exec(m)
	}
}

// ---------- Transcoding ----------

type TranscodeHandler struct {
	db      *sql.DB
	storage storage.Driver
}

func NewTranscodeHandler(db *sql.DB, store storage.Driver) *TranscodeHandler {
	return &TranscodeHandler{db: db, storage: store}
}

func (h *TranscodeHandler) StartTranscode(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	var req struct {
		TargetFormat string `json:"target_format"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.TargetFormat == "" {
		req.TargetFormat = "mp4"
	}

	now := time.Now().Format(time.RFC3339)
	var taskID int64
	h.db.QueryRow(
		`INSERT INTO transcode_tasks (file_id, user_id, status, target_format, created_at, updated_at)
		 VALUES ($1, $2, 'pending', $3, $4, $4) RETURNING id`,
		fileID, userID, req.TargetFormat, now,
	).Scan(&taskID)

	// Start background transcode
	go h.processTranscode(taskID, fileID, userID, req.TargetFormat)

	writeJSON(w, http.StatusCreated, map[string]int64{"task_id": taskID})
}

func (h *TranscodeHandler) processTranscode(taskID, fileID, userID int64, targetFormat string) {
	db := h.db
	now := time.Now().Format(time.RFC3339)
	db.Exec(`UPDATE transcode_tasks SET status = 'processing', updated_at = $1 WHERE id = $2`, now, taskID)

	// Get file path
	var filePath, origFilename string
	err := db.QueryRow(
		`SELECT file_path, COALESCE(original_filename, filename) FROM file_items WHERE id = $1 AND user_id = $2`,
		fileID, userID,
	).Scan(&filePath, &origFilename)
	if err != nil {
		db.Exec(`UPDATE transcode_tasks SET status = 'failed', error_msg = 'file not found', updated_at = $1 WHERE id = $2`, now, taskID)
		return
	}

	// Build ffmpeg command
	outputPath := filePath + "." + targetFormat
	cmd := exec.Command("ffmpeg", "-i", filePath, "-preset", "fast",
		"-c:v", "libx264", "-c:a", "aac",
		"-y", outputPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		db.Exec(`UPDATE transcode_tasks SET status = 'failed', error_msg = $1, updated_at = $2 WHERE id = $3`,
			string(output[:min(len(output), 500)]), now, taskID)
		return
	}

	// Update task
	db.Exec(`UPDATE transcode_tasks SET status = 'completed', output_path = $1, progress = 100, updated_at = $2 WHERE id = $3`,
		outputPath, now, taskID)

	// Create notification
	db.Exec(`INSERT INTO notifications (user_id, type, title, body, created_at)
		VALUES ($1, 'transcode', '转码完成', $2, $3)`,
		userID, fmt.Sprintf("文件 %s 转码为 %s 已完成", origFilename, targetFormat), now)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------- Version Management ----------

type VersionHandler struct {
	db *sql.DB
}

func NewVersionHandler(db *sql.DB) *VersionHandler {
	return &VersionHandler{db: db}
}

func (h *VersionHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	rows, err := h.db.Query(
		`SELECT id, version_num, file_size, created_at FROM file_versions
		 WHERE file_id = $1 AND created_by = $2 ORDER BY version_num DESC`,
		fileID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type Version struct {
		ID        int64   `json:"id"`
		Version   int     `json:"version_num"`
		FileSize  int64   `json:"file_size"`
		CreatedAt *string `json:"created_at"`
	}
	var versions = make([]Version, 0)
	for rows.Next() {
		var v Version
		rows.Scan(&v.ID, &v.Version, &v.FileSize, &v.CreatedAt)
		versions = append(versions, v)
	}
	writeJSON(w, http.StatusOK, versions)
}

// ---------- Batch Operations ----------

type BatchHandler struct {
	db      *sql.DB
	storage storage.Driver
}

func NewBatchHandler(db *sql.DB, store storage.Driver) *BatchHandler {
	return &BatchHandler{db: db, storage: store}
}

func (h *BatchHandler) BatchDelete(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		FileIDs []int64 `json:"file_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.FileIDs) == 0 {
		writeError(w, http.StatusBadRequest, "file_ids required")
		return
	}

	now := time.Now().Format(time.RFC3339)
	result, err := h.db.Exec(
		`UPDATE file_items SET deleted_at = $1 WHERE id = ANY($2) AND user_id = $3 AND deleted_at IS NULL`,
		now, pq.Array(req.FileIDs), userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "batch delete failed")
		return
	}
	n, _ := result.RowsAffected()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"deleted": len(req.FileIDs),
		"affected": n,
	})
}

func (h *BatchHandler) BatchDownload(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		FileIDs []int64 `json:"file_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.FileIDs) == 0 {
		writeError(w, http.StatusBadRequest, "file_ids required")
		return
	}

	// Query file metadata (single query instead of N+1)
	type fileInfo struct {
		FilePath string
		Filename string
		MimeType string
	}
	var files []fileInfo
	rows, err := h.db.Query(
		`SELECT file_path, COALESCE(original_filename, filename), COALESCE(mime_type, 'application/octet-stream')
		 FROM file_items WHERE id = ANY($1) AND user_id = $2 AND deleted_at IS NULL`,
		pq.Array(req.FileIDs), userID,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var fi fileInfo
			if err := rows.Scan(&fi.FilePath, &fi.Filename, &fi.MimeType); err == nil {
				files = append(files, fi)
			}
		}
	}

	if len(files) == 0 {
		writeError(w, http.StatusNotFound, "no files found")
		return
	}

	// Stream ZIP response
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="stora-batch-%d.zip"`, len(files)))
	w.WriteHeader(http.StatusOK)

	zw := zip.NewWriter(w)
	for _, f := range files {
		reader, err := h.storage.Retrieve(f.FilePath)
		if err != nil {
			continue
		}

		hdr := &zip.FileHeader{
			Name:   f.Filename,
			Method: zip.Deflate,
		}
		writer, err := zw.CreateHeader(hdr)
		if err != nil {
			reader.Close()
			continue
		}

		io.Copy(writer, reader)
		reader.Close()
	}
	zw.Close()
}

func (h *BatchHandler) BatchMove(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		FileIDs  []int64 `json:"file_ids"`
		FolderID *int64  `json:"folder_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.FileIDs) == 0 {
		writeError(w, http.StatusBadRequest, "file_ids required")
		return
	}

	now := time.Now().Format(time.RFC3339)
	_, err := h.db.Exec(
		`UPDATE file_items SET folder_id = $1, updated_at = $2 WHERE id = ANY($3) AND user_id = $4`,
		req.FolderID, now, pq.Array(req.FileIDs), userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "batch move failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"moved": len(req.FileIDs)})
}

// ---------- Trash ----------

type TrashHandler struct {
	db *sql.DB
}

func NewTrashHandler(db *sql.DB) *TrashHandler {
	return &TrashHandler{db: db}
}

func (h *TrashHandler) ListTrash(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	rows, err := h.db.Query(
		`SELECT id, filename, file_type, file_size, deleted_at FROM file_items
		 WHERE user_id = $1 AND deleted_at IS NOT NULL ORDER BY deleted_at DESC LIMIT 100`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type TrashItem struct {
		ID        int64   `json:"id"`
		Filename  *string `json:"filename"`
		FileType  string  `json:"file_type"`
		FileSize  int64   `json:"file_size"`
		DeletedAt *string `json:"deleted_at"`
	}
	var items = make([]TrashItem, 0)
	for rows.Next() {
		var t TrashItem
		rows.Scan(&t.ID, &t.Filename, &t.FileType, &t.FileSize, &t.DeletedAt)
		items = append(items, t)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
		"total": len(items),
	})
}

func (h *TrashHandler) RestoreFile(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	result, err := h.db.Exec(
		`UPDATE file_items SET deleted_at = NULL WHERE id = $1 AND user_id = $2 AND deleted_at IS NOT NULL`,
		fileID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "restore failed")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "restored"})
}

func (h *TrashHandler) EmptyTrash(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	h.db.Exec(`DELETE FROM file_items WHERE user_id = $1 AND deleted_at IS NOT NULL`, userID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "trash emptied"})
}

// DestroyFile permanently deletes a trash file.
func (h *TrashHandler) DestroyFile(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	result, err := h.db.Exec(
		`DELETE FROM file_items WHERE id = $1 AND user_id = $2 AND deleted_at IS NOT NULL`,
		fileID, userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "destroy failed")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "destroyed"})
}

// BatchDestroy permanently deletes multiple trash items.
func (h *TrashHandler) BatchDestroy(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		FileIDs []int64 `json:"file_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.FileIDs) == 0 {
		writeError(w, http.StatusBadRequest, "file_ids required")
		return
	}

	destroyed := int64(0)
	res, err := h.db.Exec(
		`DELETE FROM file_items WHERE id = ANY($1) AND user_id = $2 AND deleted_at IS NOT NULL`,
		pq.Array(req.FileIDs), userID,
	)
	if err == nil {
		destroyed, _ = res.RowsAffected()
	}
	writeJSON(w, http.StatusOK, map[string]int64{"destroyed": destroyed})
}

// BatchRestore restores multiple trash items at once.
func (h *TrashHandler) BatchRestore(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		FileIDs []int64 `json:"file_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.FileIDs) == 0 {
		writeError(w, http.StatusBadRequest, "file_ids required")
		return
	}

	restored := int64(0)
	res, err := h.db.Exec(
		`UPDATE file_items SET deleted_at = NULL WHERE id = ANY($1) AND user_id = $2 AND deleted_at IS NOT NULL`,
		pq.Array(req.FileIDs), userID,
	)
	if err == nil {
		restored, _ = res.RowsAffected()
	}
	writeJSON(w, http.StatusOK, map[string]int64{"restored": restored})
}

// ClearTrash permanently deletes all trash items for the user.
func (h *TrashHandler) ClearTrash(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	h.db.Exec(`DELETE FROM file_items WHERE user_id = $1 AND deleted_at IS NOT NULL`, userID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "trash cleared"})
}

// ---------- Encryption utilities ----------

var encryptionKey []byte // set from config

func SetEncryptionKey(key string) {
	if key == "" {
		encryptionKey = []byte("stora-vault-fallback-32byte!!!!!!!")
		return
	}
	// Derive 32-byte key from any-length secret via SHA256
	h := sha256.Sum256([]byte(key))
	encryptionKey = h[:]
}

func init() {
	SetEncryptionKey("") // use fallback key for backward compat; production MUST set via config
}

func hashPassword(pw string) (string, error) {
	return auth.HashPassword(pw)
}

func checkPassword(password, hash string) bool {
	return auth.CheckPassword(password, hash)
}

func encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func decrypt(encoded string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
