package files

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/Athenavi/Stora/pkg/storage"
	"github.com/Athenavi/Stora/pkg/utils"
)

// UploadSessionHandler manages OneDrive-style fragment upload sessions.
type UploadSessionHandler struct {
	db      *sql.DB
	storage storage.Driver
	tempDir string
}

func NewUploadSessionHandler(db *sql.DB, store storage.Driver, tempDir string) *UploadSessionHandler {
	return &UploadSessionHandler{db: db, storage: store, tempDir: tempDir}
}

// CreateSession - POST /api/v2/sync/upload/session
func (h *UploadSessionHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		Filename string `json:"filename"`
		FileSize int64  `json:"file_size"`
		Path     string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Filename == "" || req.FileSize <= 0 || req.Path == "" {
		utils.WriteError(w, http.StatusBadRequest, "filename, file_size, path required"); return
	}
	cleanPath := filepath.Clean(req.Path)
	if strings.HasPrefix(cleanPath, "..") || strings.Contains(cleanPath, "..") || cleanPath[0] == '/' || cleanPath[0] == '\\' {
		utils.WriteError(w, http.StatusBadRequest, "invalid path"); return
	}
	sid := uuid.New().String()
	now := time.Now().Format(time.RFC3339)
	_, err := h.db.Exec(
		`INSERT INTO upload_sessions (session_id,user_id,filename,file_path,file_size,received_bytes,status,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,0,'active',$6,$6)`,
		sid, userID, req.Filename, req.Path, req.FileSize, now)
	if err != nil { utils.WriteError(w, http.StatusInternalServerError, "create session failed"); return }
	os.MkdirAll(filepath.Join(h.tempDir, "sessions", sid), 0755)
	utils.WriteJSON(w, http.StatusCreated, map[string]interface{}{"session_id": sid})
}

// UploadFragment - PUT /api/v2/sync/upload/session/{sessionId}
func (h *UploadSessionHandler) UploadFragment(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "sessionId")

	var uid int64; var st string; var fn, pth string; var ts, rcvd int64
	err := h.db.QueryRow(`SELECT user_id,status,filename,file_path,file_size,received_bytes FROM upload_sessions WHERE session_id=$1 FOR UPDATE`, sid,
	).Scan(&uid, &st, &fn, &pth, &ts, &rcvd)
	if err != nil { utils.WriteError(w, http.StatusNotFound, "session not found"); return }
	if st == "completed" { utils.WriteError(w, http.StatusConflict, "already completed"); return }
	userID, _ := middleware.GetUserID(r.Context())
	if uid != userID { utils.WriteError(w, http.StatusForbidden, "not your session"); return }

	rh := r.Header.Get("Content-Range")
	if rh == "" { utils.WriteError(w, http.StatusBadRequest, "Content-Range required"); return }
	var s2, en int64
	if _, err := fmt.Sscanf(rh, "bytes %d-%d/%d", &s2, &en, &ts); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid Content-Range"); return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil || int64(len(data)) != (en-s2+1) {
		utils.WriteError(w, http.StatusBadRequest, "body size mismatch"); return
	}

	fragDir := filepath.Join(h.tempDir, "sessions", sid)
	os.MkdirAll(fragDir, 0755)
	os.WriteFile(filepath.Join(fragDir, fmt.Sprintf("%020d", s2)), data, 0644)

	nr := rcvd + int64(len(data))
	h.db.Exec(`UPDATE upload_sessions SET received_bytes=$1,updated_at=$2 WHERE session_id=$3`, nr, time.Now().Format(time.RFC3339), sid)

	if nr >= ts {
		h.finalize(sid, userID, fn, pth, ts, fragDir)
		utils.WriteJSON(w, http.StatusOK, map[string]interface{}{"status": "completed", "session_id": sid})
	} else {
		utils.WriteJSON(w, http.StatusAccepted, map[string]interface{}{"status": "active", "received": nr, "total": ts})
	}
}

// GetSessionStatus - GET /api/v2/sync/upload/session/{sessionId}
func (h *UploadSessionHandler) GetSessionStatus(w http.ResponseWriter, r *http.Request) {
	sid := chi.URLParam(r, "sessionId")
	var st string; var rcvd, total int64
	err := h.db.QueryRow(`SELECT status,received_bytes,file_size FROM upload_sessions WHERE session_id=$1`, sid,
	).Scan(&st, &rcvd, &total)
	if err != nil { utils.WriteError(w, http.StatusNotFound, "session not found"); return }
	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{"status": st, "received": rcvd, "total": total})
}

func (h *UploadSessionHandler) finalize(sid string, uid int64, fn, pth string, totalSize int64, fragDir string) {
	var readers []io.Reader
	for i := int64(0); i < totalSize; {
		data, err := os.ReadFile(filepath.Join(fragDir, fmt.Sprintf("%020d", i)))
		if err != nil { return }
		readers = append(readers, bytes.NewReader(data))
		i += int64(len(data))
	}
	fh, sp, err := h.storage.StoreHash(io.MultiReader(readers...))
	if err != nil { return }
	now := time.Now().Format(time.RFC3339)

	dp := filepath.Dir(pth)
	var pid *int64
	if dp != "." {
		for _, seg := range strings.Split(dp, string(filepath.Separator)) {
			if seg == "" || seg == "." { continue }
			var fid int64
			if pid != nil { h.db.QueryRow(`SELECT id FROM file_items WHERE user_id=$1 AND folder_id=$2 AND filename=$3 AND is_folder=true AND deleted_at IS NULL`, uid, *pid, seg).Scan(&fid) } else { h.db.QueryRow(`SELECT id FROM file_items WHERE user_id=$1 AND folder_id IS NULL AND filename=$2 AND is_folder=true AND deleted_at IS NULL`, uid, seg).Scan(&fid) }
			if fid == 0 { h.db.QueryRow(`INSERT INTO file_items (user_id,folder_id,filename,is_folder,created_at,updated_at) VALUES ($1,$2,$3,true,$4,$4) RETURNING id`, uid, pid, seg, now).Scan(&fid) }
			pid = &fid
		}
	}
	var fileID int64
	err = h.db.QueryRow(`INSERT INTO file_items (user_id,folder_id,filename,original_filename,file_path,file_size,mime_type,file_type,file_hash,is_folder,created_at,updated_at) VALUES ($1,$2,$3,$3,$4,$5,$6,$7,$8,false,$9,$9) RETURNING id`,
		uid, pid, fn, sp, totalSize, "application/octet-stream", "other", fh, now).Scan(&fileID)
	if err != nil {
		var eid int64; h.db.QueryRow(`SELECT id FROM file_items WHERE user_id=$1 AND folder_id IS NOT DISTINCT FROM $2 AND filename=$3 AND is_folder=false AND deleted_at IS NULL`, uid, pid, fn).Scan(&eid)
		if eid > 0 { h.db.Exec(`UPDATE file_items SET file_path=$1,file_hash=$2,file_size=$3,updated_at=$4 WHERE id=$5`, sp, fh, totalSize, now, eid) }
	}
	h.db.Exec(`UPDATE upload_sessions SET status='completed',updated_at=$1 WHERE session_id=$2`, now, sid)
	os.RemoveAll(fragDir)
}

// Ensure uuid import is available
var _ = strconv.Itoa
var _ = hex.EncodeToString
var _ = sha256.Sum256
