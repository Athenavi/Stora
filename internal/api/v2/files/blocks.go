package files

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"database/sql"

	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/Athenavi/Stora/pkg/storage"
	"github.com/Athenavi/Stora/pkg/utils"
	"github.com/go-chi/chi/v5"
)

// BlockHandler manages block-level storage for the .Stora sync engine.
type BlockHandler struct {
	db      *sql.DB
	storage storage.Driver
	tempDir string
}

func NewBlockHandler(db *sql.DB, store storage.Driver, tempDir string) *BlockHandler {
	return &BlockHandler{db: db, storage: store, tempDir: tempDir}
}

// maxBlockBytes bounds a single uploaded block. The sync engine uses 4 MiB
// blocks; the cap only needs headroom for multipart overhead.
const maxBlockBytes = int64(16) << 20

// UploadBlock stores a single 4MB block in content-addressable storage.
func (h *BlockHandler) UploadBlock(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.GetUserID(r.Context()); !ok {
		utils.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Hard cap the request body: without it a client could stream to disk freely.
	r.Body = http.MaxBytesReader(w, r.Body, maxBlockBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid form or block too large")
		return
	}

	file, header, err := r.FormFile("block")
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "block field required")
		return
	}
	defer file.Close()

	if header.Size > maxBlockBytes {
		utils.WriteError(w, http.StatusRequestEntityTooLarge, "block too large")
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "read failed")
		return
	}

	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	_, storagePath, err := h.storage.StoreHash(bytes.NewReader(data))
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "storage failed")
		return
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
		`SELECT COALESCE(file_hash, '') FROM file_items WHERE id = $1 AND user_id = $2`,
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

