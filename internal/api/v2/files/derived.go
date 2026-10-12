package files

import (
	"context"
	"database/sql"
	"log"
	"time"
)

// Derived state — thumbnails and OCR text — is produced asynchronously from a
// file's content.
//
// Both kinds share one queue and one worker on purpose: they are the same shape
// of work, and the thumbnail worker was already the first copy of this polling
// pattern. A second table with a third copy of the same SQL would be a liability.

type derivedJob struct {
	id     int64
	fileID int64
	kind   string
}

const (
	jobKindThumbnail = "thumbnail"
	jobKindOCR       = "ocr"
)

// enqueueDerived schedules work for a file, replacing any pending job of the same
// kind — a re-upload invalidates whatever the previous one would have produced.
//
// Errors are logged, never returned: derived state must not fail an upload.
func enqueueDerived(db *sql.DB, fileID int64, kind string) {
	if db == nil || fileID == 0 {
		return
	}
	now := time.Now().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO derived_jobs (file_id, kind, status, attempts, updated_at)
		 VALUES ($1, $2, 'pending', 0, $3)
		 ON CONFLICT (file_id, kind) DO UPDATE
		     SET status = 'pending', attempts = 0, last_error = NULL, updated_at = $3`,
		fileID, kind, now,
	); err != nil {
		log.Printf("[Derived] enqueue %s failed for file %d: %v", kind, fileID, err)
	}
}

// EnqueueThumbnail schedules thumbnail generation for a file.
func EnqueueThumbnail(db *sql.DB, fileID int64) { enqueueDerived(db, fileID, jobKindThumbnail) }

// EnqueueOCR schedules text extraction from an image; the text then becomes
// searchable (see buildSearchContent).
func EnqueueOCR(db *sql.DB, fileID int64) { enqueueDerived(db, fileID, jobKindOCR) }

// Run keeps the derived-state queue drained until ctx is cancelled.
//
// It polls the job table rather than using an in-process channel, so pending work
// survives a restart and several server instances can share the queue.
func (h *ThumbnailHandler) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	log.Println("[Derived] worker started (every 5s)")
	for {
		select {
		case <-ctx.Done():
			log.Println("[Derived] worker stopped")
			return
		case <-ticker.C:
			h.drain(ctx)
		}
	}
}

func (h *ThumbnailHandler) drain(ctx context.Context) {
	for {
		jobs, err := h.claimJobs(ctx, 5)
		if err != nil {
			log.Printf("[Derived] claim failed: %v", err)
			return
		}
		if len(jobs) == 0 {
			return
		}
		for _, job := range jobs {
			h.processJob(ctx, job)
		}
	}
}

// claimJobs atomically flips up to `limit` pending jobs to 'running'.
//
// SKIP LOCKED lets several workers coexist without contending over the same row.
func (h *ThumbnailHandler) claimJobs(ctx context.Context, limit int) ([]derivedJob, error) {
	rows, err := h.db.QueryContext(ctx,
		`UPDATE derived_jobs j
		    SET status = 'running', attempts = attempts + 1, updated_at = $1
		   FROM (
		       SELECT id FROM derived_jobs
		        WHERE status = 'pending'
		        ORDER BY id
		        LIMIT $2
		        FOR UPDATE SKIP LOCKED
		   ) claimed
		  WHERE j.id = claimed.id
		RETURNING j.id, j.file_id, j.kind`,
		time.Now().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []derivedJob
	for rows.Next() {
		var j derivedJob
		if err := rows.Scan(&j.id, &j.fileID, &j.kind); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (h *ThumbnailHandler) processJob(ctx context.Context, job derivedJob) {
	switch job.kind {
	case jobKindThumbnail:
		h.processThumbnail(ctx, job)
	case jobKindOCR:
		h.processOCR(ctx, job)
	default:
		// Unknown kinds are marked failed rather than silently dropped, so a
		// typo in an enqueue call is visible instead of invisible.
		h.finishJob(job.id, "failed", "unknown job kind "+job.kind)
	}
}

// finishJob records the outcome. Both handlers funnel through here so the status
// vocabulary lives in one place.
func (h *ThumbnailHandler) finishJob(jobID int64, status, errMsg string) {
	now := time.Now().Format(time.RFC3339)
	if status == "done" {
		h.db.Exec(`UPDATE derived_jobs SET status = 'done', last_error = NULL, updated_at = $1 WHERE id = $2`,
			now, jobID)
		return
	}
	if errMsg != "" {
		log.Printf("[Derived] job %d %s: %s", jobID, status, errMsg)
	}
	h.db.Exec(`UPDATE derived_jobs SET status = $1, last_error = $2, updated_at = $3 WHERE id = $4`,
		status, errMsg, now, jobID)
}
