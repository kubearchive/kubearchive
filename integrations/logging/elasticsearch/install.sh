#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

# From https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-deploy-eck.html
#
# Note that everything goes into the elastic-system namespace.

set -o errexit -o xtrace

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

cd ${SCRIPT_DIR}

NAMESPACE="elastic-system"

kubectl apply -f https://download.elastic.co/downloads/eck/2.14.0/crds.yaml
kubectl apply -f https://download.elastic.co/downloads/eck/2.14.0/operator.yaml

helm upgrade --install --wait --create-namespace \
    --namespace ${NAMESPACE} \
    logging-operator oci://ghcr.io/kube-logging/helm-charts/logging-operator
kubectl rollout status deployment --namespace=${NAMESPACE} --timeout=90s

kubectl -n ${NAMESPACE} apply -f .

kubectl rollout status deployment --namespace=${NAMESPACE} --timeout=90s
kubectl rollout status statefulset --namespace=${NAMESPACE} --timeout=90s
kubectl wait --namespace=${NAMESPACE} --for=jsonpath='{.status.health}'=green kibana/kubearchive --timeout=180s

# If KubeArchive is installed, updated the password.
KUBEARCHIVE_NS="kubearchive"
if kubectl get ns ${KUBEARCHIVE_NS} >& /dev/null; then
    ELASTIC_PWD=$(kubectl -n ${NAMESPACE} get secret kubearchive-es-elastic-user -o=jsonpath='{.data.elastic}')
    kubectl patch -n ${KUBEARCHIVE_NS} secret kubearchive-logging -p "{\"data\": {\"password\": \"${ELASTIC_PWD}\"}}"
fi
