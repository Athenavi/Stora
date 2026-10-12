package files

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"

	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/Athenavi/Stora/pkg/storage"
	"github.com/go-chi/chi/v5"
)

// thumbnailDims maps a size token to the maximum edge length in pixels.
var thumbnailDims = map[string]int{"sm": 256, "md": 512, "lg": 1024}

// thumbnailFallback lists, in order, the size tokens that may satisfy a request.
// A client asking for "sm" can be served a larger image (the browser scales it
// down); serving a smaller one in place of a larger would just look blurry.
var thumbnailFallback = map[string][]string{
	"sm": {"sm", "md", "lg"},
	"md": {"md", "lg"},
	"lg": {"lg"},
}

// errNoResize means the source already fits in the requested box. We never
// upscale — an enlarged thumbnail is strictly worse than the original.
var errNoResize = errors.New("no resize needed")

// maxThumbnailSourceBytes caps how much of a source we buffer. Images have to be
// decoded into memory anyway, so this is a guard rather than a streaming path.
const maxThumbnailSourceBytes = 64 << 20

type ThumbnailHandler struct {
	db      *sql.DB
	storage storage.Driver
}

func NewThumbnailHandler(db *sql.DB, store storage.Driver) *ThumbnailHandler {
	return &ThumbnailHandler{db: db, storage: store}
}

