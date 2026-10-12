package cleanup

import (
	"database/sql"
	"log"
	"os"
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

// ─── Observer 2: TempDirCleaner ─────────────────────────────────────────────

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
