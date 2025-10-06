#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o xtrace

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd ${SCRIPT_DIR}/..

bash integrations/database/postgresql/install.sh

# renovate: datasource=github-releases depName=cert-manager packageName=cert-manager/cert-manager
export CERT_MANAGER_VERSION=v1.17.2
# renovate: datasource=github-releases depName=knative-eventing packageName=knative/eventing
export KNATIVE_EVENTING_VERSION=v1.18.1

kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml
kubectl rollout status deployment --namespace=cert-manager --timeout=30s

bash hack/kubearchive-install.sh
