#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0
#
# Create a temporary index on resource.updated_at to speed up the label
# backfill in migration 10. The index is dropped later in migration 11.
#
# This runs as a shell script (not a .sql migration) because
# CREATE INDEX CONCURRENTLY cannot execute inside a transaction, and
# golang-migrate wraps multi-statement .sql files in a transaction.
#
# Required environment variables:
#   DATABASE_URL      - PostgreSQL host
#   DATABASE_PORT     - PostgreSQL port
#   DATABASE_DB       - Database name
#   DATABASE_USER     - Database user
#   DATABASE_PASSWORD - Database password

set -euo pipefail

log() { echo "$(date '+%Y-%m-%d %H:%M:%S') $*"; }

PGPASSWORD="${DATABASE_PASSWORD}"; export PGPASSWORD

PSQL_OPTS="-h ${DATABASE_URL} -p ${DATABASE_PORT} -U ${DATABASE_USER} -d ${DATABASE_DB}"

log "Creating index on resource.updated_at..."

# Drop any invalid index left by a previously failed CONCURRENTLY build,
# otherwise IF NOT EXISTS would match the invalid index and skip creation.
# shellcheck disable=SC2086
psql ${PSQL_OPTS} -t -A -c "
  SELECT 1 FROM pg_class c JOIN pg_index i ON c.oid = i.indexrelid
  WHERE c.relname = 'idx_resource_updated_at' AND NOT i.indisvalid;
" | grep -q 1 && {
  log "Dropping invalid index idx_resource_updated_at from a previous failed run..."
  # shellcheck disable=SC2086
  psql ${PSQL_OPTS} -c "DROP INDEX IF EXISTS idx_resource_updated_at;"
}

# shellcheck disable=SC2086
psql ${PSQL_OPTS} -c "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_resource_updated_at ON resource (updated_at);"

log "Index idx_resource_updated_at created."
