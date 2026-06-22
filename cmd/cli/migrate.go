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

// ── 迁移文件模型 ──────────────────────────────────

type migrationFile struct {
	Version     int
	Description string
	Filename    string
	Content     string
	Checksum    string
}

// ── 主命令 ────────────────────────────────────────

func cmdMigrate(cfg *appConfig, args []string) error {
	// Determine subcommand
	sub := ""
	subArgs := args
	if len(args) > 0 && args[0][0] != '-' {
		sub = args[0]
		subArgs = args[1:]
	}

	switch sub {
	case "generate":
		return cmdMigrateGenerate(cfg, subArgs)
	case "up":
		return cmdMigrateUp(cfg, subArgs)
	case "list", "ls":
		return cmdMigrateList(cfg, subArgs)
	case "current":
		return cmdMigrateCurrent(cfg, subArgs)
	case "", "help":
		printMigrateHelp()
		return nil
	default:
		fmt.Printf("未知子命令: %s\n\n", sub)
		printMigrateHelp()
		return nil
	}
}

func printMigrateHelp() {
	fmt.Println(`stora-cli migrate — 数据库版本迁移管理

子命令:
  generate   从 config/models.yaml 生成迁移 SQL 文件
  up         执行待迁移（写入 schema_version + 更新 version.ini）
  list       列出所有迁移文件及其状态
  current    显示当前数据库版本

用法:
  stora-cli migrate generate [--description "说明"]
  stora-cli migrate up [--dry-run]
  stora-cli migrate list
  stora-cli migrate current`)
}

// ── generate: 从 models.yaml 生成迁移 ─────────────

func cmdMigrateGenerate(cfg *appConfig, args []string) error {
	desc := ""
	for i, a := range args {
		if a == "--description" && i+1 < len(args) {
			desc = args[i+1]
		}
		if a == "-h" || a == "--help" {
			fmt.Println(`stora-cli migrate generate — 生成迁移

从 config/models.yaml 生成迁移 SQL 文件到 migrations/ 目录。

用法:
  stora-cli migrate generate [--description "说明"]

说明:
  读取 config/models.yaml 中的模型定义
  对比 version.ini 中的当前版本
  生成增量迁移 SQL`)
			return nil
		}
	}

	// Read schema definition
	schema, err := ParseSchema("config/models.yaml")
	if err != nil {
		return fmt.Errorf("读取 models.yaml 失败: %w", err)
	}

	// Read current version
	currentVer, err := readVersionINI("version.ini")
	if err != nil {
		return fmt.Errorf("读取 version.ini 失败: %w", err)
	}

	if schema.Version <= currentVer {
		fmt.Printf("✅ schema 无变更 (v%d)\n", currentVer)
		return nil
	}

	// Ensure migrations dir
	if err := os.MkdirAll("migrations", 0755); err != nil {
		return err
	}

	// Build migration SQL for all models
	var b strings.Builder
	b.WriteString(fmt.Sprintf("-- Migration v%d: %s\n", schema.Version, desc))
	b.WriteString(fmt.Sprintf("-- Generated from config/models.yaml\n"))
	b.WriteString(fmt.Sprintf("-- %s\n\n", time.Now().Format(time.RFC3339)))

	// Generate CREATE TABLE for each model
	var modelNames []string
	for n := range schema.Models {
		modelNames = append(modelNames, n)
	}
	sort.Strings(modelNames)

	for _, name := range modelNames {
		model := schema.Models[name]
		b.WriteString(fmt.Sprintf("-- %s: %s\n", name, model.Description))
		b.WriteString(model.GenerateCreateSQL())
		b.WriteString("\n")
	}

	// Write migration file
	timestamp := time.Now().Format("20060102_150405")
	nextVer := currentVer + 1
	descPart := "update"
	if desc != "" {
		descPart = desc
	}
	filename := fmt.Sprintf("%s_%s.sql", timestamp, descPart)
	// Also embed version number
	content := b.String()

	if err := os.WriteFile(filepath.Join("migrations", filename), []byte(content), 0644); err != nil {
		return fmt.Errorf("写入迁移文件失败: %w", err)
	}

	// Update version.ini
	if err := writeVersionINI("version.ini", schema.Version); err != nil {
		return fmt.Errorf("更新 version.ini 失败: %w", err)
	}

	fmt.Printf("  ✅ 生成迁移: migrations/%s (v%d)\n", filename, nextVer)
	fmt.Printf("  运行 'stora-cli migrate up' 执行迁移\n")
	return nil
}

// ── up: 执行迁移 ──────────────────────────────────

