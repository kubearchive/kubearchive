#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

set -o errexit -o errtrace

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd ${SCRIPT_DIR}

NAMESPACE="mariadb"

# Install MariaDB operator.
kubectl create ns mariadb-operator --dry-run=client -o yaml | kubectl apply -f -
helm upgrade --install --namespace mariadb-operator mariadb-operator mariadb-operator/mariadb-operator
kubectl -n mariadb-operator rollout status deployment --timeout=90s

# Create the MariaDB database server.
echo "Creating the MariaDB database server."
kubectl create ns ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -
kubectl -n ${NAMESPACE} apply -f .
kubectl -n ${NAMESPACE} wait statefulset/kubearchive --for=create --timeout=60s
kubectl -n ${NAMESPACE} wait pod --all --for=condition=ready --timeout=60s

# Create the kubearchive database
LOCAL_PORT=3307
echo Forwarding local port ${LOCAL_PORT} to mariadb/kubearchive:3306.
export ROOT_PWD=$(kubectl -n ${NAMESPACE} get secret kubearchive -o jsonpath='{.data.root-password}' | base64 --decode)
kubectl -n ${NAMESPACE} port-forward service/kubearchive ${LOCAL_PORT}:3306 & # >& /dev/null &

echo Waiting for port ${LOCAL_PORT} to become available.
while ! nc -vz localhost ${LOCAL_PORT} > /dev/null 2>&1 ; do
    echo -n .
    sleep 0.5
done
echo .
mariadb -u root -p${ROOT_PWD} -h localhost -P 3307 -D kubearchive < kubearchive.sql

# Kill all background jobs, including the port-forward started earlier.
trap 'kill $(jobs -p)' EXIT