// EnqueueThumbnail schedules thumbnail generation for a file.
//
// A package-level function rather than a method, so every upload path (files,
// upload, blocks, share, wopi) can call it without holding a ThumbnailHandler.
// Errors are logged and swallowed on purpose: a thumbnail is a nicety, and an
// upload must never fail because of it.
func EnqueueThumbnail(db *sql.DB, fileID int64) {
	now := time.Now().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO thumbnail_jobs (file_id, status, attempts, updated_at)
		 VALUES ($1, 'pending', 0, $2)
		 ON CONFLICT (file_id) DO UPDATE
		     SET status = 'pending', attempts = 0, last_error = NULL, updated_at = $2`,
		fileID, now,
	); err != nil {
		log.Printf("[Thumbnail] enqueue failed for file %d: %v", fileID, err)
	}
}

// Run processes queued thumbnail jobs until ctx is cancelled.
//
// It polls the job table rather than using an in-process channel, so pending work
// survives a restart and several server instances can share the queue.
func (h *ThumbnailHandler) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	log.Println("[Thumbnail] worker started (every 5s)")
	for {
		select {
		case <-ctx.Done():
			log.Println("[Thumbnail] worker stopped")
			return
		case <-ticker.C:
			for {
				jobs, err := h.claimJobs(ctx, 5)
				if err != nil {
					log.Printf("[Thumbnail] claim failed: %v", err)
					break
				}
				if len(jobs) == 0 {
					break
				}
				for _, job := range jobs {
					h.processJob(ctx, job.id, job.fileID)
				}
			}
		}
	}
}

type thumbJob struct {
	id     int64
	fileID int64
}

// claimJobs atomically flips up to `limit` pending jobs to 'running'.
//
// SKIP LOCKED lets multiple workers coexist without contending; without it two
// instances would process the same file twice.
func (h *ThumbnailHandler) claimJobs(ctx context.Context, limit int) ([]thumbJob, error) {
	rows, err := h.db.QueryContext(ctx,
		`UPDATE thumbnail_jobs j
		    SET status = 'running', attempts = attempts + 1, updated_at = $1
		   FROM (
		       SELECT id FROM thumbnail_jobs
		        WHERE status = 'pending'
		        ORDER BY id
		        LIMIT $2
		        FOR UPDATE SKIP LOCKED
		   ) claimed
		  WHERE j.id = claimed.id
		RETURNING j.id, j.file_id`,
		time.Now().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []thumbJob
	for rows.Next() {
		var j thumbJob
		if err := rows.Scan(&j.id, &j.fileID); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (h *ThumbnailHandler) processJob(ctx context.Context, jobID, fileID int64) {
	now := func() string { return time.Now().Format(time.RFC3339) }
	fail := func(reason string) {
		log.Printf("[Thumbnail] file %d: %s", fileID, reason)
		h.db.Exec(`UPDATE thumbnail_jobs SET status = 'failed', last_error = $1, updated_at = $2 WHERE id = $3`,
			reason, now(), jobID)
	}

	var filePath, mimeType, fileType string
	err := h.db.QueryRowContext(ctx,
		`SELECT COALESCE(file_path, ''), COALESCE(mime_type, ''), COALESCE(file_type, 'other')
		   FROM file_items WHERE id = $1 AND deleted_at IS NULL AND is_folder = false`,
		fileID).Scan(&filePath, &mimeType, &fileType)
	if err == sql.ErrNoRows {
		// Deleted between enqueue and processing — drop the job with the file.
		h.db.Exec(`DELETE FROM thumbnail_jobs WHERE id = $1`, jobID)
		return
	}
	if err != nil {
		fail(fmt.Sprintf("lookup: %v", err))
		return
	}

	kind := thumbnailSourceKind(mimeType, fileType)
	if kind == thumbUnsupported {
		// Nothing to render (pdf, archive, audio, ...). Marking it 'done' records
		// that we tried, so the GET endpoint can answer 404 instead of "come back later".
		h.db.Exec(`UPDATE thumbnail_jobs SET status = 'done', updated_at = $1 WHERE id = $2`, now(), jobID)
		return
	}

	reader, err := h.storage.Retrieve(filePath)
	if err != nil {
		fail(fmt.Sprintf("retrieve: %v", err))
		return
	}
	defer reader.Close()

	src, err := loadThumbnailSource(ctx, kind, reader)
	if err != nil {
		fail(err.Error())
		return
	}

	stored := 0
	for _, size := range []string{"sm", "md", "lg"} {
		data, w, hgt, err := encodeThumbnail(src, thumbnailDims[size])
		if errors.Is(err, errNoResize) {
			continue
		}
		if err != nil {
			fail(fmt.Sprintf("%s: %v", size, err))
			return
		}
		_, path, err := h.storage.StoreHash(bytes.NewReader(data))
		if err != nil {
			fail(fmt.Sprintf("%s: store: %v", size, err))
			return
		}
		if _, err := h.db.Exec(
			`INSERT INTO file_thumbnails (file_id, size, width, height, storage_path)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (file_id, size) DO UPDATE
			     SET width = $3, height = $4, storage_path = $5`,
			fileID, size, w, hgt, path,
		); err != nil {
			fail(fmt.Sprintf("%s: persist: %v", size, err))
			return
		}
		stored++
	}

	h.db.Exec(`UPDATE thumbnail_jobs SET status = 'done', updated_at = $1 WHERE id = $2`, now(), jobID)
	if stored > 0 {
		log.Printf("[Thumbnail] file %d: generated %d sizes", fileID, stored)
	}
}

// Get serves a generated thumbnail.
// GET /api/v2/files/{id}/thumbnail?size=sm|md|lg
func (h *ThumbnailHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	fileID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	size := r.URL.Query().Get("size")
	if _, known := thumbnailFallback[size]; !known {
		size = "md"
	}

	// Ownership check — otherwise a thumbnail becomes a side door into other
	// users' files.
	var ownerID int64
	if err := h.db.QueryRow(
		`SELECT user_id FROM file_items WHERE id = $1 AND deleted_at IS NULL AND is_folder = false`,
		fileID).Scan(&ownerID); err != nil || ownerID != userID {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	if path, found, err := h.lookupThumbnail(fileID, size); err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	} else if found {
		reader, err := h.storage.Retrieve(path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "storage error")
			return
		}
		defer reader.Close()

		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, max-age=86400")
		w.Header().Set("ETag", `"`+path+`"`)
		w.WriteHeader(http.StatusOK)
		io.Copy(w, reader)
		return
	}

	// Distinguish "not ready yet" from "never will be", so the browser knows
	// whether retrying is worth it.
	var jobStatus string
	switch err := h.db.QueryRow(`SELECT status FROM thumbnail_jobs WHERE file_id = $1`, fileID).Scan(&jobStatus); {
	case err == sql.ErrNoRows, jobStatus == "done", jobStatus == "failed":
		writeError(w, http.StatusNotFound, "no thumbnail for this file")
	default:
		w.Header().Set("Retry-After", "3")
		writeError(w, http.StatusAccepted, "thumbnail is being generated")
	}
}

// lookupThumbnail walks the fallback chain and returns the stored key.
func (h *ThumbnailHandler) lookupThumbnail(fileID int64, size string) (string, bool, error) {
	for _, candidate := range thumbnailFallback[size] {
		var path string
		err := h.db.QueryRow(
			`SELECT storage_path FROM file_thumbnails WHERE file_id = $1 AND size = $2`,
			fileID, candidate).Scan(&path)
		if err == nil {
			return path, true, nil
		}
		if err != sql.ErrNoRows {
			return "", false, err
		}
	}
	return "", false, nil
}

type thumbSource int

const (
	thumbUnsupported thumbSource = iota
	thumbImage
	thumbVideo
)

// thumbnailSourceKind decides whether a file can be rendered, using the stored
// file_type rather than re-sniffing the MIME type.
func thumbnailSourceKind(mimeType, fileType string) thumbSource {
	switch fileType {
	case "video":
		return thumbVideo
	case "image":
		switch mimeType {
		case "image/svg+xml", "image/avif", "image/heic", "image/heif":
			// Not decodable by the standard library, and pulling in a decoder for
			// each is not worth it (see docs/design-next-phase.md).
			return thumbUnsupported
		}
		return thumbImage
	default:
		return thumbUnsupported
	}
}

// loadThumbnailSource decodes the source into an image once; every size is then
// scaled from that single decode.
func loadThumbnailSource(ctx context.Context, kind thumbSource, reader io.Reader) (image.Image, error) {
	switch kind {
	case thumbImage:
		data, err := io.ReadAll(io.LimitReader(reader, maxThumbnailSourceBytes))
		if err != nil {
			return nil, fmt.Errorf("read source: %w", err)
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("decode image: %w", err)
		}
		return img, nil
	case thumbVideo:
		frame, err := extractVideoFrame(ctx, reader)
		if err != nil {
			return nil, err
		}
		img, _, err := image.Decode(bytes.NewReader(frame))
		if err != nil {
			return nil, fmt.Errorf("decode video frame: %w", err)
		}
		return img, nil
	}
	return nil, errors.New("unsupported thumbnail source")
}

// extractVideoFrame pulls a single frame through ffmpeg, reading the video from
// stdin and writing JPEG to stdout so nothing touches the disk.
//
// Ceiling: requires ffmpeg on PATH (already a dependency for transcoding). When
// it is missing the job fails and the UI falls back to a file-type icon.
func extractVideoFrame(ctx context.Context, video io.Reader) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-v", "error",
		"-ss", "1", // one second in, to skip black/logo leader frames
		"-i", "pipe:0",
		"-frames:v", "1",
		"-f", "image2",
		"-vcodec", "mjpeg",
		"pipe:1",
	)
	cmd.Stdin = video
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w (%s)", err, strings.TrimSpace(errBuf.String()))
	}
	if out.Len() == 0 {
		return nil, errors.New("ffmpeg produced no frame")
	}
	return out.Bytes(), nil
}

// encodeThumbnail scales src to fit a maxDim box and encodes it as JPEG.
func encodeThumbnail(src image.Image, maxDim int) ([]byte, int, int, error) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, 0, 0, errors.New("empty image")
	}
	tw, th := fitWithin(w, h, maxDim)
	if tw == w && th == h {
		return nil, 0, 0, errNoResize
	}

	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 82}); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), tw, th, nil
}

// fitWithin scales (w,h) down so that the longest edge becomes maxDim, keeping
// the aspect ratio. Images already inside the box are returned unchanged.
func fitWithin(w, h, maxDim int) (int, int) {
	if w <= maxDim && h <= maxDim {
		return w, h
	}
	if w >= h {
		return maxDim, max(1, int(math.Round(float64(h)*float64(maxDim)/float64(w))))
	}
	return max(1, int(math.Round(float64(w)*float64(maxDim)/float64(h)))), maxDim
}
