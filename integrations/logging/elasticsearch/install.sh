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

# If KubeArchive is installed, update the credentials and configmaps.
KUBEARCHIVE_NS="kubearchive"
ES_ENDPOINT="https://kubearchive-es-http.${NAMESPACE}.svc.cluster.local:9200"
if kubectl get ns ${KUBEARCHIVE_NS} >& /dev/null; then
    # Configure the writer configmap (sink)
    kubectl patch -n ${KUBEARCHIVE_NS} configmap kubearchive-logging-writer --patch-file ${SCRIPT_DIR}/patch-logging-configmap.yaml
    kubectl -n ${KUBEARCHIVE_NS} rollout restart deployment kubearchive-sink
    # Configure the reader configmap (api server)
    kubectl patch -n ${KUBEARCHIVE_NS} configmap kubearchive-logging-reader --patch-file ${SCRIPT_DIR}/patch-logging-reader-configmap.yaml
    # Configure the HEADERS secret for the api server
    ELASTIC_PWD=$(kubectl -n ${NAMESPACE} get secret kubearchive-es-elastic-user -o=jsonpath='{.data.elastic}' | base64 --decode)
    ELASTIC_AUTH="Basic $(echo -n "elastic:${ELASTIC_PWD}" | base64)"
    kubectl patch -n ${KUBEARCHIVE_NS} secret kubearchive-logging -p "{\"stringData\": {\"HEADERS\": \"${ES_ENDPOINT}:\\n  Authorization: \\\"${ELASTIC_AUTH}\\\"\\n\"}}"
    kubectl -n ${KUBEARCHIVE_NS} rollout restart deployment kubearchive-api-server

    sleep 10 # FIXME - There is an issue with rollout and sometimes the old pod is running
    kubectl -n ${KUBEARCHIVE_NS} rollout status deployment kubearchive-sink --timeout=60s
    kubectl -n ${KUBEARCHIVE_NS} rollout status deployment kubearchive-api-server --timeout=60s
fi
