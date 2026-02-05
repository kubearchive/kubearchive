-- ============================================================================
-- Phase 1: Create normalized label tables (without unique constraints)
-- ============================================================================
-- Unique constraints are deferred to after the bulk insert (Phase 2) for
-- performance. Building the unique index after the data is loaded is much
-- faster than checking the index on every inserted row.

-- Create label_key table to store unique label keys
CREATE TABLE public.label_key (
    id BIGSERIAL PRIMARY KEY,
    key character varying NOT NULL
);

-- Create label_value table to store unique label values
CREATE TABLE public.label_value (
    id BIGSERIAL PRIMARY KEY,
    value character varying NOT NULL
);

-- Create label_key_value table to store unique key-value combinations
-- Foreign keys are added after bulk insert (Phase 3) to avoid row-level locks
CREATE TABLE public.label_key_value (
    id BIGSERIAL PRIMARY KEY,
    key_id BIGINT NOT NULL,
    value_id BIGINT NOT NULL
);

-- Create resource_label junction table to link resources to label pairs
-- Foreign keys are added after bulk insert (Phase 4) to avoid row-level locks
CREATE TABLE public.resource_label (
    id BIGSERIAL PRIMARY KEY,
    resource_id BIGINT NOT NULL,
    label_id BIGINT NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

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
SELECT DISTINCT key
FROM resource r,
     LATERAL jsonb_object_keys(r.data -> 'metadata' -> 'labels') AS key
WHERE r.data->'metadata'->'labels' IS NOT NULL;

-- Extract all unique label values from existing resource labels
INSERT INTO label_value (value)
SELECT DISTINCT kv.value
FROM resource r,
     LATERAL jsonb_each_text(r.data->'metadata'->'labels') AS kv(key, value)
WHERE r.data->'metadata'->'labels' IS NOT NULL;

-- Create unique label pairs (key-value combinations)
INSERT INTO label_key_value (key_id, value_id)
SELECT DISTINCT lk.id, lv.id
FROM resource r,
     LATERAL jsonb_each_text(r.data->'metadata'->'labels') AS kv(key, value)
         INNER JOIN label_key lk ON lk.key = kv.key
         INNER JOIN label_value lv ON lv.value = kv.value
WHERE r.data->'metadata'->'labels' IS NOT NULL;

-- ============================================================================
-- Phase 3: Add unique constraints
-- ============================================================================
-- Now that data is loaded, add constraints. This builds the unique indexes in
-- a single pass which is much faster than maintaining them during bulk insert.
-- These constraints are required by the trigger (Phase 5) which uses ON CONFLICT
-- and in phase 4 to make the JOINS for the INSERT of resource_label faster.

ALTER TABLE label_key ADD CONSTRAINT unique_label_key UNIQUE (key);
ALTER TABLE label_value ADD CONSTRAINT unique_label_value UNIQUE (value);
ALTER TABLE label_key_value ADD CONSTRAINT unique_label_key_value UNIQUE (key_id, value_id);

-- ============================================================================
-- Phase 4: Fill resource-label
-- ============================================================================

-- Create resource-label associations using label_id
-- This links each resource to its labels via label pairs
INSERT INTO resource_label (resource_id, label_id)
SELECT DISTINCT r.id, lp.id
FROM resource r,
     LATERAL jsonb_each_text(r.data->'metadata'->'labels') AS kv(key, value)
         INNER JOIN label_key lk ON lk.key = kv.key
         INNER JOIN label_value lv ON lv.value = kv.value
         INNER JOIN label_key_value lp ON lp.key_id = lk.id AND lp.value_id = lv.id
WHERE r.data->'metadata'->'labels' IS NOT NULL;


ALTER TABLE resource_label ADD CONSTRAINT unique_resource_label UNIQUE (resource_id, label_id);

-- Create index for label lookups (get all resources with a specific label pair)
CREATE INDEX idx_resource_label_key_value ON resource_label(label_id);

-- Add foreign keys with NOT VALID to avoid scanning/locking referenced tables.
-- NOT VALID skips validation of existing rows and does not take locks on the
-- referenced tables. Future inserts/updates are still checked.
-- These can be validated later during a low-traffic window with:
--   ALTER TABLE label_key_value VALIDATE CONSTRAINT fk_label_key_value_key_id;
--   ALTER TABLE label_key_value VALIDATE CONSTRAINT fk_label_key_value_value_id;
--   ALTER TABLE resource_label VALIDATE CONSTRAINT fk_resource_label_resource_id;
--   ALTER TABLE resource_label VALIDATE CONSTRAINT fk_resource_label_label_id;
ALTER TABLE label_key_value ADD CONSTRAINT fk_label_key_value_key_id
    FOREIGN KEY (key_id) REFERENCES label_key(id) ON DELETE CASCADE NOT VALID;
ALTER TABLE label_key_value ADD CONSTRAINT fk_label_key_value_value_id
    FOREIGN KEY (value_id) REFERENCES label_value(id) ON DELETE CASCADE NOT VALID;
ALTER TABLE resource_label ADD CONSTRAINT fk_resource_label_resource_id
    FOREIGN KEY (resource_id) REFERENCES resource(id) ON DELETE CASCADE NOT VALID;
ALTER TABLE resource_label ADD CONSTRAINT fk_resource_label_label_id
    FOREIGN KEY (label_id) REFERENCES label_key_value(id) ON DELETE CASCADE NOT VALID;

-- ============================================================================
-- Phase 5: Create trigger to keep normalized tables in sync
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
-- Phase 6: Index and statistics tuning
-- ============================================================================

-- Drop the legacy GIN index on JSONB labels; queries now use normalized tables
DROP INDEX IF EXISTS idx_json_labels;

-- Increase statistics target for label_id so the planner can use MCV estimates
-- for pre-resolved label pair IDs, enabling accurate row count predictions
ALTER TABLE resource_label ALTER COLUMN label_id SET STATISTICS 5000;
