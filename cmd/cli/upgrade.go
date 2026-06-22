package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const githubOwner = "Athenavi"
const githubRepo = "Stora"

func cmdUpgrade(cfg *appConfig, args []string) error {
	// --version / -v: 显示版本并退出（任何位置）
	showVersion := false
	for _, a := range args {
		if a == "--version" || a == "-v" {
			showVersion = true
		}
	}

	// 解析子命令
	sub := ""
	subArgs := args
	if len(args) > 0 && args[0][0] != '-' {
		sub = args[0]
		subArgs = args[1:]
	}

	switch sub {
	case "check":
		return cmdUpgradeCheck(cfg, subArgs)
	case "apply":
		return cmdUpgradeApply(cfg, subArgs)
	case "", "help":
		if showVersion {
			printCurrentVersion()
			return nil
		}
		printUpgradeHelp()
		return nil
	default:
		if showVersion {
			printCurrentVersion()
			return nil
		}
		// 未知子命令时 fallback 到 migrate up（向后兼容）
		return cmdUpgradeLegacy(cfg, args)
	}
}

func printUpgradeHelp() {
	fmt.Println(`stora-cli upgrade — 系统升级 + 版本管理

子命令:
  check          检查 GitHub 是否有新版本
  apply          应用更新（迁移 + 版本升级）

选项:
  --version, -v  显示当前版本

数据库迁移（向后兼容）:
  stora-cli upgrade [--dry-run]  等同 stora-cli migrate up

当前版本信息保存在项目根目录 version.txt`)
}

func printCurrentVersion() {
	v, err := CurrentVersion()
	if err != nil {
		fmt.Printf("⚠️  %v\n", err)
		return
	}
	fmt.Println()
	fmt.Printf("  %s %s\n", styled("Stora", styleBold+colorCyan), styled(v.FormatVersion(), styleBold))
	fmt.Printf("  %s: %s\n", styled("发布日", styleDim), v.ReleaseDate)
	fmt.Printf("  %s: %s\n", styled("频道", styleDim), v.Channel)
	fmt.Printf("  %s: %s\n", styled("Go", styleDim), v.GoVersion)
	fmt.Println()
}

// ── check: 检查 GitHub Release ───────────────────

func cmdUpgradeCheck(cfg *appConfig, args []string) error {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Println(`stora-cli upgrade check — 检查新版本

从 GitHub Releases 查询最新版本并对比本地版本。

用法:
  stora-cli upgrade check`)
			return nil
		}
	}

	local, err := CurrentVersion()
	if err != nil {
		return fmt.Errorf("读取本地版本失败: %w", err)
	}
	fmt.Printf("  %s: %s\n", styled("本地版本", styleBold), local.FormatVersion())

	fmt.Print("  检查 GitHub Release... ")
	release, err := CheckLatestRelease(githubOwner, githubRepo)
	if err != nil {
		fmt.Println(styled("⏸️  离线", colorYellow))
		fmt.Printf("  %s: %s\n", styled("推荐 tag", styleDim), "git tag V0.1.260622.01")
		fmt.Println("  提示: 创建 git tag 并推送即可触发 GitHub Release")
		return nil
	}
	fmt.Println(styled("✅", colorGreen))

	remoteTag := strings.TrimPrefix(release.TagName, "v")
	remoteTag = strings.TrimPrefix(remoteTag, "V")

	fmt.Printf("  %s: %s\n", styled("远程版本", styleBold), styled(release.TagName, colorCyan))

	if IsNewer(local.Version, release.TagName) {
		fmt.Println(styled("\n  🚀 发现新版本!\n", colorGreen))
		fmt.Print(FormatReleaseNotes(release))
		fmt.Println()
		fmt.Println("  运行 'stora-cli upgrade apply' 下载并应用更新")
	} else if release.TagName == local.Version || release.TagName == "V"+local.Version {
		fmt.Println(styled("  ✅ 已是最新版本", colorGreen))
	} else {
		fmt.Println("  ℹ️  本地版本与远程不一致（可能为开发版本）")
		fmt.Println()
		fmt.Print(FormatReleaseNotes(release))
	}
	return nil
}

// ── apply: 下载并应用更新 ─────────────────────────

func cmdUpgradeApply(cfg *appConfig, args []string) error {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Println(`stora-cli upgrade apply — 应用更新

从 GitHub Releases 下载最新版本并执行数据库迁移。

用法:
  stora-cli upgrade apply

步骤:
  1. 检查 GitHub 最新 Release
  2. 下载对应平台二进制
  3. 执行数据库迁移 (migrate up)
  4. 更新 version.txt`)
			return nil
		}
	}

	local, err := CurrentVersion()
	if err != nil {
		return fmt.Errorf("读取本地版本失败: %w", err)
	}
	fmt.Printf("  %s: %s\n", styled("本地版本", styleBold), local.FormatVersion())

	fmt.Print("  检查 GitHub Release... ")
	release, err := CheckLatestRelease(githubOwner, githubRepo)
	if err != nil {
		return fmt.Errorf("检查更新失败: %w", err)
	}
	fmt.Println(styled("✅", colorGreen))

	if !IsNewer(local.Version, release.TagName) {
		if release.TagName == local.Version || release.TagName == "V"+local.Version {
			fmt.Println(styled("  ✅ 已是最新版本", colorGreen))
			return nil
		}
		fmt.Println("  ℹ️  远程版本不更新，跳过")
		return nil
	}

	fmt.Printf("  %s: %s\n", styled("新版本", styleBold+colorGreen), release.TagName)
	fmt.Println()

	// 执行数据库迁移
	fmt.Println(styled("  ── 数据库迁移 ──", styleBold+colorBlue))
	if err := cmdMigrate(cfg, []string{"up"}); err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
	}

	// 更新 version.txt
	fmt.Println()
	fmt.Println(styled("  ── 更新版本文件 ──", styleBold+colorBlue))
	if err := updateVersionTxt(release.TagName); err != nil {
		return fmt.Errorf("更新 version.txt 失败: %w", err)
	}
	fmt.Println(styled("  ✅ version.txt 已更新", colorGreen))

	fmt.Println()
	fmt.Println(styled("  ✅ 升级完成!", colorGreen))
	fmt.Printf("  %s → %s\n", local.FormatVersion(), release.TagName)
	return nil
}

func updateVersionTxt(tagName string) error {
	tag := strings.TrimPrefix(tagName, "V")
	tag = strings.TrimPrefix(tag, "v")

	data, err := os.ReadFile("version.txt")
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	var newLines []string
	inStora := false
	for _, line := range lines {
		if strings.HasPrefix(line, "[stora]") {
			inStora = true
			newLines = append(newLines, line)
			continue
		}
		if inStora && len(line) > 0 && line[0] == '[' {
			inStora = false
			newLines = append(newLines, line)
			continue
		}
		if inStora && strings.HasPrefix(line, "version") {
			newLines = append(newLines, fmt.Sprintf("version = V%s", tag))
			continue
		}
		if inStora && strings.HasPrefix(line, "release_date") {
			newLines = append(newLines, fmt.Sprintf("release_date = %s", time.Now().Format("2006-01-02")))
			continue
		}
		newLines = append(newLines, line)
	}
	_ = tag
	return os.WriteFile("version.txt", []byte(strings.Join(newLines, "\n")), 0644)
}

// ── 向后兼容（无子命令时执行 migrate up）───────────

func cmdUpgradeLegacy(cfg *appConfig, args []string) error {
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
