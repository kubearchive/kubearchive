#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd ${SCRIPT_DIR}

# renovate: datasource=github-releases depName=cloudnative-pg packageName=cloudnative-pg/cloudnative-pg
VERSION=1.24.1
NAMESPACE="postgresql"
SERVICE="kubearchive-rw"
LOCAL_PORT=5433
REMOTE_PORT=5432

# Install cloudnative-pg operator.
echo "[INFO] Installing cloudnative-pg operator..."
kubectl apply --server-side -f \
  https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/main/releases/cnpg-${VERSION}.yaml
kubectl rollout status deployment --namespace=cnpg-system --timeout=90s

echo "[INFO] Creating postgres database server..."
kubectl create ns ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -
kubectl -n ${NAMESPACE} apply -f .
kubectl -n ${NAMESPACE} wait pod/kubearchive-1 --for=create --timeout=60s
kubectl -n ${NAMESPACE} wait pod/kubearchive-1 --for=condition=ready --timeout=90s

# Get superuser password
export PGPASSWORD=$(kubectl -n ${NAMESPACE} get secret kubearchive-superuser -o jsonpath='{.data.password}' | base64 --decode)

# Function to start port-forward and wait for it
start_port_forward() {
  echo "[INFO] Starting port-forward for PostgreSQL..."
  kubectl -n "$NAMESPACE" port-forward service/$SERVICE $LOCAL_PORT:$REMOTE_PORT > /dev/null 2>&1 &
  PF_PID=$!

  # Wait for port to be available
  echo "[INFO] Waiting for PostgreSQL port-forward to be ready..."
  for i in {1..30}; do
    if nc -z 127.0.0.1 $LOCAL_PORT; then
      echo "[INFO] Port-forward is ready."
      # Add a small delay to ensure the connection is stable
      sleep 2
      return 0
    fi
    sleep 1
  done

  echo "[ERROR] Port-forward failed to become ready after 30 seconds"
  return 1
}

# Function to cleanup port-forward
cleanup_port_forward() {
  if [[ -n "${PF_PID:-}" ]]; then
    kill $PF_PID 2>/dev/null || true
    unset PF_PID
  fi
}

# Run setup.sql from host (ignore errors if DB/user already exist)
echo "[INFO] Running setup.sql from host (ignore errors if DB/user already exists)..."
if start_port_forward; then
  psql -h 127.0.0.1 -U postgres -p $LOCAL_PORT -f setup.sql || true
  cleanup_port_forward
else
  echo "[ERROR] Failed to establish port-forward for setup.sql"
  exit 1
fi

# Run migrate from host
echo "[INFO] Running migrations using migrate CLI..."
export KUBEARCHIVE_PASSWORD="$(python3 -c "import urllib.parse; print(urllib.parse.quote('Dat!abas]3Pass*w0rd', ''))")"; # notsecret
if start_port_forward; then
  # Use 127.0.0.1 instead of localhost to avoid IPv6 resolution issues in CI
  migrate -verbose \
    -path migrations/ \
    -database "postgresql://kubearchive:${KUBEARCHIVE_PASSWORD}@127.0.0.1:${LOCAL_PORT}/kubearchive" \
    up
  cleanup_port_forward
else
  echo "[ERROR] Failed to establish port-forward for migrations"
  exit 1
fi

echo "[INFO] Migrations complete."
