package files

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	_, storagePath, err := h.storage.StoreHash(bytes.NewReader(data))
	if err != nil {
		storagePath = fmt.Sprintf("objects/%s/%s", hashStr[:2], hashStr[2:])
		_ = header
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

// SyncUpload uploads a file with its full relative path — auto-creates folders.
// POST /api/v2/sync/upload
func (h *BlockHandler) SyncUpload(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	if err := r.ParseMultipartForm(100 << 20); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "file too large")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "file field required")
		return
	}
	defer file.Close()

	filePath := r.FormValue("path")
	if filePath == "" {
		utils.WriteError(w, http.StatusBadRequest, "path field required")
		return
	}

	filePath = filepath.Clean(filePath)
	fileName := filepath.Base(filePath)
	dirPath := filepath.Dir(filePath)

	// 1. Find or create the parent folder hierarchy
	var parentID *int64
	if dirPath != "." {
		segments := strings.Split(dirPath, string(filepath.Separator))
		for _, seg := range segments {
			if seg == "" || seg == "." {
				continue
			}
			// Try to find existing folder
			var folderID int64
			if parentID != nil {
				err = h.db.QueryRow(
					`SELECT id FROM file_items WHERE user_id = $1 AND folder_id = $2
					 AND filename = $3 AND is_folder = true AND deleted_at IS NULL`,
					userID, *parentID, seg,
				).Scan(&folderID)
			} else {
				err = h.db.QueryRow(
					`SELECT id FROM file_items WHERE user_id = $1 AND folder_id IS NULL
					 AND filename = $2 AND is_folder = true AND deleted_at IS NULL`,
					userID, seg,
				).Scan(&folderID)
			}

			if err != nil {
				// Create folder
				now := time.Now().Format(time.RFC3339)
				err = h.db.QueryRow(
					`INSERT INTO file_items (user_id, folder_id, filename, is_folder, created_at, updated_at)
					 VALUES ($1, $2, $3, true, $4, $4) RETURNING id`,
					userID, parentID, seg, now,
				).Scan(&folderID)
				if err != nil {
					utils.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("create folder failed: %v", err))
					return
				}
			}
			parentID = &folderID
		}
	}

	// 2. Save file content and create file record
	data, err := io.ReadAll(file)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "read file failed")
		return
	}

	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	// Store content via content-addressable storage
	_, storagePath, err := h.storage.StoreHash(bytes.NewReader(data))
	if err != nil {
		storagePath = fmt.Sprintf("objects/%s/%s", hashStr[:2], hashStr[2:])
		// Also write to disk as fallback
		diskPath := fmt.Sprintf("objects/%s/%s", hashStr[:2], hashStr[2:])
		os.MkdirAll(filepath.Dir(diskPath), 0755)
		os.WriteFile(diskPath, data, 0644)
		storagePath = diskPath
	}

	now := time.Now().Format(time.RFC3339)
	mimeType := header.Header.Get("Content-Type")
	fileSize := len(data)

	// Determine file type from extension
	ext := strings.ToLower(filepath.Ext(fileName))
	fileType := "other"
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg":
		fileType = "image"
	case ".mp4", ".webm", ".avi", ".mkv", ".mov":
		fileType = "video"
	case ".mp3", ".wav", ".ogg", ".flac":
		fileType = "audio"
	case ".doc", ".docx", ".pdf", ".txt", ".md":
		fileType = "document"
	case ".zip", ".rar", ".7z", ".tar", ".gz":
		fileType = "archive"
	}

	// Insert file record (handle duplicates by updating content)
	var fileID int64
	err = h.db.QueryRow(
		`INSERT INTO file_items (user_id, folder_id, filename, original_filename, file_path, file_size,
		 mime_type, file_type, file_hash, is_folder, created_at, updated_at)
		 VALUES ($1, $2, $3, $3, $4, $5, $6, $7, $8, false, $9, $9) RETURNING id`,
		userID, parentID, fileName, storagePath, fileSize, mimeType, fileType, hashStr, now,
	).Scan(&fileID)
	if err != nil {
		// Unique violation: file with same name exists - update content
		var existingID int64
		h.db.QueryRow(
			`SELECT id FROM file_items WHERE user_id = $1 AND folder_id IS NOT DISTINCT FROM $2
			 AND filename = $3 AND is_folder = false AND deleted_at IS NULL`,
			userID, parentID, fileName,
		).Scan(&existingID)
		if existingID > 0 {
			h.db.Exec(`UPDATE file_items SET file_path = $1, file_hash = $2, file_size = $3, updated_at = $4
				WHERE id = $5`, storagePath, hashStr, fileSize, now, existingID)
			fileID = existingID
		} else {
			utils.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("insert failed: %v", err))
			return
		}
	}

	// Update fingerprint refcount
	h.db.Exec(`INSERT INTO file_fingerprints (hash, file_size, mime_type, storage_path, reference_count, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 1, $5, $5)
		ON CONFLICT (hash) DO UPDATE SET reference_count = file_fingerprints.reference_count + 1, updated_at = $5`,
		hashStr, fileSize, mimeType, storagePath, now)

	utils.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"file_id":   fileID,
		"file_hash": hashStr,
		"file_size": fileSize,
		"filename":  fileName,
		"path":      filePath,
	})
}

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
		if h != nil {
			s.Hash = *h
		}
		snaps = append(snaps, s)
	}
	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{"snapshots": snaps})
}

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
