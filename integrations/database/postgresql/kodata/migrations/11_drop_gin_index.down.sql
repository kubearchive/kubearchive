CREATE INDEX IF NOT EXISTS idx_json_labels ON public.resource
    USING gin ((((data -> 'metadata'::text) -> 'labels'::text)));
