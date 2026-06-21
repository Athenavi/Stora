package models

// Folder represents a directory/folder.
type Folder struct {
	ID         int64   `json:"id" db:"id"`
	UserID     int64   `json:"user_id" db:"user_id"`
	ParentID   *int64  `json:"parent_id" db:"parent_id"`
	Name       string  `json:"name" db:"name"`
	SortOrder  int     `json:"sort_order" db:"sort_order"`
	CreatedAt  *string `json:"created_at" db:"created_at"`
	UpdatedAt  *string `json:"updated_at" db:"updated_at"`
}

// FileItem represents a stored file.
type FileItem struct {
	ID               int64   `json:"id" db:"id"`
	UserID           int64   `json:"user_id" db:"user_id"`
	FolderID         *int64  `json:"folder_id" db:"folder_id"`
	Filename         *string `json:"filename" db:"filename"`
	OriginalFilename *string `json:"original_filename" db:"original_filename"`
	FilePath         *string `json:"file_path" db:"file_path"`
	FileURL          *string `json:"file_url" db:"file_url"`
	FileSize         int64   `json:"file_size" db:"file_size"`
	MimeType         *string `json:"mime_type" db:"mime_type"`
	FileType         string  `json:"file_type" db:"file_type"` // image/video/audio/document/archive/other
	StorageDriver    string  `json:"storage_driver" db:"storage_driver"` // local/s3/minio
	StorageBucket    *string `json:"storage_bucket" db:"storage_bucket"`
	StorageKey       *string `json:"storage_key" db:"storage_key"`
	FileHash         *string `json:"file_hash" db:"file_hash"`
	IsFolder         bool    `json:"is_folder" db:"is_folder"`
	IsFavorite       bool    `json:"is_favorite" db:"is_favorite"`
	IsEncrypted      bool    `json:"is_encrypted" db:"is_encrypted"`
	ThumbnailURL     *string `json:"thumbnail_url" db:"thumbnail_url"`
	Width            *int    `json:"width" db:"width"`
	Height           *int    `json:"height" db:"height"`
	Duration         *int    `json:"duration" db:"duration"`
	Description      *string `json:"description" db:"description"`
	DownloadCount    int     `json:"download_count" db:"download_count"`
	SortOrder        int     `json:"sort_order" db:"sort_order"`
	CreatedAt        *string `json:"created_at" db:"created_at"`
	UpdatedAt        *string `json:"updated_at" db:"updated_at"`
	DeletedAt        *string `json:"deleted_at" db:"deleted_at"`
}

// FileVersion stores file content versions.
type FileVersion struct {
	ID          int64   `json:"id" db:"id"`
	FileID      int64   `json:"file_id" db:"file_id"`
	VersionNum  int     `json:"version_num" db:"version_num"`
	FilePath    string  `json:"file_path" db:"file_path"`
	FileSize    int64   `json:"file_size" db:"file_size"`
	FileHash    *string `json:"file_hash" db:"file_hash"`
	CreatedBy   int64   `json:"created_by" db:"created_by"`
	CreatedAt   *string `json:"created_at" db:"created_at"`
}

// FileTag represents a tag that can be assigned to files.
type FileTag struct {
	ID        int64   `json:"id" db:"id"`
	UserID    int64   `json:"user_id" db:"user_id"`
	Name      string  `json:"name" db:"name"`
	Color     *string `json:"color" db:"color"`
	CreatedAt *string `json:"created_at" db:"created_at"`
}

// FileTagAssignment links files to tags.
type FileTagAssignment struct {
	ID     int64 `json:"id" db:"id"`
	FileID int64 `json:"file_id" db:"file_id"`
	TagID  int64 `json:"tag_id" db:"tag_id"`
}

// FileFingerprint stores deduplication fingerprints.
type FileFingerprint struct {
	ID             int64   `json:"id" db:"id"`
	Hash           string  `json:"hash" db:"hash"`
	FileSize       int64   `json:"file_size" db:"file_size"`
	MimeType       *string `json:"mime_type" db:"mime_type"`
	StoragePath    *string `json:"storage_path" db:"storage_path"`
	ReferenceCount int     `json:"reference_count" db:"reference_count"`
	CreatedAt      *string `json:"created_at" db:"created_at"`
	UpdatedAt      *string `json:"updated_at" db:"updated_at"`
}

