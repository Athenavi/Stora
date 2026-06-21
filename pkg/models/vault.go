package models

// Vault represents an encrypted vault/private space.
type Vault struct {
	ID          int64   `json:"id" db:"id"`
	UserID      int64   `json:"user_id" db:"user_id"`
	Name        string  `json:"name" db:"name"`
	Description *string `json:"description" db:"description"`
	CreatedAt   *string `json:"created_at" db:"created_at"`
	UpdatedAt   *string `json:"updated_at" db:"updated_at"`
}

// VaultItem represents an encrypted item within a vault.
type VaultItem struct {
	ID        int64   `json:"id" db:"id"`
	VaultID   int64   `json:"vault_id" db:"vault_id"`
	UserID    int64   `json:"user_id" db:"user_id"`
	Name      string  `json:"name" db:"name"`
	Type      string  `json:"type" db:"type"` // note/credential/file
	Content   string  `json:"-" db:"content"` // AES-256-GCM encrypted
	FileID    *int64  `json:"file_id" db:"file_id"`
	CreatedAt *string `json:"created_at" db:"created_at"`
	UpdatedAt *string `json:"updated_at" db:"updated_at"`
}

// TranscodeTask tracks video transcoding jobs.
type TranscodeTask struct {
	ID           int64   `json:"id" db:"id"`
	FileID       int64   `json:"file_id" db:"file_id"`
	UserID       int64   `json:"user_id" db:"user_id"`
	Status       string  `json:"status" db:"status"` // pending/processing/completed/failed
	Progress     int     `json:"progress" db:"progress"`
	TargetFormat string  `json:"target_format" db:"target_format"`
	OutputPath   *string `json:"output_path" db:"output_path"`
	ErrorMsg     *string `json:"error_msg" db:"error_msg"`
	CreatedAt    *string `json:"created_at" db:"created_at"`
	UpdatedAt    *string `json:"updated_at" db:"updated_at"`
}

// TranscriptionTask tracks audio transcription jobs.
type TranscriptionTask struct {
	ID        int64   `json:"id" db:"id"`
	FileID    int64   `json:"file_id" db:"file_id"`
	UserID    int64   `json:"user_id" db:"user_id"`
	Status    string  `json:"status" db:"status"`
	Content   *string `json:"content" db:"content"`
	ErrorMsg  *string `json:"error_msg" db:"error_msg"`
	CreatedAt *string `json:"created_at" db:"created_at"`
	UpdatedAt *string `json:"updated_at" db:"updated_at"`
}

// StoragePlan defines storage plan tiers.
type StoragePlan struct {
	ID          int64   `json:"id" db:"id"`
	Name        string  `json:"name" db:"name"`
	MaxBytes    int64   `json:"max_bytes" db:"max_bytes"`
	MaxFiles    int     `json:"max_files" db:"max_files"`
	Price       float64 `json:"price" db:"price"`
	IsActive    bool    `json:"is_active" db:"is_active"`
	CreatedAt   *string `json:"created_at" db:"created_at"`
	UpdatedAt   *string `json:"updated_at" db:"updated_at"`
}
