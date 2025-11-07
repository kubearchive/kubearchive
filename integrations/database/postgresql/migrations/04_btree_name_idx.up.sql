CREATE INDEX IF NOT EXISTS resource_kind_namespace_name_idx ON public.resource
    USING btree (kind, api_version, namespace, name);

DROP INDEX IF EXISTS resource_kind_namespace_idx;

DROP INDEX IF EXISTS name_idx;

CREATE INDEX IF NOT EXISTS name_idx ON resource USING GIN (name gin_trgm_ops);

