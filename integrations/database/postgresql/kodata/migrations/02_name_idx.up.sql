CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX name_idx ON resource USING GIST (name gist_trgm_ops);
