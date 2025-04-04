#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o xtrace

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
NAMESPACE="prometheus-operator"

helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm upgrade --install --wait --create-namespace --namespace ${NAMESPACE} \
	prometheus prometheus-community/kube-prometheus-stack \
	--set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false

kubectl apply -k ${SCRIPT_DIR}/

kubectl set -n kubearchive env deployment kubearchive-api-server \
    KUBEARCHIVE_OTEL_MODE="enabled" \
    OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector:4318" \
    KUBEARCHIVE_METRICS_INTERVAL="15s"

kubectl set -n kubearchive env deployment kubearchive-sink \
    KUBEARCHIVE_OTEL_MODE="enabled" \
    OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector:4318" \
    KUBEARCHIVE_METRICS_INTERVAL="15s"

kubectl set -n kubearchive env deployment kubearchive-operator -c manager \
    KUBEARCHIVE_OTEL_MODE="enabled" \
    OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector:4318" \
    KUBEARCHIVE_METRICS_INTERVAL="15s"
