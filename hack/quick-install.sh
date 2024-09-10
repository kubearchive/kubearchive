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

kubectl apply -f - << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-features
  namespace: knative-eventing
  labels:
    eventing.knative.dev/release: devel
    knative.dev/config-propagation: original
    knative.dev/config-category: eventing
data:
  new-apiserversource-filters: enabled
EOF

bash cmd/operator/generate.sh
helm template kubearchive charts/kubearchive -n kubearchive \
    --include-crds \
    --set "global.production=true" > /tmp/kubearchive-not-resolved.yaml
ko resolve -f /tmp/kubearchive-not-resolved.yaml --base-import-paths > /tmp/kubearchive.yaml
kubectl apply -n kubearchive -f /tmp/kubearchive.yaml

cat << EOF > /tmp/patch.yaml
stringData:
  POSTGRES_PORT: "5432"
  POSTGRES_URL: postgresql.databases.svc.cluster.local
  POSTGRES_USER: kubearchive
  POSTGRES_DB: kubearchive
  POSTGRES_PASSWORD: 'P0stgr3sdbP@ssword'  # notsecret
EOF
kubectl patch -n kubearchive secrets kubearchive-database-credentials --patch-file /tmp/patch.yaml
kubectl rollout restart deployment --namespace=kubearchive kubearchive-sink kubearchive-api-server
kubectl get -n kubearchive deployments
