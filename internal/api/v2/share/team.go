package share

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Athenavi/Stora/internal/middleware"
	"github.com/go-chi/chi/v5"
)

// TeamHandler handles team/group management.
type TeamHandler struct {
	db *sql.DB
}

func NewTeamHandler(db *sql.DB) *TeamHandler {
	return &TeamHandler{db: db}
}

// ─── Teams CRUD ───

func (h *TeamHandler) ListTeams(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	rows, err := h.db.Query(
		`SELECT t.id, t.name, COALESCE(t.description, ''), t.owner_id,
		        COALESCE((SELECT COUNT(*) FROM team_members WHERE team_id = t.id), 0) AS member_count,
		        t.created_at
		 FROM teams t WHERE t.owner_id = $1
		 OR t.id IN (SELECT team_id FROM team_members WHERE user_id = $1)
		 ORDER BY t.name`,
		userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type Team struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		OwnerID     int64  `json:"owner_id"`
		MemberCount int    `json:"member_count"`
		CreatedAt   string `json:"created_at"`
	}
	var teams = make([]Team, 0)
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.OwnerID, &t.MemberCount, &t.CreatedAt); err == nil {
			teams = append(teams, t)
		}
	}
	writeJSON(w, http.StatusOK, teams)
}

func (h *TeamHandler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	var req struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	now := time.Now().Format(time.RFC3339)
	var teamID int64
	err := h.db.QueryRow(
		`INSERT INTO teams (name, description, owner_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $4) RETURNING id`,
		req.Name, req.Description, userID, now,
	).Scan(&teamID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}
	// Auto-add creator as admin member
	h.db.Exec(`INSERT INTO team_members (team_id, user_id, role, created_at) VALUES ($1, $2, 'admin', $3)`,
		teamID, userID, now)
	writeJSON(w, http.StatusCreated, map[string]int64{"id": teamID})
}

func (h *TeamHandler) DeleteTeam(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	teamID, _ := strconv.ParseInt(chi.URLParam(r, "teamId"), 10, 64)
	result, err := h.db.Exec(`DELETE FROM teams WHERE id = $1 AND owner_id = $2`, teamID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "deleted"})
}

// ─── Team Members ───

func (h *TeamHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	teamID, _ := strconv.ParseInt(chi.URLParam(r, "teamId"), 10, 64)
	rows, err := h.db.Query(
		`SELECT tm.id, tm.user_id, COALESCE(u.username, ''), COALESCE(u.email, ''), tm.role, tm.created_at
		 FROM team_members tm JOIN users u ON tm.user_id = u.id
		 WHERE tm.team_id = $1 ORDER BY tm.created_at`,
		teamID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	type Member struct {
		ID        int64  `json:"id"`
		UserID    int64  `json:"user_id"`
		Username  string `json:"username"`
		Email     string `json:"email"`
		Role      string `json:"role"`
		CreatedAt string `json:"created_at"`
	}
	var members = make([]Member, 0)
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.UserID, &m.Username, &m.Email, &m.Role, &m.CreatedAt); err == nil {
			members = append(members, m)
		}
	}
	writeJSON(w, http.StatusOK, members)
}

func (h *TeamHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	teamID, _ := strconv.ParseInt(chi.URLParam(r, "teamId"), 10, 64)

	// Verify ownership or admin
	var role string
	h.db.QueryRow(`SELECT role FROM team_members WHERE team_id = $1 AND user_id = $2`, teamID, userID).Scan(&role)
	if role != "admin" {
		var ownerID int64
		h.db.QueryRow(`SELECT owner_id FROM teams WHERE id = $1`, teamID).Scan(&ownerID)
		if ownerID != userID {
			writeError(w, http.StatusForbidden, "only team admin can add members")
			return
		}
	}

	var req struct {
		UserID int64  `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == 0 {
		writeError(w, http.StatusBadRequest, "user_id required")
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}
	now := time.Now().Format(time.RFC3339)
	_, err := h.db.Exec(
		`INSERT INTO team_members (team_id, user_id, role, created_at) VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		teamID, req.UserID, req.Role, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "add member failed")
		return
	}
	// Send notification
	h.db.Exec(`INSERT INTO notifications (user_id, type, title, body, created_at)
		SELECT $1, 'team', '团队邀请', $2, $3 FROM users WHERE id = $1`,
		req.UserID, fmt.Sprintf("您已被加入团队 #%d", teamID), now)
	writeJSON(w, http.StatusCreated, map[string]string{"message": "member added"})
}

