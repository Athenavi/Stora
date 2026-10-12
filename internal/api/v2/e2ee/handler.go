// Package e2ee implements zero-knowledge private spaces.
//
// The server is a dumb store here: it keeps public KDF parameters and ciphertext,
// and has no key material and no decrypt path at all — see docs/design-e2ee.md
// for the key hierarchy and the ciphertext format.
//
// This is deliberately separate from `vaults`/`vault_items`, which are
// *server-side* encrypted and therefore decryptable, searchable and thumbnailable.
// The two carry different security promises and are never mixed.
package e2ee

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/Athenavi/Stora/pkg/audit"
	"github.com/Athenavi/Stora/pkg/storage"
	"github.com/Athenavi/Stora/pkg/utils"
	"github.com/go-chi/chi/v5"
)

// minKDFIterations rejects parameters that would silently undermine the scheme.
// The client picks them, so the server refuses obviously weak choices rather than
// trusting it.
const minKDFIterations = 100000

type Handler struct {
	db      *sql.DB
	storage storage.Driver
}

func NewHandler(db *sql.DB, store storage.Driver) *Handler {
	return &Handler{db: db, storage: store}
}

// requireVault verifies ownership. It never looks at content — the server cannot
// read any of it.
func (h *Handler) requireVault(userID, vaultID int64) error {
	var ownerID int64
	if err := h.db.QueryRow(
		`SELECT user_id FROM e2ee_vaults WHERE id = $1`, vaultID).Scan(&ownerID); err != nil {
		return err
	}
	if ownerID != userID {
		return sql.ErrNoRows
	}
	return nil
}

