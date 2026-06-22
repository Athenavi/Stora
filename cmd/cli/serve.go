package main

import (
	"fmt"
	"net/http"

	"github.com/Athenavi/Stora/internal/adminui"
)

func cmdServe(cfg *appConfig, args []string) error {
	fmt.Println(`stora-cli serve — 管理 Web UI

管理面板已集成到后端服务器中。
请在主服务器运行时访问:

  http://localhost:9421/admin/ui/

或者直接启动主服务器:

  go run ./cmd/server

命令行选项:
  -h, --help  显示此帮助`)

	// If a port is specified as argument, start a standalone admin server
	port := ""
	for i, a := range args {
		if a == "-p" && i+1 < len(args) {
			port = args[i+1]
		}
	}
	if port == "" {
		return nil
	}

	db, err := cfg.connectDB()
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}
	defer db.Close()

	adminHandler, err := adminui.NewHandler(db, cfg.Config)
	if err != nil {
		return fmt.Errorf("初始化管理 UI 失败: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", adminHandler.Dashboard)
	mux.HandleFunc("/migrate", adminHandler.MigratePage)

	addr := ":" + port
	fmt.Printf("管理 Web UI 启动于 http://localhost%s\n", addr)
	return http.ListenAndServe(addr, mux)
}
