package main

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Athenavi/Stora/pkg/database"
)

// ── 迁移文件结构 ──────────────────────────────────

type migrationFile struct {
	Revision    string // 文件名（不含 .sql）：20260622_151246_initial_schema
	Description string //
	UpSQL       string // -- UPGRADE 段
	DownSQL     string // -- DOWNGRADE 段
	Checksum    string
}

// ── 主命令 ────────────────────────────────────────

func cmdMigrate(cfg *appConfig, args []string) error {
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
	case "down":
		return cmdMigrateDown(cfg, subArgs)
	case "history", "log":
		return cmdMigrateHistory(cfg, subArgs)
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
	fmt.Println(`stora-cli migrate — 数据库迁移管理（Alembic 风格）

子命令:
  generate             从 config/models.yaml 生成迁移（UPGRADE + DOWNGRADE）
  up [N]               向前执行 N 个迁移（默认全部）
  down [N]             回退 N 个迁移
  history              列出全部迁移及其状态
  current              显示当前 revision

用法:
  stora-cli migrate generate [--description "说明"]
  stora-cli migrate up [--dry-run] [N]
  stora-cli migrate down [N]
  stora-cli migrate history
  stora-cli migrate current`)
}

// ══════════════════════════════════════════════════
// generate: 从 models.yaml 生成迁移文件
// ══════════════════════════════════════════════════

func cmdMigrateGenerate(cfg *appConfig, args []string) error {
	desc := ""
	for i, a := range args {
		if a == "--description" && i+1 < len(args) {
			desc = args[i+1]
		}
		if a == "-h" || a == "--help" {
			fmt.Println(`stora-cli migrate generate — 生成迁移

从 config/models.yaml 读取当前期望的 schema 定义，
生成一个包含 UPGRADE（建表）和 DOWNGRADE（删表）的迁移文件。

用法:
  stora-cli migrate generate [--description "说明"]`)
			return nil
		}
	}

	schema, err := ParseSchema("config/models.yaml")
	if err != nil {
		return fmt.Errorf("读取 models.yaml 失败: %w", err)
	}

	if err := os.MkdirAll("migrations", 0755); err != nil {
		return err
	}

	upSQL, downSQL := schema.GenerateFullMigration()
	timestamp := time.Now().Format("20060102_150405")
	descPart := desc
	if descPart == "" {
		descPart = "schema_update"
	}
	rev := fmt.Sprintf("%s_%s", timestamp, descPart)
	filename := rev + ".sql"

	content := fmt.Sprintf("-- UPGRADE\n%s\n-- DOWNGRADE\n%s", upSQL, downSQL)
	if err := os.WriteFile(filepath.Join("migrations", filename), []byte(content), 0644); err != nil {
		return fmt.Errorf("写入迁移文件失败: %w", err)
	}

	fmt.Printf("  ✅ 生成迁移: migrations/%s\n", filename)
	fmt.Printf("  运行 'stora-cli migrate up' 执行\n")
	return nil
}

// ══════════════════════════════════════════════════
// up: 向前执行迁移
// ══════════════════════════════════════════════════

func cmdMigrateUp(cfg *appConfig, args []string) error {
	dryRun := false
	target := -1 // 默认全部
	for _, a := range args {
		if a == "--dry-run" {
			dryRun = true
		} else if a[0] >= '0' && a[0] <= '9' {
			target, _ = fmt.Sscanf(a, "%d", &target)
			_ = target // reassign
		} else if a == "-h" || a == "--help" {
			printMigrateUpHelp()
			return nil
		}
	}
	// Parse N as last numeric arg
	for _, a := range args {
		if a[0] >= '0' && a[0] <= '9' {
			fmt.Sscanf(a, "%d", &target)
		}
	}

	db, err := database.Connect(cfg.PostgresDSN(), 2, 1)
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}
	defer database.Close()

	ensureSchemaTable(db)
	migrations, err := loadMigrations("migrations")
	if err != nil {
		return err
	}
	if len(migrations) == 0 {
		fmt.Println("📭 migrations/ 目录没有迁移文件")
		return nil
	}

	applied, err := getAppliedRevisions(db)
	if err != nil {
		return err
	}

	// 找到第一个未应用的
	var pending []migrationFile
	for _, m := range migrations {
		if !applied[m.Revision] {
			pending = append(pending, m)
		}
	}
	if len(pending) == 0 {
		fmt.Println("✅ 全部迁移已应用")
		return nil
	}

	// 限制数量
	count := len(pending)
	if target > 0 && target < count {
		count = target
	}
	toApply := pending[:count]

	if dryRun {
		fmt.Printf("📋 待执行迁移 (%d):\n", len(toApply))
		for _, m := range toApply {
			fmt.Printf("   ⏳ %s — %s\n", m.Revision, m.Description)
		}
		return nil
	}

	for _, m := range toApply {
		if err := applyUp(db, m); err != nil {
			return fmt.Errorf("迁移 %s 失败: %w", m.Revision, err)
		}
		fmt.Printf("  ✅ %s — %s\n", m.Revision, m.Description)
	}

	// 更新 version.ini 为最后一个应用的 revision
	last := toApply[len(toApply)-1]
	writeRevisionINI("version.ini", last.Revision)
	fmt.Printf("  当前 revision: %s\n", last.Revision)
	return nil
}

