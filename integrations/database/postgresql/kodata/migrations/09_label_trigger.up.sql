-- ============================================================================
-- FK to resource table + trigger to keep normalized tables in sync.
-- ============================================================================
-- This migration briefly takes SHARE ROW EXCLUSIVE lock on the resource table
-- (for the FK and trigger creation). Both statements are instant.

-- Foreign key from resource_label to resource. NOT VALID avoids scanning
-- existing rows.
ALTER TABLE resource_label ADD CONSTRAINT fk_resource_label_resource_id
    FOREIGN KEY (resource_id) REFERENCES resource(id) ON DELETE CASCADE NOT VALID;

-- Trigger function to sync labels to normalized tables on INSERT/UPDATE.
-- Uses separate statements so each gets a fresh snapshot in READ COMMITTED mode,
-- avoiding a race condition where ON CONFLICT DO NOTHING in a CTE makes a
-- concurrently-committed row invisible to later CTEs sharing the same snapshot.
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

    -- Sync labels using separate statements. Each statement gets its own snapshot
    -- in READ COMMITTED mode, ensuring visibility of rows committed by concurrent
    -- transactions between statements.
    IF NEW.data->'metadata'->'labels' IS NOT NULL THEN
        -- Insert new label keys (concurrent inserts handled by ON CONFLICT)
        INSERT INTO label_key (key)
        SELECT DISTINCT key
        FROM jsonb_each_text(NEW.data->'metadata'->'labels')
        ON CONFLICT (key) DO NOTHING;

        -- Insert new label values (new snapshot: sees keys committed above)
        INSERT INTO label_value (value)
        SELECT DISTINCT value
        FROM jsonb_each_text(NEW.data->'metadata'->'labels')
        ON CONFLICT (value) DO NOTHING;

        -- Insert new key-value pairs (new snapshot: sees keys and values)
        INSERT INTO label_key_value (key_id, value_id)
        SELECT DISTINCT lk.id, lv.id
        FROM jsonb_each_text(NEW.data->'metadata'->'labels') AS kv(key, value)
        JOIN label_key lk ON lk.key = kv.key
        JOIN label_value lv ON lv.value = kv.value
        ON CONFLICT (key_id, value_id) DO NOTHING;

        -- Insert resource-label associations (new snapshot: sees pairs)
        INSERT INTO resource_label (resource_id, label_id)
        SELECT NEW.id, lkv.id
        FROM jsonb_each_text(NEW.data->'metadata'->'labels') AS kv(key, value)
        JOIN label_key lk ON lk.key = kv.key
        JOIN label_value lv ON lv.value = kv.value
        JOIN label_key_value lkv ON lkv.key_id = lk.id AND lkv.value_id = lv.id
        ON CONFLICT (resource_id, label_id) DO NOTHING;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to sync labels on INSERT or UPDATE
CREATE TRIGGER trigger_sync_labels
    AFTER INSERT OR UPDATE OF data ON resource
    FOR EACH ROW
    EXECUTE FUNCTION sync_labels_to_relational_tables();
