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

kubectl create secret -n ${NAMESPACE} generic loki-basic-auth --from-literal=username=admin --from-literal=password=${LOKI_PWD}

helm upgrade --install --create-namespace --namespace ${NAMESPACE} --values values.loki.yaml loki grafana/loki \
 --set "loki.storage.s3.secretAccessKey=${MINIO_PWD}" \
 --set "loki.basic_auth.password=${LOKI_PWD}"
helm upgrade --install --create-namespace --namespace ${NAMESPACE} --values values.grafana.yaml grafana grafana/grafana

helm upgrade --install --wait --create-namespace \
    --namespace ${NAMESPACE} \
    logging-operator oci://ghcr.io/kube-logging/helm-charts/logging-operator
kubectl rollout status deployment --namespace=${NAMESPACE} --timeout=90s

kubectl -n ${NAMESPACE} apply -f ./manifests

kubectl rollout status deployment --namespace=${NAMESPACE} --timeout=90s
kubectl rollout status statefulset --namespace=${NAMESPACE} --timeout=90s
