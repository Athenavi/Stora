package files

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
	"github.com/Athenavi/Stora/pkg/storage"
	"github.com/go-chi/chi/v5"
)

const (
	maxMonthlyDownloads = 10
	maxAutoRetries      = 3
	maxManualRetries    = 5
)

// OfflineDownloadHandler 处理离线下载任务
type OfflineDownloadHandler struct {
	db      *sql.DB
	storage storage.Driver
	tempDir string
}

func NewOfflineDownloadHandler(db *sql.DB, store storage.Driver, tempDir string) *OfflineDownloadHandler {
	return &OfflineDownloadHandler{db: db, storage: store, tempDir: tempDir}
}

// CreateDownloadTask 创建离线下载任务
func (h *OfflineDownloadHandler) CreateDownloadTask(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	var req struct {
		URL      string  `json:"url"`
		Filename *string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		writeError(w, http.StatusBadRequest, "url required")
		return
	}

	// Monthly quota check
	var monthlyCount int
	h.db.QueryRow(
		`SELECT COUNT(*) FROM download_tasks WHERE user_id = $1 AND created_at >= date_trunc('month', CURRENT_TIMESTAMP)`,
		userID,
	).Scan(&monthlyCount)
	if monthlyCount >= maxMonthlyDownloads {
		writeError(w, http.StatusTooManyRequests,
			fmt.Sprintf("月度离线下载次数已达上限 (%d次/月)", maxMonthlyDownloads))
		return
	}

	// Auto-create "离线下载" folder if it doesn't exist
	var offlineFolderID int64
	err := h.db.QueryRow(
		`SELECT id FROM folders WHERE user_id = $1 AND name = '离线下载' AND parent_id IS NULL`,
		userID,
	).Scan(&offlineFolderID)
	if err != nil {
		h.db.QueryRow(
			`INSERT INTO folders (user_id, name, parent_id, created_at, updated_at) VALUES ($1, '离线下载', NULL, $2, $2) RETURNING id`,
			userID, time.Now().Format(time.RFC3339),
		).Scan(&offlineFolderID)
	}

	now := time.Now().Format(time.RFC3339)
	var taskID int64
	err = h.db.QueryRow(
		`INSERT INTO download_tasks (user_id, url, filename, status, progress, retry_count, manual_retries, folder_id, created_at, updated_at)
		 VALUES ($1, $2, $3, 'pending', 0, 0, 0, $4, $5, $5) RETURNING id`,
		userID, req.URL, req.Filename, offlineFolderID, now,
	).Scan(&taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	// Start download in background
	go h.processDownload(taskID, userID, req.URL, req.Filename, offlineFolderID)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"task_id":           taskID,
		"monthly_remaining": maxMonthlyDownloads - monthlyCount - 1,
	})
}

// GetDownloadTask 查询单个任务状态
func (h *OfflineDownloadHandler) GetDownloadTask(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	taskID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	var url, filename, status string
	var progress int
	var retryCount, manualRetries int
	h.db.QueryRow(
		`SELECT url, COALESCE(filename, ''), status, progress, retry_count, manual_retries
		 FROM download_tasks WHERE id = $1 AND user_id = $2`,
		taskID, userID,
	).Scan(&url, &filename, &status, &progress, &retryCount, &manualRetries)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":             taskID,
		"url":            url,
		"filename":       filename,
		"status":         status,
		"progress":       progress,
		"retry_count":    retryCount,
		"manual_retries": manualRetries,
	})
}

// RetryDownloadTask 手动重试任务
func (h *OfflineDownloadHandler) RetryDownloadTask(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	taskID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	var url, status string
	var manualRetries int64
	err := h.db.QueryRow(
		`SELECT url, status, manual_retries FROM download_tasks WHERE id = $1 AND user_id = $2`,
		taskID, userID,
	).Scan(&url, &status, &manualRetries)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if status != "failed" {
		writeError(w, http.StatusBadRequest, "只能重试失败的任务")
		return
	}
	if manualRetries >= maxManualRetries {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("手动重试已达上限 (%d次)", maxManualRetries))
		return
	}

	h.db.Exec(`UPDATE download_tasks SET status = 'pending', progress = 0, retry_count = 0, manual_retries = manual_retries + 1 WHERE id = $1`, taskID)
	go h.processDownload(taskID, userID, url, nil, 0)

	writeJSON(w, http.StatusOK, map[string]string{"message": "retrying"})
}

