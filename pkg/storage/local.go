package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalDriver stores files on the local filesystem.
type LocalDriver struct {
	BasePath string
	BaseURL  string
}

// NewLocalDriver creates a new LocalDriver.
func NewLocalDriver(basePath, baseURL string) *LocalDriver {
	absPath, _ := filepath.Abs(basePath)
	os.MkdirAll(absPath, 0755)
	return &LocalDriver{BasePath: absPath, BaseURL: baseURL}
}

func (d *LocalDriver) Store(path string, reader io.Reader) error {
	fullPath := filepath.Join(d.BasePath, path)
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

func (d *LocalDriver) Retrieve(path string) (io.ReadCloser, error) {
	fullPath := filepath.Join(d.BasePath, path)
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", fullPath, err)
	}
	return f, nil
}

func (d *LocalDriver) Delete(path string) error {
	fullPath := filepath.Join(d.BasePath, path)
	return os.Remove(fullPath)
}

func (d *LocalDriver) Exists(path string) (bool, error) {
	fullPath := filepath.Join(d.BasePath, path)
	_, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (d *LocalDriver) URL(path string) string {
	return d.BaseURL + "/" + path
}
