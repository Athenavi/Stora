package files

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ocrLanguages limits recognition to Simplified Chinese + English. Other
// languages need their tesseract data installed and added here.
const ocrLanguages = "chi_sim+eng"

// maxOCRSourceBytes caps the image handed to tesseract: it is slow and
// memory-hungry on very large images, and the marginal text is negligible.
const maxOCRSourceBytes = 32 << 20

// ocrEnabled reports whether OCR should run at all.
//
// Defaults to on. A resource-constrained host can set OCR_ENABLED=false and the
// queue will simply drain these jobs without spending CPU on tesseract.
func ocrEnabled() bool {
	return os.Getenv("OCR_ENABLED") != "false"
}

// processOCR extracts the text in an image and stores it, then refreshes the
// search index so that text becomes findable.
//
// Same shape as thumbnails: asynchronous, best-effort, and it never fails the
// upload. When tesseract is not installed the job is marked failed and search
// simply has no OCR text for that image.
func (h *ThumbnailHandler) processOCR(ctx context.Context, job derivedJob) {
	if !ocrEnabled() {
		h.finishJob(job.id, "done", "")
		return
	}

	var filePath, mimeType, fileType string
	err := h.db.QueryRowContext(ctx,
		`SELECT COALESCE(file_path, ''), COALESCE(mime_type, ''), COALESCE(file_type, 'other')
		   FROM file_items WHERE id = $1 AND deleted_at IS NULL AND is_folder = false`,
		job.fileID).Scan(&filePath, &mimeType, &fileType)
	if err == sql.ErrNoRows {
		h.db.Exec(`DELETE FROM derived_jobs WHERE id = $1`, job.id)
		return
	}
	if err != nil {
		h.finishJob(job.id, "failed", "lookup: "+err.Error())
		return
	}

	// Only formats the standard library can open are worth handing to tesseract.
	if thumbnailSourceKind(mimeType, fileType) != thumbImage {
		h.finishJob(job.id, "done", "")
		return
	}

	reader, err := h.storage.Retrieve(filePath)
	if err != nil {
		h.finishJob(job.id, "failed", "retrieve: "+err.Error())
		return
	}
	defer reader.Close()

	text, err := runTesseract(ctx, io.LimitReader(reader, maxOCRSourceBytes))
	if err != nil {
		h.finishJob(job.id, "failed", err.Error())
		return
	}
	if text = strings.TrimSpace(text); text == "" {
		// A picture with no text is a normal outcome, not a failure.
		h.finishJob(job.id, "done", "")
		return
	}

	if _, err := h.db.Exec(
		`INSERT INTO ocr_texts (file_id, content, engine, created_at)
		 VALUES ($1, $2, 'tesseract', $3)
		 ON CONFLICT (file_id) DO UPDATE SET content = $2, engine = 'tesseract', created_at = $3`,
		job.fileID, capIndexedContent(text), time.Now().Format(time.RFC3339),
	); err != nil {
		h.finishJob(job.id, "failed", "persist: "+err.Error())
		return
	}

	log.Printf("[OCR] file %d: extracted %d chars", job.fileID, len(text))
	h.finishJob(job.id, "done", "")

	// New searchable text exists, so the index has to catch up.
	ReindexSearch(h.db, job.fileID)
}

// runTesseract pipes the image through stdin and reads the text from stdout, so
// nothing touches the disk.
//
// Ceiling: needs the tesseract binary plus the language data (chi_sim for
// Chinese). Without them the job fails and search just has no OCR text.
func runTesseract(ctx context.Context, image io.Reader) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tesseract", "stdin", "stdout", "-l", ocrLanguages)
	cmd.Stdin = image
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tesseract: %w (%s)", err, strings.TrimSpace(errBuf.String()))
	}
	return out.String(), nil
}
