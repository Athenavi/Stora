import sys
p = sys.argv[1]
with open(p, 'r', encoding='utf-8') as f:
    c = f.read()

# Add SyncUploadComplete after SyncUploadAssign
marker = '\tutils.WriteJSON(w, http.StatusCreated, map[string]interface{}{"file_id":fileID,"file_hash":fh,"file_size":sz,"filename":fileName,"path":req.Path})\n}'

handler = '''

// SyncUploadComplete merges chunked upload and assigns to a target path in one atomic operation.
// POST /api/v2/sync/upload/complete
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

	// Path traversal check
	cleanPath := filepath.Clean(req.Path)
	if strings.HasPrefix(cleanPath, "..") || strings.Contains(cleanPath, "..") || cleanPath[0] == '/' || cleanPath[0] == '\\' {
		utils.WriteError(w, http.StatusBadRequest, "invalid path")
		return
	}
	req.Path = cleanPath
	fileName := filepath.Base(req.Path)
	dirPath := filepath.Dir(req.Path)

	// Get upload task and verify ownership
	var uid, uFileSize int64
	var uFilename, uMime, uStatus string
	var totalChunks, receivedChunks, chunkSize int
	err := h.db.QueryRow(
		`SELECT user_id, filename, file_size, total_chunks, received_chunks, chunk_size, COALESCE(mime_type,''), status
		 FROM upload_tasks WHERE upload_id = $1`, req.UploadID,
	).Scan(&uid, &uFilename, &uFileSize, &totalChunks, &receivedChunks, &chunkSize, &uMime, &uStatus)
	if err != nil { utils.WriteError(w, http.StatusNotFound, "upload not found"); return }
	if uid != userID { utils.WriteError(w, http.StatusForbidden, "not your upload"); return }
	if uStatus == "completed" { utils.WriteError(w, http.StatusConflict, "already completed"); return }

	// Merge chunks
	tmpDir := filepath.Join(h.tempDir, req.UploadID)
	merged, err := mergeChunks(tmpDir, totalChunks)
	if err != nil { utils.WriteError(w, http.StatusInternalServerError, "merge failed"); return }
	defer merged.Close()

	// Store content
	fileHash, storagePath, err := h.storage.StoreHash(merged)
	if err != nil { utils.WriteError(w, http.StatusInternalServerError, "storage failed"); return }

	now := time.Now().Format(time.RFC3339)

	// Create folder hierarchy
	var parentID *int64
	if dirPath != "." {
		for _, seg := range strings.Split(dirPath, string(filepath.Separator)) {
			if seg == "" || seg == "." { continue }
			var fid int64
			if parentID != nil { h.db.QueryRow(`SELECT id FROM file_items WHERE user_id=$1 AND folder_id=$2 AND filename=$3 AND is_folder=true AND deleted_at IS NULL`, userID, *parentID, seg).Scan(&fid) } else { h.db.QueryRow(`SELECT id FROM file_items WHERE user_id=$1 AND folder_id IS NULL AND filename=$2 AND is_folder=true AND deleted_at IS NULL`, userID, seg).Scan(&fid) }
			if fid == 0 { h.db.QueryRow(`INSERT INTO file_items (user_id,folder_id,filename,is_folder,created_at,updated_at) VALUES ($1,$2,$3,true,$4,$4) RETURNING id`, userID, parentID, seg, now).Scan(&fid) }
			parentID = &fid
		}
	}

	// Detect type
	ext := strings.ToLower(filepath.Ext(fileName))
	fileType := "other"
	mimeType := "application/octet-stream"
	switch ext { case ".jpg","jpeg": fileType="image"; mimeType="image/jpeg"; case ".png": fileType="image"; mimeType="image/png"; case ".gif": fileType="image"; mimeType="image/gif"; case ".mp4": fileType="video"; mimeType="video/mp4"; case ".pdf": fileType="document"; mimeType="application/pdf"; case ".doc",".docx": fileType="document"; mimeType="application/msword"; case ".mp3": fileType="audio"; mimeType="audio/mpeg" }

	// Create file item
	var fileID int64
	err = h.db.QueryRow(
		`INSERT INTO file_items (user_id,folder_id,filename,original_filename,file_path,file_size,mime_type,file_type,file_hash,is_folder,created_at,updated_at) VALUES ($1,$2,$3,$3,$4,$5,$6,$7,$8,false,$9,$9) RETURNING id`,
		userID, parentID, fileName, storagePath, uFileSize, mimeType, fileType, fileHash, now,
	).Scan(&fileID)
	if err != nil {
		// Duplicate - update existing
		var eid int64
		h.db.QueryRow(`SELECT id FROM file_items WHERE user_id=$1 AND folder_id IS NOT DISTINCT FROM $2 AND filename=$3 AND is_folder=false AND deleted_at IS NULL`, userID, parentID, fileName).Scan(&eid)
		if eid > 0 { h.db.Exec(`UPDATE file_items SET file_path=$1,file_hash=$2,file_size=$3,updated_at=$4 WHERE id=$5`, storagePath, fileHash, uFileSize, now, eid); fileID = eid }
	}
	if fileID == 0 { utils.WriteError(w, http.StatusInternalServerError, "create file failed"); return }

	// Fingerprint refcount + quota
	h.db.Exec(`INSERT INTO file_fingerprints (hash,file_size,mime_type,storage_path,reference_count,created_at,updated_at) VALUES ($1,$2,$3,$4,1,$5,$5) ON CONFLICT (hash) DO UPDATE SET reference_count=file_fingerprints.reference_count+1,updated_at=$5`, fileHash, uFileSize, mimeType, storagePath, now)
	h.db.Exec(`UPDATE users SET used_storage=used_storage+$1 WHERE id=$2`, uFileSize, userID)

	// Mark completed + cleanup
	h.db.Exec(`UPDATE upload_tasks SET status='completed',final_hash=$1,storage_path=$2,updated_at=$3 WHERE upload_id=$4`, fileHash, storagePath, now, req.UploadID)
	go func() {
		h.db.Exec(`DELETE FROM upload_chunks WHERE upload_id = $1`, req.UploadID)
		h.db.Exec(`DELETE FROM upload_tasks WHERE upload_id = $1`, req.UploadID)
		os.RemoveAll(tmpDir)
	}()

	utils.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"file_id": fileID, "file_hash": fileHash, "file_size": uFileSize,
		"filename": fileName, "path": req.Path,
	})
}

func mergeChunks(dir string, totalChunks int) (io.ReadCloser, error) {
	var readers []io.Reader
	for i := 0; i < totalChunks; i++ {
		chunkPath := filepath.Join(dir, fmt.Sprintf("chunk_%d", i))
		f, err := os.Open(chunkPath)
		if err != nil { return nil, fmt.Errorf("missing chunk %d: %v", i, err) }
		readers = append(readers, f)
	}
	return io.NopCloser(io.MultiReader(readers...)), nil
}
'''

c = c.replace(marker, marker + handler)

with open(p, 'w', encoding='utf-8') as f:
    f.write(c)
print('DONE')
