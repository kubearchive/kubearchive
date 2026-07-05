DROP INDEX IF EXISTS log_url_uuid_idx;
CREATE INDEX IF NOT EXISTS log_url_uuid_idx ON resource USING btree (uuid);
