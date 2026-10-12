package authapi

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/Athenavi/Stora/pkg/audit"
	"github.com/Athenavi/Stora/pkg/auth"
	"github.com/Athenavi/Stora/pkg/models"
	"github.com/Athenavi/Stora/pkg/utils"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	db           *sql.DB
	jwtManager   *auth.JWTManager
	cookieSecure bool
}

func NewHandler(db *sql.DB, jwtManager *auth.JWTManager, cookieSecure bool) *Handler {
	return &Handler{db: db, jwtManager: jwtManager, cookieSecure: cookieSecure}
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
		 VALUES ($1, $2, $3, true, $4, $5, 10737418240, 0) RETURNING id`,
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
		`SELECT id, username, email, password, is_superuser, is_active, profile_picture, last_login_ip
		 FROM users WHERE (username = $1 OR email = $1) AND is_active = true`,
		username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.Password, &user.IsSuperuser, &user.IsActive, &user.ProfilePicture, &user.LastLoginIP)

	if err == sql.ErrNoRows {
		audit.Log(h.db, audit.Entry{
			Action:    "auth.login_failed",
			Detail:    "unknown user",
			IPAddress: middleware.ClientIP(r),
		})
		utils.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if user.Password == nil || !auth.CheckPassword(password, *user.Password) {
		audit.Log(h.db, audit.Entry{
			UserID:    user.ID,
			Action:    "auth.login_failed",
			Resource:  fmt.Sprintf("user:%d", user.ID),
			Detail:    "bad password",
			IPAddress: middleware.ClientIP(r),
		})
		utils.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	tokens, err := h.jwtManager.GenerateTokens(user.ID, *user.Username, user.IsSuperuser)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "token generation failed")
		return
	}

	// Login anomaly detection: check if IP changed significantly
	clientIP := middleware.ClientIP(r)
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
	h.setAuthCookie(w, tokens.AccessToken, tokens.ExpiresIn)
	h.recordSession(user.ID, tokens.AccessToken, clientIP, r.UserAgent(), tokens.ExpiresIn)
	audit.Log(h.db, audit.Entry{
		UserID:    user.ID,
		Action:    "auth.login",
		Resource:  fmt.Sprintf("user:%d", user.ID),
		Detail:    fmt.Sprintf("username=%s", *user.Username),
		IPAddress: clientIP,
	})
	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"token_type":    tokens.TokenType,
		"expires_in":    tokens.ExpiresIn,
		"user": map[string]interface{}{
			"id":              user.ID,
			"username":        user.Username,
			"email":           user.Email,
			"is_superuser":    user.IsSuperuser,
			"is_active":       user.IsActive,
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

	// Keep the cookie in step with the refreshed token: it used to be left at the
	// original value, so once the original expired the browser session broke
	// while the API kept handing out fresh tokens.
	h.setAuthCookie(w, tokens.AccessToken, tokens.ExpiresIn)

	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"token_type":    tokens.TokenType,
		"expires_in":    tokens.ExpiresIn,
	})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	h.deactivateSession(userID, requestToken(r))
	h.clearAuthCookie(w)
	audit.Log(h.db, audit.Entry{
		UserID:    userID,
		Action:    "auth.logout",
		IPAddress: middleware.ClientIP(r),
	})
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
		"id":              user.ID,
		"username":        user.Username,
		"email":           user.Email,
		"is_superuser":    user.IsSuperuser,
		"is_active":       user.IsActive,
		"date_joined":     user.DateJoined,
		"locale":          user.Locale,
		"total_storage":   user.TotalStorage,
		"used_storage":    user.UsedStorage,
		"profile_picture": user.ProfilePicture,
	})
}

// UpdateMe updates the current user's profile (username, email, bio).
func (h *Handler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		utils.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var body struct {
		Username *string `json:"username"`
		Email    *string `json:"email"`
		Bio      *string `json:"bio"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Build dynamic UPDATE — only set non-nil fields
	fields := []string{}
	args := []interface{}{}
	argIdx := 1

	if body.Username != nil {
		fields = append(fields, fmt.Sprintf("username = $%d", argIdx))
		args = append(args, *body.Username)
		argIdx++
	}
	if body.Email != nil {
		fields = append(fields, fmt.Sprintf("email = $%d", argIdx))
		args = append(args, *body.Email)
		argIdx++
	}
	if body.Bio != nil {
		fields = append(fields, fmt.Sprintf("bio = $%d", argIdx))
		args = append(args, *body.Bio)
		argIdx++
	}

	if len(fields) == 0 {
		utils.WriteJSON(w, http.StatusOK, map[string]string{"message": "nothing to update"})
		return
	}

	args = append(args, userID)
	query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d",
		strings.Join(fields, ", "), argIdx)

	if _, err := h.db.Exec(query, args...); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "update failed")
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]string{"message": "profile updated"})
}