// FileOptimization stores optimization metadata.
type FileOptimization struct {
	ID          int64   `json:"id" db:"id"`
	FileID      int64   `json:"file_id" db:"file_id"`
	Format      string  `json:"format" db:"format"`
	FilePath    string  `json:"file_path" db:"file_path"`
	FileSize    int64   `json:"file_size" db:"file_size"`
	Width       *int    `json:"width" db:"width"`
	Height      *int    `json:"height" db:"height"`
	CreatedAt   *string `json:"created_at" db:"created_at"`
}

// AccessLog records file access events.
type AccessLog struct {
	ID        int64   `json:"id" db:"id"`
	FileID    int64   `json:"file_id" db:"file_id"`
	UserID    int64   `json:"user_id" db:"user_id"`
	Action    string  `json:"action" db:"action"` // view/download/edit/share
	IPAddress *string `json:"ip_address" db:"ip_address"`
	CreatedAt *string `json:"created_at" db:"created_at"`
}

// TrashItem represents a soft-deleted file in the trash.
type TrashItem struct {
	ID          int64   `json:"id" db:"id"`
	FileID      int64   `json:"file_id" db:"file_id"`
	UserID      int64   `json:"user_id" db:"user_id"`
	OriginalPath *string `json:"original_path" db:"original_path"`
	DeletedAt   *string `json:"deleted_at" db:"deleted_at"`
	ExpiresAt   *string `json:"expires_at" db:"expires_at"`
}

// UploadTask tracks file upload progress.
type UploadTask struct {
	ID            int64   `json:"id" db:"id"`
	UserID        int64   `json:"user_id" db:"user_id"`
	UploadID      string  `json:"upload_id" db:"upload_id"`
	Filename      string  `json:"filename" db:"filename"`
	FileSize      int64   `json:"file_size" db:"file_size"`
	ChunkSize     int     `json:"chunk_size" db:"chunk_size"`
	TotalChunks   int     `json:"total_chunks" db:"total_chunks"`
	ReceivedChunks int    `json:"received_chunks" db:"received_chunks"`
	Status        string  `json:"status" db:"status"` // pending/uploading/completed/failed
	CreatedAt     *string `json:"created_at" db:"created_at"`
	UpdatedAt     *string `json:"updated_at" db:"updated_at"`
}

// UploadChunk stores individual upload chunk metadata.
type UploadChunk struct {
	ID         int64  `json:"id" db:"id"`
	UploadID   string `json:"upload_id" db:"upload_id"`
	ChunkIndex int    `json:"chunk_index" db:"chunk_index"`
	ChunkHash  string `json:"chunk_hash" db:"chunk_hash"`
	ChunkSize  int    `json:"chunk_size" db:"chunk_size"`
	ChunkPath  string `json:"chunk_path" db:"chunk_path"`
	CreatedAt  string `json:"created_at" db:"created_at"`
}

// DownloadToken manages temporary download access.
type DownloadToken struct {
	ID        int64   `json:"id" db:"id"`
	Token     string  `json:"token" db:"token"`
	FileID    int64   `json:"file_id" db:"file_id"`
	UserID    *int64  `json:"user_id" db:"user_id"`
	IPAddress *string `json:"ip_address" db:"ip_address"`
	ExpiresAt *string `json:"expires_at" db:"expires_at"`
	UsedAt    *string `json:"used_at" db:"used_at"`
	CreatedAt *string `json:"created_at" db:"created_at"`
}

// DownloadTask tracks download jobs.
type DownloadTask struct {
	ID        int64   `json:"id" db:"id"`
	UserID    int64   `json:"user_id" db:"user_id"`
	URL       string  `json:"url" db:"url"`
	Filename  *string `json:"filename" db:"filename"`
	Status    string  `json:"status" db:"status"` // pending/downloading/completed/failed
	Progress  int     `json:"progress" db:"progress"`
	FileID    *int64  `json:"file_id" db:"file_id"`
	CreatedAt *string `json:"created_at" db:"created_at"`
	UpdatedAt *string `json:"updated_at" db:"updated_at"`
}
