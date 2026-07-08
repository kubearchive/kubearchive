#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0
#
# Populate all label normalization tables from existing resource JSONB data.
#
# This script runs after migration 07 (which creates the tables with unique
# constraints) and before migration 08 (which adds indexes and foreign keys).
#
# All inserts use ON CONFLICT DO NOTHING, making this script fully idempotent:
# running it multiple times has no effect beyond the first successful run.
#
# Required environment variables:
#   DATABASE_URL      - PostgreSQL host
#   DATABASE_PORT     - PostgreSQL port
#   DATABASE_DB       - Database name
#   DATABASE_USER     - Database user
#   DATABASE_PASSWORD - Database password
#
# Optional environment variables:
#   BATCH_SIZE     - Number of resource IDs per batch (default: 100000)
#   SLEEP_INTERVAL - Seconds to sleep between batches (default: 0.1)

set -euo pipefail

log() { echo "$(date '+%Y-%m-%d %H:%M:%S') $*"; }

BATCH_SIZE="${BATCH_SIZE:-10000}"
SLEEP_INTERVAL="${SLEEP_INTERVAL:-0.1}"

PGPASSWORD="${DATABASE_PASSWORD}"; export PGPASSWORD

PSQL_OPTS="-h ${DATABASE_URL} -p ${DATABASE_PORT} -U ${DATABASE_USER} -d ${DATABASE_DB} -t -A"

# Get min and max resource IDs
# shellcheck disable=SC2086
MIN_ID=$(psql ${PSQL_OPTS} -c "SELECT COALESCE(MIN(id), 0) FROM resource;")
# shellcheck disable=SC2086
MAX_ID=$(psql ${PSQL_OPTS} -c "SELECT COALESCE(MAX(id), 0) FROM resource;")

if [ "${MAX_ID}" -eq 0 ]; then
    log "No resources found, nothing to populate."
    exit 0
fi

log "Resource ID range: ${MIN_ID} to ${MAX_ID}"

# Check if a previous run made partial progress by looking at the highest
# resource_id already recorded in resource_label.
# shellcheck disable=SC2086
RESUME_ID=$(psql ${PSQL_OPTS} -c "SELECT COALESCE(MAX(resource_id), 0) FROM resource_label;")

if [ "${RESUME_ID}" -gt "${MIN_ID}" ]; then
    log "Resuming from resource ID ${RESUME_ID} (previous progress detected)"
    MIN_ID="${RESUME_ID}"
fi

# ============================================================================
# Single-pass batched population of all label tables.
#
# Each batch:
#   1. Extracts label key-value pairs from resource JSONB into a temp table
#   2. Inserts unique keys into label_key
#   3. Inserts unique values into label_value
#   4. Inserts unique key-value pairs into label_key_value
#   5. Inserts resource-label associations into resource_label
#
# This avoids parsing the JSONB data multiple times per batch.
# ============================================================================
log "Populating label tables (batch size: ${BATCH_SIZE})..."

total_keys=0
total_values=0
total_pairs=0
total_links=0
current="${MIN_ID}"

while [ "${current}" -le "${MAX_ID}" ]; do
    batch_end=$((current + BATCH_SIZE - 1))

    # shellcheck disable=SC2086
    RESULT=$(psql ${PSQL_OPTS} -c "
        BEGIN;

        -- Extract JSONB labels once into a temp table
        CREATE TEMP TABLE batch_labels ON COMMIT DROP AS
        SELECT DISTINCT r.id AS resource_id, kv.key, kv.value
        FROM resource r,
             LATERAL jsonb_each_text(r.data->'metadata'->'labels') AS kv(key, value)
        WHERE r.data->'metadata'->'labels' IS NOT NULL
          AND r.id >= ${current}
          AND r.id <= ${batch_end};

        -- Insert unique keys
        INSERT INTO label_key (key)
        SELECT DISTINCT key FROM batch_labels
        ON CONFLICT (key) DO NOTHING;

        -- Insert unique values
        INSERT INTO label_value (value)
        SELECT DISTINCT value FROM batch_labels
        ON CONFLICT (value) DO NOTHING;

        -- Insert unique key-value pairs
        INSERT INTO label_key_value (key_id, value_id)
        SELECT DISTINCT lk.id, lv.id
        FROM batch_labels bl
            INNER JOIN label_key lk ON lk.key = bl.key
            INNER JOIN label_value lv ON lv.value = bl.value
        ON CONFLICT (key_id, value_id) DO NOTHING;

        -- Insert resource-label associations
        INSERT INTO resource_label (resource_id, label_id)
        SELECT DISTINCT bl.resource_id, lkv.id
        FROM batch_labels bl
            INNER JOIN label_key lk ON lk.key = bl.key
            INNER JOIN label_value lv ON lv.value = bl.value
            INNER JOIN label_key_value lkv ON lkv.key_id = lk.id AND lkv.value_id = lv.id
        ON CONFLICT (resource_id, label_id) DO NOTHING;

        COMMIT;
    ")

    # Parse INSERT counts: label_key, label_value, label_key_value, resource_label
    INSERTS=$(echo "${RESULT}" | grep '^INSERT' | sed 's/INSERT 0 //')
    keys=$(echo "${INSERTS}" | sed -n '1p')
    values=$(echo "${INSERTS}" | sed -n '2p')
    pairs=$(echo "${INSERTS}" | sed -n '3p')
    links=$(echo "${INSERTS}" | sed -n '4p')

    total_keys=$((total_keys + keys))
    total_values=$((total_values + values))
    total_pairs=$((total_pairs + pairs))
    total_links=$((total_links + links))

    log "Batch ${current}-${batch_end}: keys=${keys} values=${values} pairs=${pairs} resource_labels=${links}"

    current=$((batch_end + 1))
    sleep "${SLEEP_INTERVAL}"
done

log "Label tables population complete."
log "  label_key:       ${total_keys} rows inserted"
log "  label_value:     ${total_values} rows inserted"
log "  label_key_value: ${total_pairs} rows inserted"
log "  resource_label:  ${total_links} rows inserted"
