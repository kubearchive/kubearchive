#!/bin/bash

set -o errexit
set -o xtrace

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

helm install kubearchive charts/kubearchive --create-namespace -n kubearchive \
    --set-string apiServer.image=$(ko build github.com/kubearchive/kubearchive/cmd/api) \
    --set-string sink.image=$(ko build github.com/kubearchive/kubearchive/cmd/sink) \
    --set-string operator.image=$(ko build github.com/kubearchive/kubearchive/cmd/operator)

kubectl rollout status deployment --namespace=kubearchive --timeout=60s
helm list -n kubearchive
kubectl get -n kubearchive deployments
