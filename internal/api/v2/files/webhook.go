package files

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/go-chi/chi/v5"
)

type WebhookHandler struct {
	db *sql.DB
}

func NewWebhookHandler(db *sql.DB) *WebhookHandler {
	return &WebhookHandler{db: db}
}

func (h *WebhookHandler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	rows, err := h.db.Query(
		`SELECT id, name, url, events, is_active, created_at FROM webhooks WHERE user_id = $1 ORDER BY name`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	type Wh struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		URL      string `json:"url"`
		Events   string `json:"events"`
		IsActive bool   `json:"is_active"`
		CreateAt string `json:"created_at"`
	}
	var items = make([]Wh, 0)
	for rows.Next() {
		var w Wh
		if err := rows.Scan(&w.ID, &w.Name, &w.URL, &w.Events, &w.IsActive, &w.CreateAt); err == nil {
			items = append(items, w)
		}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *WebhookHandler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		Name   string `json:"name"`
		URL    string `json:"url"`
		Events string `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		writeError(w, http.StatusBadRequest, "url required")
		return
	}
	now := time.Now().Format(time.RFC3339)
	var id int64
	h.db.QueryRow(
		`INSERT INTO webhooks (user_id, name, url, events, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $5) RETURNING id`,
		userID, req.Name, req.URL, req.Events, now,
	).Scan(&id)
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (h *WebhookHandler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	h.db.Exec(`DELETE FROM webhooks WHERE id = $1 AND user_id = $2`, id, userID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// ─── Event Trigger ───

// TriggerWebhooks dispatches an event to all matching active webhooks.
// Called by file handlers after create/delete/share operations.
func TriggerWebhooks(db *sql.DB, event string, payload interface{}) {
	go func() {
		data, _ := json.Marshal(payload)
		rows, err := db.Query(
			`SELECT id, url FROM webhooks WHERE is_active = true
			 AND (events = '*' OR events LIKE $1)`,
			"%"+event+"%",
		)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var url string
			rows.Scan(&id, &url)
			go fireWebhook(db, id, url, event, string(data))
		}
	}()
}

func fireWebhook(db *sql.DB, id int64, url, event, payload string) {
	body := bytes.NewBuffer([]byte(payload))
	resp, err := http.Post(url, "application/json", body)
	status := "failed"
	respBody := ""
	if err == nil {
		status = "success"
		resp.Body.Close()
	}
	now := time.Now().Format(time.RFC3339)
	db.Exec(
		`INSERT INTO webhook_events (webhook_id, event, payload, status, response, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		id, event, payload, status, respBody, now,
	)
}
