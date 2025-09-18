#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

# Install KubeArchive by itself. Used by quick install and also to update existing
# KubeArchive installations.

set -o errexit
set -o xtrace

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd ${SCRIPT_DIR}/..

PODS=$(kubectl -n kubearchive get pods | grep -E -v "NAME|No resources|apiserversource" |& awk '{print $1}')

# Extract the release version
NEXT_VERSION=$(cat VERSION)
echo ${NEXT_VERSION}
export NEXT_VERSION=${NEXT_VERSION}

bash cmd/operator/generate.sh
kubectl kustomize config/ | envsubst '$NEXT_VERSION' | ko apply --tags latest-build -f - --base-import-paths

kubectl -n kubearchive rollout status deployment --timeout=90s

# Wait for all the existing pods to terminate.
for pod in ${PODS}; do
    kubectl -n kubearchive wait pod $pod --for=delete --timeout=90s
done

# Now make sure all the new pods are ready.
kubectl -n kubearchive wait pod --all --for=condition=ready --timeout=90s

# Make sure webhooks are up and running.
LOCAL_PORT=8443
kubectl -n kubearchive port-forward service/kubearchive-operator-webhooks ${LOCAL_PORT}:443 >& /dev/null &

echo "Waiting for port forwarding on $LOCAL_PORT to become available."
while ! nc -vz localhost ${LOCAL_PORT} > /dev/null 2>&1 ; do
    echo -n .
    sleep 0.5
done
echo .

kubectl get -n kubearchive deployments

# Kill all background jobs, including the port-forward started earlier.
trap 'kill $(jobs -p)' EXIT
