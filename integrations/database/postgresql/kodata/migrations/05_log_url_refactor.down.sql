ALTER TABLE public.log_url
    DROP COLUMN IF EXISTS query,
    DROP COLUMN IF EXISTS "start",
    DROP COLUMN IF EXISTS "end";
