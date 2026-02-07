BEGIN;

-- Drop the trigger first
DROP TRIGGER IF EXISTS trigger_sync_labels ON resource;

-- Drop the trigger function
DROP FUNCTION IF EXISTS sync_labels_to_relational_tables();

-- Drop tables in reverse order (to respect foreign key constraints)
DROP TABLE IF EXISTS public.resource_label;
DROP TABLE IF EXISTS public.label_pair;
DROP TABLE IF EXISTS public.label_value;
DROP TABLE IF EXISTS public.label_key;

COMMIT;
