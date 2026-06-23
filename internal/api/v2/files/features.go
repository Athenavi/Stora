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
	"github.com/google/uuid"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	db       *sql.DB
	vaultDir string
}

func NewVaultHandler(db *sql.DB, vaultDir string) *VaultHandler {
	return &VaultHandler{db: db, vaultDir: vaultDir}
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
		`SELECT v.id, v.name, v.description,
		        COALESCE((SELECT COUNT(*) FROM vault_items WHERE vault_id = v.id), 0) AS file_count,
		        COALESCE((SELECT COALESCE(SUM(file_size), 0) FROM vault_items WHERE vault_id = v.id), 0) AS total_size,
		        CASE WHEN v.password_hash IS NOT NULL AND v.password_hash != '' THEN true ELSE false END AS has_password,
		        v.created_at
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
		`INSERT INTO vaults (user_id, name, description, password_hash, created_at, updated_at) VALUES ($1,$2,'',$3,$4,$4) RETURNING id`,
		userID, name, pwHash, now,
	).Scan(&vaultID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create failed: %v", err))
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

	// Remove the vault's encrypted file storage
	vaultStorageDir := filepath.Join(h.vaultDir, fmt.Sprintf("%d", vaultID))
	os.RemoveAll(vaultStorageDir)

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

	// Write encrypted content to disk: <vaultDir>/<vaultID>/<uuid>.enc
	vaultStorageDir := filepath.Join(h.vaultDir, fmt.Sprintf("%d", vaultID))
	if err := os.MkdirAll(vaultStorageDir, 0700); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create vault storage")
		return
	}
	itemUUID := uuid.New().String()
	filePath := filepath.Join(vaultStorageDir, itemUUID+".enc")
	if err := os.WriteFile(filePath, []byte(encrypted), 0600); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write vault file")
		return
	}

	EnsureVaultCompat(h.db)

	now := time.Now().Format(time.RFC3339)
	var itemID int64
	err = h.db.QueryRow(
		`INSERT INTO vault_items (vault_id, user_id, name, filename, file_size, mime_type, file_path, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8) RETURNING id`,
		vaultID, userID, filename, filename, fileSize, mimeType, filePath, now,
	).Scan(&itemID)

	if err != nil {
		os.Remove(filePath)
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

	var filename, mimeType, filePath string
	var fileSize int64
	err := h.db.QueryRow(
		`SELECT COALESCE(filename, name), COALESCE(mime_type, 'application/octet-stream'),
		        COALESCE(file_path, ''), COALESCE(file_size, 0)
		 FROM vault_items WHERE id = $1 AND vault_id = $2 AND user_id = $3`,
		itemID, vaultID, userID,
	).Scan(&filename, &mimeType, &filePath, &fileSize)

	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	if filePath == "" {
		writeError(w, http.StatusNotFound, "file not found on disk")
		return
	}

	// Read encrypted content from disk
	encryptedBytes, err := os.ReadFile(filePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read vault file")
		return
	}
	encrypted := string(encryptedBytes)

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

	// Get file_path before deleting the record
	var filePath string
	h.db.QueryRow(
		`SELECT COALESCE(file_path, '') FROM vault_items WHERE id = $1 AND vault_id = $2 AND user_id = $3`,
		itemID, vaultID, userID,
	).Scan(&filePath)

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

	// Remove the encrypted file from disk
	if filePath != "" {
		os.Remove(filePath)
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
		`ALTER TABLE vault_items ADD COLUMN IF NOT EXISTS file_path TEXT`,
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
	// Sum file sizes for quota update
	var deletedBytes int64
	h.db.QueryRow(`SELECT COALESCE(SUM(file_size), 0) FROM file_items WHERE id = ANY($1) AND user_id = $2 AND deleted_at IS NULL`, pq.Array(req.FileIDs), userID).Scan(&deletedBytes)
	result, err := h.db.Exec(
		`UPDATE file_items SET deleted_at = $1 WHERE id = ANY($2) AND user_id = $3 AND deleted_at IS NULL`,
		now, pq.Array(req.FileIDs), userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "batch delete failed")
		return
	}
	n, _ := result.RowsAffected()
	if deletedBytes > 0 {
		h.db.Exec(`UPDATE users SET used_storage = GREATEST(0, used_storage - $1) WHERE id = $2`, deletedBytes, userID)
	}
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

// BatchAssignTags replaces all tags on multiple files with the given tag set
func (h *BatchHandler) BatchAssignTags(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		FileIDs []int64 `json:"file_ids"`
		TagIDs  []int64 `json:"tag_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.FileIDs) == 0 {
		writeError(w, http.StatusBadRequest, "file_ids required")
		return
	}

	// For each file, replace tag assignments
	// Verify files belong to user
	for _, fid := range req.FileIDs {
		var ownerID int64
		err := h.db.QueryRow(`SELECT user_id FROM file_items WHERE id = $1 AND deleted_at IS NULL`, fid).Scan(&ownerID)
		if err != nil || ownerID != userID {
			continue
		}
		h.db.Exec(`DELETE FROM file_tag_assignments WHERE file_id = $1`, fid)
		for _, tagID := range req.TagIDs {
			h.db.Exec(`INSERT INTO file_tag_assignments (file_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, fid, tagID)
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"affected": len(req.FileIDs)})
}

// BatchSetCategory sets the category on multiple files
func (h *BatchHandler) BatchSetCategory(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		FileIDs  []int64 `json:"file_ids"`
		Category *string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.FileIDs) == 0 {
		writeError(w, http.StatusBadRequest, "file_ids required")
		return
	}
	_, err := h.db.Exec(
		`UPDATE file_items SET category = $1 WHERE id = ANY($2) AND user_id = $3`,
		req.Category, pq.Array(req.FileIDs), userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "batch category failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"affected": len(req.FileIDs)})
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

// ---------- Transcription (AI Subtitles) ----------

type TranscribeHandler struct {
	db *sql.DB
}

func NewTranscribeHandler(db *sql.DB) *TranscribeHandler {
	return &TranscribeHandler{db: db}
}

// StartTranscription starts a background transcription job.
// POST /files/transcribe/{id}
func (h *TranscribeHandler) StartTranscription(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if fileID == 0 {
		writeError(w, http.StatusBadRequest, "invalid file id")
		return
	}

	// Check file exists and is a video
	var mimeType string
	err := h.db.QueryRow(
		`SELECT COALESCE(mime_type,'') FROM file_items WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		fileID, userID,
	).Scan(&mimeType)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if !strings.HasPrefix(mimeType, "video/") {
		writeError(w, http.StatusBadRequest, "file is not a video")
		return
	}

	// Check existing task
	var existingID int64
	err = h.db.QueryRow(
		`SELECT id FROM transcription_tasks WHERE file_id = $1 AND user_id = $2 AND status != 'failed' ORDER BY id DESC LIMIT 1`,
		fileID, userID,
	).Scan(&existingID)
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"task_id": existingID, "message": "task already exists"})
		return
	}

	now := time.Now().Format(time.RFC3339)
	var taskID int64
	err = h.db.QueryRow(
		`INSERT INTO transcription_tasks (file_id, user_id, status, created_at, updated_at)
		 VALUES ($1, $2, 'pending', $3, $3) RETURNING id`,
		fileID, userID, now,
	).Scan(&taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create task failed")
		return
	}

	go h.processTranscription(taskID, fileID)
	writeJSON(w, http.StatusCreated, map[string]int64{"task_id": taskID})
}

