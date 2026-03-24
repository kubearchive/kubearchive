-- ============================================================================
-- Backfill labels for resources inserted during migration 07 scripts.
-- ============================================================================
-- The population scripts and migration 09 (trigger creation) are not atomic.
-- Resources inserted between the scripts and the trigger activation won't have
-- their labels in the normalized tables. This backfill closes that gap.
-- ON CONFLICT DO NOTHING ensures this is safe to run even if the trigger
-- already handled some of these resources.

-- Backfill label keys
INSERT INTO label_key (key)
SELECT DISTINCT key
FROM resource r,
     LATERAL jsonb_object_keys(r.data -> 'metadata' -> 'labels') AS key
WHERE r.data->'metadata'->'labels' IS NOT NULL
  AND r.updated_at >= now() - INTERVAL '7 days'
ON CONFLICT (key) DO NOTHING;

-- Backfill label values
INSERT INTO label_value (value)
SELECT DISTINCT kv.value
FROM resource r,
     LATERAL jsonb_each_text(r.data->'metadata'->'labels') AS kv(key, value)
WHERE r.data->'metadata'->'labels' IS NOT NULL
  AND r.updated_at >= now() - INTERVAL '7 days'
ON CONFLICT (value) DO NOTHING;

-- Backfill label key-value pairs
INSERT INTO label_key_value (key_id, value_id)
SELECT DISTINCT lk.id, lv.id
FROM resource r,
     LATERAL jsonb_each_text(r.data->'metadata'->'labels') AS kv(key, value)
         INNER JOIN label_key lk ON lk.key = kv.key
         INNER JOIN label_value lv ON lv.value = kv.value
WHERE r.data->'metadata'->'labels' IS NOT NULL
  AND r.updated_at >= now() - INTERVAL '7 days'
ON CONFLICT (key_id, value_id) DO NOTHING;

-- Backfill resource-label associations
INSERT INTO resource_label (resource_id, label_id)
SELECT DISTINCT r.id, lp.id
FROM resource r,
     LATERAL jsonb_each_text(r.data->'metadata'->'labels') AS kv(key, value)
         INNER JOIN label_key lk ON lk.key = kv.key
         INNER JOIN label_value lv ON lv.value = kv.value
         INNER JOIN label_key_value lp ON lp.key_id = lk.id AND lp.value_id = lv.id
WHERE r.data->'metadata'->'labels' IS NOT NULL
  AND r.updated_at >= now() - INTERVAL '7 days'
ON CONFLICT (resource_id, label_id) DO NOTHING;
