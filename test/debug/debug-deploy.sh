#!/bin/bash
# Copyright Kronicler Authors
# SPDX-License-Identifier: Apache-2.0

# Deploy one of the Kronicler components in debug mode with delve

COMPONENT="$1"

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd ${SCRIPT_DIR}/../..

CONTAINER=kronicler-${COMPONENT}
SVC=kronicler-${COMPONENT}
if [ ${COMPONENT} == "operator" ]; then
    SVC=kronicler-${COMPONENT}-webhook
    CONTAINER=manager
fi

BUILT_IMAGE=$(ko build github.com/kronicler/kronicler/cmd/${COMPONENT%%-*} --debug)

YAML=$(mktemp --suffix=.yaml -t kronicler-XXX)
cat ${SCRIPT_DIR}/patch-${COMPONENT}.yaml | envsubst > ${YAML}

# Pause the deployment updates
kubectl -n kronicler rollout pause deployment kronicler-${COMPONENT}

# Set the new image
kubectl -n kronicler set image deployment kronicler-${COMPONENT} ${CONTAINER}=${BUILT_IMAGE}
# Add -- as the first argument
kubectl -n kronicler get deployment kronicler-${COMPONENT} -o json | \
jq '.spec.template.spec.containers |= map(select(."name"=="'${CONTAINER}'") .args |= ["--"] + .)' | \
kubectl apply -f -
# Remove the probes, the resource limits and the security context
kubectl -n kronicler get deployment kronicler-${COMPONENT} -o json | \
jq 'del(.spec.template.spec.containers[] | select(."name"=="'${CONTAINER}'") | .livenessProbe, .readinessProbe, .resources, .securityContext)' | \
kubectl apply -f -
# Update security context
kubectl -n kronicler patch deployment kronicler-${COMPONENT} --patch-file ${SCRIPT_DIR}/patch-security-context.yaml
# Update the service
kubectl -n kronicler patch svc ${SVC} --patch-file ${SCRIPT_DIR}/patch-service.yaml

# Resume the deployment updates
kubectl -n kronicler rollout resume deployment kronicler-${COMPONENT}
kubectl -n kronicler rollout status deployment kronicler-${COMPONENT}  --timeout=30s