func (h *TeamHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	teamID, _ := strconv.ParseInt(chi.URLParam(r, "teamId"), 10, 64)
	memberID, _ := strconv.ParseInt(chi.URLParam(r, "memberId"), 10, 64)
	// Only owner or admin can remove
	var role string
	h.db.QueryRow(`SELECT role FROM team_members WHERE team_id = $1 AND user_id = $2`, teamID, userID).Scan(&role)
	if role != "admin" {
		var ownerID int64
		h.db.QueryRow(`SELECT owner_id FROM teams WHERE id = $1`, teamID).Scan(&ownerID)
		if ownerID != userID {
			writeError(w, http.StatusForbidden, "permission denied")
			return
		}
	}
	_, err := h.db.Exec(`DELETE FROM team_members WHERE id = $1 AND team_id = $2`, memberID, teamID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "remove failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "removed"})
}

// ─── Team Folders (shared space) ───

func (h *TeamHandler) ListTeamFolders(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	teamID, _ := strconv.ParseInt(chi.URLParam(r, "teamId"), 10, 64)
	// Verify membership
	var count int
	h.db.QueryRow(`SELECT COUNT(*) FROM team_members WHERE team_id = $1 AND user_id = $2`, teamID, userID).Scan(&count)
	if count == 0 {
		writeError(w, http.StatusForbidden, "not a team member")
		return
	}
	rows, err := h.db.Query(
		`SELECT tf.id, tf.folder_id, COALESCE(f.filename, ''), tf.permission, tf.created_at
		 FROM team_folders tf JOIN file_items f ON tf.folder_id = f.id AND f.is_folder = true
		 WHERE tf.team_id = $1 ORDER BY f.filename`,
		teamID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	type Folder struct {
		ID         int64  `json:"id"`
		FolderID   int64  `json:"folder_id"`
		Name       string `json:"name"`
		Permission string `json:"permission"`
		CreatedAt  string `json:"created_at"`
	}
	var folders = make([]Folder, 0)
	for rows.Next() {
		var f Folder
		if err := rows.Scan(&f.ID, &f.FolderID, &f.Name, &f.Permission, &f.CreatedAt); err == nil {
			folders = append(folders, f)
		}
	}
	writeJSON(w, http.StatusOK, folders)
}

func (h *TeamHandler) AddTeamFolder(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	teamID, _ := strconv.ParseInt(chi.URLParam(r, "teamId"), 10, 64)
	// Verify admin
	var role string
	h.db.QueryRow(`SELECT role FROM team_members WHERE team_id = $1 AND user_id = $2`, teamID, userID).Scan(&role)
	if role != "admin" {
		var ownerID int64
		h.db.QueryRow(`SELECT owner_id FROM teams WHERE id = $1`, teamID).Scan(&ownerID)
		if ownerID != userID {
			writeError(w, http.StatusForbidden, "admin required")
			return
		}
	}
	var req struct {
		FolderID   int64  `json:"folder_id"`
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FolderID == 0 {
		writeError(w, http.StatusBadRequest, "folder_id required")
		return
	}
	if req.Permission == "" {
		req.Permission = "read"
	}
	now := time.Now().Format(time.RFC3339)
	var tfID int64
	err := h.db.QueryRow(
		`INSERT INTO team_folders (team_id, folder_id, permission, created_at)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		teamID, req.FolderID, req.Permission, now,
	).Scan(&tfID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "add folder failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": tfID})
}

func (h *TeamHandler) RemoveTeamFolder(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.GetUserID(r.Context())
	teamID, _ := strconv.ParseInt(chi.URLParam(r, "teamId"), 10, 64)
	folderID, _ := strconv.ParseInt(chi.URLParam(r, "folderId"), 10, 64)
	var role string
	h.db.QueryRow(`SELECT role FROM team_members WHERE team_id = $1 AND user_id = $2`, teamID, userID).Scan(&role)
	if role != "admin" {
		var ownerID int64
		h.db.QueryRow(`SELECT owner_id FROM teams WHERE id = $1`, teamID).Scan(&ownerID)
		if ownerID != userID {
			writeError(w, http.StatusForbidden, "admin required")
			return
		}
	}
	_, err := h.db.Exec(`DELETE FROM team_folders WHERE team_id = $1 AND folder_id = $2`, teamID, folderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "remove failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "removed"})
}

// ─── Search Users (for adding members) ───

func (h *TeamHandler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "query required")
		return
	}
	rows, err := h.db.Query(
		`SELECT id, username, COALESCE(email, '') FROM users
		 WHERE username ILIKE $1 OR email ILIKE $1 LIMIT 20`,
		"%"+q+"%",
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	type User struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	var users = make([]User, 0)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email); err == nil {
			users = append(users, u)
		}
	}
	writeJSON(w, http.StatusOK, users)
}
