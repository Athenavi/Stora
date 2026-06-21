package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

// DB holds the database connection pool.
var DB *sql.DB

// Connect initializes the PostgreSQL connection pool.
func Connect(dsn string, maxOpenConns, maxIdleConns int) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	DB = db
	log.Println("[DB] PostgreSQL connection pool established")
	return db, nil
}

// Close gracefully closes the database connection pool.
func Close() {
	if DB != nil {
		DB.Close()
		log.Println("[DB] Connection pool closed")
	}
}
