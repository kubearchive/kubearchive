#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o xtrace

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd)
cd ${SCRIPT_DIR}

# renovate: datasource=github-releases depName=knative-kafka-broker packageName=knative-extensions/eventing-kafka-broker
export KNATIVE_KAFKA_BROKER_VERSION=v1.18.0
# renovate: datasource=github-releases depName=knative-eventing packageName=knative/eventing
export KNATIVE_EVENTING_VERSION=v1.18.1
# renovate: datasource=github-releases depName=cert-manager packageName=cert-manager/cert-manager
export CERT_MANAGER_VERSION=v1.17.2

# don't install strimzi if already installed
if ! kubectl get ns kafka >& /dev/null; then
    kubectl create namespace kafka
    kubectl create -f https://strimzi.io/install/latest?namespace=kafka -n kafka
    kubectl rollout status deployment --namespace=kafka --timeout=300s

    # create kafka cluster
    kubectl apply -f https://strimzi.io/examples/latest/kafka/kafka-single-node.yaml -n kafka
    kubectl wait kafka/my-cluster --for=condition=Ready --timeout=300s -n kafka
fi

# install knative-eventing kafka extensions
kubectl apply -f https://github.com/knative-extensions/eventing-kafka-broker/releases/download/knative-${KNATIVE_KAFKA_BROKER_VERSION}/eventing-kafka-controller.yaml
kubectl apply -f https://github.com/knative-extensions/eventing-kafka-broker/releases/download/knative-${KNATIVE_KAFKA_BROKER_VERSION}/eventing-kafka-broker.yaml
kubectl rollout status deployment --namespace=knative-eventing --timeout=300s

# set kafka as the default broker class for kubearchive
kubectl apply -f ${SCRIPT_DIR}/kafka-broker-config.yaml
kubectl apply -f ${SCRIPT_DIR}/config-br-defaults-kafka.yaml

cd ${SCRIPT_DIR}/../..
NEXT_VERSION=$(cat VERSION)
echo ${NEXT_VERSION}
export NEXT_VERSION=${NEXT_VERSION}

# remove brokers so they can be redeployed as kafka brokers
kubectl delete --namespace=kubearchive broker --all
envsubst < config/templates/eventing/brokers.yaml | kubectl apply -f -
