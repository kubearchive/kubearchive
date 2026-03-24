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

BATCH_SIZE="${BATCH_SIZE:-100000}"
SLEEP_INTERVAL="${SLEEP_INTERVAL:-0.1}"

PGPASSWORD="${DATABASE_PASSWORD}"; export PGPASSWORD

PSQL_OPTS="-h ${DATABASE_URL} -p ${DATABASE_PORT} -U ${DATABASE_USER} -d ${DATABASE_DB} -t -A"

# ============================================================================
# Step 1: Extract all unique label keys from existing resources
# ============================================================================
echo "Inserting label keys..."
# shellcheck disable=SC2086
psql ${PSQL_OPTS} -c "
    INSERT INTO label_key (key)
    SELECT DISTINCT key
    FROM resource r,
         LATERAL jsonb_object_keys(r.data -> 'metadata' -> 'labels') AS key
    WHERE r.data->'metadata'->'labels' IS NOT NULL
    ON CONFLICT (key) DO NOTHING;
"
echo "Label keys populated."

# ============================================================================
# Step 2: Extract all unique label values from existing resources
# ============================================================================
echo "Inserting label values..."
# shellcheck disable=SC2086
psql ${PSQL_OPTS} -c "
    INSERT INTO label_value (value)
    SELECT DISTINCT kv.value
    FROM resource r,
         LATERAL jsonb_each_text(r.data->'metadata'->'labels') AS kv(key, value)
    WHERE r.data->'metadata'->'labels' IS NOT NULL
    ON CONFLICT (value) DO NOTHING;
"
echo "Label values populated."

# ============================================================================
# Step 3: Create unique label pairs (key-value combinations)
# ============================================================================
echo "Inserting label key-value pairs..."
# shellcheck disable=SC2086
psql ${PSQL_OPTS} -c "
    INSERT INTO label_key_value (key_id, value_id)
    SELECT DISTINCT lk.id, lv.id
    FROM resource r,
         LATERAL jsonb_each_text(r.data->'metadata'->'labels') AS kv(key, value)
             INNER JOIN label_key lk ON lk.key = kv.key
             INNER JOIN label_value lv ON lv.value = kv.value
    WHERE r.data->'metadata'->'labels' IS NOT NULL
    ON CONFLICT (key_id, value_id) DO NOTHING;
"
echo "Label key-value pairs populated."

# ============================================================================
# Step 4: Populate resource_label in batches
# ============================================================================
echo "Populating resource_label table (batch size: ${BATCH_SIZE})..."

# Get min and max resource IDs
# shellcheck disable=SC2086
MIN_ID=$(psql ${PSQL_OPTS} -c "SELECT COALESCE(MIN(id), 0) FROM resource;")
# shellcheck disable=SC2086
MAX_ID=$(psql ${PSQL_OPTS} -c "SELECT COALESCE(MAX(id), 0) FROM resource;")

if [ "${MAX_ID}" -eq 0 ]; then
    echo "No resources found, nothing to populate."
    exit 0
fi

echo "Resource ID range: ${MIN_ID} to ${MAX_ID}"

total=0
current="${MIN_ID}"

while [ "${current}" -le "${MAX_ID}" ]; do
    batch_end=$((current + BATCH_SIZE - 1))

    # shellcheck disable=SC2086
    RESULT=$(psql ${PSQL_OPTS} -c "
        INSERT INTO resource_label (resource_id, label_id)
        SELECT DISTINCT r.id, lp.id
        FROM resource r
        CROSS JOIN LATERAL jsonb_each_text(r.data->'metadata'->'labels') AS kv(key, value)
            INNER JOIN label_key lk ON lk.key = kv.key
            INNER JOIN label_value lv ON lv.value = kv.value
            INNER JOIN label_key_value lp ON lp.key_id = lk.id AND lp.value_id = lv.id
        WHERE r.data->'metadata'->'labels' IS NOT NULL
          AND r.id >= ${current}
          AND r.id <= ${batch_end}
        ON CONFLICT (resource_id, label_id) DO NOTHING;
    ")

    count=$(echo "${RESULT}" | sed 's/INSERT 0 //')
    total=$((total + count))
    echo "Batch ${current}-${batch_end}: inserted ${count} rows (total: ${total})"

    current=$((batch_end + 1))
    sleep "${SLEEP_INTERVAL}"
done

echo "Resource label population complete. Total rows inserted: ${total}"
