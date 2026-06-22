-- 002_create_file_tables.sql
-- 文件相关表

CREATE TABLE IF NOT EXISTS file_items (
    id                BIGSERIAL PRIMARY KEY,
    user_id           BIGINT REFERENCES users(id) ON DELETE CASCADE,
    folder_id         BIGINT,
    filename          VARCHAR(255),
    original_filename VARCHAR(255),
    file_path         TEXT,
    file_url          TEXT,
    file_size         BIGINT DEFAULT 0,
    mime_type         VARCHAR(255),
    file_type         VARCHAR(50) DEFAULT 'other',
    storage_driver    VARCHAR(50) DEFAULT 'local',
    storage_bucket    TEXT,
    storage_key       TEXT,
    file_hash         VARCHAR(64),
    is_folder         BOOLEAN DEFAULT false,
    is_favorite       BOOLEAN DEFAULT false,
    is_encrypted      BOOLEAN DEFAULT false,
    thumbnail_url     TEXT,
    width             INT,
    height            INT,
    duration          INT,
    description       TEXT,
    download_count    INT DEFAULT 0,
    sort_order        INT DEFAULT 0,
    created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at        TIMESTAMP
);

CREATE TABLE IF NOT EXISTS folders (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT REFERENCES users(id) ON DELETE CASCADE,
    parent_id  BIGINT REFERENCES folders(id) ON DELETE CASCADE,
    name       VARCHAR(255) NOT NULL,
    sort_order INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS file_fingerprints (
    id              BIGSERIAL PRIMARY KEY,
    hash            VARCHAR(64) UNIQUE NOT NULL,
    file_size       BIGINT DEFAULT 0,
    mime_type       VARCHAR(255),
    storage_path    TEXT,
    reference_count INT DEFAULT 1,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
