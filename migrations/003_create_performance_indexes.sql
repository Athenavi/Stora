-- 003_create_performance_indexes.sql
-- 关键性能索引（解决 Phase 4 中分析的查询性能问题）

-- file_items 索引（最常用查询）
CREATE INDEX IF NOT EXISTS idx_file_items_user_deleted
    ON file_items (user_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_file_items_user_folder
    ON file_items (user_id, folder_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_file_items_file_type
    ON file_items (user_id, file_type, deleted_at);
CREATE INDEX IF NOT EXISTS idx_file_items_favorite
    ON file_items (user_id, is_favorite, deleted_at)
    WHERE is_favorite = true;
CREATE INDEX IF NOT EXISTS idx_file_items_created_at
    ON file_items (user_id, created_at DESC, deleted_at);

-- folders 索引
CREATE INDEX IF NOT EXISTS idx_folders_user_parent
    ON folders (user_id, parent_id);

-- share_links 查询索引
CREATE INDEX IF NOT EXISTS idx_share_links_short_code
    ON share_links (short_code)
    WHERE short_code != '';
CREATE INDEX IF NOT EXISTS idx_share_links_user
    ON share_links (user_id, created_at DESC);

-- file_fingerprints 去重查询索引
CREATE INDEX IF NOT EXISTS idx_file_fingerprints_hash
    ON file_fingerprints (hash);

-- upload_tasks 清理查询索引
CREATE INDEX IF NOT EXISTS idx_upload_tasks_status_updated
    ON upload_tasks (status, updated_at);

-- vault_items 查询索引
CREATE INDEX IF NOT EXISTS idx_vault_items_vault_user
    ON vault_items (vault_id, user_id);

-- login_attempts 限流查询索引
CREATE INDEX IF NOT EXISTS idx_login_attempts_ip_created
    ON login_attempts (ip_address, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_login_attempts_username_created
    ON login_attempts (username, created_at DESC);
