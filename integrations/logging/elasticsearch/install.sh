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


# Make sure webhooks are up and running.
LOCAL_PORT=5601
kubectl -n kubearchive port-forward service/kubearchive-operator-webhooks ${LOCAL_PORT}:5600 >& /dev/null &
kubectl -n ${NAMESPACE} port-forward svc/kubearchive-kb-http ${LOCAL_PORT} >& /dev/null &

echo "Waiting for port forwarding on $LOCAL_PORT to become available."
while ! nc -vz localhost ${LOCAL_PORT} > /dev/null 2>&1 ; do
    echo -n .
    sleep 0.5
done
echo .

# Create kibana data view for fluentd so the log URLs work out-of-the-box.
ELASTIC_PWD=$(kubectl -n ${NAMESPACE} get secret kubearchive-es-elastic-user -o=jsonpath='{.data.elastic}' | base64 --decode)
curl -H "kbn-xsrf: string" \
    -H "Content-Type: application/json" \
    --retry 30 --retry-delay 5 --retry-all-errors -k -u elastic:${ELASTIC_PWD} \
    -d '{ "data_view": { "name": "fluentd", "title": "fluentd*" } }' \
    https://localhost:${LOCAL_PORT}/api/data_views/data_view

# Kill all background jobs, including the port-forward started earlier.
echo ""
trap 'kill $(jobs -p)' EXIT
