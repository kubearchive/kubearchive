-- ============================================================================
-- Create normalized label tables with unique constraints.
-- ============================================================================
-- Unique constraints are included here (instant on empty tables) so that the
-- population scripts can use ON CONFLICT DO NOTHING for idempotency.
--
-- After this migration, the population script runs before migration 08.

-- Create label_key table to store unique label keys
CREATE TABLE IF NOT EXISTS public.label_key (
    id BIGSERIAL PRIMARY KEY,
    key character varying NOT NULL,
    CONSTRAINT unique_label_key UNIQUE (key)
);

-- Create label_value table to store unique label values
CREATE TABLE IF NOT EXISTS public.label_value (
    id BIGSERIAL PRIMARY KEY,
    value character varying NOT NULL,
    CONSTRAINT unique_label_value UNIQUE (value)
);

-- Create label_key_value table to store unique key-value combinations
CREATE TABLE IF NOT EXISTS public.label_key_value (
    id BIGSERIAL PRIMARY KEY,
    key_id BIGINT NOT NULL,
    value_id BIGINT NOT NULL,
    CONSTRAINT unique_label_key_value UNIQUE (key_id, value_id)
);

-- Create resource_label junction table to link resources to label pairs
CREATE TABLE IF NOT EXISTS public.resource_label (
    id BIGSERIAL PRIMARY KEY,
    resource_id BIGINT NOT NULL,
    label_id BIGINT NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT unique_resource_label UNIQUE (resource_id, label_id)
);

-- Revoke UPDATE permissions on immutable tables to prevent accidental modifications
-- These tables should only support INSERT and DELETE operations
REVOKE UPDATE ON public.label_key FROM PUBLIC;
REVOKE UPDATE ON public.label_value FROM PUBLIC;
REVOKE UPDATE ON public.label_key_value FROM PUBLIC;
