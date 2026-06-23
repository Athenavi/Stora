package files

import (
	"crypto/sha256"
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
	"github.com/google/uuid"
)

type UploadHandler struct {
	db      *sql.DB
	storage storage.Driver
	tempDir string
}

func NewUploadHandler(db *sql.DB, store storage.Driver, tempDir string) *UploadHandler {
	return &UploadHandler{db: db, storage: store, tempDir: tempDir}
}

// ---------- Init Upload ----------

func (h *UploadHandler) InitUpload(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		r.ParseForm()
	}

	filename := r.FormValue("filename")
	fileSizeStr := r.FormValue("file_size")
	chunkSizeStr := r.FormValue("chunk_size")
	totalChunksStr := r.FormValue("total_chunks")

	if filename == "" || fileSizeStr == "" || chunkSizeStr == "" || totalChunksStr == "" {
		utils.WriteError(w, http.StatusBadRequest, "filename, file_size, chunk_size, total_chunks required")
		return
	}

	fileSize, _ := strconv.ParseInt(fileSizeStr, 10, 64)
	chunkSize, _ := strconv.Atoi(chunkSizeStr)
	totalChunks, _ := strconv.Atoi(totalChunksStr)
	mimeType := r.FormValue("mime_type")
	folderIDStr := r.FormValue("folder_id")

	uploadID := uuid.New().String()

	// Create temp directory
	tmpDir := filepath.Join(h.tempDir, uploadID)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "failed to create temp dir")
		return
	}

	// Save upload task
	now := time.Now().Format(time.RFC3339)
	_, err := h.db.Exec(
		`INSERT INTO upload_tasks (upload_id, user_id, filename, file_size, chunk_size, total_chunks,
		                           received_chunks, status, mime_type, folder_id,
		                           first_chunk_hash, last_chunk_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 0, 'pending', $7, $8, $9, $10, $11, $11)`,
		uploadID, userID, filename, fileSize, chunkSize, totalChunks,
		mimeType, folderIDStr, r.FormValue("first_chunk_hash"), r.FormValue("last_chunk_hash"), now,
	)
	if err != nil {
		os.RemoveAll(tmpDir)
		utils.WriteError(w, http.StatusInternalServerError, "failed to create upload task")
		return
	}

	// Check for already-uploaded chunks (resume support)
	receivedChunks, err := h.getReceivedChunks(uploadID)
	if err != nil {
		receivedChunks = []int{}
	}

	utils.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"upload_id":       uploadID,
		"chunk_size":      chunkSize,
		"total_chunks":    totalChunks,
		"received_chunks": receivedChunks,
		"resume":          len(receivedChunks) > 0,
	})
}

// ---------- Upload Chunk ----------

