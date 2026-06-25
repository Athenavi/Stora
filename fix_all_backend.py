import sys, os

# Fix blocks.go
p = sys.argv[1]
with open(p, 'r', encoding='utf-8') as f:
    c = f.read()

# 1. Add tempDir to struct
c = c.replace(
    'type BlockHandler struct {\n\tdb      *sql.DB\n\tstorage storage.Driver\n}\n\nfunc NewBlockHandler(db *sql.DB, store storage.Driver) *BlockHandler {\n\treturn &BlockHandler{db: db, storage: store}\n}',
    'type BlockHandler struct {\n\tdb      *sql.DB\n\tstorage storage.Driver\n\ttempDir string\n}\n\nfunc NewBlockHandler(db *sql.DB, store storage.Driver, tempDir string) *BlockHandler {\n\treturn &BlockHandler{db: db, storage: store, tempDir: tempDir}\n}'
)

# 2. Add SyncUploadComplete handler at end of file
handler = '''
// SyncUploadComplete merges chunked upload and assigns to target path.
func (h *BlockHandler) SyncUploadComplete(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		UploadID string `json:"upload_id"`
		Path     string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UploadID == "" || req.Path == "" {
		utils.WriteError(w, http.StatusBadRequest, "upload_id and path required"); return
	}
	cleanPath := filepath.Clean(req.Path)
	if strings.HasPrefix(cleanPath, "..") || strings.Contains(cleanPath, "..") || cleanPath[0] == '/' || cleanPath[0] == '\\\\' {
		utils.WriteError(w, http.StatusBadRequest, "invalid path"); return
	}
	req.Path = cleanPath
	fileName := filepath.Base(req.Path)
	dirPath := filepath.Dir(req.Path)

	var uID, uSize int64; var uName, uMime, uStatus string; var total, recv int
	err = h.db.QueryRow(`SELECT user_id,file_size,COALESCE(mime_type,''),status,total_chunks,received_chunks FROM upload_tasks WHERE upload_id=$1`, req.UploadID,
	).Scan(&uID, &uSize, &uMime, &uStatus, &total, &recv)
	if err != nil { utils.WriteError(w, http.StatusNotFound, "upload not found"); return }
	if uID != userID { utils.WriteError(w, http.StatusForbidden, "not your upload"); return }
	if uStatus == "completed" { utils.WriteError(w, http.StatusConflict, "already completed"); return }

	dir := filepath.Join(h.tempDir, req.UploadID)
	var readers []io.Reader
	for i := 0; i < total; i++ {
		f, e := os.Open(filepath.Join(dir, fmt.Sprintf("chunk_%d", i)))
		if e != nil { utils.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("missing chunk %d", i)); return }
		readers = append(readers, f)
	}
	merged := io.NopCloser(io.MultiReader(readers...))
	fh, sp, err := h.storage.StoreHash(merged)
	merged.Close()
	if err != nil { utils.WriteError(w, http.StatusInternalServerError, "storage failed"); return }

	now := time.Now().Format(time.RFC3339)
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
	ext := strings.ToLower(filepath.Ext(fileName)); ft := "other"; mt := "application/octet-stream"
	switch ext { case ".jpg","jpeg": ft="image";mt="image/jpeg"; case ".png": ft="image";mt="image/png"; case ".mp4": ft="video";mt="video/mp4"; case ".pdf": ft="document";mt="application/pdf"; case ".doc",".docx": ft="document";mt="application/msword"; case ".mp3": ft="audio";mt="audio/mpeg" }

	var fileID int64
	err = h.db.QueryRow(`INSERT INTO file_items (user_id,folder_id,filename,original_filename,file_path,file_size,mime_type,file_type,file_hash,is_folder,created_at,updated_at) VALUES ($1,$2,$3,$3,$4,$5,$6,$7,$8,false,$9,$9) RETURNING id`,
		userID, parentID, fileName, sp, uSize, mt, ft, fh, now).Scan(&fileID)
	if err != nil {
		var eid int64; h.db.QueryRow(`SELECT id FROM file_items WHERE user_id=$1 AND folder_id IS NOT DISTINCT FROM $2 AND filename=$3 AND is_folder=false AND deleted_at IS NULL`, userID, parentID, fileName).Scan(&eid)
		if eid > 0 { h.db.Exec(`UPDATE file_items SET file_path=$1,file_hash=$2,file_size=$3,updated_at=$4 WHERE id=$5`, sp, fh, uSize, now, eid); fileID = eid }
	}
	if fileID == 0 { utils.WriteError(w, http.StatusInternalServerError, "create file failed"); return }

	h.db.Exec(`INSERT INTO file_fingerprints (hash,file_size,mime_type,storage_path,reference_count,created_at,updated_at) VALUES ($1,$2,$3,$4,1,$5,$5) ON CONFLICT (hash) DO UPDATE SET reference_count=file_fingerprints.reference_count+1,updated_at=$5`, fh, uSize, mt, sp, now)
	h.db.Exec(`UPDATE users SET used_storage=used_storage+$1 WHERE id=$2`, uSize, userID)
	h.db.Exec(`UPDATE upload_tasks SET status='completed',final_hash=$1,storage_path=$2,updated_at=$3 WHERE upload_id=$4`, fh, sp, now, req.UploadID)
	go func() { h.db.Exec(`DELETE FROM upload_chunks WHERE upload_id=$1`, req.UploadID); h.db.Exec(`DELETE FROM upload_tasks WHERE upload_id=$1 AND status='completed'`, req.UploadID); os.RemoveAll(dir) }()

	utils.WriteJSON(w, http.StatusCreated, map[string]interface{}{"file_id":fileID,"file_hash":fh,"file_size":uSize,"filename":fileName,"path":req.Path})
}
'''

c = c.rstrip() + handler

with open(p, 'w', encoding='utf-8') as f:
    f.write(c)
print('BLOCKS DONE')
