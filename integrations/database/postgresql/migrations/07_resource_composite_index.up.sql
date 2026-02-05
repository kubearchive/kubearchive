-- Replace the old timestamp+id index with one that includes kind, api_version,
-- and namespace as leading columns. All queries filter on kind+api_version, most
-- also filter on namespace, and all order by creationTimestamp+id descending.
-- This allows index-ordered scans with early termination for LIMIT queries.
--
-- This must be a single-statement migration (no multi-statement mode) so that
-- golang-migrate does not wrap it in a transaction. CREATE INDEX CONCURRENTLY
-- cannot run inside a transaction.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_resource_kind_apiversion_ns_ts_id
    ON public.resource (kind, api_version, namespace, (data->'metadata'->>'creationTimestamp') DESC, id DESC);
