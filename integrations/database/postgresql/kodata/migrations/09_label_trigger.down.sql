DROP TRIGGER IF EXISTS trigger_sync_labels ON resource;
DROP FUNCTION IF EXISTS sync_labels_to_relational_tables();
ALTER TABLE resource_label DROP CONSTRAINT IF EXISTS fk_resource_label_resource_id;
