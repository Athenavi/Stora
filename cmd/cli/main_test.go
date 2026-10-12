package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeMigrationFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMigrations(t *testing.T) {
	dir := t.TempDir()

	writeMigrationFile(t, dir, "002_add_email.sql",
		"-- UPGRADE\nALTER TABLE users ADD email TEXT;\n-- DOWNGRADE\nALTER TABLE users DROP COLUMN email;")
	writeMigrationFile(t, dir, "010_add_indexes.sql",
		"-- UPGRADE\nCREATE INDEX idx_users ON users(id);")
	writeMigrationFile(t, dir, "001_create_users.sql", "CREATE TABLE users (id INT);")
	writeMigrationFile(t, dir, "not_a_migration.txt", "this should be skipped")
	writeMigrationFile(t, dir, "readme.md", "also skipped")

	migrations, err := loadMigrations(dir)
	if err != nil {
		t.Fatalf("loadMigrations failed: %v", err)
	}
	if len(migrations) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(migrations))
	}

	// Sorted by revision, ascending
	wantRevisions := []string{"001_create_users", "002_add_email", "010_add_indexes"}
	for i, want := range wantRevisions {
		if migrations[i].Revision != want {
			t.Errorf("migration %d: revision = %q, want %q", i, migrations[i].Revision, want)
		}
	}

	// -- UPGRADE / -- DOWNGRADE sections are split apart
	withSections := migrations[1]
	if withSections.UpSQL != "ALTER TABLE users ADD email TEXT;" {
		t.Errorf("UpSQL = %q", withSections.UpSQL)
	}
	if withSections.DownSQL != "ALTER TABLE users DROP COLUMN email;" {
		t.Errorf("DownSQL = %q", withSections.DownSQL)
	}

	// Without markers the whole file is the upgrade
	if migrations[0].UpSQL != "CREATE TABLE users (id INT);" {
		t.Errorf("fallback UpSQL = %q", migrations[0].UpSQL)
	}
	if migrations[0].DownSQL != "" {
		t.Errorf("expected empty DownSQL, got %q", migrations[0].DownSQL)
	}

	// Every migration carries a description and a content-derived checksum
	for _, m := range migrations {
		if m.Description == "" {
			t.Errorf("migration %q has no description", m.Revision)
		}
		if m.Checksum == "" {
			t.Errorf("migration %q has no checksum", m.Revision)
		}
	}
	if migrations[0].Checksum == migrations[1].Checksum {
		t.Error("checksums should differ for different content")
	}
}

func TestLoadMigrationsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	migrations, err := loadMigrations(dir)
	if err != nil {
		t.Fatalf("loadMigrations failed for empty dir: %v", err)
	}
	if len(migrations) != 0 {
		t.Errorf("expected 0 migrations, got %d", len(migrations))
	}
}

func TestLoadMigrationsNonExistentDir(t *testing.T) {
	migrations, err := loadMigrations("/nonexistent/path")
	if err != nil {
		t.Fatalf("loadMigrations failed for nonexistent dir: %v", err)
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
