package cleanup

import (
	"database/sql"
	"log"
	"time"
)

// CleanupScheduler periodically scans for expired uploads and fires cleanup events.
type CleanupScheduler struct {
	db         *sql.DB
	notifier   *CleanupNotifier
	expireAfter time.Duration
	interval    time.Duration
	stopCh      chan struct{}
}

// NewCleanupScheduler creates a new scheduler.
// expireHours: uploads older than this many hours are considered expired.
func NewCleanupScheduler(db *sql.DB, notifier *CleanupNotifier, expireHours int) *CleanupScheduler {
	return &CleanupScheduler{
		db:          db,
		notifier:    notifier,
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

		for {
			select {
			case <-ticker.C:
				s.sweep()
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
			TempDir:  "temp/upload/" + uploadID,
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

// FireNow immediately fires a cleanup event for a specific upload and deletes its task record.
// This is used by the CancelUpload and CompleteUpload handlers.
func (s *CleanupScheduler) FireNow(uploadID string, userID int64) {
	event := CleanupEvent{
		UploadID: uploadID,
		UserID:   userID,
		TempDir:  "temp/upload/" + uploadID,
	}
	s.notifier.Fire(event)
	s.db.Exec(`DELETE FROM upload_tasks WHERE upload_id = $1`, uploadID)
}
