BEGIN;

-- ============================================================================
-- Phase 1: Create normalized label tables
-- ============================================================================

-- Create label_key table to store unique label keys
CREATE TABLE public.label_key (
    id BIGSERIAL PRIMARY KEY,
    key character varying NOT NULL,
    CONSTRAINT unique_label_key UNIQUE (key)
);

-- Create label_value table to store unique label values
CREATE TABLE public.label_value (
    id BIGSERIAL PRIMARY KEY,
    value character varying NOT NULL,
    CONSTRAINT unique_label_value UNIQUE (value)
);

-- Create label_key_value table to store unique key-value combinations
CREATE TABLE public.label_key_value (
    id BIGSERIAL PRIMARY KEY,
    key_id BIGINT NOT NULL REFERENCES label_key(id) ON DELETE CASCADE,
    value_id BIGINT NOT NULL REFERENCES label_value(id) ON DELETE CASCADE,
    CONSTRAINT unique_label_key_value UNIQUE (key_id, value_id)
);

-- Create resource_label junction table to link resources to label pairs
CREATE TABLE public.resource_label (
    id BIGSERIAL PRIMARY KEY,
    resource_id BIGINT NOT NULL REFERENCES resource(id) ON DELETE CASCADE,
    label_id BIGINT NOT NULL REFERENCES label_key_value(id) ON DELETE CASCADE,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT unique_resource_label UNIQUE (resource_id, label_id)
);

-- Create index for label lookups (get all resources with a specific label pair)
-- This is the most important index for query performance
CREATE INDEX idx_resource_label_key_value ON resource_label(label_id);

-- Replace the old timestamp+id index with one that includes kind, api_version,
-- and namespace as leading columns. All queries filter on kind+api_version, most
-- also filter on namespace, and all order by creationTimestamp+id descending.
-- This allows index-ordered scans with early termination for LIMIT queries.
DROP INDEX IF EXISTS idx_creation_timestamp_id;
CREATE INDEX idx_resource_kind_apiversion_ns_ts_id
    ON public.resource (kind, api_version, namespace, (data->'metadata'->>'creationTimestamp') DESC, id DESC);

-- Revoke UPDATE permissions on immutable tables to prevent accidental modifications
-- These tables should only support INSERT and DELETE operations
REVOKE UPDATE ON public.label_key FROM PUBLIC;
REVOKE UPDATE ON public.label_value FROM PUBLIC;
REVOKE UPDATE ON public.label_key_value FROM PUBLIC;

-- ============================================================================
-- Phase 2: Populate tables from existing JSONB data
-- ============================================================================

-- Extract all unique label keys from existing resource labels
INSERT INTO label_key (key)
SELECT DISTINCT kv.key
FROM resource r,
     LATERAL jsonb_each_text(r.data->'metadata'->'labels') AS kv(key, value)
WHERE r.data->'metadata'->'labels' IS NOT NULL
ON CONFLICT (key) DO NOTHING;

-- Extract all unique label values from existing resource labels
INSERT INTO label_value (value)
SELECT DISTINCT kv.value
FROM resource r,
     LATERAL jsonb_each_text(r.data->'metadata'->'labels') AS kv(key, value)
WHERE r.data->'metadata'->'labels' IS NOT NULL
ON CONFLICT (value) DO NOTHING;

-- Create unique label pairs (key-value combinations)
INSERT INTO label_key_value (key_id, value_id)
SELECT DISTINCT lk.id, lv.id
FROM resource r,
     LATERAL jsonb_each_text(r.data->'metadata'->'labels') AS kv(key, value)
INNER JOIN label_key lk ON lk.key = kv.key
INNER JOIN label_value lv ON lv.value = kv.value
WHERE r.data->'metadata'->'labels' IS NOT NULL
ON CONFLICT (key_id, value_id) DO NOTHING;

-- Create resource-label associations using label_id
-- This links each resource to its labels via label pairs
INSERT INTO resource_label (resource_id, label_id)
SELECT DISTINCT
    r.id,
    lp.id
FROM resource r,
     LATERAL jsonb_each_text(r.data->'metadata'->'labels') AS kv(key, value)
INNER JOIN label_key lk ON lk.key = kv.key
INNER JOIN label_value lv ON lv.value = kv.value
INNER JOIN label_key_value lp ON lp.key_id = lk.id AND lp.value_id = lv.id
WHERE r.data->'metadata'->'labels' IS NOT NULL
ON CONFLICT (resource_id, label_id) DO NOTHING;

-- ============================================================================
-- Phase 3: Create trigger to keep normalized tables in sync
-- ============================================================================

