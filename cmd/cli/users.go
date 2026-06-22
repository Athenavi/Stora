package main

import (
	"database/sql"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/Athenavi/Stora/pkg/database"
)

func cmdUsers(cfg *appConfig, args []string) error {
	db, err := database.Connect(cfg.PostgresDSN(), 1, 1)
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}
	defer database.Close()

	rows, err := db.Query(`SELECT id, username, email, is_superuser, is_active FROM users ORDER BY id`)
	if err != nil {
		return fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\t用户名\t邮箱\t管理员\t状态")
	fmt.Fprintln(w, "--\t------\t----\t----\t----")

	for rows.Next() {
		var id int64
		var username, email sql.NullString
		var isSuperuser, isActive bool
		if err := rows.Scan(&id, &username, &email, &isSuperuser, &isActive); err != nil {
			return fmt.Errorf("行读取失败: %w", err)
		}
		admin := ""
		if isSuperuser {
			admin = "✓"
		}
		active := "✓"
		if !isActive {
			active = "✗"
		}
		uName := nullStr(username)
		uEmail := nullStr(email)
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", id, uName, uEmail, admin, active)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("行迭代错误: %w", err)
	}
	w.Flush()
	return nil
}

func nullStr(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return "-"
}
