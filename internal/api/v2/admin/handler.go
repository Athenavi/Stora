package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/Athenavi/Stora/pkg/utils"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	db *sql.DB
}

func NewHandler(db *sql.DB) *Handler {
	return &Handler{db: db}
}

// ---------- User Management ----------

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(
		`SELECT id, username, email, is_active, is_superuser, is_staff, date_joined, last_login_at,
		        total_storage, used_storage FROM users ORDER BY id`,
	)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type User struct {
		ID           int64      `json:"id"`
		Username     *string    `json:"username"`
		Email        *string    `json:"email"`
		IsActive     bool       `json:"is_active"`
		IsSuperuser  bool       `json:"is_superuser"`
		IsStaff      bool       `json:"is_staff"`
		DateJoined   *string    `json:"date_joined"`
		LastLoginAt  *string    `json:"last_login_at"`
		TotalStorage int64      `json:"total_storage"`
		UsedStorage  int64      `json:"used_storage"`
	}
	var users []User
	for rows.Next() {
		var u User
		rows.Scan(&u.ID, &u.Username, &u.Email, &u.IsActive, &u.IsSuperuser, &u.IsStaff,
			&u.DateJoined, &u.LastLoginAt, &u.TotalStorage, &u.UsedStorage)
		users = append(users, u)
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	userID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	var req struct {
		IsActive    *bool  `json:"is_active"`
		IsSuperuser *bool  `json:"is_superuser"`
		Quota       *int64 `json:"total_storage"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	if req.IsActive != nil {
		h.db.Exec(`UPDATE users SET is_active = $1 WHERE id = $2`, *req.IsActive, userID)
	}
	if req.IsSuperuser != nil {
		h.db.Exec(`UPDATE users SET is_superuser = $1 WHERE id = $2`, *req.IsSuperuser, userID)
	}
	if req.Quota != nil {
		h.db.Exec(`UPDATE users SET total_storage = $1 WHERE id = $2`, *req.Quota, userID)
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

// ---------- Roles & Permissions ----------

func (h *Handler) ListRoles(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`SELECT id, name, slug, description, is_system FROM roles ORDER BY name`)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Role struct {
		ID          int64   `json:"id"`
		Name        string  `json:"name"`
		Slug        string  `json:"slug"`
		Description *string `json:"description"`
		IsSystem    bool    `json:"is_system"`
	}
	var roles []Role
	for rows.Next() {
		var r Role
		rows.Scan(&r.ID, &r.Name, &r.Slug, &r.Description, &r.IsSystem)
		roles = append(roles, r)
	}
	writeJSON(w, http.StatusOK, roles)
}

func (h *Handler) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"name required"}`, http.StatusBadRequest)
		return
	}

	now := time.Now().Format(time.RFC3339)
	var roleID int64
	h.db.QueryRow(
		`INSERT INTO roles (name, slug, created_at, updated_at) VALUES ($1,$2,$3,$3) RETURNING id`,
		req.Name, req.Slug, now,
	).Scan(&roleID)
	writeJSON(w, http.StatusCreated, map[string]int64{"id": roleID})
}

// ---------- System Settings ----------

func (h *Handler) ListSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(
		`SELECT setting_key, setting_value, description, is_public FROM system_settings ORDER BY setting_key`,
	)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var key, val, desc string
		var isPublic bool
		rows.Scan(&key, &val, &desc, &isPublic)
		settings[key] = val
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *Handler) UpdateSetting(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		http.Error(w, `{"error":"key and value required"}`, http.StatusBadRequest)
		return
	}

	now := time.Now().Format(time.RFC3339)
	_, err := h.db.Exec(
		`INSERT INTO system_settings (setting_key, setting_value, updated_at)
		 VALUES ($1, $2, $3) ON CONFLICT (setting_key) DO UPDATE SET setting_value = $2, updated_at = $3`,
		req.Key, req.Value, now,
	)
	if err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "updated"})
}

// ---------- Audit Logs ----------

func (h *Handler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}
	offset := (page - 1) * perPage

	var total int
	h.db.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&total)

	rows, err := h.db.Query(
		`SELECT id, user_id, action, resource, detail, ip_address, created_at
		 FROM audit_logs ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		perPage, offset,
	)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Log struct {
		ID        int64   `json:"id"`
		UserID    int64   `json:"user_id"`
		Action    string  `json:"action"`
		Resource  *string `json:"resource"`
		Detail    *string `json:"detail"`
		IPAddress *string `json:"ip_address"`
		CreatedAt *string `json:"created_at"`
	}
	var logs []Log
	for rows.Next() {
		var l Log
		rows.Scan(&l.ID, &l.UserID, &l.Action, &l.Resource, &l.Detail, &l.IPAddress, &l.CreatedAt)
		logs = append(logs, l)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": logs,
		"total": total,
		"page":  page,
	})
}

// ---------- Notifications ----------

func (h *Handler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	rows, err := h.db.Query(
		`SELECT id, type, title, body, is_read, created_at
		 FROM notifications WHERE user_id = $1 ORDER BY created_at DESC LIMIT 50`,
		userID,
	)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Notification struct {
		ID        int64   `json:"id"`
		Type      string  `json:"type"`
		Title     string  `json:"title"`
		Body      string  `json:"body"`
		IsRead    bool    `json:"is_read"`
		CreatedAt *string `json:"created_at"`
	}
	var notifications []Notification
	for rows.Next() {
		var n Notification
		rows.Scan(&n.ID, &n.Type, &n.Title, &n.Body, &n.IsRead, &n.CreatedAt)
		notifications = append(notifications, n)
	}
	writeJSON(w, http.StatusOK, notifications)
}

func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	notifID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	h.db.Exec(`UPDATE notifications SET is_read = true WHERE id = $1 AND user_id = $2`, notifID, userID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "marked read"})
}

// ---------- 2FA ----------

func (h *Handler) Setup2FA(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	h.db.Exec(`UPDATE users SET is_2fa_enabled = true WHERE id = $1`, userID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "2FA enabled"})
}

func (h *Handler) Disable2FA(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	h.db.Exec(`UPDATE users SET is_2fa_enabled = false, totp_secret = NULL WHERE id = $1`, userID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "2FA disabled"})
}

// ---------- Sensitive Words ----------

func (h *Handler) ListSensitiveWords(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`SELECT id, word, replacement, level, is_active FROM sensitive_words ORDER BY word`)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Word struct {
		ID          int64   `json:"id"`
		Word        string  `json:"word"`
		Replacement *string `json:"replacement"`
		Level       int     `json:"level"`
		IsActive    bool    `json:"is_active"`
	}
	var words []Word
	for rows.Next() {
		var w Word
		rows.Scan(&w.ID, &w.Word, &w.Replacement, &w.Level, &w.IsActive)
		words = append(words, w)
	}
	writeJSON(w, http.StatusOK, words)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	utils.WriteJSON(w, status, data)
}
