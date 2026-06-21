package authapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Athenavi/Stora/pkg/auth"
	"github.com/Athenavi/Stora/pkg/models"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	db         *sql.DB
	jwtManager *auth.JWTManager
}

func NewHandler(db *sql.DB, jwtManager *auth.JWTManager) *Handler {
	return &Handler{db: db, jwtManager: jwtManager}
}

// RegisterRequest is the registration payload.
type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginRequest is the login payload.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// RefreshRequest is the token refresh payload.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, `{"error":"username and password required"}`, http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, `{"error":"failed to hash password"}`, http.StatusInternalServerError)
		return
	}

	now := time.Now().Format(time.RFC3339)
	locale := "zh_CN"

	var userID int64
	err = h.db.QueryRow(
		`INSERT INTO users (username, email, password, is_active, date_joined, locale, total_storage, used_storage)
		 VALUES ($1, $2, $3, true, $4, $5, 1073741824, 0) RETURNING id`,
		req.Username, req.Email, string(hashedPassword), now, locale,
	).Scan(&userID)

	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			http.Error(w, `{"error":"username or email already exists"}`, http.StatusConflict)
			return
		}
		http.Error(w, `{"error":"registration failed"}`, http.StatusInternalServerError)
		return
	}

	tokens, err := h.jwtManager.GenerateTokens(userID, req.Username, false)
	if err != nil {
		http.Error(w, `{"error":"token generation failed"}`, http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"user_id":  userID,
		"username": req.Username,
		"tokens":   tokens,
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	var user models.User
	err := h.db.QueryRow(
		`SELECT id, username, password, is_superuser, is_active FROM users
		 WHERE (username = $1 OR email = $1) AND is_active = true`,
		req.Username,
	).Scan(&user.ID, &user.Username, &user.Password, &user.IsSuperuser, &user.IsActive)

	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	if user.Password == nil || !auth.CheckPassword(req.Password, *user.Password) {
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	tokens, err := h.jwtManager.GenerateTokens(user.ID, *user.Username, user.IsSuperuser)
	if err != nil {
		http.Error(w, `{"error":"token generation failed"}`, http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_id":  user.ID,
		"username": user.Username,
		"tokens":   tokens,
	})
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	tokens, err := h.jwtManager.RefreshTokens(req.RefreshToken)
	if err != nil {
		http.Error(w, `{"error":"invalid refresh token"}`, http.StatusUnauthorized)
		return
	}

	writeJSON(w, http.StatusOK, tokens)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	// In a full implementation, blacklist the token
	// For now, the client should discard the token
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int64)

	var user models.User
	err := h.db.QueryRow(
		`SELECT id, username, email, is_superuser, is_active, date_joined, locale,
		        total_storage, used_storage, profile_picture, last_login_at
		 FROM users WHERE id = $1`, userID,
	).Scan(&user.ID, &user.Username, &user.Email, &user.IsSuperuser, &user.IsActive,
		&user.DateJoined, &user.Locale, &user.TotalStorage, &user.UsedStorage,
		&user.ProfilePicture, &user.LastLoginAt)

	if err != nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":             user.ID,
		"username":       user.Username,
		"email":          user.Email,
		"is_superuser":   user.IsSuperuser,
		"is_active":      user.IsActive,
		"date_joined":    user.DateJoined,
		"locale":         user.Locale,
		"total_storage":  user.TotalStorage,
		"used_storage":   user.UsedStorage,
		"profile_picture": user.ProfilePicture,
		"last_login_at":  user.LastLoginAt,
	})
}

func (h *Handler) SendCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		http.Error(w, `{"error":"email required"}`, http.StatusBadRequest)
		return
	}
	// TODO: generate and send verification code via email/SMS
	writeJSON(w, http.StatusOK, map[string]string{"message": "code sent"})
}

func (h *Handler) LoginWithCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	// TODO: validate code, find/create user, generate tokens
	http.Error(w, `{"error":"not implemented"}`, http.StatusNotImplemented)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
