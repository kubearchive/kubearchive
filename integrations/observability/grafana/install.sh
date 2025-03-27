#!/bin/bash
# Copyright Kronicler Authors
# SPDX-License-Identifier: Apache-2.0

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

set -o errexit
set -o xtrace

kubectl apply -k ${SCRIPT_DIR}/

kubectl set -n kronicler env deployment kronicler-api-server \
    KRONICLER_OTEL_MODE="enabled" \
    OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector.observability.svc.cluster.local:4318" \
    KRONICLER_METRICS_INTERVAL="15s" \
    KRONICLER_OTLP_SEND_LOGS="true"

kubectl set -n kronicler env deployment kronicler-sink \
    KRONICLER_OTEL_MODE="enabled" \
    OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector.observability.svc.cluster.local:4318" \
    KRONICLER_METRICS_INTERVAL="15s" \
    KRONICLER_OTLP_SEND_LOGS="true"

kubectl set -n kronicler env deployment kronicler-operator -c manager \
    KRONICLER_OTEL_MODE="enabled" \
    OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector.observability.svc.cluster.local:4318" \
    KRONICLER_METRICS_INTERVAL="15s" \
    KRONICLER_OTLP_SEND_LOGS="true"

kubectl -n observability wait pod --all --for=condition=ready --timeout=60s
