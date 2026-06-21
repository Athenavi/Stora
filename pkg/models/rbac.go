package models

// Role defines a named role for RBAC.
type Role struct {
	ID          int64   `json:"id" db:"id"`
	Name        string  `json:"name" db:"name"`
	Slug        string  `json:"slug" db:"slug"`
	Description *string `json:"description" db:"description"`
	IsSystem    bool    `json:"is_system" db:"is_system"`
	CreatedAt   *string `json:"created_at" db:"created_at"`
	UpdatedAt   *string `json:"updated_at" db:"updated_at"`
}

// Capability defines a permission capability.
type Capability struct {
	ID          int64   `json:"id" db:"id"`
	Codename    string  `json:"codename" db:"codename"`
	Name        string  `json:"name" db:"name"`
	Description *string `json:"description" db:"description"`
	Module      *string `json:"module" db:"module"`
	CreatedAt   *string `json:"created_at" db:"created_at"`
}

// RoleCapability links roles to capabilities.
type RoleCapability struct {
	ID           int64 `json:"id" db:"id"`
	RoleID       int64 `json:"role_id" db:"role_id"`
	CapabilityID int64 `json:"capability_id" db:"capability_id"`
}

// UserRole links users to roles.
type UserRole struct {
	ID     int64 `json:"id" db:"id"`
	UserID int64 `json:"user_id" db:"user_id"`
	RoleID int64 `json:"role_id" db:"role_id"`
}

// PermissionAuditLog records permission changes.
type PermissionAuditLog struct {
	ID         int64   `json:"id" db:"id"`
	ActorID    int64   `json:"actor_id" db:"actor_id"`
	TargetID   *int64  `json:"target_id" db:"target_id"`
	Action     string  `json:"action" db:"action"`
	Detail     *string `json:"detail" db:"detail"`
	CreatedAt  *string `json:"created_at" db:"created_at"`
}

// FieldPermission defines field-level access control.
type FieldPermission struct {
	ID          int64  `json:"id" db:"id"`
	RoleID      int64  `json:"role_id" db:"role_id"`
	ModelName   string `json:"model_name" db:"model_name"`
	FieldName   string `json:"field_name" db:"field_name"`
	CanRead     bool   `json:"can_read" db:"can_read"`
	CanWrite    bool   `json:"can_write" db:"can_write"`
}
