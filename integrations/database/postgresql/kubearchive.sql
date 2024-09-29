--
-- PostgreSQL database dump
--

-- Dumped from database version 16.4 (Debian 16.4-1.pgdg110+2)
-- Dumped by pg_dump version 16.1

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
-- Name: insert_owners(); Type: FUNCTION; Schema: public; Owner: kubearchive
--

CREATE FUNCTION public.insert_owners() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
  ref json;
BEGIN
  FOR ref IN SELECT * FROM jsonb_array_elements(NEW.data->'metadata'->'ownerReferences')
  LOOP
    INSERT INTO owner (uuid, owner_uuid) VALUES (NEW.uuid, (ref->>'uid')::uuid) ON CONFLICT DO NOTHING;
  END LOOP;

  RETURN NEW;
END;
$$;


ALTER FUNCTION public.insert_owners() OWNER TO kubearchive;

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
-- Name: owner; Type: TABLE; Schema: public; Owner: kubearchive
--

CREATE TABLE public.owner (
    uuid uuid NOT NULL,
    owner_uuid uuid NOT NULL
);


ALTER TABLE public.owner OWNER TO kubearchive;

--
-- Name: resource; Type: TABLE; Schema: public; Owner: kubearchive
--

CREATE TABLE public.resource (
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
-- Name: owner owner_pkey; Type: CONSTRAINT; Schema: public; Owner: kubearchive
--

ALTER TABLE ONLY public.owner
    ADD CONSTRAINT owner_pkey PRIMARY KEY (uuid, owner_uuid);


--
-- Name: resource resource_pkey; Type: CONSTRAINT; Schema: public; Owner: kubearchive
--

ALTER TABLE ONLY public.resource
    ADD CONSTRAINT resource_pkey PRIMARY KEY (uuid);


--
-- Name: resource insert_owners; Type: TRIGGER; Schema: public; Owner: kubearchive
--

CREATE TRIGGER insert_owners AFTER INSERT ON public.resource FOR EACH ROW EXECUTE FUNCTION public.insert_owners();


--
-- Name: resource set_timestamp; Type: TRIGGER; Schema: public; Owner: kubearchive
--

CREATE TRIGGER set_timestamp BEFORE UPDATE ON public.resource FOR EACH ROW EXECUTE FUNCTION public.trigger_set_timestamp();


--
-- Name: owner fk_resource_uuid; Type: FK CONSTRAINT; Schema: public; Owner: kubearchive
--

ALTER TABLE ONLY public.owner
    ADD CONSTRAINT fk_resource_uuid FOREIGN KEY (uuid) REFERENCES public.resource(uuid) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

