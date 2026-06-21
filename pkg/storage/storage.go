package storage

import (
	"io"
)

// Driver defines the interface for file storage backends.
type Driver interface {
	// Store saves a file from a reader to the given path.
	Store(path string, reader io.Reader) error

	// Retrieve reads a file from the given path.
	Retrieve(path string) (io.ReadCloser, error)

	// Delete removes a file.
	Delete(path string) error

	// Exists checks if a file exists.
	Exists(path string) (bool, error)

	// URL returns the public URL for a file.
	URL(path string) string
}
