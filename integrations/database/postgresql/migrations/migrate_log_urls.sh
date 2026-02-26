#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0
#
# Batch data migration script for log_url table.
# Migrates old full-URL records to the new split format (base URL + query/start/end columns).
#
# This script processes rows in batches to minimize database locks.
# Run it after applying migration 05 (which adds the new columns) and before
# applying migration 06 (which drops the json_path column).
#
# Required environment variables:
#   DATABASE_URL      - PostgreSQL host
#   DATABASE_PORT     - PostgreSQL port
#   DATABASE_DB       - Database name
#   DATABASE_USER     - Database user
#   DATABASE_PASSWORD - Database password
#
# Usage:
#   ./migrate_log_urls.sh

set -euo pipefail

BATCH_SIZE="${BATCH_SIZE:-1000}"
SLEEP_INTERVAL="${SLEEP_INTERVAL:-0.1}"

PGPASSWORD="${DATABASE_PASSWORD}"; export PGPASSWORD

PSQL_OPTS="-h ${DATABASE_URL} -p ${DATABASE_PORT} -U ${DATABASE_USER} -d ${DATABASE_DB} -t -A"

echo "Starting batch migration of log_url records (batch size: ${BATCH_SIZE})..."

total=0
while true; do
    # shellcheck disable=SC2086
    RESULT=$(psql ${PSQL_OPTS} -c "
        WITH batch AS (
            SELECT id FROM public.log_url
            WHERE url ~ '^https?://[^/]+/'
            LIMIT ${BATCH_SIZE}
            FOR UPDATE SKIP LOCKED
        )
        UPDATE public.log_url SET
            query = CASE WHEN url ILIKE '%loki%' AND query IS NULL
                THEN substring(url from 'query=([^&]+)') ELSE query END,
            \"start\" = CASE WHEN url ILIKE '%loki%' AND \"start\" IS NULL
                THEN substring(url from 'start=([^&]+)') ELSE \"start\" END,
            \"end\" = CASE WHEN url ILIKE '%loki%' AND \"end\" IS NULL
                THEN substring(url from 'end=([^&]+)') ELSE \"end\" END,
            url = regexp_replace(url, '(https?://[^/]+).*', '\1')
        WHERE id IN (SELECT id FROM batch);
    ")

    if [ "${RESULT}" = "UPDATE 0" ]; then
        break
    fi

    count=$(echo "${RESULT}" | sed 's/UPDATE //')
    total=$((total + count))
    echo "Migrated ${count} rows (total: ${total})..."
    sleep "${SLEEP_INTERVAL}"
done

echo "Batch migration complete. Total rows migrated: ${total}"
