#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

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

# If KubeArchive is installed, update the credentials and set the jsonpath
KUBEARCHIVE_NS="kubearchive"
if kubectl get ns ${KUBEARCHIVE_NS} >& /dev/null; then
    # Configure the jsonpath and the server for the sink
    kubectl patch -n ${KUBEARCHIVE_NS} configmap kubearchive-logging --patch-file ${SCRIPT_DIR}/patch-jsonpath.yaml
    kubectl -n ${KUBEARCHIVE_NS} rollout restart deployment kubearchive-sink
    # Configure the password for the api server
    SPLUNK_PWD=$(kubectl -n ${NAMESPACE} get secret splunk-splunk-operator-secret -o jsonpath='{.data.password}')
    kubectl patch -n ${KUBEARCHIVE_NS} secret kubearchive-logging -p "{\"data\": {\"PASSWORD\": \"${SPLUNK_PWD}\"}}"
    kubectl -n ${KUBEARCHIVE_NS} rollout restart deployment kubearchive-api-server

    sleep 10 # FIXME - There is an issue with rollout and sometimes the old pod is running
    kubectl -n ${KUBEARCHIVE_NS} rollout status deployment kubearchive-sink --timeout=60s
    kubectl -n ${KUBEARCHIVE_NS} rollout status deployment kubearchive-api-server --timeout=60s
fi
