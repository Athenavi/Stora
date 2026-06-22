package main

// embeddedMigrations holds the core SQL migrations compiled into the binary.
// These are used by `stora-cli init` to bootstrap a fresh database.
// Custom migrations in the `migrations/` directory take precedence.
type embeddedMigration struct {
	Version     int
	Description string
	SQL         string
}

var coreMigrations = []embeddedMigration{
	{
		Version: 1, Description: "initial schema: users + auth",
		SQL: `
CREATE TABLE IF NOT EXISTS schema_version (
    version     INTEGER PRIMARY KEY,
    description TEXT NOT NULL DEFAULT '',
    applied_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    checksum    TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS users (
    id              BIGSERIAL PRIMARY KEY,
    username        VARCHAR(150) UNIQUE,
    email           VARCHAR(254) UNIQUE,
    password        TEXT,
    profile_picture TEXT,
    bio             TEXT,
    is_active       BOOLEAN DEFAULT true,
    is_superuser    BOOLEAN DEFAULT false,
    is_staff        BOOLEAN DEFAULT false,
    date_joined     TIMESTAMP,
    last_login_at   TIMESTAMP,
    last_login_ip   VARCHAR(45),
    register_ip     VARCHAR(45),
    locale          VARCHAR(10) DEFAULT 'zh_CN',
    is_2fa_enabled  BOOLEAN DEFAULT false,
    totp_secret     TEXT,
    backup_codes    TEXT,
    total_storage   BIGINT DEFAULT 1073741824,
    used_storage    BIGINT DEFAULT 0
);

CREATE TABLE IF NOT EXISTS user_sessions (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT REFERENCES users(id) ON DELETE CASCADE,
    token      TEXT,
    ip_address VARCHAR(45),
    user_agent TEXT,
    is_active  BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP
);

CREATE TABLE IF NOT EXISTS login_attempts (
    id         BIGSERIAL PRIMARY KEY,
    username   VARCHAR(150),
    ip_address VARCHAR(45),
    success    BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS token_blacklist (
    id             BIGSERIAL PRIMARY KEY,
    token          TEXT,
    blacklisted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at     TIMESTAMP
);`,
	},
	{
		Version: 2, Description: "file management tables",
		SQL: `
CREATE TABLE IF NOT EXISTS folders (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT REFERENCES users(id) ON DELETE CASCADE,
    parent_id  BIGINT REFERENCES folders(id) ON DELETE CASCADE,
    name       VARCHAR(255) NOT NULL,
    sort_order INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS file_items (
    id                BIGSERIAL PRIMARY KEY,
    user_id           BIGINT REFERENCES users(id) ON DELETE CASCADE,
    folder_id         BIGINT REFERENCES folders(id) ON DELETE SET NULL,
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

CREATE TABLE IF NOT EXISTS file_versions (
    id          BIGSERIAL PRIMARY KEY,
    file_id     BIGINT REFERENCES file_items(id) ON DELETE CASCADE,
    version_num INT DEFAULT 1,
    file_path   TEXT,
    file_size   BIGINT DEFAULT 0,
    file_hash   VARCHAR(64),
    created_by  BIGINT REFERENCES users(id),
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS file_tags (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT REFERENCES users(id) ON DELETE CASCADE,
    name       VARCHAR(100) NOT NULL,
    color      VARCHAR(20),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS file_tag_assignments (
    id      BIGSERIAL PRIMARY KEY,
    file_id BIGINT REFERENCES file_items(id) ON DELETE CASCADE,
    tag_id  BIGINT REFERENCES file_tags(id) ON DELETE CASCADE
);`,
	},
	{
		Version: 3, Description: "sharing + vault + system",
		SQL: `
CREATE TABLE IF NOT EXISTS share_links (
    id              BIGSERIAL PRIMARY KEY,
    file_id         BIGINT REFERENCES file_items(id) ON DELETE SET NULL,
    folder_id       BIGINT REFERENCES folders(id) ON DELETE SET NULL,
    user_id         BIGINT REFERENCES users(id) ON DELETE CASCADE,
    token           VARCHAR(64) DEFAULT '',
    short_code      VARCHAR(32) DEFAULT '',
    permission      VARCHAR(20) DEFAULT 'read',
    password        VARCHAR(128) DEFAULT '',
    is_active       BOOLEAN DEFAULT true,
    view_count      INT DEFAULT 0,
    download_count  INT DEFAULT 0,
    max_downloads   INT DEFAULT 0,
    expires_at      TIMESTAMP,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS file_shares (
    id          BIGSERIAL PRIMARY KEY,
    file_id     BIGINT REFERENCES file_items(id) ON DELETE CASCADE,
    owner_id    BIGINT REFERENCES users(id) ON DELETE CASCADE,
    shared_with BIGINT REFERENCES users(id) ON DELETE CASCADE,
    permission  VARCHAR(20) DEFAULT 'view',
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at  TIMESTAMP
);

CREATE TABLE IF NOT EXISTS vaults (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT REFERENCES users(id) ON DELETE CASCADE,
    name           VARCHAR(255) NOT NULL,
    description    TEXT,
    password_hash  VARCHAR(128) DEFAULT '',
    created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS vault_items (
    id           BIGSERIAL PRIMARY KEY,
    vault_id     BIGINT REFERENCES vaults(id) ON DELETE CASCADE,
    user_id      BIGINT REFERENCES users(id) ON DELETE CASCADE,
    name         VARCHAR(255) NOT NULL,
    filename     VARCHAR(512) DEFAULT '',
    file_size    BIGINT DEFAULT 0,
    mime_type    VARCHAR(128) DEFAULT '',
    content_type VARCHAR(64) DEFAULT 'file',
    content      TEXT,
    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS upload_tasks (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT REFERENCES users(id) ON DELETE CASCADE,
    upload_id       VARCHAR(255),
    filename        VARCHAR(255),
    file_size       BIGINT DEFAULT 0,
    chunk_size      INT DEFAULT 0,
    total_chunks    INT DEFAULT 0,
    received_chunks INT DEFAULT 0,
    status          VARCHAR(20) DEFAULT 'pending',
    mime_type       VARCHAR(255),
    folder_id       VARCHAR(50),
    first_chunk_hash VARCHAR(64),
    last_chunk_hash  VARCHAR(64),
    final_hash      VARCHAR(64),
    storage_path    TEXT,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS upload_chunks (
    id          BIGSERIAL PRIMARY KEY,
    upload_id   VARCHAR(255),
    chunk_index INT DEFAULT 0,
    chunk_hash  VARCHAR(64),
    chunk_size  INT DEFAULT 0,
    chunk_path  TEXT,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);`,
	},
	{
		Version: 4, Description: "RBAC + audit + settings",
		SQL: `
CREATE TABLE IF NOT EXISTS roles (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(100) NOT NULL,
    slug        VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    is_system   BOOLEAN DEFAULT false,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS capabilities (
    id          BIGSERIAL PRIMARY KEY,
    codename    VARCHAR(100) UNIQUE NOT NULL,
    name        VARCHAR(200),
    description TEXT,
    module      VARCHAR(100),
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS role_capabilities (
    id            BIGSERIAL PRIMARY KEY,
    role_id       BIGINT REFERENCES roles(id) ON DELETE CASCADE,
    capability_id BIGINT REFERENCES capabilities(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS user_roles (
    id      BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    role_id BIGINT REFERENCES roles(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id        BIGSERIAL PRIMARY KEY,
    user_id   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    action    VARCHAR(100) NOT NULL,
    resource  TEXT,
    detail    TEXT,
    ip_address VARCHAR(45),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS system_settings (
    id            BIGSERIAL PRIMARY KEY,
    setting_key   VARCHAR(100) UNIQUE NOT NULL,
    setting_value TEXT NOT NULL,
    setting_type  VARCHAR(50) DEFAULT 'string',
    description   TEXT,
    is_public     BOOLEAN DEFAULT false,
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS notifications (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT REFERENCES users(id) ON DELETE CASCADE,
    type       VARCHAR(50) NOT NULL,
    title      VARCHAR(255) NOT NULL,
    body       TEXT NOT NULL,
    data       TEXT,
    is_read    BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sensitive_words (
    id          BIGSERIAL PRIMARY KEY,
    word        VARCHAR(255) NOT NULL,
    replacement VARCHAR(255),
    level       INT DEFAULT 1,
    is_active   BOOLEAN DEFAULT true,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);`,
	},
	{
		Version: 5, Description: "performance indexes",
		SQL: `
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

CREATE INDEX IF NOT EXISTS idx_file_items_user_deleted ON file_items (user_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_file_items_user_folder ON file_items (user_id, folder_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_file_items_file_type ON file_items (user_id, file_type, deleted_at);
CREATE INDEX IF NOT EXISTS idx_file_items_favorite ON file_items (user_id, is_favorite, deleted_at) WHERE is_favorite = true;
CREATE INDEX IF NOT EXISTS idx_file_items_created_at ON file_items (user_id, created_at DESC, deleted_at);

CREATE INDEX IF NOT EXISTS idx_folders_user_parent ON folders (user_id, parent_id);
CREATE INDEX IF NOT EXISTS idx_share_links_short_code ON share_links (short_code) WHERE short_code != '';
CREATE INDEX IF NOT EXISTS idx_share_links_user ON share_links (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_file_fingerprints_hash ON file_fingerprints (hash);
CREATE INDEX IF NOT EXISTS idx_upload_tasks_status_updated ON upload_tasks (status, updated_at);
CREATE INDEX IF NOT EXISTS idx_vault_items_vault_user ON vault_items (vault_id, user_id);
CREATE INDEX IF NOT EXISTS idx_login_attempts_ip_created ON login_attempts (ip_address, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_login_attempts_username_created ON login_attempts (username, created_at DESC);`,
	},
}