func printMigrateUpHelp() {
	fmt.Println(`stora-cli migrate up — 执行迁移

用法:
  stora-cli migrate up [N] [--dry-run]

  N         执行 N 个迁移（默认全部）
  --dry-run 仅预览，不实际执行`)
}

// ══════════════════════════════════════════════════
// down: 回退迁移
// ══════════════════════════════════════════════════

func cmdMigrateDown(cfg *appConfig, args []string) error {
	target := 1 // 默认回退 1 个
	for _, a := range args {
		if a[0] >= '0' && a[0] <= '9' {
			fmt.Sscanf(a, "%d", &target)
		}
		if a == "-h" || a == "--help" {
			fmt.Println(`stora-cli migrate down — 回退迁移

用法:
  stora-cli migrate down [N]

  N  回退 N 个迁移（默认 1）`)
			return nil
		}
	}
	if target < 1 {
		target = 1
	}

	db, err := database.Connect(cfg.PostgresDSN(), 2, 1)
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}
	defer database.Close()

	ensureSchemaTable(db)
	migrations, err := loadMigrations("migrations")
	if err != nil {
		return err
	}

	applied, err := getAppliedRevisions(db)
	if err != nil {
		return err
	}

	// 从后往前找已应用的迁移
	var toRollback []migrationFile
	for i := len(migrations) - 1; i >= 0 && len(toRollback) < target; i-- {
		if applied[migrations[i].Revision] {
			toRollback = append([]migrationFile{migrations[i]}, toRollback...)
		}
	}
	if len(toRollback) == 0 {
		fmt.Println("📭 没有已应用的迁移可回退")
		return nil
	}

	// 回退顺序：从新到旧
	for i := len(toRollback) - 1; i >= 0; i-- {
		m := toRollback[i]
		if err := applyDown(db, m); err != nil {
			return fmt.Errorf("回退 %s 失败: %w", m.Revision, err)
		}
		fmt.Printf("  🔙 %s — %s\n", m.Revision, m.Description)
	}

	// 回退后，找到最新的仍已应用的 revision
	// Simply find the latest migration whose revision is STILL in schema_version
	var newRev string
	for _, m := range migrations {
		if applied[m.Revision] {
			// Check if it was rolled back in this batch
			rolledBack := false
			for _, rb := range toRollback {
				if rb.Revision == m.Revision {
					rolledBack = true
					break
				}
			}
			if !rolledBack {
				newRev = m.Revision
			}
		}
	}
	writeRevisionINI("version.ini", newRev)
	fmt.Printf("  当前 revision: %s\n", newRev)
	return nil
}

func isApplied(rev string, applied map[string]bool, rolledBack []migrationFile) bool {
	for _, rb := range rolledBack {
		if rb.Revision == rev {
			return false
		}
	}
	return applied[rev]
}

// ══════════════════════════════════════════════════
// history: 列出迁移状态
// ══════════════════════════════════════════════════

func cmdMigrateHistory(cfg *appConfig, args []string) error {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Println(`stora-cli migrate history — 列出迁移历史

用法:
  stora-cli migrate history`)
			return nil
		}
	}

	migrations, err := loadMigrations("migrations")
	if err != nil {
		return err
	}

	applied := make(map[string]bool)
	db, err := database.Connect(cfg.PostgresDSN(), 1, 1)
	if err == nil {
		applied, _ = getAppliedRevisions(db)
		database.Close()
	}

	if len(migrations) == 0 {
		fmt.Println("📭 migrations/ 目录没有迁移文件")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "状态\trevision\t描述")
	fmt.Fprintln(w, "----\t--------\t----")
	for _, m := range migrations {
		status := "⏳"
		if applied[m.Revision] {
			status = "✅"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", status, m.Revision, m.Description)
	}
	w.Flush()
	return nil
}

