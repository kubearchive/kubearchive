#!/bin/bash
# Copyright Kronicler Authors
# SPDX-License-Identifier: Apache-2.0

set -o errexit -o errtrace

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd ${SCRIPT_DIR}

VERSION="9.0.1-2.2.1"
NAMESPACE="mysql"

# Install MySQL operator.
kubectl apply -f https://raw.githubusercontent.com/mysql/mysql-operator/${VERSION}/deploy/deploy-crds.yaml
kubectl apply -f https://raw.githubusercontent.com/mysql/mysql-operator/${VERSION}/deploy/deploy-operator.yaml
kubectl -n mysql-operator rollout status deployment --timeout=90s

# Create the MySQL database server.
kubectl create ns ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -
kubectl -n ${NAMESPACE} apply -f .
kubectl -n ${NAMESPACE} wait statefulset/kronicler --for=create --timeout=60s
kubectl -n ${NAMESPACE} rollout status statefulset --timeout=90s
kubectl -n ${NAMESPACE} wait pod --all --for=condition=ready --timeout=60s

# Create the kronicler database
LOCAL_PORT=3307
echo Forwarding local port ${LOCAL_PORT} to mysql/kronicler:3306.
export MYSQL_PWD=$(kubectl -n ${NAMESPACE} get secret root-secret -o jsonpath='{.data.rootPassword}' | base64 --decode)
export MYSQL_USER=$(kubectl -n ${NAMESPACE} get secret root-secret -o jsonpath='{.data.rootUser}' | base64 --decode)
kubectl -n ${NAMESPACE} port-forward service/kronicler ${LOCAL_PORT}:3306 >& /dev/null &

echo Waiting for ${LOCAL_PORT} to become available.
while ! nc -vz 127.0.0.1 ${LOCAL_PORT} > /dev/null 2>&1 ; do
    echo -n .
    sleep 0.5
done
echo .
mysql -u ${MYSQL_USER} -p${MYSQL_PWD} -h 127.0.0.1 -P ${LOCAL_PORT} -D mysql < kronicler.sql
mysql -u ${MYSQL_USER} -p${MYSQL_PWD} -h 127.0.0.1 -P ${LOCAL_PORT} -D mysql < create-user.sql

# Kill all background jobs, including the port-forward started earlier.
trap 'kill $(jobs -p)' EXIT
