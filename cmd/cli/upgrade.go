package main

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/Athenavi/Stora/pkg/database"
)

func cmdUpgrade(cfg *appConfig, args []string) error {
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
		fmt.Println(`stora-cli upgrade — 系统升级

自动执行数据库迁移（使用内嵌迁移 + 外部迁移文件）。

用法:
  stora-cli upgrade [--dry-run]

选项:
  --dry-run  仅显示待执行操作，不实际执行`)
		return nil
	}

	fmt.Println(styled("  🔍 检查系统版本...", styleBold))

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

	// Check current version
	applied, err := getAppliedVersions(db)
	if err != nil {
		return fmt.Errorf("查询版本失败: %w", err)
	}

	// Determine pending embedded migrations
	var pending []embeddedMigration
	for _, m := range coreMigrations {
		if !applied[m.Version] {
			pending = append(pending, m)
		}
	}

	if len(pending) == 0 {
		latest := 0
		if len(coreMigrations) > 0 {
			latest = coreMigrations[len(coreMigrations)-1].Version
		}
		fmt.Printf("  ✅ 系统已最新 (v%d)\n", latest)
		return nil
	}

	fmt.Printf("  发现 %d 个待执行迁移:\n", len(pending))
	for _, m := range pending {
		fmt.Printf("    · v%d — %s\n", m.Version, m.Description)
	}

	if dryRun {
		fmt.Println(styled("\n  使用 --dry-run 模式，未实际执行。去掉 --dry-run 运行。", colorYellow))
		return nil
	}

	// Confirm with user
	fmt.Print(styled("  是否执行升级? [Y/n] ", styleBold))
	var confirm string
	fmt.Scanln(&confirm)
	if confirm == "n" || confirm == "N" {
		fmt.Println("  已取消")
		return nil
	}

	// Execute
	fmt.Println(styled("  开始执行迁移...", styleBold))
	for _, m := range pending {
		if err := applyEmbeddedMigration(db, m); err != nil {
			return fmt.Errorf("迁移 v%d (%s) 失败: %w", m.Version, m.Description, err)
		}
		fmt.Printf(styled("  ✅ v%d %s\n", colorGreen), m.Version, m.Description)
	}

	// Check for file-system migrations too
	fsMigrations, err := loadMigrationFiles("migrations")
	if err == nil && len(fsMigrations) > 0 {
		var fsPending int
		for _, m := range fsMigrations {
			if !applied[m.Version] {
				fsPending++
			}
		}
		if fsPending > 0 {
			fmt.Printf(styled("\n  还有 %d 个自定义迁移文件在 migrations/ 目录中\n", colorYellow), fsPending)
			fmt.Println("  运行 'stora-cli migrate' 执行它们")
		}
	}

	fmt.Println(styled("\n  ✅ 系统升级完成", colorGreen))
	return nil
}

func applyEmbeddedMigration(db *sql.DB, m embeddedMigration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(m.SQL); err != nil {
		return fmt.Errorf("SQL 执行失败: %w", err)
	}

	now := time.Now().Format(time.RFC3339)
	if _, err := tx.Exec(
		`INSERT INTO schema_version (version, description, applied_at, checksum) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (version) DO UPDATE SET applied_at = $3`,
		m.Version, m.Description, now, "",
	); err != nil {
		return fmt.Errorf("记录版本失败: %w", err)
	}

	return tx.Commit()
}
