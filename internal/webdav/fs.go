package webdav

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"mime"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	fileapi "github.com/Athenavi/Stora/internal/api/v2/files"
	"github.com/Athenavi/Stora/pkg/audit"
	"github.com/Athenavi/Stora/pkg/storage"
	"golang.org/x/net/webdav"
)

// ── caller identity ───────────────────────────────────────────────────────
//
// webdav.FileSystem methods receive a context but no *http.Request, so the
// resolved identity travels in the context.

type caller struct {
	userID int64
	ip     string
}

type callerKey struct{}

func withCaller(ctx context.Context, c caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

func callerOf(ctx context.Context) (caller, error) {
	c, ok := ctx.Value(callerKey{}).(caller)
	if !ok || c.userID == 0 {
		return caller{}, os.ErrPermission
	}
	return c, nil
}

// ── file info ─────────────────────────────────────────────────────────────

type fileInfo struct {
	name    string
	size    int64
	modTime time.Time
	isDir   bool
}

func (fi fileInfo) Name() string { return fi.name }
func (fi fileInfo) Size() int64  { return fi.size }
func (fi fileInfo) Mode() os.FileMode {
	if fi.isDir {
		return os.ModeDir | 0o755
	}
	return 0o644
}
func (fi fileInfo) ModTime() time.Time { return fi.modTime }
func (fi fileInfo) IsDir() bool        { return fi.isDir }
func (fi fileInfo) Sys() interface{}   { return nil }

// node is the file_items row a WebDAV path resolves to.
type node struct {
	id       int64 // 0 == the virtual root
	name     string
	isFolder bool
	size     int64
	storeKey string // storage key; files only
	modTime  time.Time
}

func (n node) fileInfo() os.FileInfo {
	return fileInfo{name: n.name, size: n.size, modTime: n.modTime, isDir: n.isFolder}
}

// ── filesystem ────────────────────────────────────────────────────────────

type fileSystem struct {
	db      *sql.DB
	storage storage.Driver
}

func (f *fileSystem) Mkdir(ctx context.Context, name string, _ os.FileMode) error {
	c, err := callerOf(ctx)
	if err != nil {
		return err
	}
	dir, base := splitPath(name)
	if base == "" {
		return os.ErrExist // the root always exists
	}
	parentID, err := f.resolveFolder(ctx, c.userID, dir)
	if err != nil {
		return err
	}

	if _, err := f.lookup(ctx, c.userID, name); err == nil {
		return os.ErrExist
	}

	now := time.Now().Format(time.RFC3339)
	_, err = f.db.ExecContext(ctx,
		`INSERT INTO file_items (user_id, folder_id, filename, is_folder, created_at, updated_at)
		 VALUES ($1, $2, $3, true, $4, $4)`,
		c.userID, nullableID(parentID), base, now)
	return err
}

func (f *fileSystem) OpenFile(ctx context.Context, name string, flag int, _ os.FileMode) (webdav.File, error) {
	c, err := callerOf(ctx)
	if err != nil {
		return nil, err
	}
	writable := flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_APPEND) != 0

	n, err := f.lookup(ctx, c.userID, name)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) || !writable {
			return nil, err
		}
		// A new file: hand back a writer that materialises it on Close.
		return f.newWriteFile(ctx, c.userID, name), nil
	}
	if n.isFolder {
		if writable {
			return nil, os.ErrPermission // WebDAV has no "write into a folder"
		}
		return &dirFile{fs: f, ctx: ctx, userID: c.userID, node: n}, nil
	}
	if writable {
		return f.newWriteFile(ctx, c.userID, name), nil
	}
	return f.openReadFile(ctx, n)
}

func (f *fileSystem) RemoveAll(ctx context.Context, name string) error {
	c, err := callerOf(ctx)
	if err != nil {
		return err
	}
	n, err := f.lookup(ctx, c.userID, name)
	if err != nil {
		return err
	}
	if n.id == 0 {
		return os.ErrPermission // never delete the root
	}

	// Soft delete, exactly like the web UI: it goes to the trash and the cleanup
	// worker purges it after the retention window.
	if _, err := f.db.ExecContext(ctx,
		`UPDATE file_items SET deleted_at = $1 WHERE id = $2 AND user_id = $3 AND deleted_at IS NULL`,
		time.Now().Format(time.RFC3339), n.id, c.userID); err != nil {
		return err
	}
	audit.Log(f.db, audit.Entry{
		UserID:    c.userID,
		Action:    "webdav.delete",
		Resource:  "path:" + cleanPath(name),
		IPAddress: c.ip,
	})
	return nil
}

