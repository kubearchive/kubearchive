-- Drop the legacy GIN index on JSONB labels; queries now use normalized tables.
-- This takes ACCESS EXCLUSIVE lock on resource but is instant.
DROP INDEX IF EXISTS idx_json_labels;