// ListDownloadTasks 列出用户的下载任务
func (h *OfflineDownloadHandler) ListDownloadTasks(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	rows, err := h.db.Query(
		`SELECT id, url, COALESCE(filename, ''), status, progress, retry_count, manual_retries, created_at, updated_at
		 FROM download_tasks WHERE user_id = $1 ORDER BY created_at DESC LIMIT 20`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type Task struct {
		ID           int64   `json:"id"`
		URL          string  `json:"url"`
		Filename     string  `json:"filename"`
		Status       string  `json:"status"`
		Progress     int     `json:"progress"`
		RetryCount   int     `json:"retry_count"`
		ManualRetry  int     `json:"manual_retries"`
		CreatedAt    *string `json:"created_at"`
		UpdatedAt    *string `json:"updated_at"`
	}
	var tasks = make([]Task, 0)
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.URL, &t.Filename, &t.Status, &t.Progress, &t.RetryCount, &t.ManualRetry, &t.CreatedAt, &t.UpdatedAt); err == nil {
			tasks = append(tasks, t)
		}
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (h *OfflineDownloadHandler) processDownload(taskID int64, userID int64, url string, filename *string, folderID int64) {
	now := time.Now().Format(time.RFC3339)
	h.db.Exec(`UPDATE download_tasks SET status = 'downloading', updated_at = $1 WHERE id = $2`, now, taskID)

	// Handle magnet links via aria2c
	if strings.HasPrefix(url, "magnet:") {
		h.processMagnet(taskID, userID, url, filename, now)
		return
	}

	// Download with retries
	var lastErr error
	for attempt := 0; attempt <= maxAutoRetries; attempt++ {
		if attempt > 0 {
			h.db.Exec(`UPDATE download_tasks SET retry_count = $1, status = 'downloading', progress = 0, updated_at = $2 WHERE id = $3`,
				attempt, now, taskID)
			time.Sleep(time.Duration(attempt*5) * time.Second) // progressive delay
		}

		lastErr = h.tryDownload(taskID, userID, url, filename, folderID, now)
		if lastErr == nil {
			return // success
		}
	}

	// All retries exhausted
	errMsg := lastErr.Error()
	if len(errMsg) > 500 {
		errMsg = errMsg[:500]
	}
	h.db.Exec(`UPDATE download_tasks SET status = 'failed', error_msg = $1, updated_at = $2 WHERE id = $3`,
		errMsg, now, taskID)
	log.Printf("[OfflineDownload] task %d failed after %d retries: %v", taskID, maxAutoRetries, lastErr)
}

func (h *OfflineDownloadHandler) tryDownload(taskID, userID int64, url string, filename *string, folderID int64, now string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Determine filename
	fname := ""
	if filename != nil && *filename != "" {
		fname = *filename
	} else if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, after, ok := strings.Cut(cd, "filename="); ok {
			fname = strings.Trim(after, "\" ")
		}
	}
	if fname == "" {
		parts := strings.Split(url, "/")
		fname = parts[len(parts)-1]
		if fname == "" {
			fname = fmt.Sprintf("download_%d", taskID)
		}
	}

	// Get content length for progress
	contentLen := resp.ContentLength
	knownSize := contentLen > 0

	// Stream to temp file
	tmpFile := filepath.Join(h.tempDir, fmt.Sprintf("offline_%d_%d", userID, taskID))
	f, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	var written int64
	buf := make([]byte, 32768)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				f.Close()
				os.Remove(tmpFile)
				return fmt.Errorf("write temp file: %w", writeErr)
			}
			written += int64(n)
			if knownSize {
				pct := int(written * 100 / contentLen)
				h.db.Exec(`UPDATE download_tasks SET progress = $1, updated_at = $2 WHERE id = $3`, pct, now, taskID)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			f.Close()
			os.Remove(tmpFile)
			return fmt.Errorf("read response: %w", readErr)
		}
	}
	f.Close()

	// If size unknown, set progress to indicate "in progress but unknown"
	if !knownSize {
		h.db.Exec(`UPDATE download_tasks SET progress = -1, updated_at = $1 WHERE id = $2`, now, taskID)
	}

	// Store via content-addressable storage
	fReader, err := os.Open(tmpFile)
	if err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("open temp: %w", err)
	}
	defer fReader.Close()
	defer os.Remove(tmpFile)

	fileHash, storagePath, err := h.storage.StoreHash(fReader)
	if err != nil {
		return fmt.Errorf("store hash: %w", err)
	}

	// Create file item linked to the "离线下载" folder
	mimeType := resp.Header.Get("Content-Type")
	fileType := detectFileType(mimeType, fname)
	var fileID int64
	err = h.db.QueryRow(
		`INSERT INTO file_items (user_id, folder_id, filename, original_filename, file_path, file_size,
		                         mime_type, file_type, storage_driver, file_hash, is_folder, deleted_at, description, file_url, duration, created_at, updated_at)
		 VALUES ($1, $2, $3, $3, $4, $5, $6, $7, 'local', $8, false, NULL, NULL, '', 0, $9, $9) RETURNING id`,
		userID, folderID, fname, storagePath, written, mimeType, fileType, fileHash, now,
	).Scan(&fileID)
	if err != nil {
		return fmt.Errorf("insert file item: %w", err)
	}

	// Update task as completed
	h.db.Exec(
		`UPDATE download_tasks SET status = 'completed', progress = 100, file_id = $1, updated_at = $2 WHERE id = $3`,
		fileID, now, taskID,
	)
	log.Printf("[OfflineDownload] completed task %d: %s (%d bytes)", taskID, fname, written)
	return nil
}

