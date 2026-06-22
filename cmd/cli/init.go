package main

import (
	"bufio"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Athenavi/Stora/internal/api/v2/files"
	"github.com/Athenavi/Stora/pkg/auth"
)

// 颜色 / 样式常量（终端输出美化）
const (
	styleReset  = "\033[0m"
	styleBold   = "\033[1m"
	styleDim    = "\033[2m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorRed    = "\033[31m"
)

func styled(s, style string) string {
	// Windows 终端可能不支持 ANSI
	if runtime.GOOS == "windows" {
		return s
	}
	return style + s + styleReset
}

func cmdInit(cfg *appConfig, args []string) error {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			printInitHelp()
			return nil
		}
	}

	fmt.Println(styled(`
   ╔══════════════════════════════════════════╗
   ║        Stora 系统初始化向导              ║
   ║    自部署企业网盘 — 快速起步             ║
   ╚══════════════════════════════════════════╝`, colorCyan))
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)

	// ── Step 1: 环境检查 ──
	printStep(1, "环境检查")
	checkPrerequisites(scanner)

	// ── Step 2: 配置生成 ──
	printStep(2, "配置生成")
	envVars := gatherConfig(scanner)
	if err := writeEnvFile(envVars); err != nil {
		return fmt.Errorf("写入 .env 失败: %w", err)
	}
	fmt.Println(styled("  ✅ .env 配置文件已生成", colorGreen))

	// ── Step 3: 数据库初始化 ──
	printStep(3, "数据库初始化")
	if err := initDatabase(envVars); err != nil {
		return fmt.Errorf("数据库初始化失败: %w", err)
	}

	// ── Step 4: 创建管理员 ──
	printStep(4, "创建管理员账户")
	adminUser, adminPass, err := createAdminUser(envVars, scanner)
	if err != nil {
		return fmt.Errorf("创建管理员失败: %w", err)
	}

	// ── Step 5: 完成 ──
	printStep(5, "初始化完成")
	printSummary(envVars, adminUser, adminPass)

	return nil
}

func printInitHelp() {
	fmt.Println(`stora-cli init — 系统初始化向导

交互式完成以下步骤:
  1. 环境检查（Go/PostgreSQL/Redis）
  2. 配置生成（.env 文件）
  3. 数据库建表
  4. 创建管理员账户
  5. 启动指引

用法:
  stora-cli init`)
}

func printStep(step int, title string) {
	fmt.Println()
	fmt.Println(styled(fmt.Sprintf("  ── Step %d: %s ──", step, title), styleBold+colorBlue))
}

func checkPrerequisites(scanner *bufio.Scanner) {
	checks := []struct {
		name   string
		check  func() bool
		hint   string
	}{
		{"Go 运行环境", func() bool {
			_, err := exec.LookPath("go")
			return err == nil
		}, "请安装 Go 1.26+: https://go.dev/dl/"},
		{"PostgreSQL", func() bool {
			_, err := exec.LookPath("psql")
			return err == nil
		}, "请安装 PostgreSQL 15+: https://postgresql.org/download/"},
	}

	allOk := true
	for _, c := range checks {
		if c.check() {
			fmt.Printf(styled("  ✅ %s\n", colorGreen), c.name)
		} else {
			fmt.Printf(styled("  ⚠️  %s — %s\n", colorYellow), c.name, c.hint)
			allOk = false
		}
	}
	if !allOk {
		fmt.Print(styled("  按 Enter 继续（跳过缺失项）...", styleDim))
		scanner.Scan()
	}
}

func gatherConfig(scanner *bufio.Scanner) map[string]string {
	vars := make(map[string]string)

	prompt := func(key, label, defaultVal string) string {
		fmt.Printf("  %s", label)
		if defaultVal != "" {
			fmt.Printf(" [%s]", defaultVal)
		}
		fmt.Print(": ")
		scanner.Scan()
		val := strings.TrimSpace(scanner.Text())
		if val == "" {
			val = defaultVal
		}
		vars[key] = val
		return val
	}

	fmt.Println(styled("  以下配置将写入 .env 文件，按 Enter 使用默认值", styleDim))

	// 生成随机密钥
	secretKey := make([]byte, 32)
	rand.Read(secretKey)
	defaultSecret := hex.EncodeToString(secretKey)

	prompt("SECRET_KEY", "  密钥 (SECRET_KEY)", defaultSecret)
	prompt("DB_HOST", "  数据库主机", "localhost")
	prompt("DB_PORT", "  数据库端口", "5432")
	prompt("DB_USER", "  数据库用户", "postgres")
	prompt("DB_PASSWORD", "  数据库密码", "")
	prompt("DB_NAME", "  数据库名", "stora")
	prompt("REDIS_HOST", "  Redis 主机", "localhost")
	prompt("REDIS_PORT", "  Redis 端口", "6379")
	prompt("PORT", "  服务端口", "9421")
	prompt("TITLE", "  站点名称", "Stora")

	return vars
}

