package cleanup

import (
	"database/sql"
	"log"
	"path/filepath"
	"time"
)

// CleanupScheduler periodically scans for expired uploads and expired trash,
// then fires cleanup events / purges.
type CleanupScheduler struct {
	db          *sql.DB
	notifier    *CleanupNotifier
	tempDir     string
	expireAfter time.Duration
	interval    time.Duration
	stopCh      chan struct{}
}

// NewCleanupScheduler creates a new scheduler.
// expireHours: uploads and trash older than this many hours are considered expired.
// tempDir: the configured upload temp root (e.g. "temp/upload").
func NewCleanupScheduler(db *sql.DB, notifier *CleanupNotifier, expireHours int, tempDir string) *CleanupScheduler {
	return &CleanupScheduler{
		db:          db,
		notifier:    notifier,
		tempDir:     tempDir,
		expireAfter: time.Duration(expireHours) * time.Hour,
		interval:    30 * time.Minute,
		stopCh:      make(chan struct{}),
	}
}

// Start begins the periodic cleanup loop in a background goroutine.
func (s *CleanupScheduler) Start() {
	go func() {
		log.Printf("[Cleanup] Scheduler started (expire after %v, check every %v)", s.expireAfter, s.interval)
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		// Run once immediately on start
		s.sweep()
		s.sweepTrash()

		for {
			select {
			case <-ticker.C:
				s.sweep()
				s.sweepTrash()
			case <-s.stopCh:
				log.Println("[Cleanup] Scheduler stopped")
				return
			}
		}
	}()
}

// Stop signals the scheduler to shut down.
func (s *CleanupScheduler) Stop() {
	close(s.stopCh)
}

// sweep scans for expired uploads and fires cleanup events.
func (s *CleanupScheduler) sweep() {
	cutoff := time.Now().Add(-s.expireAfter).Format(time.RFC3339)

	rows, err := s.db.Query(
		`SELECT upload_id, user_id FROM upload_tasks
		 WHERE status != 'completed' AND updated_at < $1`,
		cutoff,
	)
	if err != nil {
		log.Printf("[Cleanup] Sweep query failed: %v", err)
		return
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var uploadID string
		var userID int64
		if err := rows.Scan(&uploadID, &userID); err != nil {
			log.Printf("[Cleanup] Row scan error: %v", err)
			continue
		}

		event := CleanupEvent{
			UploadID: uploadID,
			UserID:   userID,
			TempDir:  filepath.Join(s.tempDir, uploadID),
		}

		log.Printf("[Cleanup] Firing event for expired upload %s (user %d)", uploadID, userID)
		s.notifier.Fire(event)

		// Delete the upload task after all observers have cleaned up
		if _, err := s.db.Exec(`DELETE FROM upload_tasks WHERE upload_id = $1`, uploadID); err != nil {
			log.Printf("[Cleanup] Failed to delete upload_task %s: %v", uploadID, err)
		}
		count++
	}

	if count > 0 {
		log.Printf("[Cleanup] Sweep complete: %d expired uploads cleaned", count)
	}
}

// sweepTrash permanently removes trash items past their retention window and
// releases the owners' storage quota.
//
// Grouped per user on purpose: the previous implementation ran a single global
// DELETE, so a trash item was purged as soon as *any* upload anywhere expired,
// regardless of its owner, and used_storage was never given back.
func (s *CleanupScheduler) sweepTrash() {
	cutoff := time.Now().Add(-s.expireAfter).Format(time.RFC3339)

	rows, err := s.db.Query(
		`SELECT user_id, COALESCE(file_hash, ''), COALESCE(file_size, 0) FROM file_items
		 WHERE deleted_at IS NOT NULL AND deleted_at < $1`,
		cutoff,
	)
	if err != nil {
		log.Printf("[Cleanup] Trash sweep query failed: %v", err)
		return
	}

	type userTrash struct {
		bytes  int64
		hashes []string
	}
	perUser := make(map[int64]*userTrash)
	for rows.Next() {
		var userID, size int64
		var hash string
		if err := rows.Scan(&userID, &hash, &size); err != nil {
			continue
		}
		u := perUser[userID]
		if u == nil {
			u = &userTrash{}
			perUser[userID] = u
		}
		u.bytes += size
		if hash != "" {
			u.hashes = append(u.hashes, hash)
		}
	}
	rows.Close()

	if len(perUser) == 0 {
		return
	}

	now := time.Now().Format(time.RFC3339)
	for userID, u := range perUser {
		for _, hash := range u.hashes {
			s.db.Exec(`UPDATE file_fingerprints SET reference_count = GREATEST(0, reference_count - 1), updated_at = $1 WHERE hash = $2`, now, hash)
		}
		res, err := s.db.Exec(
			`DELETE FROM file_items WHERE user_id = $1 AND deleted_at IS NOT NULL AND deleted_at < $2`,
			userID, cutoff,
		)
		if err != nil {
			log.Printf("[Cleanup] Trash delete failed for user %d: %v", userID, err)
			continue
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			continue
		}
		if u.bytes > 0 {
			s.db.Exec(`UPDATE users SET used_storage = GREATEST(0, used_storage - $1) WHERE id = $2`, u.bytes, userID)
		}
		log.Printf("[Cleanup] Purged %d trash items for user %d (released %d bytes)", n, userID, u.bytes)
	}
}