-- Create trigger function to sync labels to normalized tables
-- This function is called automatically whenever a resource is inserted or updated
-- Uses set-based operations with CTEs for optimal performance
CREATE OR REPLACE FUNCTION sync_labels_to_relational_tables()
RETURNS TRIGGER AS $$
BEGIN
    -- Optimization: Only proceed if labels have changed (or it's an INSERT)
    IF TG_OP = 'UPDATE' AND (NEW.data->'metadata'->'labels' IS NOT DISTINCT FROM OLD.data->'metadata'->'labels') THEN
        RETURN NEW;  -- Labels haven't changed, skip processing
    END IF;

    -- Delete old label associations (for UPDATE case)
    -- For INSERT, this does nothing since resource_id doesn't exist yet
    DELETE FROM resource_label WHERE resource_id = NEW.id;

    -- Extract labels from JSONB and sync to relational tables using set-based operations
    -- This is much more efficient than processing labels one-by-one in a loop
    IF NEW.data->'metadata'->'labels' IS NOT NULL THEN
        WITH
        -- Extract all labels from JSONB into a set
        labels AS (
            SELECT key, value
            FROM jsonb_each_text(NEW.data->'metadata'->'labels')
        ),
        -- Insert all new keys at once, get IDs of inserted rows
        keys_inserted AS (
            INSERT INTO label_key (key)
            SELECT DISTINCT key FROM labels
            ON CONFLICT (key) DO NOTHING
            RETURNING id, key
        ),
        -- Combine newly inserted keys with existing keys
        all_keys AS (
            SELECT id, key FROM keys_inserted
            UNION ALL
            SELECT lk.id, lk.key
            FROM label_key lk
            JOIN labels l ON l.key = lk.key
            WHERE NOT EXISTS (SELECT 1 FROM keys_inserted ki WHERE ki.key = lk.key)
        ),
        -- Insert all new values at once, get IDs of inserted rows
        values_inserted AS (
            INSERT INTO label_value (value)
            SELECT DISTINCT value FROM labels
            ON CONFLICT (value) DO NOTHING
            RETURNING id, value
        ),
        -- Combine newly inserted values with existing values
        all_values AS (
            SELECT id, value FROM values_inserted
            UNION ALL
            SELECT lv.id, lv.value
            FROM label_value lv
            JOIN labels l ON l.value = lv.value
            WHERE NOT EXISTS (SELECT 1 FROM values_inserted vi WHERE vi.value = lv.value)
        ),
        -- Prepare all pairs to insert by joining labels with their key/value IDs
        pairs_to_insert AS (
            SELECT k.id AS key_id, v.id AS value_id
            FROM labels l
            JOIN all_keys k ON k.key = l.key
            JOIN all_values v ON v.value = l.value
        ),
        -- Insert all new pairs at once
        pairs_inserted AS (
            INSERT INTO label_key_value (key_id, value_id)
            SELECT key_id, value_id FROM pairs_to_insert
            ON CONFLICT (key_id, value_id) DO NOTHING
            RETURNING id, key_id, value_id
        ),
        -- Combine newly inserted pairs with existing pairs
        all_pairs AS (
            SELECT id, key_id, value_id FROM pairs_inserted
            UNION ALL
            SELECT lp.id, lp.key_id, lp.value_id
            FROM label_key_value lp
            JOIN pairs_to_insert pti ON pti.key_id = lp.key_id AND pti.value_id = lp.value_id
            WHERE NOT EXISTS (SELECT 1 FROM pairs_inserted pi WHERE pi.key_id = lp.key_id AND pi.value_id = lp.value_id)
        )
        -- Insert all resource_label associations at once
        INSERT INTO resource_label (resource_id, label_id)
        SELECT NEW.id, ap.id
        FROM all_pairs ap
        ON CONFLICT (resource_id, label_id) DO NOTHING;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger to sync labels on INSERT or UPDATE
-- Optimization is handled inside the function using TG_OP to avoid triggering on unchanged labels
CREATE TRIGGER trigger_sync_labels
    AFTER INSERT OR UPDATE OF data ON resource
    FOR EACH ROW
    EXECUTE FUNCTION sync_labels_to_relational_tables();

-- ============================================================================
-- Phase 4: Index and statistics tuning
-- ============================================================================

-- Drop the legacy GIN index on JSONB labels; queries now use normalized tables
-- DROP INDEX IF EXISTS idx_json_labels; // TODO

-- Increase statistics target for label_id so the planner can use MCV estimates
-- for pre-resolved label pair IDs, enabling accurate row count predictions
ALTER TABLE resource_label ALTER COLUMN label_id SET STATISTICS 1000;

COMMIT;
