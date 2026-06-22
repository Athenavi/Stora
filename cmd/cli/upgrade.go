package main

import (
	"fmt"
)

func cmdUpgrade(cfg *appConfig, args []string) error {
	// Show help
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Println(`stora-cli upgrade — 系统升级

自动执行数据库迁移和数据迁移任务。

用法:
  stora-cli upgrade [--dry-run]

选项:
  --dry-run  仅显示待执行操作，不实际执行`)
			return nil
		}
	}

	// upgrade is essentially migrate + any data migration tasks
	fmt.Println("🔍 检查系统版本...")

	// Delegate to migrate
	dryRun := false
	for _, a := range args {
		if a == "--dry-run" {
			dryRun = true
			break
		}
	}

	migrateArgs := args
	if dryRun {
		migrateArgs = []string{"--dry-run"}
	}
	if err := cmdMigrate(cfg, migrateArgs); err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
	}

	fmt.Println("\n✅ 系统升级完成")
	return nil
}
