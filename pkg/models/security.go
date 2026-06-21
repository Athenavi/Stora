package models

// SensitiveWord stores words to filter/monitor.
type SensitiveWord struct {
	ID          int64   `json:"id" db:"id"`
	Word        string  `json:"word" db:"word"`
	Replacement *string `json:"replacement" db:"replacement"`
	Level       int     `json:"level" db:"level"`
	IsActive    bool    `json:"is_active" db:"is_active"`
	CreatedAt   *string `json:"created_at" db:"created_at"`
	UpdatedAt   *string `json:"updated_at" db:"updated_at"`
}

// GDPRConsent tracks user consent records.
type GDPRConsent struct {
	ID          int64   `json:"id" db:"id"`
	UserID      int64   `json:"user_id" db:"user_id"`
	ConsentType string  `json:"consent_type" db:"consent_type"`
	Granted     bool    `json:"granted" db:"granted"`
	IPAddress   *string `json:"ip_address" db:"ip_address"`
	CreatedAt   *string `json:"created_at" db:"created_at"`
}
