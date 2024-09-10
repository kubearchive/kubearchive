#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0
# WARNING: this is intended to run in a single node
#   Kubernetes environment and just for testing purposes

set -o errexit
set -o xtrace

kubectl apply -f integrations/database/postgresql/k8s-resources.yaml

kubectl rollout status deployment --namespace=databases --timeout=60s

echo "CREATE USER kubearchive WITH ENCRYPTED PASSWORD 'P0stgr3sdbP@ssword';" | kubectl exec -i -n databases deploy/postgresql -- psql
echo "CREATE DATABASE kubearchive WITH OWNER kubearchive;" | kubectl exec -i -n databases deploy/postgresql -- psql
cat database/ddl-resource.sql | kubectl exec -i -n databases deploy/postgresql -- psql --user=kubearchive kubearchive
