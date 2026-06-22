-- 001_initial_schema.sql
-- 基础表结构创建

CREATE TABLE IF NOT EXISTS schema_version (
    version     INTEGER PRIMARY KEY,
    description TEXT NOT NULL DEFAULT '',
    applied_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    checksum    TEXT NOT NULL DEFAULT ''
);

-- users
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