// CreateVault stores a wrapped vault key and the parameters needed to derive it.
// The server cannot tell whether the wrapped key even decrypts.
func (h *Handler) CreateVault(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		utils.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Name               string `json:"name"`
		KDFSalt            string `json:"kdf_salt"`
		KDFIterations      int    `json:"kdf_iterations"`
		WrappedKey         string `json:"wrapped_key"`
		RecoveryWrappedKey string `json:"recovery_wrapped_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		req.Name == "" || req.KDFSalt == "" || req.WrappedKey == "" {
		utils.WriteError(w, http.StatusBadRequest, "name, kdf_salt and wrapped_key required")
		return
	}
	if req.KDFIterations < minKDFIterations {
		utils.WriteError(w, http.StatusBadRequest,
			fmt.Sprintf("kdf_iterations must be at least %d", minKDFIterations))
		return
	}

	now := time.Now().Format(time.RFC3339)
	var id int64
	if err := h.db.QueryRow(
		`INSERT INTO e2ee_vaults (user_id, name, kdf_salt, kdf_iterations, wrapped_key,
		                          recovery_wrapped_key, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $7) RETURNING id`,
		userID, req.Name, req.KDFSalt, req.KDFIterations, req.WrappedKey,
		req.RecoveryWrappedKey, now,
	).Scan(&id); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "create failed")
		return
	}

	audit.Log(h.db, audit.Entry{
		UserID:    userID,
		Action:    "e2ee.vault_create",
		Resource:  fmt.Sprintf("e2ee_vault:%d", id),
		IPAddress: middleware.ClientIP(r),
	})
	utils.WriteJSON(w, http.StatusCreated, map[string]interface{}{"id": id})
}

// ListVaults returns the material a client needs to derive its keys.
func (h *Handler) ListVaults(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	rows, err := h.db.Query(
		`SELECT v.id, v.name, v.kdf_salt, v.kdf_iterations, v.wrapped_key,
		        COALESCE(v.recovery_wrapped_key, ''), CAST(v.created_at AS TEXT),
		        COALESCE((SELECT COUNT(*) FROM e2ee_items i WHERE i.vault_id = v.id), 0)
		   FROM e2ee_vaults v
		  WHERE v.user_id = $1
		  ORDER BY v.created_at`, userID)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type vault struct {
		ID                 int64  `json:"id"`
		Name               string `json:"name"`
		KDFSalt            string `json:"kdf_salt"`
		KDFIterations      int    `json:"kdf_iterations"`
		WrappedKey         string `json:"wrapped_key"`
		RecoveryWrappedKey string `json:"recovery_wrapped_key"`
		CreatedAt          string `json:"created_at"`
		ItemCount          int    `json:"item_count"`
	}
	items := make([]vault, 0)
	for rows.Next() {
		var v vault
		if err := rows.Scan(&v.ID, &v.Name, &v.KDFSalt, &v.KDFIterations, &v.WrappedKey,
			&v.RecoveryWrappedKey, &v.CreatedAt, &v.ItemCount); err == nil {
			items = append(items, v)
		}
	}
	utils.WriteJSON(w, http.StatusOK, items)
}

// DeleteVault removes the vault, its items and their ciphertext objects.
func (h *Handler) DeleteVault(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	vaultID, _ := strconv.ParseInt(chi.URLParam(r, "vaultId"), 10, 64)

	// Collect storage keys first: the cascade delete removes the rows that point
	// at the objects.
	rows, err := h.db.Query(
		`SELECT storage_path FROM e2ee_items WHERE vault_id = $1 AND user_id = $2`, vaultID, userID)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "query failed")
		return
	}
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err == nil {
			paths = append(paths, p)
		}
	}
	rows.Close()

	result, err := h.db.Exec(`DELETE FROM e2ee_vaults WHERE id = $1 AND user_id = $2`, vaultID, userID)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		utils.WriteError(w, http.StatusNotFound, "vault not found")
		return
	}

	// Best effort: a leftover object is wasted space, not a correctness problem.
	for _, p := range paths {
		if err := h.storage.Delete(p); err != nil {
			continue
		}
	}

	audit.Log(h.db, audit.Entry{
		UserID:    userID,
		Action:    "e2ee.vault_delete",
		Resource:  fmt.Sprintf("e2ee_vault:%d", vaultID),
		IPAddress: middleware.ClientIP(r),
	})
	utils.WriteJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// UploadItem stores client-encrypted bytes plus an encrypted name and metadata.
//
// The payload is opaque here: content-addressed storage works unchanged, and no
// derived state (thumbnail, OCR, search index) is ever produced for it — the
// server cannot read a single byte of it.
func (h *Handler) UploadItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		utils.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	vaultID, _ := strconv.ParseInt(chi.URLParam(r, "vaultId"), 10, 64)

	// Ownership before reading a potentially large body.
	if err := h.requireVault(userID, vaultID); err != nil {
		utils.WriteError(w, http.StatusNotFound, "vault not found")
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "file field required")
		return
	}
	defer file.Close()

	encName := r.FormValue("enc_name")
	encMeta := r.FormValue("enc_meta")
	if encName == "" {
		utils.WriteError(w, http.StatusBadRequest, "enc_name required")
		return
	}

	_, storagePath, err := h.storage.StoreHash(file)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "storage failed")
		return
	}

	now := time.Now().Format(time.RFC3339)
	var itemID int64
	if err := h.db.QueryRow(
		`INSERT INTO e2ee_items (vault_id, user_id, enc_name, enc_meta, storage_path,
		                         size_cipher, created_at, updated_at)
		 VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $7) RETURNING id`,
		vaultID, userID, encName, encMeta, storagePath, header.Size, now,
	).Scan(&itemID); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "insert failed")
		return
	}

	audit.Log(h.db, audit.Entry{
		UserID:    userID,
		Action:    "e2ee.item_create",
		Resource:  fmt.Sprintf("e2ee_item:%d", itemID),
		IPAddress: middleware.ClientIP(r),
	})
	utils.WriteJSON(w, http.StatusCreated, map[string]interface{}{"id": itemID})
}

// ListItems returns ciphertext metadata. Names and sizes of the *plaintext* are
// unknown to the server by design.
func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	vaultID, _ := strconv.ParseInt(chi.URLParam(r, "vaultId"), 10, 64)

	if err := h.requireVault(userID, vaultID); err != nil {
		utils.WriteError(w, http.StatusNotFound, "vault not found")
		return
	}

	rows, err := h.db.Query(
		`SELECT id, enc_name, COALESCE(enc_meta, ''), size_cipher, CAST(created_at AS TEXT)
		   FROM e2ee_items
		  WHERE vault_id = $1 AND user_id = $2
		  ORDER BY created_at DESC`, vaultID, userID)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type item struct {
		ID         int64  `json:"id"`
		EncName    string `json:"enc_name"`
		EncMeta    string `json:"enc_meta"`
		SizeCipher int64  `json:"size_cipher"`
		CreatedAt  string `json:"created_at"`
	}
	items := make([]item, 0)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.EncName, &it.EncMeta, &it.SizeCipher, &it.CreatedAt); err == nil {
			items = append(items, it)
		}
	}
	utils.WriteJSON(w, http.StatusOK, items)
}

// DownloadItem streams the ciphertext verbatim; decryption is the client's job.
func (h *Handler) DownloadItem(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	itemID, _ := strconv.ParseInt(chi.URLParam(r, "itemId"), 10, 64)

	var storagePath string
	var sizeCipher int64
	if err := h.db.QueryRow(
		`SELECT storage_path, size_cipher FROM e2ee_items WHERE id = $1 AND user_id = $2`,
		itemID, userID).Scan(&storagePath, &sizeCipher); err != nil {
		utils.WriteError(w, http.StatusNotFound, "item not found")
		return
	}

	reader, err := h.storage.Retrieve(storagePath)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "storage error")
		return
	}
	defer reader.Close()

	// Opaque bytes: no filename, no mime type — the server does not know them.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(sizeCipher, 10))
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, reader)
}

// DeleteItem removes one ciphertext object and its row.
func (h *Handler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	itemID, _ := strconv.ParseInt(chi.URLParam(r, "itemId"), 10, 64)

	var storagePath string
	h.db.QueryRow(
		`SELECT storage_path FROM e2ee_items WHERE id = $1 AND user_id = $2`,
		itemID, userID).Scan(&storagePath)

	result, err := h.db.Exec(`DELETE FROM e2ee_items WHERE id = $1 AND user_id = $2`, itemID, userID)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		utils.WriteError(w, http.StatusNotFound, "item not found")
		return
	}
	if storagePath != "" {
		h.storage.Delete(storagePath)
	}

	audit.Log(h.db, audit.Entry{
		UserID:    userID,
		Action:    "e2ee.item_delete",
		Resource:  fmt.Sprintf("e2ee_item:%d", itemID),
		IPAddress: middleware.ClientIP(r),
	})
	utils.WriteJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
