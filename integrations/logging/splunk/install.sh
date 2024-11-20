#!/bin/bash

# From https://kube-logging.dev/docs/examples/splunk/
#
# Note that everything goes into the splunk-operator namespace.

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

cd ${SCRIPT_DIR}

NAMESPACE="splunk-operator"

kubectl apply -f https://github.com/splunk/splunk-operator/releases/download/2.6.0/splunk-operator-cluster.yaml \
	--server-side --force-conflicts

helm upgrade --install --wait --create-namespace \
	--namespace ${NAMESPACE} \
	logging-operator oci://ghcr.io/kube-logging/helm-charts/logging-operator

kubectl -n ${NAMESPACE} apply -f .

kubectl rollout status deployment --namespace=${NAMESPACE} --timeout=90s

# If KubeArchive is installed, updated the password.
KUBEARCHIVE_NS="kubearchive"
if kubectl get ns ${KUBEARCHIVE_NS} >& /dev/null; then
    SPLUNK_PWD=$(kubectl -n ${NAMESPACE} get secret splunk-splunk-operator-secret -o jsonpath='{.data.password}')
    kubectl patch -n ${KUBEARCHIVE_NS} secret kubearchive-logging -p "{\"data\": {\"password\": \"${SPLUNK_PWD}\"}}"
fi
