package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestObjectKeyMatchesLocalLayout pins the contract that makes a backend swap a
// plain object copy: the key derived from a digest must be exactly what the
// local driver writes to disk. If these two ever disagree, existing rows in
// file_items silently point at nothing after switching to S3.
func TestObjectKeyMatchesLocalLayout(t *testing.T) {
	dir := t.TempDir()
	d := NewLocalDriver(dir, "/files")

	content := []byte("stora-key-layout-contract")
	want := fmt.Sprintf("%x", sha256.Sum256(content))
	wantPath := want[:2] + "/" + want[2:]

	hash, relPath, err := d.StoreHash(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("StoreHash: %v", err)
	}
	if hash != want {
		t.Errorf("hash = %s, want %s", hash, want)
	}
	if relPath != wantPath {
		t.Errorf("relPath = %s, want %s", relPath, wantPath)
	}
	if ObjectKey(hash) != relPath {
		t.Errorf("ObjectKey(%s) = %s, want %s — the two drivers disagree on layout",
			hash, ObjectKey(hash), relPath)
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(relPath))); err != nil {
		t.Errorf("object not written at the key path: %v", err)
	}

	// Storing identical content again must dedup, not fail or duplicate.
	if _, rel2, err := d.StoreHash(bytes.NewReader(content)); err != nil || rel2 != relPath {
		t.Errorf("second StoreHash = (%s, %v), want dedup to %s", rel2, err, relPath)
	}
}

func TestValidateKey(t *testing.T) {
	valid := []string{"ab/cdef", "ab/cd/ef", ".tmp-upload-1f2e"}
	for _, k := range valid {
		if err := ValidateKey(k); err != nil {
			t.Errorf("ValidateKey(%q) = %v, want nil", k, err)
		}
	}
	invalid := []string{"", "/etc/passwd", "ab/../../etc", "..", "ab//cd", `ab\cd`, "ab/cd\x00"}
	for _, k := range invalid {
		if err := ValidateKey(k); err == nil {
			t.Errorf("ValidateKey(%q) = nil, want error", k)
		}
	}
}

// TestLocalDriverRejectsTraversal proves the key validation is wired into the
// driver's read/delete paths, not merely exported.
func TestLocalDriverRejectsTraversal(t *testing.T) {
	d := NewLocalDriver(t.TempDir(), "/files")
	for _, k := range []string{"../secret", "/etc/passwd", ".."} {
		if _, err := d.Retrieve(k); err == nil {
			t.Errorf("Retrieve(%q) succeeded, want rejection", k)
		}
		if _, err := d.Exists(k); err == nil {
			t.Errorf("Exists(%q) succeeded, want rejection", k)
		}
		if err := d.Delete(k); err == nil {
			t.Errorf("Delete(%q) succeeded, want rejection", k)
		}
	}
}

// TestS3DriverRoundTrip exercises the real driver against a live S3-compatible
// endpoint (RustFS recommended). Skipped unless explicitly requested, since it
// needs a running service:
//
//	STORAGE_TEST_S3=1 S3_ENDPOINT=http://localhost:9000 S3_BUCKET=stora \
//	  S3_ACCESS_KEY=... S3_SECRET_KEY=... go test ./pkg/storage -run S3 -v
func TestS3DriverRoundTrip(t *testing.T) {
	if os.Getenv("STORAGE_TEST_S3") != "1" {
		t.Skip("set STORAGE_TEST_S3=1 (plus S3_* env) to run against a live endpoint")
	}

	d, err := NewS3Driver(context.Background(), S3Options{
		Endpoint:       os.Getenv("S3_ENDPOINT"),
		Region:         os.Getenv("S3_REGION"),
		Bucket:         os.Getenv("S3_BUCKET"),
		AccessKey:      os.Getenv("S3_ACCESS_KEY"),
		SecretKey:      os.Getenv("S3_SECRET_KEY"),
		ForcePathStyle: os.Getenv("S3_FORCE_PATH_STYLE") != "false",
	})
	if err != nil {
		t.Fatalf("NewS3Driver: %v", err)
	}

	content := []byte("stora s3 round trip " + time.Now().Format(time.RFC3339Nano))
	hash, key, err := d.StoreHash(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("StoreHash: %v", err)
	}
	want := fmt.Sprintf("%x", sha256.Sum256(content))
	if hash != want || key != ObjectKey(want) {
		t.Fatalf("StoreHash = (%s, %s), want (%s, %s)", hash, key, want, ObjectKey(want))
	}
	defer d.Delete(key)

	// Idempotent: identical content keeps the same key and leaves no temp object.
	if _, key2, err := d.StoreHash(bytes.NewReader(content)); err != nil || key2 != key {
		t.Fatalf("second StoreHash = (%s, %v), want %s", key2, err, key)
	}

	rc, err := d.Retrieve(key)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %q", got)
	}

	if ok, err := d.Exists(key); err != nil || !ok {
		t.Errorf("Exists = (%v, %v), want (true, nil)", ok, err)
	}
	if ok, err := d.Exists("ff/ffffffff"); err != nil || ok {
		t.Errorf("Exists(missing) = (%v, %v), want (false, nil)", ok, err)
	}
	if os.Getenv("S3_PRESIGN_DOWNLOAD") != "true" {
		if got := d.URL(key); got != "" {
			t.Errorf("URL = %q, want empty when presigning is disabled", got)
		}
	}
}