func cmdMigrateUp(cfg *appConfig, args []string) error {
	dryRun := false
	for _, a := range args {
		if a == "--dry-run" {
			dryRun = true
		}
		if a == "-h" || a == "--help" {
			fmt.Println(`stora-cli migrate up — 执行待迁移

用法:
  stora-cli migrate up [--dry-run]

选项:
  --dry-run  仅显示待执行迁移，不实际执行`)
			return nil
		}
	}

	db, err := database.Connect(cfg.PostgresDSN(), 2, 1)
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}
	defer database.Close()

	// Ensure schema_version table
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version     INTEGER PRIMARY KEY,
		description TEXT NOT NULL DEFAULT '',
		applied_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		checksum    TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		return fmt.Errorf("创建 schema_version 表失败: %w", err)
	}

	// Load migrations from filesystem
	migrations, err := loadMigrationFiles("migrations")
	if err != nil {
		return fmt.Errorf("加载迁移文件失败: %w", err)
	}

	if len(migrations) == 0 {
		// Fallback: try embedded migrations
		fmt.Println("📭 没有外部迁移文件，尝试内嵌迁移...")
		return runEmbeddedMigrations(db, dryRun)
	}

	// Query applied versions
	applied, err := getAppliedVersions(db)
	if err != nil {
		return fmt.Errorf("查询已应用版本失败: %w", err)
	}

	var pending []migrationFile
	for _, m := range migrations {
		if !applied[m.Version] {
			pending = append(pending, m)
		}
	}

	if len(pending) == 0 {
		fmt.Println("✅ 所有迁移已应用")
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
		return nil
	}

	for _, m := range pending {
		if err := applyMigrationFile(db, m); err != nil {
			return fmt.Errorf("迁移 %d 失败: %w", m.Version, err)
		}
		fmt.Printf("  ✅ v%d %s\n", m.Version, m.Description)
	}

	// Update version.ini to latest applied
	if len(migrations) > 0 {
		writeVersionINI("version.ini", migrations[len(migrations)-1].Version)
	}

	fmt.Println("✅ 迁移完成")
	return nil
}

// ── list: 列出迁移 ────────────────────────────────

func cmdMigrateList(cfg *appConfig, args []string) error {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Println(`stora-cli migrate list — 列出迁移文件

用法:
  stora-cli migrate list`)
			return nil
		}
	}

	migrations, err := loadMigrationFiles("migrations")
	if err != nil {
		return fmt.Errorf("加载迁移文件失败: %w", err)
	}

	// Try reading DB applied versions
	applied := make(map[int]bool)
	db, err := database.Connect(cfg.PostgresDSN(), 1, 1)
	if err == nil {
		applied, _ = getAppliedVersions(db)
		database.Close()
	}

	if len(migrations) == 0 {
		fmt.Println("📭 没有迁移文件")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "版本\t状态\t文件")
	fmt.Fprintln(w, "----\t----\t----")
	for _, m := range migrations {
		status := "⏳ 待执行"
		if applied[m.Version] {
			status = "✅ 已应用"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\n", m.Version, status, m.Filename)
	}
	w.Flush()
	return nil
}

// ── current: 显示当前版本 ─────────────────────────

func cmdMigrateCurrent(cfg *appConfig, args []string) error {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Println(`stora-cli migrate current — 显示当前版本

用法:
  stora-cli migrate current`)
			return nil
		}
	}

	// version.ini
	ver, err := readVersionINI("version.ini")
	if err != nil {
		fmt.Printf("⚠️  version.ini 读取失败: %v\n", err)
	} else {
		fmt.Printf("📄 version.ini: v%d\n", ver)
	}

	// schema_version table
	db, err := database.Connect(cfg.PostgresDSN(), 1, 1)
	if err == nil {
		var maxVer int
		db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&maxVer)
		fmt.Printf("🗄️  schema_version (DB): v%d\n", maxVer)
		database.Close()
	} else {
		fmt.Printf("⚠️  数据库连接失败: %v\n", err)
	}

	// config/models.yaml
	schema, err := ParseSchema("config/models.yaml")
	if err == nil {
		fmt.Printf("📦 config/models.yaml: v%d (%d 个模型)\n", schema.Version, len(schema.Models))
	}

	return nil
}

// ── 辅助函数 ──────────────────────────────────────

func loadMigrationFiles(dir string) ([]migrationFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var migrations []migrationFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		// Parse version: "20260622_143000_description.sql" or "001_description.sql"
		parts := strings.SplitN(e.Name(), "_", 2)
		version := 0
		if v, err := strconv.Atoi(parts[0]); err == nil {
			version = v
		} else if len(parts[0]) == 8 { // timestamp prefix like 20260622
			// Use hash of timestamp as version number
			h := sha256.Sum256([]byte(parts[0]))
			version = int(h[0])<<8 | int(h[1]) // deterministic, not too large
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

		migrations = append(migrations, migrationFile{
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

func runEmbeddedMigrations(db *sql.DB, dryRun bool) error {
	applied, err := getAppliedVersions(db)
	if err != nil {
		return err
	}
	for _, m := range coreMigrations {
		if applied[m.Version] {
			continue
		}
		if dryRun {
			fmt.Printf("  📋 v%d %s\n", m.Version, m.Description)
			continue
		}
		if err := applyEmbeddedMigration(db, m); err != nil {
			return fmt.Errorf("迁移 v%d 失败: %w", m.Version, err)
		}
		fmt.Printf("  ✅ v%d %s\n", m.Version, m.Description)
	}
	return nil
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

func applyMigrationFile(db *sql.DB, m migrationFile) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(m.Content); err != nil {
		return fmt.Errorf("SQL 执行失败: %w", err)
	}

	now := time.Now().Format(time.RFC3339)
	if _, err := tx.Exec(
		`INSERT INTO schema_version (version, description, applied_at, checksum)
		 VALUES ($1, $2, $3, $4) ON CONFLICT (version) DO NOTHING`,
		m.Version, m.Description, now, m.Checksum,
	); err != nil {
		return fmt.Errorf("记录版本失败: %w", err)
	}
	return tx.Commit()
}
