package cleanup

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"
)

// ─── Observer 1: UploadChunkCleaner ─────────────────────────────────────────

// UploadChunkCleaner deletes chunk records for a cleaned-up upload.
type UploadChunkCleaner struct {
	DB *sql.DB
}

func (o *UploadChunkCleaner) OnCleanup(event CleanupEvent) error {
	result, err := o.DB.Exec(`DELETE FROM upload_chunks WHERE upload_id = $1`, event.UploadID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		log.Printf("[Cleanup] Deleted %d chunk records for upload %s", n, event.UploadID)
	}
	return nil
}

// ─── Observer 2: OrphanFileItemCleaner ───────────────────────────────────────

// OrphanFileItemCleaner deletes file_items that were soft-deleted long ago.
type OrphanFileItemCleaner struct {
	DB          *sql.DB
	ExpireHours int // how long a soft-deleted file_item stays before permanent deletion
}

func (o *OrphanFileItemCleaner) OnCleanup(event CleanupEvent) error {
	if o.ExpireHours <= 0 {
		o.ExpireHours = 144
	}

	// Collect fingerprints and total size before deletion
	rows, err := o.DB.Query(
		`SELECT COALESCE(file_hash,''), COALESCE(file_size,0) FROM file_items WHERE deleted_at IS NOT NULL
		 AND deleted_at < NOW() - CAST($1 AS INTERVAL)`,
		fmt.Sprintf("%d hours", o.ExpireHours),
	)
	if err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339)
	var totalSize int64
	var hashes []string
	for rows.Next() {
		var hash string
		var size int64
		if err := rows.Scan(&hash, &size); err == nil {
			if hash != "" {
				hashes = append(hashes, hash)
			}
			totalSize += size
		}
	}
	rows.Close()
	for _, hash := range hashes {
		o.DB.Exec(`UPDATE file_fingerprints SET reference_count = GREATEST(0, reference_count - 1), updated_at = $1 WHERE hash = $2`, now, hash)
	}

	result, err := o.DB.Exec(
		`DELETE FROM file_items WHERE deleted_at IS NOT NULL
		 AND deleted_at < NOW() - CAST($1 AS INTERVAL)`,
		fmt.Sprintf("%d hours", o.ExpireHours),
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		log.Printf("[Cleanup] Permanently deleted %d orphan file_items (released %d bytes, updated %d fingerprints)", n, totalSize, len(hashes))
	}
	return nil
}

// ─── Observer 3: TempDirCleaner ─────────────────────────────────────────────

// TempDirCleaner removes the temporary upload directory.
type TempDirCleaner struct{}

func (o *TempDirCleaner) OnCleanup(event CleanupEvent) error {
	if err := os.RemoveAll(event.TempDir); err != nil {
		if os.IsNotExist(err) {
			return nil // already cleaned, no error
		}
		return err
	}
	log.Printf("[Cleanup] Removed temp dir %s", event.TempDir)
	return nil
}
