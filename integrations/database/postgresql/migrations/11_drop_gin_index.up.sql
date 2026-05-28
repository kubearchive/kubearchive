-- Drop the legacy GIN index on JSONB labels; queries now use normalized tables.
-- This takes ACCESS EXCLUSIVE lock on resource but is instant.
DROP INDEX IF EXISTS idx_json_labels;

-- Drop the temporary index created by 09_10_create_updated_at_index.sh
DROP INDEX IF EXISTS idx_resource_updated_at;
