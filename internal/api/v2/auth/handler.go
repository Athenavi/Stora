package authapi

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/Athenavi/Stora/pkg/auth"
	"github.com/Athenavi/Stora/pkg/models"
	"github.com/Athenavi/Stora/pkg/utils"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	db         *sql.DB
	jwtManager *auth.JWTManager
}

func NewHandler(db *sql.DB, jwtManager *auth.JWTManager) *Handler {
	return &Handler{db: db, jwtManager: jwtManager}
}

// Register accepts FormData: username, email, password, password_confirm
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if err := r.ParseForm(); err != nil {
			utils.WriteError(w, http.StatusBadRequest, "invalid form data")
			return
		}
	}

	username := r.FormValue("username")
	email := r.FormValue("email")
	password := r.FormValue("password")

	if username == "" || password == "" {
		utils.WriteError(w, http.StatusBadRequest, "username and password required")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	now := time.Now().Format(time.RFC3339)
	locale := "zh_CN"

	var userID int64
	err = h.db.QueryRow(
		`INSERT INTO users (username, email, password, is_active, date_joined, locale, total_storage, used_storage)
		 VALUES ($1, $2, $3, true, $4, $5, 1073741824, 0) RETURNING id`,
		username, email, string(hashedPassword), now, locale,
	).Scan(&userID)

	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			utils.WriteError(w, http.StatusConflict, "username or email already exists")
			return
		}
		utils.WriteError(w, http.StatusInternalServerError, "registration failed")
		return
	}

	tokens, err := h.jwtManager.GenerateTokens(userID, username, false)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "token generation failed")
		return
	}

	utils.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"user_id":  userID,
		"username": username,
		"tokens":   tokens,
	})
}

// Login accepts FormData: username, password
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if err := r.ParseForm(); err != nil {
			utils.WriteError(w, http.StatusBadRequest, "invalid form data")
			return
		}
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		utils.WriteError(w, http.StatusBadRequest, "username and password required")
		return
	}

	var user models.User
	err := h.db.QueryRow(
		`SELECT id, username, email, password, is_superuser, is_active, profile_picture
		 FROM users WHERE (username = $1 OR email = $1) AND is_active = true`,
		username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.Password, &user.IsSuperuser, &user.IsActive, &user.ProfilePicture)

	if err == sql.ErrNoRows {
		utils.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if user.Password == nil || !auth.CheckPassword(password, *user.Password) {
		utils.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	tokens, err := h.jwtManager.GenerateTokens(user.ID, *user.Username, user.IsSuperuser)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "token generation failed")
		return
	}

	// Login anomaly detection: check if IP changed significantly
	clientIP := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		clientIP = strings.Split(fwd, ",")[0]
	}
	if user.LastLoginIP != nil && *user.LastLoginIP != "" && *user.LastLoginIP != clientIP {
		h.db.Exec(`INSERT INTO notifications (user_id, type, title, body, created_at)
			VALUES ($1, 'security', '异地登录提醒', $2, $3)`,
			user.ID,
			fmt.Sprintf("您的账号从新位置登录（IP: %s）。上次登录 IP: %s", clientIP, *user.LastLoginIP),
			time.Now().Format(time.RFC3339))
	}
	// Update last login info
	h.db.Exec(`UPDATE users SET last_login_at = $1, last_login_ip = $2 WHERE id = $3`,
		time.Now().Format(time.RFC3339), clientIP, user.ID)

	// Format response matching frontend's LoginResponse expectations
	setAuthCookie(w, tokens.AccessToken, tokens.ExpiresIn)
	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"token_type":    tokens.TokenType,
		"expires_in":    tokens.ExpiresIn,
		"user": map[string]interface{}{
			"id":             user.ID,
			"username":       user.Username,
			"email":          user.Email,
			"is_superuser":   user.IsSuperuser,
			"is_active":      user.IsActive,
			"profile_picture": user.ProfilePicture,
		},
	})
}

// Refresh accepts FormData: refresh_token
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)
	r.ParseForm()

	refreshToken := r.FormValue("refresh_token")
	if refreshToken == "" {
		utils.WriteError(w, http.StatusBadRequest, "refresh_token required")
		return
	}

	tokens, err := h.jwtManager.RefreshTokens(refreshToken)
	if err != nil {
		utils.WriteError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"token_type":    tokens.TokenType,
		"expires_in":    tokens.ExpiresIn,
	})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	utils.WriteJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		utils.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var user models.User
	err := h.db.QueryRow(
		`SELECT id, username, email, is_superuser, is_active, date_joined, locale,
		        total_storage, used_storage, profile_picture
		 FROM users WHERE id = $1`, userID,
	).Scan(&user.ID, &user.Username, &user.Email, &user.IsSuperuser, &user.IsActive,
		&user.DateJoined, &user.Locale, &user.TotalStorage, &user.UsedStorage, &user.ProfilePicture)

	if err != nil {
		utils.WriteError(w, http.StatusNotFound, "user not found")
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
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
	})
}

// ── Session Management ──

func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	rows, err := h.db.Query(
		`SELECT id, ip_address, COALESCE(user_agent, ''), is_active, created_at, expires_at
		 FROM user_sessions WHERE user_id = $1 ORDER BY created_at DESC LIMIT 50`,
		userID,
	)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type Session struct {
		ID        int64   `json:"id"`
		IPAddress *string `json:"ip_address"`
		UserAgent string  `json:"user_agent"`
		IsActive  bool    `json:"is_active"`
		CreatedAt *string `json:"created_at"`
		ExpiresAt *string `json:"expires_at"`
	}
	var sessions = make([]Session, 0)
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.IPAddress, &s.UserAgent, &s.IsActive, &s.CreatedAt, &s.ExpiresAt); err == nil {
			sessions = append(sessions, s)
		}
	}
	utils.WriteJSON(w, http.StatusOK, sessions)
}

func (h *Handler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	sessionID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	result, err := h.db.Exec(
		`UPDATE user_sessions SET is_active = false WHERE id = $1 AND user_id = $2`,
		sessionID, userID,
	)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "revoke failed")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		utils.WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]string{"message": "revoked"})
}

func (h *Handler) SendCode(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)
	r.ParseForm()
	email := r.FormValue("email")
	if email == "" {
		utils.WriteError(w, http.StatusBadRequest, "email required")
		return
	}
	utils.WriteJSON(w, http.StatusOK, map[string]string{"message": "code sent"})
}

func (h *Handler) LoginWithCode(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)
	r.ParseForm()
	email := r.FormValue("email")
	code := r.FormValue("code")
	if email == "" || code == "" {
		utils.WriteError(w, http.StatusBadRequest, "email and code required")
		return
	}
	utils.WriteError(w, http.StatusNotImplemented, "not implemented")
}

// setAuthCookie sets the access_token as an HTTP-only cookie for SSR support.
func setAuthCookie(w http.ResponseWriter, token string, expiresIn int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    token,
		Path:     "/",
		MaxAge:   expiresIn,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