// SyncUploadBlocks 按「块清单」落地一个文件：客户端只上传变化过的块，
// 这里负责把块拼起来、写 file_items，并补写 file_blocks。
//
// 这是块级增量同步的最后一环，也是 file_blocks 唯一的写入者 —— 在此之前
// 这张表从建立起就没人写过，GET /files/{id}/manifest 恒返回空清单，
// 于是"只传变化的块"永远无从谈起。
func (h *BlockHandler) SyncUploadBlocks(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		utils.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		Path     string `json:"path"`
		FileSize int64  `json:"file_size"`
		Blocks   []struct {
			Index  int    `json:"index"`
			Hash   string `json:"hash"`
			Offset int64  `json:"offset"`
			Size   int    `json:"size"`
		} `json:"blocks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" || len(req.Blocks) == 0 {
		utils.WriteError(w, http.StatusBadRequest, "path and non-empty blocks required")
		return
	}

	cleanPath := filepath.Clean(req.Path)
	if strings.HasPrefix(cleanPath, "..") || strings.Contains(cleanPath, "..") ||
		strings.HasPrefix(cleanPath, "/") || strings.HasPrefix(cleanPath, "\\") {
		utils.WriteError(w, http.StatusBadRequest, "invalid path")
		return
	}
	req.Path = cleanPath

	// 清单里各块大小之和必须等于声明的文件大小，否则拼出来的是坏文件。
	var declared int64
	for _, b := range req.Blocks {
		declared += int64(b.Size)
	}
	if declared != req.FileSize {
		utils.WriteError(w, http.StatusBadRequest, "block sizes do not sum to file_size")
		return
	}

	sort.Slice(req.Blocks, func(i, j int) bool { return req.Blocks[i].Index < req.Blocks[j].Index })

	// 先确认每块都在块池里。缺了就明确报出来让客户端补传，
	// 而不是静默拼出一个不完整的文件。
	type missingBlock struct {
		Index int    `json:"index"`
		Hash  string `json:"hash"`
	}
	missing := make([]missingBlock, 0)
	paths := make([]string, 0, len(req.Blocks))
	for _, b := range req.Blocks {
		var sp string
		if err := h.db.QueryRow(`SELECT storage_path FROM block_store WHERE hash = $1`, b.Hash).Scan(&sp); err != nil {
			missing = append(missing, missingBlock{Index: b.Index, Hash: b.Hash})
			paths = append(paths, "")
			continue
		}
		paths = append(paths, sp)
	}
	if len(missing) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "some blocks are missing",
			"missing": missing,
		})
		return
	}

	readers := make([]io.Reader, 0, len(paths))
	closers := make([]io.Closer, 0, len(paths))
	for _, sp := range paths {
		rd, err := h.storage.Retrieve(sp)
		if err != nil {
			utils.WriteError(w, http.StatusInternalServerError, "block read failed")
			return
		}
		closers = append(closers, rd)
		readers = append(readers, rd)
	}
	defer func() {
		for _, c := range closers {
			c.Close()
		}
	}()

	fileHash, storagePath, err := h.storage.StoreHash(io.MultiReader(readers...))
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "assemble failed")
		return
	}

	now := time.Now().Format(time.RFC3339)

	// file_blocks.file_hash 的外键指向的是 file_fingerprints(hash)，**不是**
	// file_items。少了这一步，下面写清单会撞外键约束——而且因为 Exec 的错误
	// 没被检查而静默失败，症状就是 GET /files/{id}/manifest 永远返回空 blocks，
	// 于是"只传变化的块"根本无从谈起。
	h.db.Exec(`INSERT INTO file_fingerprints (hash, file_size, mime_type, storage_path, reference_count, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 1, $5, $5)
		ON CONFLICT (hash) DO UPDATE SET reference_count = file_fingerprints.reference_count + 1, updated_at = $5`,
		fileHash, req.FileSize, "application/octet-stream", storagePath, now)

	fileName := filepath.Base(req.Path)
	dirPath := filepath.Dir(req.Path)

	// 按路径建父目录链（与 SyncUpload 同一规则）
	var parentID *int64
	if dirPath != "." {
		for _, seg := range strings.Split(dirPath, string(filepath.Separator)) {
			if seg == "" || seg == "." {
				continue
			}
			var folderID int64
			if parentID != nil {
				h.db.QueryRow(`SELECT id FROM file_items WHERE user_id = $1 AND folder_id = $2 AND filename = $3 AND is_folder = true AND deleted_at IS NULL`,
					userID, *parentID, seg).Scan(&folderID)
			} else {
				h.db.QueryRow(`SELECT id FROM file_items WHERE user_id = $1 AND folder_id IS NULL AND filename = $2 AND is_folder = true AND deleted_at IS NULL`,
					userID, seg).Scan(&folderID)
			}
			if folderID == 0 {
				if err := h.db.QueryRow(`INSERT INTO file_items (user_id, folder_id, filename, is_folder, created_at, updated_at)
					VALUES ($1, $2, $3, true, $4, $4) RETURNING id`,
					userID, parentID, seg, now).Scan(&folderID); err != nil {
					utils.WriteError(w, http.StatusInternalServerError, "create folder failed")
					return
				}
			}
			id := folderID
			parentID = &id
		}
	}

	var fileID int64
	err = h.db.QueryRow(`INSERT INTO file_items (user_id, folder_id, filename, original_filename, file_path, file_size,
		 mime_type, file_type, file_hash, is_folder, created_at, updated_at)
		 VALUES ($1, $2, $3, $3, $4, $5, $6, $7, $8, false, $9, $9) RETURNING id`,
		userID, parentID, fileName, storagePath, req.FileSize,
		"application/octet-stream", detectFileTypeByName(fileName), fileHash, now,
	).Scan(&fileID)
	if err != nil {
		// 同名文件已存在：覆盖内容并复用它的 id。
		var existingID int64
		var oldHash string
		h.db.QueryRow(`SELECT id, coalesce(file_hash, '') FROM file_items WHERE user_id = $1 AND folder_id IS NOT DISTINCT FROM $2
			 AND filename = $3 AND is_folder = false AND deleted_at IS NULL`,
			userID, parentID, fileName,
		).Scan(&existingID, &oldHash)
		if existingID == 0 {
			utils.WriteError(w, http.StatusInternalServerError, "insert failed")
			return
		}
		// 覆盖前先把旧内容记进 file_versions —— 与网页端/WebDAV 上传走同一套留档。
		// 不补这一步的话，同步上传过的文件在服务端永远没有版本历史，
		// GET /files/{id}/versions 恒为空（这张表全仓库有 5 处写入者，唯独同步路径没有）。
		h.db.Exec(`INSERT INTO file_versions (file_id, version_num, file_path, file_size, file_hash, storage_driver, created_by, created_at)
			SELECT $1, COALESCE((SELECT MAX(version_num) FROM file_versions WHERE file_id = $1), 0) + 1,
			       file_path, file_size, file_hash, storage_driver, $2, $3 FROM file_items WHERE id = $1`,
			existingID, userID, now)

		// 内容变了就先把旧内容的引用释放掉。不释放的话 file_fingerprints
		// 的 reference_count 只增不减，旧 hash 下的 file_blocks 也永远不回收。
		if oldHash != "" && oldHash != fileHash {
			releaseFingerprint(h.db, oldHash)
		}
		h.db.Exec(`UPDATE file_items SET file_path = $1, file_hash = $2, file_size = $3, updated_at = $4 WHERE id = $5`,
			storagePath, fileHash, req.FileSize, now, existingID)
		fileID = existingID
	}

	// 记录「文件 → 块」清单：下一次增量同步据此算出哪些块可以跳过。
	h.db.Exec(`DELETE FROM file_blocks WHERE file_hash = $1`, fileHash)
	for _, b := range req.Blocks {
		h.db.Exec(`INSERT INTO file_blocks (file_hash, block_index, block_hash, block_offset, block_size)
			VALUES ($1, $2, $3, $4, $5)`, fileHash, b.Index, b.Hash, b.Offset, b.Size)
	}

	utils.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"file_id":   fileID,
		"file_hash": fileHash,
		"file_size": req.FileSize,
		"filename":  fileName,
		"path":      req.Path,
	})
}

// releaseFingerprint 释放一次 file_fingerprints 引用。
// 计数归零就删掉该条，file_blocks 会随外键 on delete cascade 一起清理，
// 块池里的内容本身留给运维侧回收（同一份内容可能仍被其他记录引用）。
func releaseFingerprint(db *sql.DB, hash string) {
	if hash == "" {
		return
	}
	if _, err := db.Exec(`UPDATE file_fingerprints SET reference_count = reference_count - 1 WHERE hash = $1`, hash); err != nil {
		return
	}
	db.Exec(`DELETE FROM file_fingerprints WHERE hash = $1 AND reference_count <= 0`, hash)
}

// detectFileTypeByName 按扩展名判定 file_type（与 SyncUpload 内联的判定保持一致）。
func detectFileTypeByName(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg":
		return "image"
	case ".mp4", ".webm", ".avi", ".mkv", ".mov":
		return "video"
	case ".mp3", ".wav", ".ogg", ".flac":
		return "audio"
	case ".doc", ".docx", ".pdf", ".txt", ".md":
		return "document"
	case ".zip", ".rar", ".7z", ".tar", ".gz":
		return "archive"
	}
	return "other"
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
		utils.WriteError(w, http.StatusInternalServerError, "storage failed")
		return
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

// SyncUploadAssign assigns a chunk-uploaded file to a target path.
func (h *BlockHandler) SyncUploadAssign(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		UploadID string `json:"upload_id"`
		Path     string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UploadID == "" || req.Path == "" {
		utils.WriteError(w, http.StatusBadRequest, "upload_id and path required")
		return
	}
	// Risk 1: Path traversal
	cleanPath := filepath.Clean(req.Path)
	if strings.HasPrefix(cleanPath, "..") || strings.Contains(cleanPath, "..") || cleanPath[0] == '/' || cleanPath[0] == '\\' {
		utils.WriteError(w, http.StatusBadRequest, "invalid path")
		return
	}
	req.Path = cleanPath
	fileName := filepath.Base(req.Path)
	dirPath := filepath.Dir(req.Path)

	tx, err := h.db.Begin()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "tx failed")
		return
	}
	defer tx.Rollback()

	// Risk 2+3: Verify ownership + prevent double-assign
	var uid int64
	var status string
	var fh, sp string
	var sz int64
	err = tx.QueryRow(`SELECT user_id,status,COALESCE(final_hash,''),COALESCE(storage_path,''),file_size FROM upload_tasks WHERE upload_id=$1 FOR UPDATE`, req.UploadID).Scan(&uid, &status, &fh, &sp, &sz)
	if err != nil {
		utils.WriteError(w, http.StatusNotFound, "upload not found")
		return
	}
	if uid != userID {
		utils.WriteError(w, http.StatusForbidden, "not your upload")
		return
	}
	if status != "pending" && status != "chunking" {
		utils.WriteError(w, http.StatusConflict, "already assigned")
		return
	}
	now := time.Now().Format(time.RFC3339)
	tx.Exec(`UPDATE upload_tasks SET status='completed',updated_at=$1 WHERE upload_id=$2`, now, req.UploadID)

	// Create folder hierarchy
	var parentID *int64
	if dirPath != "." {
		for _, seg := range strings.Split(dirPath, string(filepath.Separator)) {
			if seg == "" || seg == "." {
				continue
			}
			var fid int64
			if parentID != nil {
				tx.QueryRow(`SELECT id FROM file_items WHERE user_id=$1 AND folder_id=$2 AND filename=$3 AND is_folder=true AND deleted_at IS NULL`, userID, *parentID, seg).Scan(&fid)
			} else {
				tx.QueryRow(`SELECT id FROM file_items WHERE user_id=$1 AND folder_id IS NULL AND filename=$2 AND is_folder=true AND deleted_at IS NULL`, userID, seg).Scan(&fid)
			}
			if fid == 0 {
				tx.QueryRow(`INSERT INTO file_items (user_id,folder_id,filename,is_folder,created_at,updated_at) VALUES ($1,$2,$3,true,$4,$4) RETURNING id`, userID, parentID, seg, now).Scan(&fid)
			}
			parentID = &fid
		}
	}

	ext := strings.ToLower(filepath.Ext(fileName))
	ft := "other"
	mt := "application/octet-stream"
	switch ext {
	case ".jpg", "jpeg":
		ft = "image"
		mt = "image/jpeg"
	case ".png":
		ft = "image"
		mt = "image/png"
	case ".gif":
		ft = "image"
		mt = "image/gif"
	case ".mp4":
		ft = "video"
		mt = "video/mp4"
	case ".pdf":
		ft = "document"
		mt = "application/pdf"
	case ".doc", ".docx":
		ft = "document"
		mt = "application/msword"
	case ".mp3":
		ft = "audio"
		mt = "audio/mpeg"
	}

	// Create file record
	var fileID int64
	err = tx.QueryRow(`INSERT INTO file_items (user_id,folder_id,filename,original_filename,file_path,file_size,mime_type,file_type,file_hash,is_folder,created_at,updated_at) VALUES ($1,$2,$3,$3,$4,$5,$6,$7,$8,false,$9,$9) RETURNING id`,
		userID, parentID, fileName, sp, sz, mt, ft, fh, now).Scan(&fileID)
	if err != nil {
		// Risk 7: Duplicate filename - update existing
		var eid int64
		tx.QueryRow(`SELECT id FROM file_items WHERE user_id=$1 AND folder_id IS NOT DISTINCT FROM $2 AND filename=$3 AND is_folder=false AND deleted_at IS NULL`, userID, parentID, fileName).Scan(&eid)
		if eid > 0 {
			tx.Exec(`UPDATE file_items SET file_path=$1,file_hash=$2,file_size=$3,updated_at=$4 WHERE id=$5`, sp, fh, sz, now, eid)
			fileID = eid
		}
	}
	if fileID == 0 {
		utils.WriteError(w, http.StatusInternalServerError, "create file failed")
		return
	}

	// Risk 5: Fingerprint refcount
	tx.Exec(`INSERT INTO file_fingerprints (hash,file_size,mime_type,storage_path,reference_count,created_at,updated_at) VALUES ($1,$2,$3,$4,1,$5,$5) ON CONFLICT (hash) DO UPDATE SET reference_count=file_fingerprints.reference_count+1,updated_at=$5`, fh, sz, mt, sp, now)
	// Risk 6: Quota
	tx.Exec(`UPDATE users SET used_storage=used_storage+$1 WHERE id=$2`, sz, userID)

	if err := tx.Commit(); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "commit failed")
		return
	}
	EnqueueThumbnail(h.db, fileID)
	EnqueueOCR(h.db, fileID)
	ReindexSearch(h.db, fileID)
	utils.WriteJSON(w, http.StatusCreated, map[string]interface{}{"file_id": fileID, "file_hash": fh, "file_size": sz, "filename": fileName, "path": req.Path})
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

// folderPaths 把每个文件夹的 id 映射到它的完整路径（不带前导斜杠）。
// 根目录不会出现在结果里，调用方以空串表示。
//
// ponytail: 每次调用都会全量拉一次该用户的文件夹树，万级目录约几毫秒。若
// /sync/changes 成为热点，可改为递归 CTE 或按用户缓存；签名不用变。
func (h *BlockHandler) folderPaths(userID int64) map[int64]string {
	rows, err := h.db.Query(
		`SELECT id, filename, folder_id FROM file_items
		 WHERE user_id = $1 AND is_folder = true AND deleted_at IS NULL`,
		userID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type folder struct {
		name   string
		parent *int64
	}
	all := make(map[int64]folder)
	for rows.Next() {
		var id int64
		var name string
		var parent *int64
		if err := rows.Scan(&id, &name, &parent); err == nil {
			all[id] = folder{name: name, parent: parent}
		}
	}

	out := make(map[int64]string, len(all))
	var resolve func(id int64, depth int) string
	resolve = func(id int64, depth int) string {
		if depth > 64 { // 数据损坏形成环时的保护，避免无限递归
			return ""
		}
		if p, ok := out[id]; ok {
			return p
		}
		f, ok := all[id]
		if !ok {
			return ""
		}
		prefix := ""
		if f.parent != nil {
			prefix = resolve(*f.parent, depth+1)
		}
		p := f.name
		if prefix != "" {
			p = prefix + "/" + f.name
		}
		out[id] = p
		return p
	}
	for id := range all {
		resolve(id, 0)
	}
	return out
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
			`SELECT fi.id, fi.filename, fi.folder_id, fi.file_hash, fi.file_size, fi.updated_at,
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
			`SELECT fi.id, fi.filename, fi.folder_id, fi.file_hash, fi.file_size, fi.updated_at,
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

	// 同步客户端以「路径」为键，而这里天然只能查到 filename——所以把 folder_id
	// 沿父链还原成完整路径。没有它，客户端无法判断一个远端变更落在本地哪个目录
	// （远端新增的文件尤其如此）。
	folders := h.folderPaths(userID)

	type Change struct {
		ID       int64  `json:"id"`
		Filename string `json:"filename"`
		Path     string `json:"path"`
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
		var folderID *int64
		rows.Scan(&c.ID, &c.Filename, &folderID, &c.Hash, &c.Size, &t, &c.Action)
		c.Time = t
		dir := ""
		if folderID != nil {
			dir = folders[*folderID]
		}
		if dir != "" {
			c.Path = dir + "/" + c.Filename
		} else {
			c.Path = c.Filename
		}
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

// SyncUploadComplete - merge chunks + create folders + create file in one call.
func (h *BlockHandler) SyncUploadComplete(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		UploadID string `json:"upload_id"`
		Path     string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UploadID == "" || req.Path == "" {
		utils.WriteError(w, http.StatusBadRequest, "upload_id and path required")
		return
	}
	cleanPath := filepath.Clean(req.Path)
	if strings.HasPrefix(cleanPath, "..") || strings.Contains(cleanPath, "..") || cleanPath[0] == '/' || cleanPath[0] == '\\' {
		utils.WriteError(w, http.StatusBadRequest, "invalid path")
		return
	}
	req.Path = cleanPath
	fileName := filepath.Base(req.Path)
	dirPath := filepath.Dir(req.Path)
	var uID, uSize int64
	var uMime, uStatus string
	var total, recv int
	err := h.db.QueryRow(`SELECT user_id,file_size,COALESCE(mime_type,''),status,total_chunks,received_chunks FROM upload_tasks WHERE upload_id=$1`, req.UploadID).Scan(&uID, &uSize, &uMime, &uStatus, &total, &recv)
	if err != nil {
		utils.WriteError(w, http.StatusNotFound, "upload not found")
		return
	}
	if uID != userID {
		utils.WriteError(w, http.StatusForbidden, "not your upload")
		return
	}
	if uStatus == "completed" {
		utils.WriteError(w, http.StatusConflict, "already completed")
		return
	}
	dir := filepath.Join(h.tempDir, req.UploadID)
	var readers []io.Reader
	for i := 0; i < total; i++ {
		// Chunks are written by UploadChunk as "<index>.part" under the upload id.
		f, e := os.Open(filepath.Join(dir, fmt.Sprintf("%d.part", i)))
		if e != nil {
			utils.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("missing chunk %d", i))
			return
		}
		readers = append(readers, f)
		defer f.Close()
	}
	merged := io.NopCloser(io.MultiReader(readers...))
	fh, sp, err := h.storage.StoreHash(merged)
	merged.Close()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "storage failed")
		return
	}
	now := time.Now().Format(time.RFC3339)
	var parentID *int64
	if dirPath != "." {
		for _, seg := range strings.Split(dirPath, string(filepath.Separator)) {
			if seg == "" || seg == "." {
				continue
			}
			var fid int64
			if parentID != nil {
				h.db.QueryRow(`SELECT id FROM file_items WHERE user_id=$1 AND folder_id=$2 AND filename=$3 AND is_folder=true AND deleted_at IS NULL`, userID, *parentID, seg).Scan(&fid)
			} else {
				h.db.QueryRow(`SELECT id FROM file_items WHERE user_id=$1 AND folder_id IS NULL AND filename=$2 AND is_folder=true AND deleted_at IS NULL`, userID, seg).Scan(&fid)
			}
			if fid == 0 {
				h.db.QueryRow(`INSERT INTO file_items (user_id,folder_id,filename,is_folder,created_at,updated_at) VALUES ($1,$2,$3,true,$4,$4) RETURNING id`, userID, parentID, seg, now).Scan(&fid)
			}
			parentID = &fid
		}
	}
	ext := strings.ToLower(filepath.Ext(fileName))
	ft := "other"
	mt := "application/octet-stream"
	switch ext {
	case ".jpg", "jpeg":
		ft = "image"
		mt = "image/jpeg"
	case ".png":
		ft = "image"
		mt = "image/png"
	case ".mp4":
		ft = "video"
		mt = "video/mp4"
	case ".pdf":
		ft = "document"
		mt = "application/pdf"
	case ".doc", ".docx":
		ft = "document"
		mt = "application/msword"
	case ".mp3":
		ft = "audio"
		mt = "audio/mpeg"
	}
	var fileID int64
	err = h.db.QueryRow(`INSERT INTO file_items (user_id,folder_id,filename,original_filename,file_path,file_size,mime_type,file_type,file_hash,is_folder,created_at,updated_at) VALUES ($1,$2,$3,$3,$4,$5,$6,$7,$8,false,$9,$9) RETURNING id`,
		userID, parentID, fileName, sp, uSize, mt, ft, fh, now).Scan(&fileID)
	if err != nil {
		var eid int64
		h.db.QueryRow(`SELECT id FROM file_items WHERE user_id=$1 AND folder_id IS NOT DISTINCT FROM $2 AND filename=$3 AND is_folder=false AND deleted_at IS NULL`, userID, parentID, fileName).Scan(&eid)
		if eid > 0 {
			h.db.Exec(`UPDATE file_items SET file_path=$1,file_hash=$2,file_size=$3,updated_at=$4 WHERE id=$5`, sp, fh, uSize, now, eid)
			fileID = eid
		}
	}
	if fileID == 0 {
		utils.WriteError(w, http.StatusInternalServerError, "create file failed")
		return
	}
	h.db.Exec(`INSERT INTO file_fingerprints (hash,file_size,mime_type,storage_path,reference_count,created_at,updated_at) VALUES ($1,$2,$3,$4,1,$5,$5) ON CONFLICT (hash) DO UPDATE SET reference_count=file_fingerprints.reference_count+1,updated_at=$5`, fh, uSize, mt, sp, now)
	h.db.Exec(`UPDATE users SET used_storage=used_storage+$1 WHERE id=$2`, uSize, userID)
	h.db.Exec(`UPDATE upload_tasks SET status='completed',final_hash=$1,storage_path=$2,updated_at=$3 WHERE upload_id=$4`, fh, sp, now, req.UploadID)
	go func() {
		h.db.Exec(`DELETE FROM upload_chunks WHERE upload_id=$1`, req.UploadID)
		h.db.Exec(`DELETE FROM upload_tasks WHERE upload_id=$1 AND status='completed'`, req.UploadID)
		os.RemoveAll(dir)
	}()
	EnqueueThumbnail(h.db, fileID)
	EnqueueOCR(h.db, fileID)
	ReindexSearch(h.db, fileID)
	utils.WriteJSON(w, http.StatusCreated, map[string]interface{}{"file_id": fileID, "file_hash": fh, "file_size": uSize, "filename": fileName, "path": req.Path})
}
