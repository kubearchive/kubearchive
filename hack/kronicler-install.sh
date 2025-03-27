#!/bin/bash
# Copyright Kronicler Authors
# SPDX-License-Identifier: Apache-2.0

# Install Kronicler by itself. Used by quick install and also to update existing
# Kronicler installations.

set -o errexit
set -o xtrace

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd ${SCRIPT_DIR}/..

PODS=$(kubectl -n kronicler get pods | grep -E -v "NAME|No resources|apiserversource" |& awk '{print $1}')

# Extract the release version
NEXT_VERSION=$(cat VERSION)
echo ${NEXT_VERSION}
export NEXT_VERSION=${NEXT_VERSION}

bash cmd/operator/generate.sh
kubectl kustomize config/ | envsubst | ko apply -f - --base-import-paths

kubectl -n kronicler rollout status deployment --timeout=90s

# Wait for all the existing pods to terminate.
for pod in ${PODS}; do
    kubectl -n kronicler wait pod $pod --for=delete --timeout=90s
done

# Now make sure all the new pods are ready.
kubectl -n kronicler wait pod --all --for=condition=ready --timeout=90s

# Make sure webhooks are up and running.
LOCAL_PORT=8443
kubectl -n kronicler port-forward service/kronicler-operator-webhooks ${LOCAL_PORT}:443 >& /dev/null &

echo "Waiting for port forwarding on $LOCAL_PORT to become available."
while ! nc -vz localhost ${LOCAL_PORT} > /dev/null 2>&1 ; do
    echo -n .
    sleep 0.5
done
echo .

kubectl get -n kronicler deployments

# Kill all background jobs, including the port-forward started earlier.
trap 'kill $(jobs -p)' EXIT
