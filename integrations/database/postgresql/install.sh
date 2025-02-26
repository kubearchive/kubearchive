#!/bin/bash 

set -x 
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

set -o errexit

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd ${SCRIPT_DIR}

VERSION="1.24.1"
NAMESPACE="postgresql"

# Install cloudnative-pg operator.
kubectl apply --server-side -f \
  https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/main/releases/cnpg-${VERSION}.yaml
kubectl get ns
kubectl get pods -n cnpg-system
kubectl rollout status deployment --namespace=cnpg-system --timeout=90s || true
kubectl get events -n cnpg-system
kubectl get pods -n cnpg-system
kubectl get deployment -n cnpg-system
kubectl logs deployment/cnpg-controller-manager -n cnpg-system

# Create the postgres database server.
kubectl create ns ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -
kubectl -n ${NAMESPACE} apply -f .
kubectl -n ${NAMESPACE} wait pod/kubearchive-1 --for=create --timeout=60s
kubectl -n ${NAMESPACE} wait pod/kubearchive-1 --for=condition=ready --timeout=90s

# Create the kubearchive database
LOCAL_PORT=5433
echo Forwarding port ${LOCAL_PORT} to service/kubearchive-wr:5432.
export PGPASSWORD=$(kubectl -n ${NAMESPACE} get secret kubearchive-superuser -o jsonpath='{.data.password}' | base64 --decode)
kubectl -n ${NAMESPACE} port-forward service/kubearchive-rw ${LOCAL_PORT}:5432 >& /dev/null &

echo Waiting for port $LOCAL_PORT to become available.
while ! nc -vz localhost ${LOCAL_PORT} > /dev/null 2>&1 ; do
    echo -n .
    sleep 0.5
done
echo .
psql -h localhost -U postgres -p ${LOCAL_PORT} -f kubearchive.sql

# Kill all background jobs, including the port-forward started earlier.
trap 'kill $(jobs -p)' EXIT
