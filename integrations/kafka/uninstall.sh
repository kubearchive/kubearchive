#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o xtrace

SCRIPT_DIR=$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd)
cd ${SCRIPT_DIR}

# set channel based brokers as the default broker class for kubearchive
kubectl apply -f ${SCRIPT_DIR}/config-br-defaults.yaml

cd ${SCRIPT_DIR}/../..
NEXT_VERSION=$(cat VERSION)
echo ${NEXT_VERSION}
export NEXT_VERSION=$(cat VERSION)

# remove brokers so they can be redeployed as channel based brokers
kubectl delete --namespace=kubearchive broker --all
envsubst < config/templates/eventing/brokers.yaml | kubectl apply -f -
