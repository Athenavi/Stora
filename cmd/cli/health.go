package main

import (
	"fmt"
	"os"
)

func cmdHealth(cfg *appConfig, args []string) error {
	db, err := cfg.connectDB()
	if err != nil {
		fmt.Printf("❌ 数据库连接失败: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fmt.Printf("❌ 数据库 Ping 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ 数据库: 已连接")
	fmt.Println("✅ 系统: 运行正常")
	return nil
}
