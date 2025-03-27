#!/bin/bash
# Copyright Kronicler Authors
# SPDX-License-Identifier: Apache-2.0

# From https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-deploy-eck.html
#
# Note that everything goes into the elastic-system namespace.

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

cd ${SCRIPT_DIR}

helm uninstall --namespace elastic-system logging-operator

kubectl delete -f https://download.elastic.co/downloads/eck/2.14.0/operator.yaml
kubectl delete -f https://download.elastic.co/downloads/eck/2.14.0/crds.yaml
