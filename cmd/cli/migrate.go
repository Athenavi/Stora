package main

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Athenavi/Stora/pkg/database"
)

// migration represents a single migration file.
type migration struct {
	Version     int
	Description string
	Filename    string
	Content     string
	Checksum    string
}

func cmdMigrate(cfg *appConfig, args []string) error {
	// Parse flags
	showHelp := false
	dryRun := false
	for _, a := range args {
		switch a {
		case "-h", "--help":
			showHelp = true
		case "--dry-run":
			dryRun = true
		}
	}
	if showHelp {
		fmt.Println(`stora-cli migrate — 执行数据库迁移

用法:
  stora-cli migrate [--dry-run]

选项:
  --dry-run  仅显示待执行迁移，不实际执行`)
		return nil
	}

	db, err := database.Connect(cfg.PostgresDSN(), 2, 1)
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}
	defer database.Close()

	// Ensure schema_version table exists
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version     INTEGER PRIMARY KEY,
		description TEXT NOT NULL DEFAULT '',
		applied_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		checksum    TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return fmt.Errorf("创建 schema_version 表失败: %w", err)
	}

	// Load migrations from filesystem
	migrations, err := loadMigrations("migrations")
	if err != nil {
		return fmt.Errorf("加载迁移文件失败: %w", err)
	}

	if len(migrations) == 0 {
		fmt.Println("📭 没有找到迁移文件")
		return nil
	}

	// Query applied versions
	applied, err := getAppliedVersions(db)
	if err != nil {
		return fmt.Errorf("查询已应用版本失败: %w", err)
	}

	// Determine pending
	var pending []migration
	for _, m := range migrations {
		if !applied[m.Version] {
			pending = append(pending, m)
		}
	}

	if len(pending) == 0 {
		fmt.Println("✅ 所有迁移已应用（最新版本:", migrations[len(migrations)-1].Version, "）")
		return nil
	}

	if dryRun {
		fmt.Println("📋 待执行的迁移:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "版本\t描述\t文件")
		fmt.Fprintln(w, "----\t----\t----")
		for _, m := range pending {
			fmt.Fprintf(w, "%d\t%s\t%s\n", m.Version, m.Description, m.Filename)
		}
		w.Flush()
		fmt.Printf("\n共 %d 个迁移待执行\n", len(pending))
		return nil
	}

	// Apply pending migrations
	fmt.Printf("开始执行 %d 个迁移...\n", len(pending))
	for _, m := range pending {
		if err := applyMigration(db, m); err != nil {
			return fmt.Errorf("迁移 %d (%s) 失败: %w", m.Version, m.Description, err)
		}
		fmt.Printf("  ✅ v%d %s\n", m.Version, m.Description)
	}
	fmt.Println("✅ 所有迁移执行完成")
	return nil
}

func loadMigrations(dir string) ([]migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var migrations []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}

		// Parse version from filename: "001_description.sql"
		parts := strings.SplitN(e.Name(), "_", 2)
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue // skip files that don't match the pattern
		}

		desc := strings.TrimSuffix(e.Name(), ".sql")
		if len(parts) > 1 {
			desc = strings.TrimSuffix(parts[1], ".sql")
		}

		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("读取 %s 失败: %w", e.Name(), err)
		}

		checksum := fmt.Sprintf("%x", sha256.Sum256(content))

		migrations = append(migrations, migration{
			Version:     version,
			Description: desc,
			Filename:    e.Name(),
			Content:     string(content),
			Checksum:    checksum,
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

func getAppliedVersions(db *sql.DB) (map[int]bool, error) {
	rows, err := db.Query(`SELECT version FROM schema_version ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func applyMigration(db *sql.DB, m migration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute migration SQL
	if _, err := tx.Exec(m.Content); err != nil {
		return fmt.Errorf("SQL 执行失败: %w", err)
	}

	// Record version
	now := time.Now().Format(time.RFC3339)
	if _, err := tx.Exec(
		`INSERT INTO schema_version (version, description, applied_at, checksum) VALUES ($1, $2, $3, $4)`,
		m.Version, m.Description, now, m.Checksum,
	); err != nil {
		return fmt.Errorf("记录版本失败: %w", err)
	}

	return tx.Commit()
}
