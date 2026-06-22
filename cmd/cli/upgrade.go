package main

import (
	"fmt"
)

func cmdUpgrade(cfg *appConfig, args []string) error {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Println(`stora-cli upgrade — 系统升级

自动执行待迁移（调用 migrate up）。

用法:
  stora-cli upgrade [--dry-run]

选项:
  --dry-run  仅预览待执行迁移`)
			return nil
		}
	}

	dryRun := false
	for _, a := range args {
		if a == "--dry-run" {
			dryRun = true
		}
	}

	migrateArgs := []string{"up"}
	if dryRun {
		migrateArgs = append(migrateArgs, "--dry-run")
	}

	fmt.Println(styled("  🔍 检查系统状态...", styleBold))
	if err := cmdMigrate(cfg, migrateArgs); err != nil {
		return fmt.Errorf("升级失败: %w", err)
	}

	fmt.Println(styled("\n  ✅ 系统升级完成", colorGreen))
	return nil
}
