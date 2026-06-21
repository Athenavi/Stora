package storage

import (
	"crypto/sha256"
	"fmt"
	"io"
)

// Driver defines the interface for file storage backends.
type Driver interface {
	// Store saves a file from a reader to the given path.
	Store(path string, reader io.Reader) error

	// StoreHash saves a file by its SHA256 hash, deduplicating by content.
	// Returns the hex-encoded hash and the storage path (objects/{hash[:2]}/{hash[2:]}).
	StoreHash(reader io.Reader) (hash, path string, err error)

	// Retrieve reads a file from the given path.
	Retrieve(path string) (io.ReadCloser, error)

	// Delete removes a file.
	Delete(path string) error

	// Exists checks if a file exists.
	Exists(path string) (bool, error)

	// URL returns the public URL for a file.
	URL(path string) string
}

// HashPath returns a git-style content-addressable path: objects/{hash[:2]}/{hash[2:]}.
func HashPath(hash string) string {
	if len(hash) < 2 {
		return "objects/" + hash
	}
	return fmt.Sprintf("objects/%s/%s", hash[:2], hash[2:])
}

// ComputeHash reads all data from reader, computes SHA256, and returns the hex hash and data.
func ComputeHash(reader io.Reader) (hash string, data []byte, err error) {
	buf, err := io.ReadAll(reader)
	if err != nil {
		return "", nil, err
	}
	h := sha256.Sum256(buf)
	hash = fmt.Sprintf("%x", h)
	return hash, buf, nil
}
