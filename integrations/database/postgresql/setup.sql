CREATE DATABASE kubearchive WITH TEMPLATE = template0 ENCODING = 'UTF8' LOCALE_PROVIDER = libc LOCALE = 'en_US.UTF-8';
ALTER DATABASE kubearchive SET "TimeZone" TO 'UTC';

CREATE USER kubearchive WITH ENCRYPTED PASSWORD 'Dat!abas]3Pass*w0rd';
ALTER DATABASE kubearchive OWNER TO kubearchive;
ALTER DATABASE kubearchive SET work_mem TO '64MB';
ALTER DATABASE kubearchive SET enable_bitmapscan = 'on';
