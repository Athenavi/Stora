package files

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/go-chi/chi/v5"
)

type VaultHandler struct {
	db *sql.DB
}

func NewVaultHandler(db *sql.DB) *VaultHandler {
	return &VaultHandler{db: db}
}

// ---------- Vault ----------

func (h *VaultHandler) ListVaults(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	rows, err := h.db.Query(
		`SELECT id, name, description, created_at FROM vaults WHERE user_id = $1 ORDER BY name`,
		userID,
	)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Vault struct {
		ID          int64   `json:"id"`
		Name        string  `json:"name"`
		Description *string `json:"description"`
		CreatedAt   *string `json:"created_at"`
	}
	var vaults = make([]Vault, 0)
	for rows.Next() {
		var v Vault
		rows.Scan(&v.ID, &v.Name, &v.Description, &v.CreatedAt)
		vaults = append(vaults, v)
	}
	writeJSON(w, http.StatusOK, vaults)
}

func (h *VaultHandler) CreateVault(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"name required"}`, http.StatusBadRequest)
		return
	}

	now := time.Now().Format(time.RFC3339)
	var vaultID int64
	h.db.QueryRow(
		`INSERT INTO vaults (user_id, name, description, created_at, updated_at) VALUES ($1,$2,$3,$4,$4) RETURNING id`,
		userID, req.Name, req.Description, now,
	).Scan(&vaultID)
	writeJSON(w, http.StatusCreated, map[string]int64{"id": vaultID})
}

func (h *VaultHandler) ListVaultItems(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	vaultID, _ := strconv.ParseInt(chi.URLParam(r, "vaultId"), 10, 64)

	rows, err := h.db.Query(
		`SELECT id, name, type, created_at, updated_at FROM vault_items
		 WHERE vault_id = $1 AND user_id = $2 ORDER BY created_at DESC`,
		vaultID, userID,
	)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Item struct {
		ID        int64   `json:"id"`
		Name      string  `json:"name"`
		Type      string  `json:"type"`
		CreatedAt *string `json:"created_at"`
		UpdatedAt *string `json:"updated_at"`
	}
	var items = make([]Item, 0)
	for rows.Next() {
		var it Item
		rows.Scan(&it.ID, &it.Name, &it.Type, &it.CreatedAt, &it.UpdatedAt)
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *VaultHandler) CreateVaultItem(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	vaultID, _ := strconv.ParseInt(chi.URLParam(r, "vaultId"), 10, 64)

	var req struct {
		Name    string `json:"name"`
		Type    string `json:"type"` // note/credential/file
		Content string `json:"content"`
		FileID  *int64 `json:"file_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"name and content required"}`, http.StatusBadRequest)
		return
	}

	// Encrypt content with AES-256-GCM
	encrypted, err := encrypt(req.Content)
	if err != nil {
		http.Error(w, `{"error":"encryption failed"}`, http.StatusInternalServerError)
		return
	}

	now := time.Now().Format(time.RFC3339)
	var itemID int64
	h.db.QueryRow(
		`INSERT INTO vault_items (vault_id, user_id, name, type, content, file_id, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$7) RETURNING id`,
		vaultID, userID, req.Name, req.Type, encrypted, req.FileID, now,
	).Scan(&itemID)

	writeJSON(w, http.StatusCreated, map[string]int64{"id": itemID})
}

// ---------- Transcoding ----------

type TranscodeHandler struct {
	db *sql.DB
}

func NewTranscodeHandler(db *sql.DB) *TranscodeHandler {
	return &TranscodeHandler{db: db}
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

	writeJSON(w, http.StatusCreated, map[string]int64{"task_id": taskID})
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
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
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
	db *sql.DB
}

func NewBatchHandler(db *sql.DB) *BatchHandler {
	return &BatchHandler{db: db}
}

func (h *BatchHandler) BatchDelete(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		FileIDs []int64 `json:"file_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.FileIDs) == 0 {
		http.Error(w, `{"error":"file_ids required"}`, http.StatusBadRequest)
		return
	}

	now := time.Now().Format(time.RFC3339)
	for _, fid := range req.FileIDs {
		h.db.Exec(`UPDATE file_items SET deleted_at = $1 WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL`,
			now, fid, userID)
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": len(req.FileIDs)})
}

func (h *BatchHandler) BatchMove(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		FileIDs  []int64 `json:"file_ids"`
		FolderID *int64  `json:"folder_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.FileIDs) == 0 {
		http.Error(w, `{"error":"file_ids required"}`, http.StatusBadRequest)
		return
	}

	now := time.Now().Format(time.RFC3339)
	for _, fid := range req.FileIDs {
		h.db.Exec(`UPDATE file_items SET folder_id = $1, updated_at = $2 WHERE id = $3 AND user_id = $4`,
			req.FolderID, now, fid, userID)
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
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
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
	writeJSON(w, http.StatusOK, items)
}

func (h *TrashHandler) RestoreFile(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	result, err := h.db.Exec(
		`UPDATE file_items SET deleted_at = NULL WHERE id = $1 AND user_id = $2 AND deleted_at IS NOT NULL`,
		fileID, userID,
	)
	if err != nil {
		http.Error(w, `{"error":"restore failed"}`, http.StatusInternalServerError)
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "restored"})
}

func (h *TrashHandler) EmptyTrash(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	h.db.Exec(`DELETE FROM file_items WHERE user_id = $1 AND deleted_at IS NOT NULL`, userID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "trash emptied"})
}

// ---------- Encryption utilities ----------

var encryptionKey = []byte("stora-vault-key-32bytes!!!!!!!") // TODO: derive from user password

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
