#!/bin/bash
# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0

# Deploy one of the KubeArchive components in debug mode with delve

COMPONENT="$1"

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd ${SCRIPT_DIR}/../..

CONTAINER=kubearchive-${COMPONENT}
SVC=kubearchive-${COMPONENT}
if [ ${COMPONENT} == "operator" ]; then
    SVC=kubearchive-${COMPONENT}-webhook
    CONTAINER=manager
fi

BUILT_IMAGE=$(ko build github.com/kubearchive/kubearchive/cmd/${COMPONENT%%-*} --debug)

YAML=$(mktemp --suffix=.yaml -t kubearchive-XXX)
cat ${SCRIPT_DIR}/patch-${COMPONENT}.yaml | envsubst > ${YAML}

# Pause the deployment updates
kubectl -n kubearchive rollout pause deployment kubearchive-${COMPONENT}

# Set the new image
kubectl -n kubearchive set image deployment kubearchive-${COMPONENT} ${CONTAINER}=${BUILT_IMAGE}
# Add -- as the first argument
kubectl -n kubearchive get deployment kubearchive-${COMPONENT} -o json | \
jq '.spec.template.spec.containers |= map(select(."name"=="'${CONTAINER}'") .args |= ["--"] + .)' | \
kubectl apply -f -
# Remove the probes, the resource limits and the security context
kubectl -n kubearchive get deployment kubearchive-${COMPONENT} -o json | \
jq 'del(.spec.template.spec.containers[] | select(."name"=="'${CONTAINER}'") | .livenessProbe, .readinessProbe, .resources, .securityContext)' | \
kubectl apply -f -
# Update security context
kubectl -n kubearchive patch deployment kubearchive-${COMPONENT} --patch-file ${SCRIPT_DIR}/patch-security-context.yaml
# Update the service
kubectl -n kubearchive patch svc ${SVC} --patch-file ${SCRIPT_DIR}/patch-service.yaml

# Resume the deployment updates
kubectl -n kubearchive rollout resume deployment kubearchive-${COMPONENT}
kubectl -n kubearchive rollout status deployment kubearchive-${COMPONENT}  --timeout=30s
