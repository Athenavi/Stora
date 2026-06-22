package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMigrations(t *testing.T) {
	// Create a temp directory with test migration files
	dir := t.TempDir()

	files := []struct {
		name    string
		content string
	}{
		{"001_create_users.sql", "CREATE TABLE users (id INT);"},
		{"002_add_email.sql", "ALTER TABLE users ADD email TEXT;"},
		{"010_add_indexes.sql", "CREATE INDEX idx_users ON users(id);"},
		{"not_a_migration.txt", "this should be skipped"},
		{"readme.md", "also skipped"},
	}

	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f.name), []byte(f.content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	migrations, err := loadMigrationFiles(dir)
	if err != nil {
		t.Fatalf("loadMigrationFiles failed: %v", err)
	}

	if len(migrations) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(migrations))
	}

	// Verify ordering
	if migrations[0].Version != 1 {
		t.Errorf("expected version 1 first, got %d", migrations[0].Version)
	}
	if migrations[1].Version != 2 {
		t.Errorf("expected version 2 second, got %d", migrations[1].Version)
	}
	if migrations[2].Version != 10 {
		t.Errorf("expected version 10 third, got %d", migrations[2].Version)
	}

	// Verify checksums are computed
	for _, m := range migrations {
		if m.Checksum == "" {
			t.Errorf("migration %d has no checksum", m.Version)
		}
		if m.Content == "" {
			t.Errorf("migration %d has no content", m.Version)
		}
	}

	// Verify descriptions
	if migrations[0].Description != "create_users" {
		t.Errorf("expected 'create_users', got '%s'", migrations[0].Description)
	}
}

func TestLoadMigrationsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	migrations, err := loadMigrationFiles(dir)
	if err != nil {
		t.Fatalf("loadMigrationFiles failed for empty dir: %v", err)
	}
	if len(migrations) != 0 {
		t.Errorf("expected 0 migrations, got %d", len(migrations))
	}
}

func TestLoadMigrationsNonExistentDir(t *testing.T) {
	migrations, err := loadMigrationFiles("/nonexistent/path")
	if err != nil {
		t.Fatalf("loadMigrationFiles failed for nonexistent dir: %v", err)
	}
	if len(migrations) != 0 {
		t.Errorf("expected 0 migrations, got %d", len(migrations))
	}
}

func TestMaskKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"a", "*"},
		{"abcd", "****"},
		{"abcdefgh", "****efgh"},
		{"my-secret-key-1234", "**************1234"},
	}

	for _, tt := range tests {
		result := maskKey(tt.input)
		if result != tt.expected {
			t.Errorf("maskKey(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
