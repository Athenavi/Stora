package storage

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalDriver stores files on the local filesystem.
type LocalDriver struct {
	ObjectsPath string
	BaseURL     string
}

// NewLocalDriver creates a new LocalDriver with the given objects storage root.
// objectsPath is the directory for content-addressed storage, e.g. "storage/objects".
func NewLocalDriver(objectsPath, baseURL string) *LocalDriver {
	absPath, _ := filepath.Abs(objectsPath)
	os.MkdirAll(absPath, 0755)
	return &LocalDriver{ObjectsPath: absPath, BaseURL: baseURL}
}

// Store saves a file at a manually specified path under ObjectsPath.
// Deprecated: prefer StoreHash for content-addressed storage.
func (d *LocalDriver) Store(path string, reader io.Reader) error {
	if err := ValidateKey(path); err != nil {
		return err
	}
	fullPath := filepath.Join(d.ObjectsPath, path)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", fullPath, err)
	}
	defer f.Close()
	_, err = io.Copy(f, reader)
	return err
}

// StoreHash saves a file by SHA256 hash at objects/{hash[:2]}/{hash[2:]}.
// Returns the hex hash and the relative storage path.
//
// The content is streamed to a temp file while hashing, so an upload of any
// size never has to sit in memory; the temp file is renamed into place once the
// digest is known.
func (d *LocalDriver) StoreHash(reader io.Reader) (hash, relPath string, err error) {
	tmp, err := os.CreateTemp(d.ObjectsPath, ".tmp-upload-")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	h := sha256.New()
	if _, err = io.Copy(io.MultiWriter(tmp, h), reader); err != nil {
		return "", "", fmt.Errorf("failed to read data: %w", err)
	}
	if err = tmp.Chmod(0644); err != nil {
		return "", "", fmt.Errorf("failed to set file mode: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return "", "", fmt.Errorf("failed to close temp file: %w", err)
	}

	hash = fmt.Sprintf("%x", h.Sum(nil))
	relPath = ObjectKey(hash)
	fullPath := filepath.Join(d.ObjectsPath, relPath)

	// Check if already exists
	if _, statErr := os.Stat(fullPath); statErr == nil {
		os.Remove(tmpName)
		return hash, relPath, nil // already stored, dedup
	}

	dir := filepath.Dir(fullPath)
	if err = os.MkdirAll(dir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	if err = os.Rename(tmpName, fullPath); err != nil {
		// A concurrent upload may have published the same content first.
		if _, statErr := os.Stat(fullPath); statErr == nil {
			os.Remove(tmpName)
			return hash, relPath, nil
		}
		return "", "", fmt.Errorf("failed to move file into place %s: %w", fullPath, err)
	}
	return hash, relPath, nil
}

func (d *LocalDriver) Retrieve(path string) (io.ReadCloser, error) {
	if err := ValidateKey(path); err != nil {
		return nil, err
	}
	fullPath := filepath.Join(d.ObjectsPath, path)
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", fullPath, err)
	}
	return f, nil
}

func (d *LocalDriver) Delete(path string) error {
	if err := ValidateKey(path); err != nil {
		return err
	}
	fullPath := filepath.Join(d.ObjectsPath, path)
	return os.Remove(fullPath)
}

func (d *LocalDriver) Exists(path string) (bool, error) {
	if err := ValidateKey(path); err != nil {
		return false, err
	}
	fullPath := filepath.Join(d.ObjectsPath, path)
	_, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (d *LocalDriver) URL(path string) string {
	return d.BaseURL + "/" + path
}
