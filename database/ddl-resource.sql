/* Copyright KubeArchive Authors
SPDX-License-Identifier: Apache-2.0 */

DO
$do$
    BEGIN
        IF EXISTS (
            SELECT FROM pg_catalog.pg_roles
            WHERE  rolname = 'kubearchive') THEN

            RAISE NOTICE 'Role "kubearchive" already exists. Skipping.';
        ELSE
            CREATE ROLE kubearchive LOGIN PASSWORD 'P0stgr3sdbP@ssword';
        END IF;
    END
$do$;

SELECT 'CREATE DATABASE kubearchive WITH OWNER kubearchive;'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'kubearchive')\gexec

\c kubearchive

CREATE SCHEMA IF NOT EXISTS kubearchive AUTHORIZATION kubearchive;

CREATE TABLE IF NOT EXISTS kubearchive.resource (
                                                    "uuid" uuid NOT NULL,
                                                    api_version varchar NOT NULL,
                                                    kind varchar NOT NULL,
                                                    "name" varchar NOT NULL,
                                                    "namespace" varchar NOT NULL,
                                                    resource_version varchar NULL,
                                                    created_at timestamp DEFAULT now() NOT NULL,
                                                    updated_at timestamp DEFAULT now() NOT NULL,
                                                    cluster_deleted_ts timestamp NULL,
                                                    "data" jsonb NOT NULL,
                                                    CONSTRAINT resource_pkey PRIMARY KEY (uuid)
                                                );

-- Table Triggers
CREATE OR REPLACE FUNCTION kubearchive.trigger_set_timestamp()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
BEGIN
	  NEW.updated_at = NOW();
RETURN NEW;
END;
	$function$
;

create trigger set_timestamp before
    update
    on
        kubearchive.resource for each row execute function kubearchive.trigger_set_timestamp();

-- Permissions

ALTER TABLE kubearchive.resource OWNER TO kubearchive;
GRANT ALL ON TABLE kubearchive.resource TO kubearchive;

ALTER FUNCTION kubearchive.trigger_set_timestamp() OWNER TO kubearchive;
GRANT ALL ON FUNCTION kubearchive.trigger_set_timestamp() TO kubearchive;
