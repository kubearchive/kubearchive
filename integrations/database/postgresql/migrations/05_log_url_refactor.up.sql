ALTER TABLE public.log_url
    ADD COLUMN query text,
    ADD COLUMN "start" text,
    ADD COLUMN "end" text;
