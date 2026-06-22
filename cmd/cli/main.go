// Stora CLI — 系统管理命令行工具
//
// 命令结构：
//   stora-cli <command> [args...]
//
// 内置命令：
//   health    检查数据库连接状态
//   users     列出所有用户
//   migrate   执行数据库迁移
//   upgrade   系统升级（迁移 + 数据迁移）
//   config    生成/验证 .env 配置文件
//   backup    数据库备份
//   serve     启动管理 Web UI（Go 模板页面）

package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"text/tabwriter"
)

func main() {
	log.SetFlags(0)

	cfg := loadConfig()

	if len(os.Args) < 2 {
		printUsage()
		return
	}

	cmd := os.Args[1]

	handler, ok := commands[cmd]
	if !ok {
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err := handler(cfg, os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %s: %v\n", cmd, err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Stora CLI — 系统管理工具

用法:
  stora-cli <command> [args...]

命令:`)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	// 按名称排序
	var names []string
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(w, "  %s\t%s\n", n, summaries[n])
	}
	w.Flush()
	fmt.Println(`
运行 'stora-cli <command> -h' 查看命令详情。`)
}

// 命令注册表
var commands = map[string]func(*appConfig, []string) error{
	"health":  cmdHealth,
	"users":   cmdUsers,
	"migrate": cmdMigrate,
	"upgrade": cmdUpgrade,
	"config":  cmdConfig,
	"backup":  cmdBackup,
	"init":    cmdInit,
	"serve":   cmdServe,
}

var summaries = map[string]string{
	"health":  "检查数据库连接状态",
	"users":   "列出所有用户",
	"migrate": "执行数据库迁移",
	"upgrade": "系统升级（迁移 + 数据迁移）",
	"config":  "生成/验证 .env 配置文件",
	"backup":  "数据库备份",
	"init":    "⚡ 系统初始化向导（交互式）",
	"serve":   "启动管理 Web UI",
}