func (f *fileSystem) Rename(ctx context.Context, oldName, newName string) error {
	c, err := callerOf(ctx)
	if err != nil {
		return err
	}
	src, err := f.lookup(ctx, c.userID, oldName)
	if err != nil {
		return err
	}
	if src.id == 0 {
		return os.ErrPermission
	}
	dir, base := splitPath(newName)
	if base == "" {
		return os.ErrPermission
	}
	dstParent, err := f.resolveFolder(ctx, c.userID, dir)
	if err != nil {
		return err
	}

	_, err = f.db.ExecContext(ctx,
		`UPDATE file_items SET filename = $1, original_filename = $1, folder_id = $2, updated_at = $3
		  WHERE id = $4 AND user_id = $5 AND is_folder = $6`,
		base, nullableID(dstParent), time.Now().Format(time.RFC3339), src.id, c.userID, src.isFolder)
	return err
}

func (f *fileSystem) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	c, err := callerOf(ctx)
	if err != nil {
		return nil, err
	}
	n, err := f.lookup(ctx, c.userID, name)
	if err != nil {
		return nil, err
	}
	return n.fileInfo(), nil
}

// ── path ↔ row resolution ─────────────────────────────────────────────────

// splitPath returns the parent directory and base name of a WebDAV path.
func splitPath(p string) (dir, base string) {
	clean := cleanPath(p)
	if clean == "" {
		return "", ""
	}
	dir, base = path.Split(clean)
	return strings.Trim(dir, "/"), base
}

func cleanPath(p string) string {
	return strings.Trim(path.Clean("/"+p), "/")
}

// lookup resolves a path to its file_items row; the root is a synthetic node.
func (f *fileSystem) lookup(ctx context.Context, userID int64, p string) (node, error) {
	dir, base := splitPath(p)
	if base == "" {
		return node{name: "/", isFolder: true}, nil
	}
	parentID, err := f.resolveFolder(ctx, userID, dir)
	if err != nil {
		return node{}, err
	}

	var (
		n        node
		row      *sql.Row
		isFolder bool
		updated  string
	)
	n.name = base
	if parentID == 0 {
		row = f.db.QueryRowContext(ctx,
			`SELECT id, is_folder, COALESCE(file_size, 0), COALESCE(file_path, ''), CAST(updated_at AS TEXT)
			   FROM file_items
			  WHERE user_id = $1 AND folder_id IS NULL AND filename = $2 AND deleted_at IS NULL`,
			userID, base)
	} else {
		row = f.db.QueryRowContext(ctx,
			`SELECT id, is_folder, COALESCE(file_size, 0), COALESCE(file_path, ''), CAST(updated_at AS TEXT)
			   FROM file_items
			  WHERE user_id = $1 AND folder_id = $2 AND filename = $3 AND deleted_at IS NULL`,
			userID, parentID, base)
	}
	if err := row.Scan(&n.id, &isFolder, &n.size, &n.storeKey, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return node{}, os.ErrNotExist
		}
		return node{}, err
	}
	n.isFolder = isFolder
	n.modTime, _ = time.Parse(time.RFC3339, updated)
	return n, nil
}

// resolveFolder walks a folder path one segment at a time. An empty path is the
// root, represented as id 0.
func (f *fileSystem) resolveFolder(ctx context.Context, userID int64, p string) (int64, error) {
	clean := cleanPath(p)
	if clean == "" {
		return 0, nil
	}
	var parent int64
	for _, seg := range strings.Split(clean, "/") {
		var (
			id  int64
			err error
		)
		if parent == 0 {
			err = f.db.QueryRowContext(ctx,
				`SELECT id FROM file_items
				  WHERE user_id = $1 AND folder_id IS NULL AND filename = $2 AND is_folder = true AND deleted_at IS NULL`,
				userID, seg).Scan(&id)
		} else {
			err = f.db.QueryRowContext(ctx,
				`SELECT id FROM file_items
				  WHERE user_id = $1 AND folder_id = $2 AND filename = $3 AND is_folder = true AND deleted_at IS NULL`,
				userID, parent, seg).Scan(&id)
		}
		if errors.Is(err, sql.ErrNoRows) {
			return 0, os.ErrNotExist
		}
		if err != nil {
			return 0, err
		}
		parent = id
	}
	return parent, nil
}

