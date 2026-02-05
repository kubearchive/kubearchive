BEGIN;

-- Drop the trigger first
DROP TRIGGER IF EXISTS trigger_sync_labels ON resource;

-- Drop the trigger function
DROP FUNCTION IF EXISTS sync_labels_to_relational_tables();

-- Drop tables in reverse order (to respect foreign key constraints)
DROP TABLE IF EXISTS public.resource_label;
DROP TABLE IF EXISTS public.label_key_value;
DROP TABLE IF EXISTS public.label_value;
DROP TABLE IF EXISTS public.label_key;

-- Restore the original timestamp+id index that was replaced
DROP INDEX IF EXISTS idx_resource_kind_apiversion_ns_ts_id;
CREATE INDEX idx_creation_timestamp_id ON public.resource
    USING btree ((((data -> 'metadata'::text) ->> 'creationTimestamp'::text)) DESC, id DESC);

-- Restore the GIN index on JSONB labels
CREATE INDEX idx_json_labels ON public.resource
    USING gin ((((data -> 'metadata'::text) -> 'labels'::text)));

COMMIT;
