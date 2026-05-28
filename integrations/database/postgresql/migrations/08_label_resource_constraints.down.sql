ALTER TABLE resource_label ALTER COLUMN label_id SET STATISTICS -1;
ALTER TABLE resource_label DROP CONSTRAINT IF EXISTS fk_resource_label_label_id;
ALTER TABLE label_key_value DROP CONSTRAINT IF EXISTS fk_label_key_value_value_id;
ALTER TABLE label_key_value DROP CONSTRAINT IF EXISTS fk_label_key_value_key_id;
DROP INDEX IF EXISTS idx_resource_label_key_value;