func (f *fileSystem) listChildren(ctx context.Context, userID, folderID int64) ([]os.FileInfo, error) {
	const cols = `SELECT COALESCE(filename, ''), is_folder, COALESCE(file_size, 0), CAST(updated_at AS TEXT)
	                FROM file_items
	               WHERE user_id = $1 AND deleted_at IS NULL AND `

	var (
		rows *sql.Rows
		err  error
	)
	if folderID == 0 {
		rows, err = f.db.QueryContext(ctx, cols+`folder_id IS NULL ORDER BY is_folder DESC, filename`, userID)
	} else {
		rows, err = f.db.QueryContext(ctx, cols+`folder_id = $2 ORDER BY is_folder DESC, filename`, userID, folderID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]os.FileInfo, 0)
	for rows.Next() {
		var (
			name     string
			isFolder bool
			size     int64
			updated  string
		)
		if err := rows.Scan(&name, &isFolder, &size, &updated); err != nil {
			continue
		}
		modTime, _ := time.Parse(time.RFC3339, updated)
		out = append(out, fileInfo{name: name, size: size, modTime: modTime, isDir: isFolder})
	}
	return out, rows.Err()
}

// ── webdav.File implementations ───────────────────────────────────────────

// readFile buffers the object into a temp file so the WebDAV layer can Seek it
// (Range requests, client retries). Buffering in memory would cap downloads by
// the server's RAM.
type readFile struct {
	*os.File
	info os.FileInfo
}

func (f *fileSystem) openReadFile(ctx context.Context, n node) (webdav.File, error) {
	src, err := f.storage.Retrieve(n.storeKey)
	if err != nil {
		return nil, os.ErrNotExist
	}
	defer src.Close()

	tmp, err := os.CreateTemp("", "stora-dav-read-*")
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, err
	}
	_ = ctx
	return &readFile{File: tmp, info: n.fileInfo()}, nil
}

func (rf *readFile) Readdir(int) ([]os.FileInfo, error) { return nil, os.ErrInvalid }
func (rf *readFile) Stat() (os.FileInfo, error)         { return rf.info, nil }

// Close also removes the temp buffer (which cannot be unlinked at creation time
// on Windows).
func (rf *readFile) Close() error {
	name := rf.File.Name()
	err := rf.File.Close()
	os.Remove(name)
	return err
}

// dirFile serves directory listings for PROPFIND.
type dirFile struct {
	fs      *fileSystem
	ctx     context.Context
	userID  int64
	node    node
	entries []os.FileInfo
	pos     int
}

func (d *dirFile) Readdir(count int) ([]os.FileInfo, error) {
	if d.entries == nil {
		entries, err := d.fs.listChildren(d.ctx, d.userID, d.node.id)
		if err != nil {
			return nil, err
		}
		d.entries = entries
	}
	if d.pos >= len(d.entries) {
		return nil, io.EOF
	}
	if count <= 0 {
		out := d.entries[d.pos:]
		d.pos = len(d.entries)
		return out, nil
	}
	end := d.pos + count
	if end > len(d.entries) {
		end = len(d.entries)
	}
	out := d.entries[d.pos:end]
	d.pos = end
	return out, nil
}

func (d *dirFile) Stat() (os.FileInfo, error)     { return d.node.fileInfo(), nil }
func (d *dirFile) Read([]byte) (int, error)       { return 0, os.ErrInvalid }
func (d *dirFile) Write([]byte) (int, error)      { return 0, os.ErrInvalid }
func (d *dirFile) Seek(int64, int) (int64, error) { return 0, os.ErrInvalid }
func (d *dirFile) Close() error                   { return nil }

// writeFile buffers an upload locally and publishes it on Close — WebDAV clients
// write the whole body and then close, so Close is the natural commit point.
type writeFile struct {
	fs      *fileSystem
	ctx     context.Context
	userID  int64
	davPath string
	tmp     *os.File
	closed  bool
}

func (f *fileSystem) newWriteFile(ctx context.Context, userID int64, davPath string) *writeFile {
	tmp, err := os.CreateTemp("", "stora-dav-write-*")
	if err != nil {
		// Reported by the first Write/Close instead; OpenFile has to return a File.
		return &writeFile{fs: f, ctx: ctx, userID: userID, davPath: davPath}
	}
	return &writeFile{fs: f, ctx: ctx, userID: userID, davPath: davPath, tmp: tmp}
}

func (w *writeFile) Write(p []byte) (int, error) {
	if w.tmp == nil {
		return 0, errors.New("webdav: upload buffer unavailable")
	}
	return w.tmp.Write(p)
}

func (w *writeFile) Read([]byte) (int, error)           { return 0, os.ErrInvalid }
func (w *writeFile) Seek(int64, int) (int64, error)     { return 0, os.ErrInvalid }
func (w *writeFile) Readdir(int) ([]os.FileInfo, error) { return nil, os.ErrInvalid }

