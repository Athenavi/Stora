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
func (d *LocalDriver) StoreHash(reader io.Reader) (hash, relPath string, err error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", "", fmt.Errorf("failed to read data: %w", err)
	}
	h := sha256.Sum256(data)
	hash = fmt.Sprintf("%x", h)
	relPath = fmt.Sprintf("objects/%s/%s", hash[:2], hash[2:])
	fullPath := filepath.Join(d.ObjectsPath, relPath)

	// Check if already exists
	if _, err := os.Stat(fullPath); err == nil {
		return hash, relPath, nil // already stored, dedup
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", "", fmt.Errorf("failed to write file %s: %w", fullPath, err)
	}
	return hash, relPath, nil
}

func (d *LocalDriver) Retrieve(path string) (io.ReadCloser, error) {
	fullPath := filepath.Join(d.ObjectsPath, path)
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", fullPath, err)
	}
	return f, nil
}

func (d *LocalDriver) Delete(path string) error {
	fullPath := filepath.Join(d.ObjectsPath, path)
	return os.Remove(fullPath)
}

func (d *LocalDriver) Exists(path string) (bool, error) {
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
