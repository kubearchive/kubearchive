#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

set -o errexit -o xtrace

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

cd ${SCRIPT_DIR}

NAMESPACE="grafana-loki"
LOKI_PWD="password"

helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update

helm upgrade --install --create-namespace --namespace ${NAMESPACE} --values values.minio.yaml minio bitnami/minio
MINIO_PWD=`kubectl -n ${NAMESPACE} get secret minio -o jsonpath='{.data.root-password}' | base64 --decode`

kubectl get secret -n ${NAMESPACE} loki-basic-auth > /dev/null 2>&1

if [ $? -ne 0 ]; then
  echo "Secret 'loki-basic-auth' not found, creating it..."
  kubectl create secret -n ${NAMESPACE} generic loki-basic-auth --from-literal=username=admin --from-literal=password=${LOKI_PWD}
else
  echo "Secret 'loki-basic-auth' already exists."
fi

helm upgrade --install --create-namespace --namespace ${NAMESPACE} --values values.loki.yaml loki grafana/loki \
 --set "loki.storage.s3.secretAccessKey=${MINIO_PWD}" \
 --set "loki.basic_auth.password=${LOKI_PWD}"
helm upgrade --install --create-namespace --namespace ${NAMESPACE} --values values.grafana.yaml grafana grafana/grafana

helm upgrade --install --wait --create-namespace \
    --namespace ${NAMESPACE} \
    logging-operator oci://ghcr.io/kube-logging/helm-charts/logging-operator
kubectl rollout status deployment --namespace=${NAMESPACE} --timeout=180s

kubectl -n ${NAMESPACE} apply -f ./manifests

kubectl rollout status deployment --namespace=${NAMESPACE} --timeout=180s
kubectl rollout status statefulset --namespace=${NAMESPACE} --timeout=180s

# If KubeArchive is installed, update the credentials and set the jsonpath
KUBEARCHIVE_NS="kubearchive"
if kubectl get ns ${KUBEARCHIVE_NS} >& /dev/null; then
    # Configure the logging configmap
    kubectl patch -n ${KUBEARCHIVE_NS} configmap kubearchive-logging --patch-file ${SCRIPT_DIR}/patch-logging-configmap.yaml
    kubectl -n ${KUBEARCHIVE_NS} rollout restart deployment kubearchive-sink
    # Configure the password and tenant for the api server
    kubectl patch -n ${KUBEARCHIVE_NS} secret kubearchive-logging --patch-file ${SCRIPT_DIR}/patch-logging-secret.yaml
    kubectl patch -n ${KUBEARCHIVE_NS} secret kubearchive-logging -p "{\"stringData\": {\"Authorization\": \"Basic $(echo -n "admin:${LOKI_PWD}" | base64)\"}}"
    kubectl -n ${KUBEARCHIVE_NS} rollout restart deployment kubearchive-api-server

    sleep 10 # FIXME - There is an issue with rollout and sometimes the old pod is running
    kubectl -n ${KUBEARCHIVE_NS} rollout status deployment kubearchive-sink --timeout=60s
    kubectl -n ${KUBEARCHIVE_NS} rollout status deployment kubearchive-api-server --timeout=60s
fi
