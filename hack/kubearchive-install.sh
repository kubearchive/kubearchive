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

kubectl get -n kubearchive deployments
