package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

func cmdBackup(cfg *appConfig, args []string) error {
	// Parse flags
	outputPath := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			fmt.Println(`stora-cli backup — 数据库备份

用法:
  stora-cli backup [output_file]

不指定输出文件时，自动生成 stora-backup-<日期>.sql

依赖: pg_dump（PostgreSQL 客户端工具）`)
			return nil
		case "-o":
			if i+1 < len(args) {
				outputPath = args[i+1]
				i++
			}
		default:
			if outputPath == "" && args[i][0] != '-' {
				outputPath = args[i]
			}
		}
	}

	if outputPath == "" {
		outputPath = fmt.Sprintf("stora-backup-%s.sql", time.Now().Format("20060102-150405"))
	}

	// Verify pg_dump exists
	if _, err := exec.LookPath("pg_dump"); err != nil {
		return fmt.Errorf("未找到 pg_dump，请确保已安装 PostgreSQL 客户端: %w", err)
	}

	// Build pg_dump command
	cmd := exec.Command("pg_dump",
		"-h", cfg.DBHost,
		"-p", fmt.Sprintf("%d", cfg.DBPort),
		"-U", cfg.DBUser,
		"-d", cfg.DBName,
		"-F", "p", // plain SQL format
		"--no-owner",
		"--no-acl",
		"-f", outputPath,
	)

	// Set password via env
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", cfg.DBPassword))

	fmt.Printf("📦 正在备份数据库 %s@%s:%d/%s → %s\n",
		cfg.DBUser, cfg.DBHost, cfg.DBPort, cfg.DBName, outputPath)

	start := time.Now()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("备份失败: %w\n%s", err, string(output))
	}

	elapsed := time.Since(start)
	fmt.Printf("✅ 备份完成（耗时 %v）\n", elapsed)

	// Show file size
	if fi, err := os.Stat(outputPath); err == nil {
		fmt.Printf("   文件大小: %d MB\n", fi.Size()/(1024*1024))
	}

	return nil
}