// ══════════════════════════════════════════════════
// current: 显示当前 revision
// ══════════════════════════════════════════════════

func cmdMigrateCurrent(cfg *appConfig, args []string) error {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Println(`stora-cli migrate current — 显示当前 revision

用法:
  stora-cli migrate current`)
			return nil
		}
	}

	rev, _ := readRevisionINI("version.ini")
	if rev != "" {
		fmt.Printf("📄 version.ini → %s\n", rev)
	} else {
		fmt.Println("📄 version.ini → (未设置)")
	}

	db, err := database.Connect(cfg.PostgresDSN(), 1, 1)
	if err == nil {
		var count int
		db.QueryRow(`SELECT COUNT(*) FROM schema_version`).Scan(&count)
		var latest string
		db.QueryRow(`SELECT revision FROM schema_version ORDER BY revision DESC LIMIT 1`).Scan(&latest)
		fmt.Printf("🗄️  schema_version (DB): %d 条记录, 最新: %s\n", count, latest)
		database.Close()
	}

	schema, err := ParseSchema("config/models.yaml")
	if err == nil {
		fmt.Printf("📦 config/models.yaml: %d 个模型\n", len(schema.Models))
	}

	return nil
}

// ══════════════════════════════════════════════════
// 辅助函数
// ══════════════════════════════════════════════════

func ensureSchemaTable(db *sql.DB) {
	db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		revision     VARCHAR(64) PRIMARY KEY,
		description  TEXT NOT NULL DEFAULT '',
		applied_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
}

// loadMigrations 扫描 migrations/ 目录，解析 -- UPGRADE / -- DOWNGRADE 段
func loadMigrations(dir string) ([]migrationFile, error) {
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
		rev := strings.TrimSuffix(e.Name(), ".sql")
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("读取 %s 失败: %w", e.Name(), err)
		}

		text := string(content)
		upSQL, downSQL := splitMigration(text)

		// Description: extract from filename after timestamp
		desc := rev
		parts := strings.SplitN(rev, "_", 3)
		if len(parts) >= 3 {
			desc = parts[2] // after YYYYMMDD_HHMMSS_
		}
		desc = strings.ReplaceAll(desc, "_", " ")

		checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(upSQL+downSQL)))

		migrations = append(migrations, migrationFile{
			Revision:    rev,
			Description: desc,
			UpSQL:       upSQL,
			DownSQL:     downSQL,
			Checksum:    checksum,
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Revision < migrations[j].Revision
	})
	return migrations, nil
}

// splitMigration 按 -- UPGRADE / -- DOWNGRADE 标记拆分 SQL
func splitMigration(text string) (up, down string) {
	upper := strings.ToUpper(text)

	upIdx := strings.Index(upper, "-- UPGRADE")
	downIdx := strings.Index(upper, "-- DOWNGRADE")

	if upIdx >= 0 && downIdx > upIdx {
		up = strings.TrimSpace(text[upIdx+len("-- UPGRADE") : downIdx])
		down = strings.TrimSpace(text[downIdx+len("-- DOWNGRADE"):])
	} else if upIdx >= 0 {
		up = strings.TrimSpace(text[upIdx+len("-- UPGRADE"):])
	} else {
		up = text // fallback: whole file
	}
	return
}

func getAppliedRevisions(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query(`SELECT revision FROM schema_version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	applied := make(map[string]bool)
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		applied[r] = true
	}
	return applied, rows.Err()
}

func applyUp(db *sql.DB, m migrationFile) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if m.UpSQL != "" {
		if _, err := tx.Exec(m.UpSQL); err != nil {
			return fmt.Errorf("UPGRADE SQL 失败: %w", err)
		}
	}
	now := time.Now().Format(time.RFC3339)
	if _, err := tx.Exec(
		`INSERT INTO schema_version (revision, description, applied_at) VALUES ($1, $2, $3)
		 ON CONFLICT (revision) DO NOTHING`,
		m.Revision, m.Description, now,
	); err != nil {
		return fmt.Errorf("记录 revision 失败: %w", err)
	}
	return tx.Commit()
}

func applyDown(db *sql.DB, m migrationFile) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete schema_version record FIRST, before DOWNGRADE SQL drops the table
	if _, err := tx.Exec(`DELETE FROM schema_version WHERE revision = $1`, m.Revision); err != nil {
		return fmt.Errorf("删除 revision 记录失败: %w", err)
	}

	if m.DownSQL != "" {
		if _, err := tx.Exec(m.DownSQL); err != nil {
			return fmt.Errorf("DOWNGRADE SQL 失败: %w", err)
		}
	}
	return tx.Commit()
}
