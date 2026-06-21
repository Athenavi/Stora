package models

// AuditLog records administrative actions.
type AuditLog struct {
	ID        int64   `json:"id" db:"id"`
	UserID    int64   `json:"user_id" db:"user_id"`
	Action    string  `json:"action" db:"action"`
	Resource  *string `json:"resource" db:"resource"`
	Detail    *string `json:"detail" db:"detail"`
	IPAddress *string `json:"ip_address" db:"ip_address"`
	CreatedAt *string `json:"created_at" db:"created_at"`
}

// SystemSettings stores key-value configuration.
type SystemSettings struct {
	ID           int    `json:"id" db:"id"`
	SettingKey   string `json:"setting_key" db:"setting_key"`
	SettingValue string `json:"setting_value" db:"setting_value"`
	SettingType  *string `json:"setting_type" db:"setting_type"`
	Description  *string `json:"description" db:"description"`
	IsPublic     bool   `json:"is_public" db:"is_public"`
	CreatedAt    *string `json:"created_at" db:"created_at"`
	UpdatedAt    *string `json:"updated_at" db:"updated_at"`
}

// Notification represents a user notification.
type Notification struct {
	ID        int64   `json:"id" db:"id"`
	UserID    int64   `json:"user_id" db:"user_id"`
	Type      string  `json:"type" db:"type"`
	Title     string  `json:"title" db:"title"`
	Body      string  `json:"body" db:"body"`
	Data      *string `json:"data" db:"data"`
	IsRead    bool    `json:"is_read" db:"is_read"`
	CreatedAt *string `json:"created_at" db:"created_at"`
}

// SearchHistory stores user search queries.
type SearchHistory struct {
	ID        int64   `json:"id" db:"id"`
	UserID    int64   `json:"user_id" db:"user_id"`
	Query     string  `json:"query" db:"query"`
	CreatedAt *string `json:"created_at" db:"created_at"`
}

// StorageQuota tracks storage usage per user.
type StorageQuota struct {
	ID          int64  `json:"id" db:"id"`
	UserID      int64  `json:"user_id" db:"user_id"`
	TotalBytes  int64  `json:"total_bytes" db:"total_bytes"`
	UsedBytes   int64  `json:"used_bytes" db:"used_bytes"`
	CreatedAt   *string `json:"created_at" db:"created_at"`
	UpdatedAt   *string `json:"updated_at" db:"updated_at"`
}
