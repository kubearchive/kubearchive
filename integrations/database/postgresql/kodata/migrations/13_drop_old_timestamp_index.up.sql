-- Drop the legacy timestamp+id index now that the composite index
-- (kind, api_version, namespace, creationTimestamp, id) from migration 12
-- covers all query patterns that used this index.
DROP INDEX IF EXISTS idx_creation_timestamp_id;
