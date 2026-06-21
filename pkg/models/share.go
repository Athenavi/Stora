package models

// ShareLink represents a file sharing link.
type ShareLink struct {
	ID           int64   `json:"id" db:"id"`
	FileID       int64   `json:"file_id" db:"file_id"`
	UserID       int64   `json:"user_id" db:"user_id"`
	Token        string  `json:"token" db:"token"`
	Password     *string `json:"-" db:"password"`
	ExpiresAt    *string `json:"expires_at" db:"expires_at"`
	MaxDownloads *int    `json:"max_downloads" db:"max_downloads"`
	DownloadCount int    `json:"download_count" db:"download_count"`
	IsActive     bool    `json:"is_active" db:"is_active"`
	CreatedAt    *string `json:"created_at" db:"created_at"`
}

// FileShare represents a direct file share between users.
type FileShare struct {
	ID         int64   `json:"id" db:"id"`
	FileID     int64   `json:"file_id" db:"file_id"`
	OwnerID    int64   `json:"owner_id" db:"owner_id"`
	SharedWith int64   `json:"shared_with" db:"shared_with"`
	Permission string  `json:"permission" db:"permission"` // view/download/edit
	CreatedAt  *string `json:"created_at" db:"created_at"`
	ExpiresAt  *string `json:"expires_at" db:"expires_at"`
}