// GetTranscriptionStatus returns the current status of a transcription task.
// GET /files/transcribe/{id}/status
func (h *TranscribeHandler) GetTranscriptionStatus(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	var status, errorMsg string
	var hasContent bool
	err := h.db.QueryRow(
		`SELECT COALESCE(status,''), COALESCE(error_msg,''),
		        CASE WHEN content IS NOT NULL AND content != '' THEN true ELSE false END
		 FROM transcription_tasks
		 WHERE file_id = $1 AND user_id = $2
		 ORDER BY id DESC LIMIT 1`,
		fileID, userID,
	).Scan(&status, &errorMsg, &hasContent)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"available": false,
			"status":    "not_found",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"available":    status == "completed",
		"status":       status,
		"error_message": errorMsg,
	})
}

// GetSubtitleFile returns the SRT subtitle content.
// GET /files/transcribe/{id}/subtitle
func (h *TranscribeHandler) GetSubtitleFile(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	var content string
	err := h.db.QueryRow(
		`SELECT COALESCE(content,'') FROM transcription_tasks
		 WHERE file_id = $1 AND status = 'completed'
		 ORDER BY id DESC LIMIT 1`,
		fileID,
	).Scan(&content)
	if err != nil || content == "" {
		http.Error(w, "subtitle not available", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="subtitle_%d.srt"`, fileID))
	w.Write([]byte(content))
}

func (h *TranscribeHandler) processTranscription(taskID, fileID int64) {
	db := h.db
	now := time.Now().Format(time.RFC3339)
	db.Exec(`UPDATE transcription_tasks SET status = 'processing', updated_at = $1 WHERE id = $2`, now, taskID)

	// Get file path
	var filePath string
	err := db.QueryRow(
		`SELECT file_path FROM file_items WHERE id = $1`,
		fileID,
	).Scan(&filePath)
	if err != nil {
		db.Exec(`UPDATE transcription_tasks SET status = 'failed', error_msg = 'file not found', updated_at = $1 WHERE id = $2`, now, taskID)
		return
	}

	// Step 1: Extract audio using ffmpeg
	audioPath := filepath.Join(os.TempDir(), fmt.Sprintf("stora_audio_%d.wav", taskID))
	extractCmd := exec.Command("ffmpeg", "-i", filePath,
		"-vn", "-acodec", "pcm_s16le", "-ar", "16000", "-ac", "1",
		"-y", audioPath)
	if output, err := extractCmd.CombinedOutput(); err != nil {
		db.Exec(`UPDATE transcription_tasks SET status = 'failed', error_msg = $1, updated_at = $2 WHERE id = $3`,
			"音频提取失败: "+string(output[:min(len(output), 300)]), now, taskID)
		return
	}
	defer os.Remove(audioPath)

	// Step 2: Try whisper CLI
	var srtContent string
	whisperCmd := exec.Command("whisper", audioPath, "--model", "tiny", "--output_format", "srt", "--language", "zh")
	if _, err := whisperCmd.CombinedOutput(); err == nil {
		// Try to read the generated SRT file
		srtPath := audioPath + ".srt"
		if data, err := os.ReadFile(srtPath); err == nil {
			srtContent = string(data)
		}
		os.Remove(srtPath)
	}

	// Fallback: if whisper not available, generate placeholder content
	if srtContent == "" {
		srtContent = fmt.Sprintf(`1
00:00:01,000 --> 00:00:04,000
[字幕需要安装 Whisper]
 whisper 未在服务器上安装

2
00:00:04,000 --> 00:00:08,000
安装命令: pip install openai-whisper
或使用云端 API
`)
	}

	// Step 3: Save result
	db.Exec(`UPDATE transcription_tasks SET status = 'completed', content = $1, updated_at = $2 WHERE id = $3`,
		srtContent, now, taskID)
}

// ---------- Transcode Tasks List ----------

// ListTranscodeTasks returns transcode tasks for a file.
// GET /files/transcode/{id}/tasks
func (h *TranscodeHandler) ListTranscodeTasks(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	rows, err := h.db.Query(
		`SELECT id, status, progress, COALESCE(target_format,''), COALESCE(error_msg,''), updated_at
		 FROM transcode_tasks WHERE file_id = $1 AND user_id = $2 ORDER BY id DESC`,
		fileID, userID,
	)
	if err != nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	defer rows.Close()

	type Task struct {
		ID           int64  `json:"id"`
		Status       string `json:"status"`
		Progress     int    `json:"progress"`
		TargetFormat string `json:"target_format"`
		ErrorMsg     string `json:"error_msg"`
		UpdatedAt    string `json:"updated_at"`
	}
	tasks := make([]Task, 0)
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Status, &t.Progress, &t.TargetFormat, &t.ErrorMsg, &t.UpdatedAt); err == nil {
			tasks = append(tasks, t)
		}
	}
	writeJSON(w, http.StatusOK, tasks)
}

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
