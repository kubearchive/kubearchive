#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o xtrace

YAML=$(mktemp --suffix=.yaml -t kubearchive-XXX)

helm template kubearchive charts/kubearchive -n kubearchive \
    --include-crds \
    --set "global.production=true" > ${YAML}
kubectl delete -n kubearchive -f ${YAML}

rm -f ${YAML}
