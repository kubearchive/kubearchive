#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

set -o errexit
set -o xtrace

kubectl apply -k ${SCRIPT_DIR}/

kubectl set -n kubearchive env deployment kubearchive-api-server \
    KUBEARCHIVE_OTEL_MODE="enabled" \
    OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector.observability.svc.cluster.local:4318" \
    KUBEARCHIVE_METRICS_INTERVAL="15s"

kubectl set -n kubearchive env deployment kubearchive-sink \
    KUBEARCHIVE_OTEL_MODE="enabled" \
    OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector.observability.svc.cluster.local:4318" \
    KUBEARCHIVE_METRICS_INTERVAL="15s"

kubectl set -n kubearchive env deployment kubearchive-operator -c manager \
    KUBEARCHIVE_OTEL_MODE="enabled" \
    OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector.observability.svc.cluster.local:4318" \
    KUBEARCHIVE_METRICS_INTERVAL="15s"

kubectl -n observability wait pod --all --for=condition=ready --timeout=60s
