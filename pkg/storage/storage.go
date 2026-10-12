package storage

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"
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
//
// Deprecated: prefer ObjectKey. This variant carries an "objects/" prefix that
// no driver actually uses, so it does not describe the layout on disk.
func HashPath(hash string) string {
	if len(hash) < 2 {
		return "objects/" + hash
	}
	return fmt.Sprintf("objects/%s/%s", hash[:2], hash[2:])
}

// ObjectKey returns the content-addressed key for a SHA256 hex digest:
// "<hash[:2]>/<hash[2:]>".
//
// Every driver must agree on this layout: it is what `file_items.file_path`
// stores, so switching backends (local <-> S3/RustFS) is a plain object copy
// and never requires rewriting rows.
func ObjectKey(hash string) string {
	if len(hash) < 2 {
		return hash
	}
	return hash[:2] + "/" + hash[2:]
}

// ValidateKey rejects storage keys that could escape the storage root. Both
// drivers call it before touching a key, so a malicious or corrupted
// file_path can never address something outside the object namespace.
func ValidateKey(key string) error {
	if key == "" {
		return errors.New("empty storage key")
	}
	if strings.HasPrefix(key, "/") || strings.ContainsAny(key, "\\\x00") {
		return fmt.Errorf("invalid storage key %q", key)
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("invalid segment in storage key %q", key)
		}
	}
	return nil
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
