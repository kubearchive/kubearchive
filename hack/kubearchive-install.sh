#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

# Install KubeArchive by itself. Used by quick install and also to update existing
# KubeArchive installations.

set -o errexit
set -o xtrace

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd ${SCRIPT_DIR}/..

if ! command -v jq &> /dev/null; then
  echo "[ERROR] jq is required but not installed. Please install jq and try again."
  exit 1
fi

PODS=$(kubectl -n kubearchive get pods | grep -E -v "NAME|No resources|apiserversource" |& awk '{print $1}')

# Extract the release version
NEXT_VERSION=$(cat VERSION)
echo ${NEXT_VERSION}
export NEXT_VERSION=${NEXT_VERSION}

bash cmd/operator/generate.sh
bash cmd/installer/generate.sh
# Delete the migration job if it already exists because Job specs are immutable.
kubectl -n kubearchive delete job kubearchive-schema-migration --ignore-not-found
# Save database credentials before ko apply overwrites the Secret with empty values.
# xtrace is disabled here to avoid printing credentials to logs.
{ set +x; } 2>/dev/null
DB_KIND=$(kubectl get secret -n kubearchive kubearchive-database-credentials \
  -o jsonpath='{.data.DATABASE_KIND}' 2>/dev/null | base64 --decode || true)
DB_URL=$(kubectl get secret -n kubearchive kubearchive-database-credentials \
  -o jsonpath='{.data.DATABASE_URL}' 2>/dev/null | base64 --decode || true)
DB_PORT=$(kubectl get secret -n kubearchive kubearchive-database-credentials \
  -o jsonpath='{.data.DATABASE_PORT}' 2>/dev/null | base64 --decode || true)
DB_DB=$(kubectl get secret -n kubearchive kubearchive-database-credentials \
  -o jsonpath='{.data.DATABASE_DB}' 2>/dev/null | base64 --decode || true)
DB_USER=$(kubectl get secret -n kubearchive kubearchive-database-credentials \
  -o jsonpath='{.data.DATABASE_USER}' 2>/dev/null | base64 --decode || true)
DB_PASSWORD=$(kubectl get secret -n kubearchive kubearchive-database-credentials \
  -o jsonpath='{.data.DATABASE_PASSWORD}' 2>/dev/null | base64 --decode || true)
set -o xtrace

# Run ko apply and capture its exit code without triggering errexit, so that
# credential restore always runs even if ko apply fails.
kubectl kustomize config/ | envsubst '$NEXT_VERSION' | ko apply --tags latest-build -f - --base-import-paths || KO_EXIT_CODE=$?

# Restore credentials if they were set before ko apply.
PATCH_FILE=$(mktemp)
trap 'rm -f "${PATCH_FILE}"' EXIT
{ set +x; } 2>/dev/null
if [ -n "${DB_PASSWORD}" ]; then
  jq -n \
    --arg kind "${DB_KIND}" \
    --arg url "${DB_URL}" \
    --arg port "${DB_PORT}" \
    --arg db "${DB_DB}" \
    --arg user "${DB_USER}" \
    --arg password "${DB_PASSWORD}" \
    '{"stringData": {"DATABASE_KIND": $kind, "DATABASE_URL": $url, "DATABASE_PORT": $port, "DATABASE_DB": $db, "DATABASE_USER": $user, "DATABASE_PASSWORD": $password}}' > "${PATCH_FILE}"
  kubectl patch -n kubearchive secret kubearchive-database-credentials --patch-file "${PATCH_FILE}"
fi
unset DB_KIND DB_URL DB_PORT DB_DB DB_USER DB_PASSWORD
set -o xtrace

# Propagate ko apply failure after credentials have been safely restored.
if [ -n "${KO_EXIT_CODE:-}" ]; then
  exit "${KO_EXIT_CODE}"
fi

kubectl -n kubearchive rollout status deployment --timeout=90s

# Wait for all the existing pods to terminate.
for pod in ${PODS}; do
    kubectl -n kubearchive wait pod $pod --for=delete --timeout=90s
done

# Now make sure all the new pods are ready.
kubectl -n kubearchive wait pod --all --for=condition=ready --timeout=90s

kubectl get -n kubearchive deployments
