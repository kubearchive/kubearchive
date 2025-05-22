#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0
NAMESPACE="grafana-loki"

kubectl delete namespace ${NAMESPACE}
kubectl delete clusterrole logging-operator logging-operator-edit
kubectl delete clusterrolebinding logging-operator
