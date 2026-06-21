package mobile

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/Athenavi/Stora/pkg/auth"
)

type AuthHandler struct {
	db         *sql.DB
	jwtManager *auth.JWTManager
}

func NewAuthHandler(db *sql.DB, jwtManager *auth.JWTManager) *AuthHandler {
	return &AuthHandler{db: db, jwtManager: jwtManager}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	var userID int64
	var username, passwordHash string
	var isSuperuser bool
	err := h.db.QueryRow(
		`SELECT id, username, password, is_superuser FROM users
		 WHERE (username = $1 OR email = $1) AND is_active = true`,
		req.Username,
	).Scan(&userID, &username, &passwordHash, &isSuperuser)

	if err != nil {
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	if !auth.CheckPassword(req.Password, passwordHash) {
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	tokens, err := h.jwtManager.GenerateTokens(userID, username, isSuperuser)
	if err != nil {
		http.Error(w, `{"error":"token generation failed"}`, http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_id":  userID,
		"username": username,
		"tokens":   tokens,
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
