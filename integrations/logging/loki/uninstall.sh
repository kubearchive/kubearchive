#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0
NAMESPACE="grafana-loki"

for i in "$@"
do
case $i in
    --vector)
    VECTOR=True
    ;;
    *)
    echo "Unknown option $i" # unknown option
    HELP=True
    UNKNOWN=True
    ;;
esac
done

VECTOR=${VECTOR:-"False"}

kubectl delete namespace ${NAMESPACE}

if [ "${VECTOR}" == "False" ]; then
    kubectl delete clusterrole logging-operator logging-operator-edit
    kubectl delete clusterrolebinding logging-operator
else
    kubectl delete clusterrole kubearchive-vector
    kubectl delete clusterrolebinding kubearchive-vector
fi