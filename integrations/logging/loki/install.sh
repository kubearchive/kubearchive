#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

# Parse command line arguments
for i in "$@"
do
case $i in
    --namespace=*)
    NAMESPACE=`echo $i | sed 's/[-a-zA-Z0-9]*=//'`
    ;;
    --loki-pwd=*)
    LOKI_PWD=`echo $i | sed 's/[-a-zA-Z0-9]*=//'`
    ;;
    --grafana)
    GRAFANA=True
    ;;
    --help)
    HELP=True
    ;;
    *)
    echo "Unknown option $i" # unknown option
    HELP=True
    UNKNOWN=True
    ;;
esac
done

HELP=${HELP:-"False"}
UNKNOWN=${UNKNOWN:-"False"}
NAMESPACE=${NAMESPACE:-"grafana-loki"}
LOKI_PWD=${LOKI_PWD:-"password"}
GRAFANA=${GRAFANA:-"False"}

# Help and usage
if [ "${HELP}" == "True" ] || [ "${UNKNOWN}" == "True" ]; then
    echo -e "$0

    --namespace    Namespace to use to deploy loki.
                   Default value is ${NAMESPACE}

    --loki-pwd     The password to use to deploy loki.
                   Default value is ${LOKI_PWD}

    --grafana      If enabled Grafana UI is deployed along loki.
                   Default value is ${GRAFANA}

    "
    if [ "${UNKNOWN}" == "True" ]; then
      exit 1;
    else
      exit 0;
    fi
fi

set -o errexit -o xtrace
cd ${SCRIPT_DIR}

helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update

# Deploy MinIO for S3 bucket like storage
helm upgrade --install --create-namespace --namespace ${NAMESPACE} --values values.minio.yaml minio bitnami/minio
MINIO_PWD=`kubectl -n ${NAMESPACE} get secret minio -o jsonpath='{.data.root-password}' | base64 --decode`

set +e
kubectl get secret -n ${NAMESPACE} loki-basic-auth > /dev/null 2>&1

if [ $? -ne 0 ]; then
  echo "Secret 'loki-basic-auth' not found, creating it..."
  kubectl create secret -n ${NAMESPACE} generic loki-basic-auth --from-literal=username=admin --from-literal=password=${LOKI_PWD}
else
  echo "Secret 'loki-basic-auth' already exists."
fi

set -e

# Deploy loki
helm upgrade --install --create-namespace --namespace ${NAMESPACE} --values values.loki.yaml loki grafana/loki \
 --set "loki.storage.s3.secretAccessKey=${MINIO_PWD}" \
 --set "loki.basic_auth.password=${LOKI_PWD}"

# Deploy Grafana
if [ "${GRAFANA}" == "True" ]; then
  helm upgrade --install --create-namespace --namespace ${NAMESPACE} --values values.grafana.yaml grafana grafana/grafana
fi

helm upgrade --install --wait --create-namespace \
    --namespace ${NAMESPACE} \
    logging-operator oci://ghcr.io/kube-logging/helm-charts/logging-operator
kubectl rollout status deployment --namespace=${NAMESPACE} --timeout=180s

# Deploy the log-forwarder
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