// processMagnet handles BitTorrent magnet links via aria2c.
func (h *OfflineDownloadHandler) processMagnet(taskID, userID int64, url string, filename *string, now string) {
	h.db.Exec(`UPDATE download_tasks SET status = 'downloading', updated_at = $1 WHERE id = $2`, now, taskID)

	tmpDir := filepath.Join(h.tempDir, fmt.Sprintf("bt_%d", taskID))
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("aria2c", "--bt-metadata-only=true", "--bt-save-metadata=true",
		"-d", tmpDir, url)
	output, err := cmd.CombinedOutput()
	if err != nil {
		h.db.Exec(`UPDATE download_tasks SET status = 'failed', updated_at = $1 WHERE id = $2`, now, taskID)
		log.Printf("[BT] aria2c failed for task %d: %v - %s", taskID, err, string(output))
		return
	}

	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".torrent") {
			torrentPath := filepath.Join(tmpDir, e.Name())
			dataDir := filepath.Join(tmpDir, "data")
			os.MkdirAll(dataDir, 0755)

			cmd2 := exec.Command("aria2c", "--seed-time=0", "-d", dataDir, torrentPath)
			output2, err := cmd2.CombinedOutput()
			if err != nil {
				h.db.Exec(`UPDATE download_tasks SET status = 'failed', updated_at = $1 WHERE id = $2`, now, taskID)
				log.Printf("[BT] aria2c download failed for task %d: %v - %s", taskID, err, string(output2))
				return
			}
			h.storeDownloadedFiles(taskID, userID, dataDir, now)
			return
		}
	}
	h.db.Exec(`UPDATE download_tasks SET status = 'failed', updated_at = $1 WHERE id = $2`, now, taskID)
}

func (h *OfflineDownloadHandler) storeDownloadedFiles(taskID, userID int64, dir, now string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			h.storeDownloadedFiles(taskID, userID, filepath.Join(dir, e.Name()), now)
			continue
		}
		fullPath := filepath.Join(dir, e.Name())
		f, err := os.Open(fullPath)
		if err != nil {
			continue
		}
		fileHash, storagePath, err := h.storage.StoreHash(f)
		f.Close()
		if err != nil {
			continue
		}
		fi, _ := os.Stat(fullPath)
		var fileSize int64
		if fi != nil {
			fileSize = fi.Size()
		}
		mimeType := detectFileType("", e.Name())
		fileType := detectFileType("", e.Name())
		var fileID int64
		h.db.QueryRow(
			`INSERT INTO file_items (user_id, folder_id, filename, original_filename, file_path, file_size,
			                         mime_type, file_type, storage_driver, file_hash, is_folder, deleted_at, description, file_url, duration, created_at, updated_at)
			 VALUES ($1, NULL, $2, $2, $3, $4, $5, $6, 'local', $7, false, NULL, NULL, '', 0, $8, $8) RETURNING id`,
			userID, e.Name(), storagePath, fileSize, mimeType, fileType, fileHash, now,
		).Scan(&fileID)
	}
	h.db.Exec(`UPDATE download_tasks SET status = 'completed', progress = 100, updated_at = $1 WHERE id = $2`, now, taskID)
	log.Printf("[BT] completed task %d", taskID)
}
