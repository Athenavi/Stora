package files

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"
)

// maxIndexedContentBytes caps how much text per file goes into the trigram
// index. Transcripts alone can be tens of thousands of characters; beyond this
// the index balloons while the extra recall is negligible.
const maxIndexedContentBytes = 64 << 10

// buildSearchContent collects every piece of text a user might search for and
// joins it into one string.
//
// One column and one index is deliberate: it keeps the query cheap, and adding a
// new source (OCR text, for instance) means touching exactly this function.
func buildSearchContent(ctx context.Context, db *sql.DB, fileID int64) (userID int64, content string, err error) {
	var filename, original, description, category, fileType string
	if err = db.QueryRowContext(ctx,
		`SELECT user_id, COALESCE(filename, ''), COALESCE(original_filename, ''),
		        COALESCE(description, ''), COALESCE(category, ''), COALESCE(file_type, '')
		   FROM file_items WHERE id = $1`,
		fileID,
	).Scan(&userID, &filename, &original, &description, &category, &fileType); err != nil {
		return 0, "", err
	}

	parts := []string{filename, original, description, category, fileType}

	appendColumn := func(query string) {
		rows, err := db.QueryContext(ctx, query, fileID)
		if err != nil {
			return // a missing source must not break indexing entirely
		}
		defer rows.Close()
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err == nil && value != "" {
				parts = append(parts, value)
			}
		}
	}

	appendColumn(`SELECT t.name FROM file_tags t
	                JOIN file_tag_assignments a ON a.tag_id = t.id
	               WHERE a.file_id = $1`)
	appendColumn(`SELECT content FROM file_comments WHERE file_id = $1`)
	appendColumn(`SELECT COALESCE(content, '') FROM transcription_tasks
	               WHERE file_id = $1 AND status = 'completed'`)
	// Text pulled out of images by the OCR worker (see ocr.go).
	appendColumn(`SELECT content FROM ocr_texts WHERE file_id = $1`)

	content = capIndexedContent(strings.Join(nonEmptyParts(parts), "\n"))
	return userID, content, nil
}

// capIndexedContent bounds the indexed text.
//
// ToValidUTF8 drops the partial rune a byte-wise cut may leave behind, so a
// Chinese transcript does not end in a broken character.
func capIndexedContent(s string) string {
	if len(s) <= maxIndexedContentBytes {
		return s
	}
	return strings.ToValidUTF8(s[:maxIndexedContentBytes], "")
}

func nonEmptyParts(parts []string) []string {
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ReindexSearch rebuilds a file's search entry.
//
// Asynchronous and best-effort, like thumbnail generation: search freshness must
// never fail — or slow down — the upload, rename or comment that triggered it.
func ReindexSearch(db *sql.DB, fileID int64) {
	if db == nil || fileID == 0 {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Search] panic while reindexing file %d: %v", fileID, r)
			}
		}()

		userID, content, err := buildSearchContent(context.Background(), db, fileID)
		if err != nil {
			// Most likely the file was deleted in the meantime; the FK cascade
			// already removed its index row.
			return
		}

		if _, err := db.Exec(
			`INSERT INTO search_index (file_id, user_id, content, updated_at)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (file_id) DO UPDATE SET content = $3, updated_at = $4`,
			fileID, userID, content, time.Now().Format(time.RFC3339),
		); err != nil {
			log.Printf("[Search] reindex failed for file %d: %v", fileID, err)
		}
	}()
}

// escapeLike neutralises LIKE wildcards so that searching for "50%" does not
// match every file. The query must use ESCAPE '\' (the default).
func escapeLike(q string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
}
