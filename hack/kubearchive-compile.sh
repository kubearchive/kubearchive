#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

# Install KubeArchive by itself. Used by quick install and also to update existing
# KubeArchive installations.

set -o errexit
set -o xtrace

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd ${SCRIPT_DIR}/..

echo "Started compilation at $(date)"
# Extract the release version
NEXT_VERSION=$(cat VERSION)
echo ${NEXT_VERSION}
export NEXT_VERSION=${NEXT_VERSION}

bash cmd/operator/generate.sh
kubectl kustomize config/ | envsubst | ko resolve -f - --base-import-paths > kubearchive.yaml
echo "Ended compilation at $(date)"