// ChangePassword updates the current user's password.
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		utils.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var body struct {
		CurrentPassword    string `json:"current_password"`
		NewPassword        string `json:"new_password"`
		NewPasswordConfirm string `json:"new_password_confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.NewPassword != body.NewPasswordConfirm {
		utils.WriteError(w, http.StatusBadRequest, "passwords do not match")
		return
	}
	if len(body.NewPassword) < 6 {
		utils.WriteError(w, http.StatusBadRequest, "password too short (min 6)")
		return
	}

	// Verify current password
	var hash string
	err := h.db.QueryRow(`SELECT password FROM users WHERE id = $1`, userID).Scan(&hash)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "query failed")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.CurrentPassword)); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "current password is incorrect")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "hash failed")
		return
	}

	if _, err := h.db.Exec(`UPDATE users SET password = $1 WHERE id = $2`, string(newHash), userID); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "update failed")
		return
	}

	audit.Log(h.db, audit.Entry{
		UserID:    userID,
		Action:    "auth.password_change",
		Resource:  fmt.Sprintf("user:%d", userID),
		IPAddress: middleware.ClientIP(r),
	})
	utils.WriteJSON(w, http.StatusOK, map[string]string{"message": "password changed"})
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
	audit.Log(h.db, audit.Entry{
		UserID:    userID,
		Action:    "auth.session_revoke",
		Resource:  fmt.Sprintf("session:%d", sessionID),
		IPAddress: middleware.ClientIP(r),
	})
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

// setAuthCookie mirrors the access token into a same-origin cookie so that
// browser-issued requests (SSR route loaders, <img>/<a> tags, downloads)
// authenticate without an Authorization header.
//
// HttpOnly is deliberately NOT set: the SPA currently reads this cookie from
// JavaScript to decide whether a user is signed in. Turning it on requires the
// frontend auth-probe rework described in the auth hardening assessment.
func (h *Handler) setAuthCookie(w http.ResponseWriter, token string, expiresIn int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    token,
		Path:     "/",
		MaxAge:   expiresIn,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearAuthCookie expires the auth cookie. Logout must call this: the SPA's own
// logout only dropped its in-memory token, so the cookie outlived the session
// and the next navigation was still authenticated.
func (h *Handler) clearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// recordSession stores a login session so /auth/sessions reflects reality.
//
// Security ceiling: access tokens are stateless, so flipping is_active here does
// NOT invalidate the token itself — it stops the session being listed as active
// and is what the UI and audit trail show. Real revocation needs a jti claim
// plus a checked blacklist (see docs/design-next-phase.md).
func (h *Handler) recordSession(userID int64, token, ip, userAgent string, expiresIn int) {
	now := time.Now()
	if _, err := h.db.Exec(
		`INSERT INTO user_sessions (user_id, token, ip_address, user_agent, is_active, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, true, $5, $6)`,
		userID, hashToken(token), nullable(ip), nullable(userAgent),
		now.Format(time.RFC3339),
		now.Add(time.Duration(expiresIn)*time.Second).Format(time.RFC3339),
	); err != nil {
		log.Printf("[Auth] failed to record session for user %d: %v", userID, err)
	}
}

// deactivateSession marks the session belonging to the presented token inactive.
func (h *Handler) deactivateSession(userID int64, token string) {
	if token == "" {
		return
	}
	h.db.Exec(`UPDATE user_sessions SET is_active = false WHERE user_id = $1 AND token = $2`,
		userID, hashToken(token))
}

// hashToken stores only a digest, so a database leak does not hand out live tokens.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// requestToken extracts the bearer token or the access_token cookie, mirroring
// the lookup order of middleware.AuthMiddleware.
func requestToken(r *http.Request) string {
	if header := r.Header.Get("Authorization"); header != "" {
		if parts := strings.SplitN(header, " ", 2); len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			return parts[1]
		}
	}
	if c, err := r.Cookie("access_token"); err == nil {
		return c.Value
	}
	return ""
}

// nullable maps an empty string to SQL NULL.
func nullable(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// ── Application-specific passwords ─────────────────────────────────────────
//
// WebDAV clients (Finder, Windows Explorer, rclone) only speak HTTP Basic auth,
// so they cannot present a JWT. Giving them the account password instead would
// leave it stored in plain text on the client and impossible to revoke on its
// own — hence a separate, individually revocable credential.

// CreateAppPassword issues an application-specific password.
func (h *Handler) CreateAppPassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		utils.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Password == "" {
		utils.WriteError(w, http.StatusBadRequest, "name and password required")
		return
	}
	if len(req.Password) < 8 {
		utils.WriteError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "hash failed")
		return
	}

	var id int64
	if err := h.db.QueryRow(
		`INSERT INTO app_passwords (user_id, name, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		userID, req.Name, hash,
	).Scan(&id); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "create failed")
		return
	}

	audit.Log(h.db, audit.Entry{
		UserID:    userID,
		Action:    "auth.app_password_create",
		Resource:  fmt.Sprintf("app_password:%d", id),
		Detail:    "name=" + req.Name,
		IPAddress: middleware.ClientIP(r),
	})
	utils.WriteJSON(w, http.StatusCreated, map[string]interface{}{"id": id, "name": req.Name})
}

// ListAppPasswords returns the caller's app passwords — never their hashes.
func (h *Handler) ListAppPasswords(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())

	rows, err := h.db.Query(
		`SELECT id, name, COALESCE(CAST(last_used_at AS TEXT), ''), CAST(created_at AS TEXT)
		   FROM app_passwords WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type appPassword struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		LastUsedAt string `json:"last_used_at"`
		CreatedAt  string `json:"created_at"`
	}
	items := make([]appPassword, 0)
	for rows.Next() {
		var p appPassword
		if err := rows.Scan(&p.ID, &p.Name, &p.LastUsedAt, &p.CreatedAt); err == nil {
			items = append(items, p)
		}
	}
	utils.WriteJSON(w, http.StatusOK, items)
}

// DeleteAppPassword revokes a single app password.
func (h *Handler) DeleteAppPassword(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	result, err := h.db.Exec(`DELETE FROM app_passwords WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		utils.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	audit.Log(h.db, audit.Entry{
		UserID:    userID,
		Action:    "auth.app_password_delete",
		Resource:  fmt.Sprintf("app_password:%d", id),
		IPAddress: middleware.ClientIP(r),
	})
	utils.WriteJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}
