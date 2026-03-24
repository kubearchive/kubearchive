-- ============================================================================
-- Add indexes, foreign keys, and statistics on label tables.
-- ============================================================================
-- This runs after the population script has filled all label tables.

-- Index for label lookups (get all resources with a specific label pair)
CREATE INDEX IF NOT EXISTS idx_resource_label_key_value ON resource_label(label_id);

-- Foreign keys with NOT VALID to avoid scanning/locking referenced tables.
-- NOT VALID skips validation of existing rows. Future inserts/updates are
-- still checked. These can be validated later during a low-traffic window with:
--   ALTER TABLE label_key_value VALIDATE CONSTRAINT fk_label_key_value_key_id;
--   ALTER TABLE label_key_value VALIDATE CONSTRAINT fk_label_key_value_value_id;
--   ALTER TABLE resource_label VALIDATE CONSTRAINT fk_resource_label_label_id;
ALTER TABLE label_key_value ADD CONSTRAINT fk_label_key_value_key_id
    FOREIGN KEY (key_id) REFERENCES label_key(id) ON DELETE CASCADE NOT VALID;
ALTER TABLE label_key_value ADD CONSTRAINT fk_label_key_value_value_id
    FOREIGN KEY (value_id) REFERENCES label_value(id) ON DELETE CASCADE NOT VALID;
ALTER TABLE resource_label ADD CONSTRAINT fk_resource_label_label_id
    FOREIGN KEY (label_id) REFERENCES label_key_value(id) ON DELETE CASCADE NOT VALID;

-- Increase statistics target for label_id so the planner can use MCV estimates
-- for pre-resolved label pair IDs, enabling accurate row count predictions
ALTER TABLE resource_label ALTER COLUMN label_id SET STATISTICS 5000;