func (h *UploadHandler) UploadChunk(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "uploadId")
	chunkIndexStr := chi.URLParam(r, "index")

	var body []byte
	ct := r.Header.Get("Content-Type")

	if uploadID == "" || strings.HasPrefix(ct, "multipart/form-data") {
		// Frontend format: POST /files/upload/chunk with FormData
		if err := r.ParseMultipartForm(100 << 20); err != nil {
			utils.WriteError(w, http.StatusBadRequest, "invalid form")
			return
		}
		uploadID = r.FormValue("upload_id")
		chunkIndexStr = r.FormValue("chunk_index")

		file, _, err := r.FormFile("chunk")
		if err != nil {
			utils.WriteError(w, http.StatusBadRequest, "chunk field required")
			return
		}
		defer file.Close()
		body, err = io.ReadAll(file)
		if err != nil {
			utils.WriteError(w, http.StatusInternalServerError, "failed to read chunk")
			return
		}
	} else {
		// Legacy format: PUT /files/upload/{uploadId}/chunk/{index} with raw body
		var err error
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 100<<20))
		if err != nil {
			utils.WriteError(w, http.StatusBadRequest, "failed to read chunk body")
			return
		}
		defer r.Body.Close()
	}

	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid chunk index")
		return
	}

	// Verify upload task exists
	var status string
	err = h.db.QueryRow(`SELECT status FROM upload_tasks WHERE upload_id = $1`, uploadID).Scan(&status)
	if err == sql.ErrNoRows {
		utils.WriteError(w, http.StatusNotFound, "upload not found")
		return
	}
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "query failed")
		return
	}
	if status == "completed" {
		utils.WriteError(w, http.StatusConflict, "upload already completed")
		return
	}

	// Compute SHA256 of chunk
	chunkHash := fmt.Sprintf("%x", sha256.Sum256(body))

	// Check if chunk already exists (resume dedup)
	var exists int
	h.db.QueryRow(
		`SELECT COUNT(*) FROM upload_chunks WHERE upload_id = $1 AND chunk_index = $2 AND chunk_hash = $3`,
		uploadID, chunkIndex, chunkHash,
	).Scan(&exists)

	if exists > 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"received":    true,
			"chunk_index": chunkIndex,
			"hash":        chunkHash,
		})
		return
	}

	// Write chunk to temp dir
	chunkPath := filepath.Join(h.tempDir, uploadID, fmt.Sprintf("%d.part", chunkIndex))
	if err := os.MkdirAll(filepath.Dir(chunkPath), 0755); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "failed to create chunk dir")
		return
	}
	if err := os.WriteFile(chunkPath, body, 0644); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "failed to write chunk")
		return
	}

	// Save chunk metadata
	now := time.Now().Format(time.RFC3339)
	_, err = h.db.Exec(
		`INSERT INTO upload_chunks (upload_id, chunk_index, chunk_hash, chunk_size, chunk_path, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		uploadID, chunkIndex, chunkHash, len(body), chunkPath, now,
	)
	if err != nil {
		os.Remove(chunkPath)
		utils.WriteError(w, http.StatusInternalServerError, "failed to save chunk metadata")
		return
	}

	// Update received count
	h.db.Exec(
		`UPDATE upload_tasks SET received_chunks = received_chunks + 1, status = 'uploading', updated_at = $1
		 WHERE upload_id = $2`, now, uploadID,
	)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"received":    true,
		"chunk_index": chunkIndex,
		"hash":        chunkHash,
	})
}

// ---------- Upload Status ----------

func (h *UploadHandler) UploadStatus(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "uploadId")

	var filename string
	var fileSize, totalChunks, receivedChunks int64
	err := h.db.QueryRow(
		`SELECT filename, file_size, total_chunks, received_chunks
		 FROM upload_tasks WHERE upload_id = $1`,
		uploadID,
	).Scan(&filename, &fileSize, &totalChunks, &receivedChunks)

	if err == sql.ErrNoRows {
		utils.WriteError(w, http.StatusNotFound, "upload not found")
		return
	}
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "query failed")
		return
	}

	chunks, err := h.getReceivedChunks(uploadID)
	if err != nil {
		chunks = []int{}
	}

	progress := 0.0
	if totalChunks > 0 {
		progress = float64(receivedChunks) / float64(totalChunks) * 100
	}

	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"upload_id":       uploadID,
		"filename":        filename,
		"file_size":       fileSize,
		"total_chunks":    totalChunks,
		"received_chunks": receivedChunks,
		"chunks":          chunks,
		"progress_pct":    progress,
	})
}

// ---------- Complete Upload ----------

func (h *UploadHandler) CompleteUpload(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "uploadId")

	// Fallback: read from JSON body (frontend sends POST /files/upload/complete)
	if uploadID == "" {
		var req struct {
			UploadID string `json:"upload_id"`
			FolderID *int64 `json:"folder_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UploadID == "" {
			utils.WriteError(w, http.StatusBadRequest, "upload_id required")
			return
		}
		uploadID = req.UploadID
	}

	// Get upload task
	var task struct {
		UserID             int64
		Filename           string
		FileSize           int64
		TotalChunks       int
		ReceivedChunks    int
		ChunkSize         int
		MimeType          string
		FolderID          sql.NullString
	}

	err := h.db.QueryRow(
		`SELECT user_id, filename, file_size, total_chunks, received_chunks, chunk_size,
		        COALESCE(mime_type, ''), COALESCE(folder_id, '')
		 FROM upload_tasks WHERE upload_id = $1 AND status != 'completed'`,
		uploadID,
	).Scan(&task.UserID, &task.Filename, &task.FileSize, &task.TotalChunks,
		&task.ReceivedChunks, &task.ChunkSize, &task.MimeType, &task.FolderID)

	if err == sql.ErrNoRows {
		utils.WriteError(w, http.StatusNotFound, "upload not found or already completed")
		return
	}
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "query failed")
		return
	}

	// Verify all chunks received
	if task.ReceivedChunks < task.TotalChunks {
		utils.WriteError(w, http.StatusBadRequest,
			fmt.Sprintf("incomplete upload: %d/%d chunks received", task.ReceivedChunks, task.TotalChunks))
		return
	}

	// Merge all chunks in order
	tmpDir := filepath.Join(h.tempDir, uploadID)
	mergedReader, err := h.mergeChunks(tmpDir, task.TotalChunks)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "failed to merge chunks")
		return
	}

	// Store via content-addressable storage
	fileHash, storagePath, err := h.storage.StoreHash(mergedReader)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "storage failed")
		return
	}

	// Update fingerprint
	now := time.Now().Format(time.RFC3339)
	var existingID int64
	if err := h.db.QueryRow(`SELECT id FROM file_fingerprints WHERE hash = $1`, fileHash).Scan(&existingID); err == nil {
		h.db.Exec(`UPDATE file_fingerprints SET reference_count = reference_count + 1, updated_at = $1 WHERE id = $2`, now, existingID)
	} else {
		h.db.Exec(
			`INSERT INTO file_fingerprints (hash, file_size, mime_type, storage_path, reference_count, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, 1, $5, $5)`,
			fileHash, task.FileSize, task.MimeType, storagePath, now,
		)
	}

	// Create file item
	fileType := detectFileType(task.MimeType, task.Filename)
	var fileID int64
	err = h.db.QueryRow(
		`INSERT INTO file_items (user_id, filename, original_filename, file_path, file_size,
		                         mime_type, file_type, storage_driver, file_hash, is_folder, deleted_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'local', $8, false, NULL, $9, $9) RETURNING id`,
		task.UserID, task.Filename, task.Filename, storagePath, task.FileSize,
		task.MimeType, fileType, fileHash, now,
	).Scan(&fileID)

	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "failed to create file item")
		return
	}

	// Mark upload as completed and schedule cleanup
	h.db.Exec(`UPDATE upload_tasks SET status = 'completed', final_hash = $1, storage_path = $2, updated_at = $3 WHERE upload_id = $4`,
		fileHash, storagePath, now, uploadID)

	// Clean up temp files
	go func() {
		os.RemoveAll(tmpDir)
		h.db.Exec(`DELETE FROM upload_chunks WHERE upload_id = $1`, uploadID)
		h.db.Exec(`DELETE FROM upload_tasks WHERE upload_id = $1 AND status = 'completed'`, uploadID)
	}()

	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"file_id":   fileID,
		"file_hash": fileHash,
		"file_size": task.FileSize,
	})
}

