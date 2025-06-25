#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0
NAMESPACE="grafana-loki"
VECTOR_NAMESPACE="kubearchive-vector"

kubectl delete namespace ${NAMESPACE}
kubectl delete namespace ${VECTOR_NAMESPACE}
kubectl delete clusterrole logging-operator logging-operator-edit
kubectl delete clusterrolebinding logging-operator
