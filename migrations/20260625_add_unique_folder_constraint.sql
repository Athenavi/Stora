-- Prevent duplicate files/folders at same level (user_id, folder_id, filename)
-- Two indexes because folder_id can be NULL (root items)
CREATE UNIQUE INDEX IF NOT EXISTS idx_file_items_uniq_root
    ON file_items(user_id, filename, is_folder)
    WHERE folder_id IS NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_file_items_uniq_folder
    ON file_items(user_id, folder_id, filename, is_folder)
    WHERE folder_id IS NOT NULL AND deleted_at IS NULL;