func (w *writeFile) Stat() (os.FileInfo, error) {
	if w.tmp == nil {
		return nil, os.ErrInvalid
	}
	info, err := w.tmp.Stat()
	if err != nil {
		return nil, err
	}
	_, base := splitPath(w.davPath)
	return fileInfo{name: base, size: info.Size(), modTime: time.Now()}, nil
}

func (w *writeFile) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if w.tmp == nil {
		return errors.New("webdav: upload buffer unavailable")
	}
	tmpName := w.tmp.Name()
	defer os.Remove(tmpName)

	info, err := w.tmp.Stat()
	if err != nil {
		w.tmp.Close()
		return err
	}
	if _, err := w.tmp.Seek(0, io.SeekStart); err != nil {
		w.tmp.Close()
		return err
	}

	fileID, err := w.fs.publish(w.ctx, w.userID, w.davPath, w.tmp, info.Size())
	closeErr := w.tmp.Close()
	if err != nil {
		return err
	}
	fileapi.EnqueueThumbnail(w.fs.db, fileID)
	return closeErr
}

// publish stores the buffered bytes and creates or updates the file row, keeping
// a version of the previous content exactly like the HTTP upload path does.
func (f *fileSystem) publish(ctx context.Context, userID int64, davPath string, body io.Reader, size int64) (int64, error) {
	dir, base := splitPath(davPath)
	if base == "" {
		return 0, os.ErrPermission // cannot write to the root
	}
	parentID, err := f.resolveFolder(ctx, userID, dir)
	if err != nil {
		return 0, err
	}

	hash, storePath, err := f.storage.StoreHash(body)
	if err != nil {
		return 0, err
	}

	now := time.Now().Format(time.RFC3339)

	// Existing file? Snapshot its current content into file_versions first.
	var (
		existingID   int64
		existingPath string
		row          *sql.Row
	)
	if parentID == 0 {
		row = f.db.QueryRowContext(ctx,
			`SELECT id, COALESCE(file_path, '') FROM file_items
			  WHERE user_id = $1 AND folder_id IS NULL AND filename = $2 AND is_folder = false AND deleted_at IS NULL`,
			userID, base)
	} else {
		row = f.db.QueryRowContext(ctx,
			`SELECT id, COALESCE(file_path, '') FROM file_items
			  WHERE user_id = $1 AND folder_id = $2 AND filename = $3 AND is_folder = false AND deleted_at IS NULL`,
			userID, parentID, base)
	}
	if err := row.Scan(&existingID, &existingPath); err == nil && existingID > 0 {
		f.db.ExecContext(ctx,
			`INSERT INTO file_versions (file_id, version_num, file_path, file_size, file_hash, storage_driver, created_by, created_at)
			 SELECT $1, COALESCE((SELECT MAX(version_num) FROM file_versions WHERE file_id = $1), 0) + 1,
			        file_path, file_size, file_hash, storage_driver, $2, $3 FROM file_items WHERE id = $1`,
			existingID, userID, now)
		if _, err := f.db.ExecContext(ctx,
			`UPDATE file_items SET file_path = $1, file_hash = $2, updated_at = $3 WHERE id = $4`,
			storePath, hash, now, existingID); err != nil {
			return 0, err
		}
		f.auditWrite(userID, davPath, size, ctx)
		return existingID, nil
	}

	mimeType := mime.TypeByExtension(strings.ToLower(path.Ext(base)))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	fileType := fileapi.DetectFileType(mimeType, base)

	var fileID int64
	if err := f.db.QueryRowContext(ctx,
		`INSERT INTO file_items (user_id, folder_id, filename, original_filename, file_path, file_size,
		                         mime_type, file_type, storage_driver, file_hash, is_folder, created_at, updated_at)
		 VALUES ($1, $2, $3, $3, $4, $5, $6, $7, 'local', $8, false, $9, $9) RETURNING id`,
		userID, nullableID(parentID), base, storePath, size, mimeType, fileType, hash, now,
	).Scan(&fileID); err != nil {
		return 0, err
	}

	f.auditWrite(userID, davPath, size, ctx)
	return fileID, nil
}

func (f *fileSystem) auditWrite(userID int64, davPath string, size int64, ctx context.Context) {
	c, _ := callerOf(ctx)
	audit.Log(f.db, audit.Entry{
		UserID:    userID,
		Action:    "webdav.put",
		Resource:  "path:" + cleanPath(davPath),
		Detail:    "bytes=" + strconv.FormatInt(size, 10),
		IPAddress: c.ip,
	})
}

// nullableID keeps folder_id NULL at the root, matching the rest of the codebase.
func nullableID(id int64) interface{} {
	if id == 0 {
		return nil
	}
	return id
}
