// stora-sync —— Stora 同步客户端（M1：双向同步 + 冲突暂停询问）。
//
// 用法：
//
//	stora-sync login -server http://127.0.0.1:9421 -user alice -root ~/Stora
//	stora-sync once      # 跑一轮就退出（适合放进系统计划任务）
//	stora-sync watch     # 常驻，按间隔轮询
//
// 冲突策略：绝不自动合并或覆盖。双方都改过的文件会被停下来问用户，用户不回答
// 就保持两边不动并保留时间游标，下一轮继续问。
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "login":
		cmdLogin(os.Args[2:])
	case "once":
		cmdOnce(os.Args[2:])
	case "watch":
		cmdWatch(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "未知命令：%s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `stora-sync —— Stora 同步客户端

  login   登录并写入配置（server / user / root）
  once    执行一轮双向同步后退出
  watch   常驻并按间隔轮询同步

登录示例：
  stora-sync login -server http://127.0.0.1:9421 -user alice -root ~/Stora
`)
}

func cmdLogin(args []string) {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	server := fs.String("server", "http://127.0.0.1:9421", "服务端地址")
	user := fs.String("user", "", "用户名")
	root := fs.String("root", "", "本地同步根目录")
	remoteRoot := fs.String("remote-root", "", "远端对应目录（留空表示账号根目录）")
	cfgPath := fs.String("config", DefaultConfigPath(), "配置文件路径")
	_ = fs.Parse(args)

	if *user == "" || *root == "" {
		fmt.Fprintln(os.Stderr, "login 需要 -user 与 -root")
		os.Exit(2)
	}

	password, err := readPassword()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取密码失败: %v\n", err)
		os.Exit(1)
	}

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析本地目录失败: %v\n", err)
		os.Exit(1)
	}
	if st, err := os.Stat(absRoot); err != nil || !st.IsDir() {
		fmt.Fprintf(os.Stderr, "本地目录不存在或不是目录：%s\n", absRoot)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	token, err := NewClient(*server, "").Login(ctx, *user, password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "登录失败: %v\n", err)
		os.Exit(1)
	}

	cfg := &Config{
		Server:     *server,
		Username:   *user,
		Token:      token,
		Root:       absRoot,
		RemoteRoot: *remoteRoot,
	}
	if err := cfg.Save(*cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "保存配置失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("已登录，配置写入 %s\n本地目录：%s\n", *cfgPath, absRoot)
}

// readPassword 优先取 STORA_PASSWORD，其次在终端提示输入。
//
// ponytail: 没做终端回显抑制——那需要 golang.org/x/term（新依赖，且 Windows 上
// 还要额外处理）。脚本里请用 STORA_PASSWORD 环境变量，交互输入时注意四周有人。
func readPassword() (string, error) {
	if p := os.Getenv("STORA_PASSWORD"); p != "" {
		return p, nil
	}
	fmt.Print("密码（或设置 STORA_PASSWORD 环境变量）: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func cmdOnce(args []string) {
	fs := flag.NewFlagSet("once", flag.ExitOnError)
	cfgPath := fs.String("config", DefaultConfigPath(), "配置文件路径")
	_ = fs.Parse(args)

	cfg, st, c := load(*cfgPath)
	if err := RunOnce(context.Background(), cfg, st, c, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "同步失败: %v\n", err)
		os.Exit(1)
	}
}

func cmdWatch(args []string) {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	cfgPath := fs.String("config", DefaultConfigPath(), "配置文件路径")
	interval := fs.Duration("interval", 3*time.Second, "轮询间隔")
	_ = fs.Parse(args)

	cfg, st, c := load(*cfgPath)
	if *interval <= 0 {
		if d, err := time.ParseDuration(cfg.Interval); err == nil {
			*interval = d
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("开始监听：%s ↔ %s（每 %s 检查一次，Ctrl+C 退出）\n", cfg.Root, cfg.Server, *interval)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// ponytail: 本端改动靠定时全量扫描发现，所以「保存即同步」最迟滞后
			// 一个轮询周期。要更低延迟就加 fsnotify，把扫描换成事件驱动即可，
			// RunOnce 的其余部分不用动。
			if err := RunOnce(ctx, cfg, st, c, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "同步出错: %v\n", err)
			}
		case <-ctx.Done():
			fmt.Println("\n已停止。")
			return
		}
	}
}

func load(cfgPath string) (*Config, *State, *Client) {
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败（先执行 login）: %v\n", err)
		os.Exit(1)
	}
	st, err := LoadState(DefaultStatePath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取同步状态失败: %v\n", err)
		os.Exit(1)
	}
	return cfg, st, NewClient(cfg.Server, cfg.Token)
}
