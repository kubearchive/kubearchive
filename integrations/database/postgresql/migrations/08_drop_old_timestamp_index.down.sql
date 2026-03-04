-- Restore the original timestamp+id index
CREATE INDEX idx_creation_timestamp_id ON public.resource
    USING btree ((((data -> 'metadata'::text) ->> 'creationTimestamp'::text)) DESC, id DESC);
