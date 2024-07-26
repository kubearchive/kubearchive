/* Copyright KubeArchive Authors
SPDX-License-Identifier: Apache-2.0 */

CREATE SCHEMA IF NOT EXISTS kubearchive;

CREATE OR REPLACE FUNCTION kubearchive.trigger_set_timestamp()
	RETURNS TRIGGER AS $$
	BEGIN
	  NEW.updated_at = NOW();
	  RETURN NEW;
	  END;
	$$ LANGUAGE plpgsql;

CREATE TABLE IF NOT EXISTS kubearchive.resource (
    "uuid" uuid PRIMARY KEY,
    "api_version" varchar NOT NULL,
    "kind" varchar NOT NULL,
    "name" varchar NOT NULL,
    "namespace" varchar NOT NULL,
    "resource_version" varchar NULL,
    "created_at" timestamp NOT NULL DEFAULT now(),
    "updated_at" timestamp NOT NULL DEFAULT now(),
    "cluster_deleted_ts" timestamp NULL,
    "data" jsonb NOT NULL
    );

CREATE OR REPLACE TRIGGER set_timestamp
BEFORE UPDATE ON kubearchive.resource
FOR EACH ROW
EXECUTE PROCEDURE kubearchive.trigger_set_timestamp();
