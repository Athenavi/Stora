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
	"strings"
	"time"

	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/Athenavi/Stora/pkg/storage"
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

	now := time.Now().Format(time.RFC3339)
	var taskID int64
	err := h.db.QueryRow(
		`INSERT INTO download_tasks (user_id, url, filename, status, progress, created_at, updated_at)
		 VALUES ($1, $2, $3, 'pending', 0, $4, $4) RETURNING id`,
		userID, req.URL, req.Filename, now,
	).Scan(&taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	// Start download in background
	go h.processDownload(taskID, userID, req.URL, req.Filename)

	writeJSON(w, http.StatusCreated, map[string]int64{"task_id": taskID, "user_id": userID})
}

// ListDownloadTasks 列出用户的下载任务
func (h *OfflineDownloadHandler) ListDownloadTasks(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	rows, err := h.db.Query(
		`SELECT id, url, COALESCE(filename, ''), status, progress, created_at, updated_at
		 FROM download_tasks WHERE user_id = $1 ORDER BY created_at DESC LIMIT 20`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type Task struct {
		ID        int64   `json:"id"`
		URL       string  `json:"url"`
		Filename  string  `json:"filename"`
		Status    string  `json:"status"`
		Progress  int     `json:"progress"`
		CreatedAt *string `json:"created_at"`
		UpdatedAt *string `json:"updated_at"`
	}
	var tasks = make([]Task, 0)
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.URL, &t.Filename, &t.Status, &t.Progress, &t.CreatedAt, &t.UpdatedAt); err == nil {
			tasks = append(tasks, t)
		}
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (h *OfflineDownloadHandler) processDownload(taskID int64, userID int64, url string, filename *string) {
	now := time.Now().Format(time.RFC3339)
	h.db.Exec(`UPDATE download_tasks SET status = 'downloading', updated_at = $1 WHERE id = $2`, now, taskID)

	// Handle magnet links via aria2c
	if strings.HasPrefix(url, "magnet:") {
		h.processMagnet(taskID, userID, url, filename, now)
		return
	}

	// Download the file
	resp, err := http.Get(url)
	if err != nil {
		h.db.Exec(`UPDATE download_tasks SET status = 'failed', updated_at = $1 WHERE id = $2`, now, taskID)
		log.Printf("[OfflineDownload] GET %s failed: %v", url, err)
		return
	}
	defer resp.Body.Close()

	// Determine filename from Content-Disposition or URL
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

	// Stream to a temp file first
	tmpFile := filepath.Join(h.tempDir, fmt.Sprintf("offline_%d_%d", userID, taskID))
	f, err := os.Create(tmpFile)
	if err != nil {
		h.db.Exec(`UPDATE download_tasks SET status = 'failed', updated_at = $1 WHERE id = $2`, now, taskID)
		log.Printf("[OfflineDownload] create temp file failed: %v", err)
		return
	}

	written, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmpFile)
		h.db.Exec(`UPDATE download_tasks SET status = 'failed', updated_at = $1 WHERE id = $2`, now, taskID)
		log.Printf("[OfflineDownload] download failed: %v", err)
		return
	}

	// Store via content-addressable storage
	fReader, err := os.Open(tmpFile)
	if err != nil {
		os.Remove(tmpFile)
		h.db.Exec(`UPDATE download_tasks SET status = 'failed', updated_at = $1 WHERE id = $2`, now, taskID)
		return
	}
	defer fReader.Close()
	defer os.Remove(tmpFile)

	fileHash, storagePath, err := h.storage.StoreHash(fReader)
	if err != nil {
		h.db.Exec(`UPDATE download_tasks SET status = 'failed', updated_at = $1 WHERE id = $2`, now, taskID)
		log.Printf("[OfflineDownload] store failed: %v", err)
		return
	}

	// Create file item
	mimeType := resp.Header.Get("Content-Type")
	fileType := detectFileType(mimeType, fname)
	var fileID int64
	h.db.QueryRow(
		`INSERT INTO file_items (user_id, filename, original_filename, file_path, file_size,
		                         mime_type, file_type, storage_driver, file_hash, is_folder, created_at, updated_at)
		 VALUES ($1, $2, $2, $3, $4, $5, $6, 'local', $7, false, $8, $8) RETURNING id`,
		userID, fname, storagePath, written, mimeType, fileType, fileHash, now,
	).Scan(&fileID)

	// Update task as completed
	h.db.Exec(
		`UPDATE download_tasks SET status = 'completed', progress = 100, file_id = $1, updated_at = $2 WHERE id = $3`,
		fileID, now, taskID,
	)
	log.Printf("[OfflineDownload] completed task %d: %s (%d bytes)", taskID, fname, written)
}

// processMagnet handles BitTorrent magnet links via aria2c.
func (h *OfflineDownloadHandler) processMagnet(taskID, userID int64, url string, filename *string, now string) {
	h.db.Exec(`UPDATE download_tasks SET status = 'downloading', updated_at = $1 WHERE id = $2`, now, taskID)

	// Use aria2c for BT downloads (must be installed separately)
	// aria2c --bt-metadata-only=true --bt-save-metadata=true -d <dir> <magnet>
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

	// Find the downloaded .torrent file
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".torrent") {
			// Found torrent metadata — now download the actual data
			torrentPath := filepath.Join(tmpDir, e.Name())
			dataDir := filepath.Join(tmpDir, "data")
			os.MkdirAll(dataDir, 0755)

			cmd2 := exec.Command("aria2c", "--seed-time=0",
				"-d", dataDir, torrentPath)
			output2, err := cmd2.CombinedOutput()
			if err != nil {
				h.db.Exec(`UPDATE download_tasks SET status = 'failed', updated_at = $1 WHERE id = $2`, now, taskID)
				log.Printf("[BT] aria2c download failed for task %d: %v - %s", taskID, err, string(output2))
				return
			}

			// Find downloaded files and store them
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
			`INSERT INTO file_items (user_id, filename, original_filename, file_path, file_size,
			                         mime_type, file_type, storage_driver, file_hash, is_folder, created_at, updated_at)
			 VALUES ($1, $2, $2, $3, $4, $5, $6, 'local', $7, false, $8, $8) RETURNING id`,
			userID, e.Name(), storagePath, fileSize, mimeType, fileType, fileHash, now,
		).Scan(&fileID)
	}

	h.db.Exec(`UPDATE download_tasks SET status = 'completed', progress = 100, updated_at = $1 WHERE id = $2`, now, taskID)
	log.Printf("[BT] completed task %d", taskID)
}
