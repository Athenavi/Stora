package main

import (
	"database/sql"

	"github.com/Athenavi/Stora/pkg/config"
	"github.com/Athenavi/Stora/pkg/database"
)

// appConfig wraps the common config for CLI commands.
type appConfig struct {
	*config.Config
}

func loadConfig() *appConfig {
	cfg := config.Load()
	return &appConfig{Config: cfg}
}

// connectDB opens a DB connection for CLI use with a small pool.
func (a *appConfig) connectDB() (*sql.DB, error) {
	return database.Connect(a.PostgresDSN(), 2, 1)
}