func writeEnvFile(vars map[string]string) error {
	content := `# Stora Configuration — 由 stora-cli init 生成
# 生成时间: ` + time.Now().Format("2006-01-02 15:04:05") + `

# ---- Server ----
PORT=` + vars["PORT"] + `
HOST=0.0.0.0
SECRET_KEY=` + vars["SECRET_KEY"] + `
DEBUG=false
TIME_ZONE=Asia/Shanghai
TITLE=` + vars["TITLE"] + `
DOMAIN=http://localhost:` + vars["PORT"] + `

# ---- Database ----
DB_HOST=` + vars["DB_HOST"] + `
DB_PORT=` + vars["DB_PORT"] + `
DB_USER=` + vars["DB_USER"] + `
DB_PASSWORD=` + vars["DB_PASSWORD"] + `
DB_NAME=` + vars["DB_NAME"] + `

# ---- Redis ----
REDIS_HOST=` + vars["REDIS_HOST"] + `
REDIS_PORT=` + vars["REDIS_PORT"] + `
REDIS_PASSWORD=
REDIS_DB=0

# ---- JWT ----
JWT_EXPIRATION_DELTA=7200
REFRESH_TOKEN_EXPIRATION_DELTA=64800

# ---- Storage ----
STORAGE_DRIVER=local
STORAGE_OBJECTS_DIR=storage/objects
TEMP_FOLDER=temp/upload

# ---- Vault ----
VAULT_ENCRYPTION_KEY=` + vars["SECRET_KEY"] + `
`
	return os.WriteFile(".env", []byte(content), 0644)
}

func initDatabase(vars map[string]string) error {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		vars["DB_USER"], vars["DB_PASSWORD"], vars["DB_HOST"], vars["DB_PORT"], vars["DB_NAME"])

	fmt.Print("  连接数据库... ")
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("打开数据库连接失败: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("数据库连接失败 (请确保 PostgreSQL 已启动且数据库 '%s' 已创建): %w", vars["DB_NAME"], err)
	}
	fmt.Println(styled("✅", colorGreen))

	// Read models.yaml and generate + apply migration
	fmt.Print("  生成迁移... ")
	schema, err := ParseSchema("config/models.yaml")
	if err != nil {
		return fmt.Errorf("读取 models.yaml 失败: %w", err)
	}
	upSQL, _ := schema.GenerateFullMigration()

	// Apply UPGRADE SQL
	if _, err := db.Exec(upSQL); err != nil {
		return fmt.Errorf("数据库建表失败: %w", err)
	}
	fmt.Println(styled("✅", colorGreen))

	// Record migration in schema_version
	rev := "init_" + time.Now().Format("20060102_150405")
	if _, err := db.Exec(
		`INSERT INTO schema_version (revision, description, applied_at) VALUES ($1, $2, $3)
		 ON CONFLICT (revision) DO NOTHING`,
		rev, "initial schema from models.yaml", time.Now().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("记录版本失败: %w", err)
	}
	writeRevisionINI("version.ini", rev)
	fmt.Printf("  已创建 %d 个表\n", len(schema.Models))
	return nil
}

func createAdminUser(vars map[string]string, scanner *bufio.Scanner) (username, password string, err error) {
	fmt.Println(styled("  创建超级管理员账户", styleDim))

	fmt.Print("  管理员用户名 [admin]: ")
	scanner.Scan()
	username = strings.TrimSpace(scanner.Text())
	if username == "" {
		username = "admin"
	}

	// Generate random password
	pwBytes := make([]byte, 12)
	rand.Read(pwBytes)
	defaultPass := hex.EncodeToString(pwBytes)[:16]

	fmt.Printf("  管理员密码 [随机生成]: ")
	scanner.Scan()
	password = strings.TrimSpace(scanner.Text())
	if password == "" {
		password = defaultPass
	}

	// Connect and create user
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		vars["DB_USER"], vars["DB_PASSWORD"], vars["DB_HOST"], vars["DB_PORT"], vars["DB_NAME"])

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return "", "", fmt.Errorf("连接数据库失败: %w", err)
	}
	defer db.Close()

	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		return "", "", fmt.Errorf("密码哈希失败: %w", err)
	}

	now := time.Now().Format(time.RFC3339)
	_, err = db.Exec(
		`INSERT INTO users (username, password, is_active, is_superuser, is_staff, date_joined, locale, total_storage, used_storage)
		 VALUES ($1, $2, true, true, true, $3, 'zh_CN', 1073741824, 0)
		 ON CONFLICT (username) DO UPDATE SET is_superuser = true, is_active = true`,
		username, hashedPassword, now,
	)
	if err != nil {
		return "", "", fmt.Errorf("创建用户失败: %w", err)
	}

	// Initialize vault encryption
	files.SetEncryptionKey(vars["SECRET_KEY"])

	return username, password, nil
}

func printSummary(vars map[string]string, adminUser, adminPass string) {
	fmt.Println()
	fmt.Println(styled("  ═══════════════════════════════════════", colorGreen))
	fmt.Println(styled("   ✅  初始化完成！", styleBold+colorGreen))
	fmt.Println(styled("  ═══════════════════════════════════════", colorGreen))
	fmt.Println()
	fmt.Println(styled("  启动服务:", styleBold))
	fmt.Println("    go run ./cmd/server")
	fmt.Println()
	fmt.Println(styled("  管理地址:", styleBold))
	fmt.Printf("    http://localhost:%s/admin/ui/\n", vars["PORT"])
	fmt.Println()
	fmt.Println(styled("  管理员账户:", styleBold))
	fmt.Printf("    用户名: %s\n", adminUser)
	fmt.Printf("    密码:   %s\n", adminPass)
	fmt.Println()
	fmt.Println(styled("  ⚠️  请妥善保管管理员密码！", colorYellow))
}
