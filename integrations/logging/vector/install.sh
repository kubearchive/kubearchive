#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

set -o errexit -o xtrace

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

cd ${SCRIPT_DIR}

NAMESPACE="kubearchive-vector"

helm repo add vector https://helm.vector.dev
helm repo update

helm upgrade --install --create-namespace --namespace ${NAMESPACE} --values values.vector.yaml

kubectl rollout restart --namespace ${NAMESPACE} daemonset/vector

kubectl rollout status daemonset --namespace ${NAMESPACE} --timeout=90s
