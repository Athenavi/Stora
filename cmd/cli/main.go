package main

import (
	"fmt"
	"log"
	"os"

	"github.com/Athenavi/Stora/pkg/config"
	"github.com/Athenavi/Stora/pkg/database"
)

func main() {
	log.SetFlags(0)

	cfg := config.Load()

	if len(os.Args) < 2 {
		fmt.Println(`Stora CLI
Usage: stora-cli <command> [args]
Commands:
  health         - Check database connectivity
  migrate        - Run database migrations
  backup         - Backup database
  users          - List or manage users
  cache          - Manage Redis cache
  shell          - Open interactive shell
  upgrade        - Upgrade system`)
		return
	}

	cmd := os.Args[1]

	switch cmd {
	case "health":
		cmdHealth(cfg)
	case "users":
		cmdUsers(cfg)
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		os.Exit(1)
	}
}

func cmdHealth(cfg *config.Config) {
	db, err := database.Connect(cfg.PostgresDSN(), 1, 1)
	if err != nil {
		fmt.Printf("❌ Database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := db.Ping(); err != nil {
		fmt.Printf("❌ Database ping failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Database: connected")
	fmt.Println("✅ System: healthy")
}

func cmdUsers(cfg *config.Config) {
	db, err := database.Connect(cfg.PostgresDSN(), 1, 1)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer database.Close()

	rows, err := db.Query(`SELECT id, username, email, is_superuser, is_active FROM users ORDER BY id`)
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	fmt.Printf("%-5s %-20s %-30s %-5s %-5s\n", "ID", "Username", "Email", "Admin", "Active")
	fmt.Println("----- -------------------- ----------------------------- ----- -----")
	for rows.Next() {
		var id int64
		var username, email string
		var isSuperuser, isActive bool
		rows.Scan(&id, &username, &email, &isSuperuser, &isActive)
		admin := "✓"
		if !isSuperuser {
			admin = ""
		}
		active := "✓"
		if !isActive {
			active = "✗"
		}
		fmt.Printf("%-5d %-20s %-30s %-5s %-5s\n", id, username, email, admin, active)
	}
}
