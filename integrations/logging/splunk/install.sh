#!/bin/bash

# From https://kube-logging.dev/docs/examples/splunk/
#
# Note that everything goes into the splunk-operator namespace.

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

cd ${SCRIPT_DIR}

kubectl apply -f https://github.com/splunk/splunk-operator/releases/download/2.6.0/splunk-operator-cluster.yaml \
	--server-side --force-conflicts

helm upgrade --install --wait --create-namespace \
	--namespace splunk-operator \
	logging-operator oci://ghcr.io/kube-logging/helm-charts/logging-operator

kubectl -n splunk-operator apply -f .

kubectl rollout status deployment --namespace=splunk-operator --timeout=90s
