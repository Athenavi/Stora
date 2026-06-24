-- Add storage_driver column to file_versions for restore support
ALTER TABLE file_versions
ADD COLUMN IF NOT EXISTS storage_driver VARCHAR(50) NOT NULL DEFAULT 'local';

-- Ensure file_hash is indexed for fingerprint lookups
CREATE INDEX IF NOT EXISTS idx_file_versions_file_hash ON file_versions(file_hash);
