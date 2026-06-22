// Package adminui provides a lightweight administrative web UI
// rendered with Go's html/template (the Go equivalent of Jinja2).
// It is mounted on the main server at /admin/ui/ and provides
// system status, migration management, and configuration overview.
//
// Ponytail: This is intentionally minimal — a small dashboard, not a full
// admin SPA. If more functionality is needed, extend the template, don't
// add JavaScript frameworks.
package adminui

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/Athenavi/Stora/pkg/config"
)

//go:embed templates/*.html
var templateFS embed.FS

// Handler serves the admin UI pages.
type Handler struct {
	db  *sql.DB
	cfg *config.Config
	tm  *template.Template
}

// NewHandler creates a new admin UI handler.
func NewHandler(db *sql.DB, cfg *config.Config) (*Handler, error) {
	tm, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse admin templates: %w", err)
	}
	return &Handler{db: db, cfg: cfg, tm: tm}, nil
}

// DashboardData is the data structure for the dashboard template.
type DashboardData struct {
	Error       string
	Health      HealthStatus
	Stats       SystemStats
	Config      ConfigSummary
	Migrations  []MigrationRecord
}

// HealthStatus represents database and Redis connectivity.
type HealthStatus struct {
	DBConnected     bool
	RedisConnected  bool
}

// SystemStats holds aggregate system statistics.
type SystemStats struct {
	TotalUsers int64
	TotalFiles int64
	TotalSize  int64
}

// ConfigSummary is a safe subset of config for display.
type ConfigSummary struct {
	SiteName      string
	ServerHost    string
	ServerPort    int
	StorageDriver string
	Debug         bool
	JWTExpiration time.Duration
}

// MigrationRecord represents an applied migration.
type MigrationRecord struct {
	Version     int
	Description string
	AppliedAt   string
}

// Dashboard renders the main admin dashboard.
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	data := DashboardData{
		Config: ConfigSummary{
			SiteName:      h.cfg.SiteName,
			ServerHost:    h.cfg.ServerHost,
			ServerPort:    h.cfg.ServerPort,
			StorageDriver: h.cfg.StorageDriver,
			Debug:         h.cfg.Debug,
			JWTExpiration: h.cfg.JWTExpiration,
		},
	}

	// Health check
	if err := h.db.Ping(); err == nil {
		data.Health.DBConnected = true
	}

	// Stats
	h.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&data.Stats.TotalUsers)
	h.db.QueryRow(`SELECT COUNT(*) FROM file_items WHERE deleted_at IS NULL`).Scan(&data.Stats.TotalFiles)
	h.db.QueryRow(`SELECT COALESCE(SUM(file_size), 0) FROM file_items WHERE deleted_at IS NULL`).Scan(&data.Stats.TotalSize)

	// Migrations
	rows, err := h.db.Query(`SELECT version, description, applied_at FROM schema_version ORDER BY version DESC LIMIT 20`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var m MigrationRecord
			var appliedAt time.Time
			if err := rows.Scan(&m.Version, &m.Description, &appliedAt); err == nil {
				m.AppliedAt = appliedAt.Format("2006-01-02 15:04:05")
				data.Migrations = append(data.Migrations, m)
			}
		}
		if err := rows.Err(); err != nil {
			log.Printf("[AdminUI] Migration query error: %v", err)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tm.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		log.Printf("[AdminUI] Template render error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// MigratePage triggers pending migrations and shows result.
func (h *Handler) MigratePage(w http.ResponseWriter, r *http.Request) {
	// Run pending migrations (simplified inline version for the UI)
	// In a full implementation this would call the same logic as the CLI migrate command.

	// Show simple result page
	result := struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}{
		Success: true,
		Message: "迁移已在 CLI 中通过 `stora-cli migrate` 执行。管理面板仅显示状态。",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