// ---------- Cancel Upload ----------

func (h *UploadHandler) CancelUpload(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "uploadId")

	// Clean up temp dir
	tmpDir := filepath.Join(h.tempDir, uploadID)
	os.RemoveAll(tmpDir)

	// Clean up DB records
	h.db.Exec(`DELETE FROM upload_chunks WHERE upload_id = $1`, uploadID)
	result, err := h.db.Exec(`DELETE FROM upload_tasks WHERE upload_id = $1`, uploadID)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "failed to cancel upload")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		utils.WriteError(w, http.StatusNotFound, "upload not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "upload cancelled"})
}

// ---------- Helpers ----------

func (h *UploadHandler) getReceivedChunks(uploadID string) ([]int, error) {
	rows, err := h.db.Query(
		`SELECT chunk_index FROM upload_chunks WHERE upload_id = $1 ORDER BY chunk_index`,
		uploadID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []int
	for rows.Next() {
		var idx int
		if err := rows.Scan(&idx); err == nil {
			chunks = append(chunks, idx)
		}
	}
	return chunks, nil
}

func (h *UploadHandler) mergeChunks(tmpDir string, totalChunks int) (io.Reader, error) {
	// Collect all chunk files in order
	var readers []io.Reader
	for i := range totalChunks {
		chunkPath := filepath.Join(tmpDir, fmt.Sprintf("%d.part", i))
		f, err := os.Open(chunkPath)
		if err != nil {
			// Close already-opened files on error
			for _, r := range readers {
				if rc, ok := r.(io.ReadCloser); ok {
					rc.Close()
				}
			}
			return nil, fmt.Errorf("missing chunk %d: %w", i, err)
		}
		readers = append(readers, f)
	}
	return io.MultiReader(readers...), nil
}
