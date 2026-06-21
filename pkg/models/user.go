package models

import "time"

// User represents a user account.
type User struct {
	ID              int64      `json:"id" db:"id"`
	Username        *string    `json:"username" db:"username"`
	Email           *string    `json:"email" db:"email"`
	Password        *string    `json:"-" db:"password"`
	ProfilePicture  *string    `json:"profile_picture" db:"profile_picture"`
	Bio             *string    `json:"bio" db:"bio"`
	IsActive        bool       `json:"is_active" db:"is_active"`
	IsSuperuser     bool       `json:"is_superuser" db:"is_superuser"`
	IsStaff         bool       `json:"is_staff" db:"is_staff"`
	DateJoined      *time.Time `json:"date_joined" db:"date_joined"`
	LastLoginAt     *time.Time `json:"last_login_at" db:"last_login_at"`
	LastLoginIP     *string    `json:"last_login_ip" db:"last_login_ip"`
	RegisterIP      *string    `json:"register_ip" db:"register_ip"`
	Locale          string     `json:"locale" db:"locale"`
	Is2FAEnabled    bool       `json:"is_2fa_enabled" db:"is_2fa_enabled"`
	TOTPSecret      *string    `json:"-" db:"totp_secret"`
	BackupCodes     *string    `json:"-" db:"backup_codes"`
	TotalStorage    int64      `json:"total_storage" db:"total_storage"`       // bytes, default 1GB
	UsedStorage     int64      `json:"used_storage" db:"used_storage"`         // bytes
}

// UserSession represents a user login session.
type UserSession struct {
	ID         int64      `json:"id" db:"id"`
	UserID     int64      `json:"user_id" db:"user_id"`
	Token      string     `json:"token" db:"token"`
	IPAddress  *string    `json:"ip_address" db:"ip_address"`
	UserAgent  *string    `json:"user_agent" db:"user_agent"`
	IsActive   bool       `json:"is_active" db:"is_active"`
	CreatedAt  *string    `json:"created_at" db:"created_at"`
	ExpiresAt  *string    `json:"expires_at" db:"expires_at"`
}

// UserBlock represents a blocked user entry.
type UserBlock struct {
	ID        int64  `json:"id" db:"id"`
	UserID    int64  `json:"user_id" db:"user_id"`
	BlockedID int64  `json:"blocked_id" db:"blocked_id"`
	CreatedAt string `json:"created_at" db:"created_at"`
}

// LoginAttempt tracks login attempts for brute-force protection.
type LoginAttempt struct {
	ID        int64   `json:"id" db:"id"`
	Username  string  `json:"username" db:"username"`
	IPAddress string  `json:"ip_address" db:"ip_address"`
	Success   bool    `json:"success" db:"success"`
	CreatedAt string  `json:"created_at" db:"created_at"`
}

// TokenBlacklist stores revoked JWT tokens.
type TokenBlacklist struct {
	ID        int64  `json:"id" db:"id"`
	Token     string `json:"token" db:"token"`
	BlacklistedAt string `json:"blacklisted_at" db:"blacklisted_at"`
	ExpiresAt string `json:"expires_at" db:"expires_at"`
}
