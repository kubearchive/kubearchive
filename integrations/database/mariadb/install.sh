#!/bin/bash
# Copyright Kronicler Authors
# SPDX-License-Identifier: Apache-2.0

set -o errexit -o errtrace

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd ${SCRIPT_DIR}

NAMESPACE="mariadb"

# Install MariaDB operator.
kubectl create ns mariadb-operator --dry-run=client -o yaml | kubectl apply -f -
helm repo add mariadb-operator https://helm.mariadb.com/mariadb-operator
helm repo update mariadb-operator
helm upgrade --install --namespace mariadb-operator mariadb-operator-crds mariadb-operator/mariadb-operator-crds
helm upgrade --install --namespace mariadb-operator mariadb-operator mariadb-operator/mariadb-operator
kubectl -n mariadb-operator rollout status deployment --timeout=90s

# Create the MariaDB database server.
echo "Creating the MariaDB database server."
kubectl create ns ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -
kubectl -n ${NAMESPACE} apply -f .
kubectl -n ${NAMESPACE} wait statefulset/kronicler --for=create --timeout=60s
kubectl -n ${NAMESPACE} wait pod --all --for=condition=ready --timeout=60s

# Create the kronicler database
LOCAL_PORT=3307
echo Forwarding local port ${LOCAL_PORT} to mariadb/kronicler:3306.
export ROOT_PWD=$(kubectl -n ${NAMESPACE} get secret kronicler -o jsonpath='{.data.root-password}' | base64 --decode)
kubectl -n ${NAMESPACE} port-forward service/kronicler ${LOCAL_PORT}:3306 & # >& /dev/null &

echo Waiting for port ${LOCAL_PORT} to become available.
while ! nc -vz localhost ${LOCAL_PORT} > /dev/null 2>&1 ; do
    echo -n .
    sleep 0.5
done
echo .
mariadb -u root -p${ROOT_PWD} -h localhost -P 3307 -D kronicler < kronicler.sql

# Kill all background jobs, including the port-forward started earlier.
trap 'kill $(jobs -p)' EXIT
