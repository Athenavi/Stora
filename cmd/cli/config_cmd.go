package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Athenavi/Stora/pkg/config"
)

func cmdConfig(cfg *appConfig, args []string) error {
	// Parse subcommand
	sub := "show"
	if len(args) > 0 && args[0][0] != '-' {
		sub = args[0]
		args = args[1:]
	}

	switch sub {
	case "show":
		return cmdConfigShow(cfg, args)
	case "init":
		return cmdConfigInit(cfg, args)
	case "verify":
		return cmdConfigVerify(cfg, args)
	default:
		fmt.Printf("未知子命令: %s\n\n", sub)
		fmt.Println(`stora-cli config — 配置管理

子命令:
  show    显示当前配置摘要
  init    从 .env.example 生成 .env 文件
  verify  验证 .env 配置完整性

用法:
  stora-cli config show
  stora-cli config init
  stora-cli config verify`)
		return nil
	}
}

func cmdConfigShow(cfg *appConfig, args []string) error {
	fmt.Println("📋 当前配置摘要")
	fmt.Println("================")
	fmt.Printf("  服务器:     %s:%d\n", cfg.ServerHost, cfg.ServerPort)
	fmt.Printf("  数据库:     %s@%s:%d/%s\n",
		cfg.DBUser, cfg.DBHost, cfg.DBPort, cfg.DBName)
	fmt.Printf("  Redis:      %s:%d (db:%d)\n",
		cfg.RedisHost, cfg.RedisPort, cfg.RedisDB)
	fmt.Printf("  存储驱动:   %s\n", cfg.StorageDriver)
	fmt.Printf("  存储路径:   %s\n", cfg.StorageObjectsDir)
	fmt.Printf("  SecretKey:  %s\n", maskKey(cfg.SecretKey))
	fmt.Printf("  调试模式:   %v\n", cfg.Debug)
	fmt.Printf("  站点名称:   %s\n", cfg.SiteName)
	fmt.Printf("  JWT 过期:   %v\n", cfg.JWTExpiration)
	return nil
}

func cmdConfigInit(cfg *appConfig, args []string) error {
	// Check if .env already exists
	if _, err := os.Stat(".env"); err == nil {
		fmt.Print("⚠️  .env 文件已存在。覆盖？(y/N): ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		answer := strings.TrimSpace(scanner.Text())
		if answer != "y" && answer != "Y" {
			fmt.Println("已取消")
			return nil
		}
	}

	// Read .env.example
	src, err := os.ReadFile(".env.example")
	if err != nil {
		return fmt.Errorf("读取 .env.example 失败: %w（请确保文件存在）", err)
	}

	if err := os.WriteFile(".env", src, 0644); err != nil {
		return fmt.Errorf("写入 .env 失败: %w", err)
	}

	fmt.Println("✅ .env 文件已从 .env.example 生成")
	fmt.Println("   请编辑 .env 中的配置项（尤其是 SECRET_KEY）")
	return nil
}

func cmdConfigVerify(cfg *appConfig, args []string) error {
	var warnings []string
	var errors []string

	// Check critical settings
	if cfg.SecretKey == "" {
		errors = append(errors, "SECRET_KEY 未设置 — 这是必需的")
	}
	if len(cfg.SecretKey) > 0 && len(cfg.SecretKey) < 16 {
		warnings = append(warnings, "SECRET_KEY 长度不足 16 字符，建议使用更强的密钥")
	}
	if cfg.DBHost == "" {
		errors = append(errors, "DB_HOST 未设置")
	}
	if cfg.DBPassword == "" {
		warnings = append(warnings, "DB_PASSWORD 未设置 — 生产环境应设置数据库密码")
	}
	if cfg.RedisHost != "" && cfg.RedisPassword == "" {
		warnings = append(warnings, "Redis 未设置密码 — 生产环境建议设置")
	}

	// Check .env file exists
	if _, err := os.Stat(".env"); os.IsNotExist(err) {
		warnings = append(warnings, ".env 文件不存在 — 运行 'stora-cli config init' 创建")
	}

	fmt.Println("🔍 配置验证结果")
	fmt.Println("================")

	if len(errors) > 0 {
		fmt.Println("❌ 错误:")
		for _, e := range errors {
			fmt.Printf("   • %s\n", e)
		}
	}
	if len(warnings) > 0 {
		fmt.Println("⚠️  警告:")
		for _, w := range warnings {
			fmt.Printf("   • %s\n", w)
		}
	}
	if len(errors) == 0 && len(warnings) == 0 {
		fmt.Println("✅ 配置检查通过")
	} else if len(errors) == 0 {
		fmt.Println("\n⚠️  请处理上述警告后启动服务")
	} else {
		fmt.Println("\n❌ 请修复上述错误后启动服务")
	}
	return nil
}

// maskKey masks all but the last 4 chars of a secret key.
func maskKey(key string) string {
	if len(key) <= 4 {
		return strings.Repeat("*", len(key))
	}
	return strings.Repeat("*", len(key)-4) + key[len(key)-4:]
}

// Ensure config satisfies the interface used in other files.
var _ = (*config.Config)(nil)
