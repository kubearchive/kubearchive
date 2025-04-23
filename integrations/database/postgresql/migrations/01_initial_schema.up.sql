BEGIN;

CREATE FUNCTION public.trigger_set_timestamp() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = NOW();
RETURN NEW;
END;
    $$;

CREATE TABLE public.resource (
    id BIGSERIAL PRIMARY KEY,
    uuid uuid UNIQUE NOT NULL,
    api_version character varying NOT NULL,
    kind character varying NOT NULL,
    name character varying NOT NULL,
    namespace character varying NOT NULL,
    resource_version character varying,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    cluster_updated_ts timestamp with time zone NOT NULL,
    cluster_deleted_ts timestamp with time zone,
    data jsonb NOT NULL
);

CREATE TRIGGER set_timestamp BEFORE UPDATE ON public.resource FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();

CREATE TABLE public.log_url (
    id BIGSERIAL PRIMARY KEY,
    uuid uuid NOT NULL REFERENCES public.resource(uuid) ON DELETE CASCADE,
    url text NOT NULL,
    container_name text NOT NULL,
    json_path text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TRIGGER set_timestamp BEFORE UPDATE ON public.log_url FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();

CREATE INDEX idx_creation_timestamp_id ON public.resource
    USING btree ((((data -> 'metadata'::text) ->> 'creationTimestamp'::text)) DESC, id DESC);

CREATE INDEX idx_json_annotations ON public.resource
    USING gin ((((data -> 'metadata'::text) -> 'annotations'::text)));

CREATE INDEX idx_json_labels ON public.resource
    USING gin ((((data -> 'metadata'::text) -> 'labels'::text)));

CREATE INDEX idx_json_owners ON public.resource
    USING gin ((((data -> 'metadata'::text) -> 'ownerReferences'::text)) jsonb_path_ops);

CREATE INDEX log_url_uuid_idx ON public.resource
    USING btree (uuid);

CREATE INDEX resource_kind_namespace_idx ON public.resource
    USING btree (kind, api_version, namespace);

COMMIT;
