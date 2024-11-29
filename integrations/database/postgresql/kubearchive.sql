--
-- PostgreSQL database dump
--

-- Dumped from database version 16.2 (Debian 16.2-1.pgdg110+2)
-- Dumped by pg_dump version 16.2 (Debian 16.2-1.pgdg110+2)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

DROP DATABASE IF EXISTS kubearchive;
--
-- Name: kubearchive; Type: DATABASE; Schema: -; Owner: kubearchive
--

CREATE DATABASE kubearchive WITH TEMPLATE = template0 ENCODING = 'UTF8' LOCALE_PROVIDER = libc LOCALE = 'en_US.UTF-8';


ALTER DATABASE kubearchive OWNER TO kubearchive;

\connect kubearchive

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: kubearchive; Type: DATABASE PROPERTIES; Schema: -; Owner: kubearchive
--

ALTER DATABASE kubearchive SET "TimeZone" TO 'UTC';


\connect kubearchive

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: trigger_set_timestamp(); Type: FUNCTION; Schema: public; Owner: kubearchive
--

CREATE FUNCTION public.trigger_set_timestamp() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	  NEW.updated_at = NOW();
RETURN NEW;
END;
	$$;


ALTER FUNCTION public.trigger_set_timestamp() OWNER TO kubearchive;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: log_url; Type: TABLE; Schema: public; Owner: kubearchive
--

CREATE TABLE public.log_url (
    id bigint NOT NULL,
    uuid uuid NOT NULL,
    url text NOT NULL,
    container_name text NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.log_url OWNER TO kubearchive;

--
-- Name: log_url_id_seq; Type: SEQUENCE; Schema: public; Owner: kubearchive
--

CREATE SEQUENCE public.log_url_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.log_url_id_seq OWNER TO kubearchive;

--
-- Name: log_url_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: kubearchive
--

ALTER SEQUENCE public.log_url_id_seq OWNED BY public.log_url.id;


--
-- Name: resource; Type: TABLE; Schema: public; Owner: kubearchive
--

CREATE TABLE public.resource (
    id bigint NOT NULL,
    uuid uuid NOT NULL,
    api_version character varying NOT NULL,
    kind character varying NOT NULL,
    name character varying NOT NULL,
    namespace character varying NOT NULL,
    resource_version character varying,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    cluster_deleted_ts timestamp without time zone,
    data jsonb NOT NULL
);


ALTER TABLE public.resource OWNER TO kubearchive;

--
-- Name: resource_id_seq; Type: SEQUENCE; Schema: public; Owner: kubearchive
--

CREATE SEQUENCE public.resource_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.resource_id_seq OWNER TO kubearchive;

--
-- Name: resource_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: kubearchive
--

ALTER SEQUENCE public.resource_id_seq OWNED BY public.resource.id;


--
-- Name: log_url id; Type: DEFAULT; Schema: public; Owner: kubearchive
--

ALTER TABLE ONLY public.log_url ALTER COLUMN id SET DEFAULT nextval('public.log_url_id_seq'::regclass);


--
-- Name: resource id; Type: DEFAULT; Schema: public; Owner: kubearchive
--

ALTER TABLE ONLY public.resource ALTER COLUMN id SET DEFAULT nextval('public.resource_id_seq'::regclass);


--
-- Name: log_url log_url_pkey; Type: CONSTRAINT; Schema: public; Owner: kubearchive
--

ALTER TABLE ONLY public.log_url
    ADD CONSTRAINT log_url_pkey PRIMARY KEY (id);


--
-- Name: resource resource_pkey; Type: CONSTRAINT; Schema: public; Owner: kubearchive
--

ALTER TABLE ONLY public.resource
    ADD CONSTRAINT resource_pkey PRIMARY KEY (id);


--
-- Name: resource resource_uuid_key; Type: CONSTRAINT; Schema: public; Owner: kubearchive
--

ALTER TABLE ONLY public.resource
    ADD CONSTRAINT resource_uuid_key UNIQUE (uuid);


--
-- Name: idx_creation_timestamp_id; Type: INDEX; Schema: public; Owner: kubearchive
--

CREATE INDEX idx_creation_timestamp_id ON public.resource USING btree ((((data -> 'metadata'::text) ->> 'creationTimestamp'::text)) DESC, id DESC);


--
-- Name: idx_json_annotations; Type: INDEX; Schema: public; Owner: kubearchive
--

CREATE INDEX idx_json_annotations ON public.resource USING gin ((((data -> 'metadata'::text) -> 'annotations'::text)) jsonb_path_ops);


--
-- Name: idx_json_labels; Type: INDEX; Schema: public; Owner: kubearchive
--

CREATE INDEX idx_json_labels ON public.resource USING gin ((((data -> 'metadata'::text) -> 'labels'::text)) jsonb_path_ops);


--
-- Name: idx_json_owners; Type: INDEX; Schema: public; Owner: kubearchive
--

CREATE INDEX idx_json_owners ON public.resource USING gin ((((data -> 'metadata'::text) -> 'ownerReferences'::text)) jsonb_path_ops);


--
-- Name: log_url_uuid_idx; Type: INDEX; Schema: public; Owner: kubearchive
--

CREATE INDEX log_url_uuid_idx ON public.resource USING btree (uuid);


--
-- Name: resource_kind_namespace_idx; Type: INDEX; Schema: public; Owner: kubearchive
--

CREATE INDEX resource_kind_namespace_idx ON public.resource USING btree (kind, api_version, namespace);


--
-- Name: log_url set_timestamp; Type: TRIGGER; Schema: public; Owner: kubearchive
--

CREATE TRIGGER set_timestamp BEFORE UPDATE ON public.log_url FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();


--
-- Name: resource set_timestamp; Type: TRIGGER; Schema: public; Owner: kubearchive
--

CREATE TRIGGER set_timestamp BEFORE UPDATE ON public.resource FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();


--
-- Name: log_url log_url_uuid_fkey; Type: FK CONSTRAINT; Schema: public; Owner: kubearchive
--

ALTER TABLE ONLY public.log_url
    ADD CONSTRAINT log_url_uuid_fkey FOREIGN KEY (uuid) REFERENCES public.resource(uuid);


--
-- PostgreSQL database dump complete
--
