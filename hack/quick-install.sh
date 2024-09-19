#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o xtrace

bash integrations/database/postgresql/install.sh

export CERT_MANAGER_VERSION=v1.9.1
export KNATIVE_EVENTING_VERSION=v1.15.0

kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml
kubectl apply -f https://github.com/knative/eventing/releases/download/knative-${KNATIVE_EVENTING_VERSION}/eventing-core.yaml
kubectl rollout status deployment --namespace=cert-manager --timeout=30s
kubectl rollout status deployment --namespace=knative-eventing --timeout=30s

bash cmd/operator/generate.sh
helm template kubearchive charts/kubearchive -n kubearchive \
    --include-crds \
    --set "global.production=true" > /tmp/kubearchive-not-resolved.yaml
ko resolve -f /tmp/kubearchive-not-resolved.yaml --base-import-paths > /tmp/kubearchive.yaml
kubectl apply -n kubearchive -f /tmp/kubearchive.yaml

kubectl -n kubearchive rollout status deployment --timeout=90s
kubectl -n kubearchive wait pod --all --for=condition=ready --timeout=90s

# Make sure webhooks are up and running.
LOCAL_PORT=8443
kubectl -n kubearchive port-forward service/kubearchive-operator-webhooks ${LOCAL_PORT}:443 >& /dev/null &

# Wait for $LOCAL_PORT to become available.
while ! nc -vz localhost ${LOCAL_PORT} > /dev/null 2>&1 ; do
    echo -n .
    sleep 0.5
done
echo .

kubectl get -n kubearchive deployments

# Kill all background jobs, including the port-forward started earlier.
trap 'kill $(jobs -p)' EXIT
