package files

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"database/sql"

	"github.com/go-chi/chi/v5"
	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/Athenavi/Stora/pkg/storage"
	"github.com/Athenavi/Stora/pkg/utils"
)

// BlockHandler manages block-level storage for the .Stora sync engine.
type BlockHandler struct {
	db      *sql.DB
	storage storage.Driver
}

func NewBlockHandler(db *sql.DB, store storage.Driver) *BlockHandler {
	return &BlockHandler{db: db, storage: store}
}

// UploadBlock stores a single 4MB block in content-addressable storage.
// POST /api/v2/blocks/upload
func (h *BlockHandler) UploadBlock(w http.ResponseWriter, r *http.Request) {
	_, _ = middleware.GetUserID(r.Context())

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid form")
		return
	}

	file, header, err := r.FormFile("block")
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "block field required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "read failed")
		return
	}

	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])
	_ = header

	_, storagePath, err := h.storage.StoreHash(bytes.NewReader(data))
	if err != nil {
		storagePath = fmt.Sprintf("objects/%s/%s", hashStr[:2], hashStr[2:])
	}

	now := time.Now().Format(time.RFC3339)
	_, err = h.db.Exec(
		`INSERT INTO block_store (hash, size, storage_path, ref_count, created_at, updated_at)
		 VALUES ($1, $2, $3, 1, $4, $4)
		 ON CONFLICT (hash) DO UPDATE SET ref_count = block_store.ref_count + 1, updated_at = $4`,
		hashStr, len(data), storagePath, now,
	)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "db insert failed")
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"hash": hashStr,
		"size": len(data),
	})
}

// DownloadBlock retrieves a block by hash.
// GET /api/v2/blocks/{hash}
func (h *BlockHandler) DownloadBlock(w http.ResponseWriter, r *http.Request) {
	blockHash := chi.URLParam(r, "hash")
	if blockHash == "" {
		utils.WriteError(w, http.StatusBadRequest, "hash required")
		return
	}

	var storagePath string
	err := h.db.QueryRow(
		`SELECT storage_path FROM block_store WHERE hash = $1`, blockHash,
	).Scan(&storagePath)
	if err != nil {
		utils.WriteError(w, http.StatusNotFound, "block not found")
		return
	}

	reader, err := h.storage.Retrieve(storagePath)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "read failed")
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Block-Hash", blockHash)
	io.Copy(w, reader)
}

// GetFileManifest returns the block manifest for a file.
// GET /api/v2/files/{id}/manifest
func (h *BlockHandler) GetFileManifest(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	var fileHash string
	err := h.db.QueryRow(
		`SELECT COALESCE(file_hash, '''') FROM file_items WHERE id = $1 AND user_id = $2`,
		fileID, userID,
	).Scan(&fileHash)
	if err != nil {
		utils.WriteError(w, http.StatusNotFound, "file not found")
		return
	}

	if fileHash == "" {
		utils.WriteError(w, http.StatusNotFound, "no hash")
		return
	}

	rows, err := h.db.Query(
		`SELECT fb.block_index, fb.block_hash, fb.block_offset, fb.block_size, bs.size
		 FROM file_blocks fb
		 JOIN block_store bs ON fb.block_hash = bs.hash
		 WHERE fb.file_hash = $1
		 ORDER BY fb.block_index`,
		fileHash,
	)
	if err != nil {
		utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"file_hash":  fileHash,
			"blocks":     []interface{}{},
			"block_size": 4194304,
		})
		return
	}
	defer rows.Close()

	type Block struct {
		Index      int    `json:"index"`
		Hash       string `json:"hash"`
		Offset     int64  `json:"offset"`
		Size       int    `json:"size"`
		StoredSize int    `json:"stored_size"`
	}

	blocks := make([]Block, 0)
	for rows.Next() {
		var b Block
		rows.Scan(&b.Index, &b.Hash, &b.Offset, &b.Size, &b.StoredSize)
		blocks = append(blocks, b)
	}

	var fileSize int64
	h.db.QueryRow(`SELECT file_size FROM file_items WHERE id = $1`, fileID).Scan(&fileSize)

	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"file_hash":  fileHash,
		"file_size":  fileSize,
		"block_size": 4194304,
		"blocks":     blocks,
	})
}


// ListSnapshots returns version snapshots for a file.
// GET /api/v2/files/{id}/snapshots
func (h *BlockHandler) ListSnapshots(w http.ResponseWriter, r *http.Request) {
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	userID, _ := middleware.GetUserID(r.Context())

	rows, err := h.db.Query(
		`SELECT id, version_num, file_size, file_hash, created_at FROM file_versions
		 WHERE file_id = $1 AND created_by = $2 ORDER BY version_num DESC LIMIT 100`,
		fileID, userID,
	)
	if err != nil {
		utils.WriteJSON(w, http.StatusOK, map[string]interface{}{"snapshots": []interface{}{}})
		return
	}
	defer rows.Close()

	type Snap struct {
		ID        int64  `json:"id"`
		Version   int    `json:"version"`
		Size      int64  `json:"size"`
		Hash      string `json:"hash,omitempty"`
		CreatedAt string `json:"created_at"`
	}
	snaps := make([]Snap, 0)
	for rows.Next() {
		var s Snap
		var h *string
		rows.Scan(&s.ID, &s.Version, &s.Size, &h, &s.CreatedAt)
		if h != nil { s.Hash = *h }
		snaps = append(snaps, s)
	}
	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{"snapshots": snaps})
}// SyncChanges returns incremental changes since a cursor.
// GET /api/v2/sync/changes?since={cursor}&limit=100
func (h *BlockHandler) SyncChanges(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	since := r.URL.Query().Get("since")
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
		limit = l
	}

	var rows *sql.Rows
	var err error

	if since != "" {
		rows, err = h.db.Query(
			`SELECT fi.id, fi.filename, fi.file_hash, fi.file_size, fi.updated_at,
			        CASE WHEN fi.deleted_at IS NOT NULL THEN 'deleted' ELSE 'modified' END
			 FROM file_items fi
			 WHERE fi.user_id = $1 AND (fi.updated_at > $2 OR fi.deleted_at > $2)
			   AND fi.is_folder = false
			 ORDER BY GREATEST(fi.updated_at, fi.deleted_at) ASC
			 LIMIT $3`,
			userID, since, limit,
		)
	} else {
		rows, err = h.db.Query(
			`SELECT fi.id, fi.filename, fi.file_hash, fi.file_size, fi.updated_at,
			        CASE WHEN fi.deleted_at IS NOT NULL THEN 'deleted' ELSE 'modified' END
			 FROM file_items fi
			 WHERE fi.user_id = $1 AND fi.is_folder = false
			 ORDER BY fi.updated_at DESC
			 LIMIT $2`,
			userID, limit,
		)
	}
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type Change struct {
		ID       int64  `json:"id"`
		Filename string `json:"filename"`
		Hash     string `json:"hash,omitempty"`
		Size     int64  `json:"size"`
		Time     string `json:"time"`
		Action   string `json:"action"`
	}

	changes := make([]Change, 0)
	var lastTime string
	for rows.Next() {
		var c Change
		var t string
		rows.Scan(&c.ID, &c.Filename, &c.Hash, &c.Size, &t, &c.Action)
		c.Time = t
		if t > lastTime {
			lastTime = t
		}
		changes = append(changes, c)
	}

	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"changes":   changes,
		"next":      lastTime,
		"remaining": len(changes) >= limit,
	})
}
